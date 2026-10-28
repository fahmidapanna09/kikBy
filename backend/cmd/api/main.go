package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/admin"
	"github.com/yourorg/chatplatform/internal/ai"
	"github.com/yourorg/chatplatform/internal/auth"
	"github.com/yourorg/chatplatform/internal/channels"
	"github.com/yourorg/chatplatform/internal/chat"
	"github.com/yourorg/chatplatform/internal/communities"
	"github.com/yourorg/chatplatform/internal/contacts"
	"github.com/yourorg/chatplatform/internal/friends"
	"github.com/yourorg/chatplatform/internal/inbox"
	"github.com/yourorg/chatplatform/internal/media"
	"github.com/yourorg/chatplatform/internal/notifications"
	"github.com/yourorg/chatplatform/internal/platform/audit"
	"github.com/yourorg/chatplatform/internal/platform/config"
	"github.com/yourorg/chatplatform/internal/platform/db"
	"github.com/yourorg/chatplatform/internal/platform/logger"
	"github.com/yourorg/chatplatform/internal/platform/metrics"
	appmw "github.com/yourorg/chatplatform/internal/platform/middleware"
	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
	"github.com/yourorg/chatplatform/internal/platform/redisx"
	"github.com/yourorg/chatplatform/internal/stories"
	"github.com/yourorg/chatplatform/internal/users"
)

func main() {
	// .env is only for local dev; in staging/production, real env vars /
	// secrets manager injection is used and this call is a harmless no-op
	// if no .env file is present.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.Env)
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pgPool, err := db.New(ctx, cfg.Postgres.DSN)
	cancel()
	if err != nil {
		log.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer pgPool.Close()

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	rdb, err := redisx.New(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.TLSEnabled)
	cancel()
	if err != nil {
		log.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer rdb.Close()

	var smsSender auth.SMSSender = auth.ConsoleSMSSender{}
	// TODO(phase1-hardening): swap in a real provider (e.g. Twilio) here
	// when cfg.SMS.Provider != "console"; keep console as the local/dev default.

	otpSvc := auth.NewOTPService(rdb, smsSender, cfg.OTP.Length, cfg.OTP.TTL, cfg.OTP.MaxPerPhonePer10Min, cfg.OTP.MaxPerIPPer10Min, cfg.Env == "local")
	jwtSvc := auth.NewJWTService(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authRepo := auth.NewRepository(pgPool)
	auditLogger := audit.New(pgPool, log)
	authSvc := auth.NewService(authRepo, otpSvc, jwtSvc, auditLogger)
	// E-mail/password flow. Provider choice: RESEND_API_KEY (+ MAIL_FROM) → Resend; otherwise the console mailer
	// in APP_ENV=local only (the links are credentials, so they are never logged elsewhere); otherwise NoopMailer,
	// which fails loudly in the logs until a provider is configured.
	var mailer auth.Mailer = auth.NoopMailer{}
	switch {
	case os.Getenv("RESEND_API_KEY") != "":
		from := os.Getenv("MAIL_FROM")
		if from == "" {
			from = "KikBite <onboarding@resend.dev>" // Resend's shared test sender: it only delivers to your own Resend account address
			log.Warn("MAIL_FROM not set; using Resend's test sender (delivers only to the Resend account owner's address)")
		}
		mailer = auth.NewResendMailer(os.Getenv("RESEND_API_KEY"), from)
		log.Info("mail provider: resend")
	case cfg.Env == "local":
		mailer = auth.ConsoleMailer{Log: log}
		log.Info("mail provider: console (links are logged; local dev only)")
	default:
		log.Warn("no mail provider configured (set RESEND_API_KEY and MAIL_FROM); password-reset and verification e-mails will fail")
	}
	appBaseURL := os.Getenv("APP_BASE_URL")
	if appBaseURL == "" {
		appBaseURL = "http://localhost:4173"
	}
	if os.Getenv("RESEND_API_KEY") != "" {
		mailer = auth.SuppressingMailer{Inner: mailer, Repo: authRepo} // skip addresses Resend reported as bounced/complained
	}
	authSvc.WithEmailAuth(auth.EmailAuth{Mailer: mailer, ResetBaseURL: appBaseURL, Log: log})
	authHandler := auth.NewHandler(authSvc, log)

	chatRepo := chat.NewRepository(pgPool)
	chatHub := chat.NewHub(rdb, log)
	chatPresence := chat.NewPresence(rdb)
	outbox := notifications.NewOutbox(pgPool)

	limiter := ratelimit.New(rdb)
	usersRepo := users.NewRepository(pgPool)
	usersSvc := users.NewService(usersRepo, limiter)
	usersHandler := users.NewHandler(usersSvc, log)

	inboxSvc := inbox.NewService(inbox.NewRepository(pgPool), chatHub)
	inboxHandler := inbox.NewHandler(inboxSvc, log)
	friendsSvc := friends.NewService(friends.NewRepository(pgPool), inboxSvc)
	friendsHandler := friends.NewHandler(friendsSvc, log)

	contactsRepo := contacts.NewRepository(pgPool)
	contactsSvc := contacts.NewService(contactsRepo)
	contactsHandler := contacts.NewHandler(contactsSvc, log)

	s3Client, err := media.NewS3Client(context.Background(), media.S3Config{
		Endpoint:        cfg.S3.Endpoint,
		PresignEndpoint: cfg.S3.PresignEndpoint,
		Region:          cfg.S3.Region,
		Bucket:          cfg.S3.Bucket,
		AccessKeyID:     cfg.S3.AccessKeyID,
		SecretAccessKey: cfg.S3.SecretAccessKey,
		UsePathStyle:    cfg.S3.UsePathStyle,
		PresignTTL:      cfg.S3.PresignTTL,
	})
	if err != nil {
		log.Fatal("failed to init s3 client", zap.Error(err))
	}
	storiesRepo := stories.NewRepository(pgPool)

	mediaRepo := media.NewRepository(pgPool)
	mediaSvc := media.NewService(mediaRepo, s3Client, chatRepo, storiesRepo, media.Limits{
		MaxImageBytes:    cfg.Media.MaxImageBytes,
		MaxVideoBytes:    cfg.Media.MaxVideoBytes,
		MaxAudioBytes:    cfg.Media.MaxAudioBytes,
		MaxDocumentBytes: cfg.Media.MaxDocumentBytes,
	}, int(cfg.S3.PresignTTL.Seconds()))
	usersSvc.SetAvatarMedia(mediaSvc) // profile pictures reuse the media pipeline
	mediaHandler := media.NewHandler(mediaSvc, log)

	chatSvc := chat.NewService(chatRepo, chatHub, chatPresence, outbox, mediaSvc, contactsRepo)
	chatSvc.EnableCalls(chat.IceConfigFromEnv()) // voice / video / group calls (WebRTC signaling); media is peer-to-peer
	chatHandler := chat.NewHandler(chatSvc, log)
	pushHandler := notifications.NewHandler(notifications.NewTokenRepository(pgPool), log)
	chatWSHandler := chat.NewWSHandler(chatSvc, chatHub, chatPresence, authSvc, log)
	chatWSHandler.SetAllowedOrigins(wsAllowedOrigins(cfg.Env, os.Getenv("APP_BASE_URL"), os.Getenv("WS_ALLOWED_ORIGINS")))

	storiesSvc := stories.NewService(storiesRepo, mediaSvc, chatHub, outbox, limiter)
	storiesHandler := stories.NewHandler(storiesSvc, log)

	communitiesRepo := communities.NewRepository(pgPool)
	communitiesSvc := communities.NewService(communitiesRepo, chatSvc, chatSvc, chatRepo)
	communitiesHandler := communities.NewHandler(communitiesSvc, log)

	channelsRepo := channels.NewRepository(pgPool)
	channelsSvc := channels.NewService(channelsRepo, mediaSvc, chatHub, outbox, limiter)
	channelsHandler := channels.NewHandler(channelsSvc, log)

	// Isolated on purpose: ai.NewService takes only its own repo, the
	// Gemini HTTP client, and the shared rate limiter — never chatRepo,
	// chatHub, contactsRepo, etc. See internal/ai/model.go's package doc
	// for why that's a hard rule, not an oversight.
	if cfg.Gemini.APIKey == "" {
		log.Warn("GEMINI_API_KEY is not set — /v1/ai routes are mounted but sending a message will fail until it's configured")
	}
	aiRepo := ai.NewRepository(pgPool)
	geminiClient := ai.NewGeminiClient(cfg.Gemini.APIKey, cfg.Gemini.BaseURL)
	aiSvc := ai.NewService(aiRepo, geminiClient, limiter, cfg.Gemini.Model)
	aiHandler := ai.NewHandler(aiSvc, log)

	// admin's UserDirectory/UserModerator/ReportsProvider are satisfied
	// structurally by authSvc and channelsSvc (see internal/admin/service.go) —
	// neither of those packages imports admin, or knows it exists.
	adminRepo := admin.NewRepository(pgPool)
	adminMetrics := admin.NewMetricsRepository(pgPool)
	adminSvc := admin.NewService(adminRepo, adminMetrics, authSvc, authSvc, channelsSvc, auditLogger)
	adminHandler := admin.NewHandler(adminSvc, log)
	if len(cfg.Admin.BootstrapPhoneNumbers) > 0 {
		if err := adminSvc.BootstrapSuperadmins(ctx, cfg.Admin.BootstrapPhoneNumbers); err != nil {
			log.Error("admin bootstrap failed", zap.Error(err))
		}
	}

	r := chi.NewRouter()
	r.Use(appmw.RequestID)
	r.Use(appmw.Recover(log))
	r.Use(appmw.SecurityHeaders)
	r.Use(chimw.RealIP)
	r.Use(chimw.Timeout(30 * time.Second))

	// Coarse global rate limit as defense-in-depth; per-route limits (OTP,
	// login) are enforced inside the auth service against Redis.
	r.Use(httprate.LimitByIP(100, time.Minute))

	// Records http_requests_total / http_request_duration_seconds for every
	// request, labeled by chi's matched route *pattern* (e.g.
	// "/v1/users/{id}"), never the raw path, so label cardinality stays
	// bounded regardless of how many distinct user IDs get requested. See
	// internal/platform/metrics for why this is hand-rolled rather than
	// prometheus/client_golang.
	r.Use(metrics.Middleware(func(r *http.Request) string {
		return chi.RouteContext(r.Context()).RoutePattern()
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pgPool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Resend bounce/complaint webhooks (Svix-signed). Set RESEND_WEBHOOK_SECRET (the "whsec_..." value from the Resend dashboard)
	// and point the webhook at https://YOUR-DOMAIN/api/v1/webhooks/resend for the events email.bounced and email.complained.
	if secret := os.Getenv("RESEND_WEBHOOK_SECRET"); secret != "" {
		r.Post("/v1/webhooks/resend", auth.ResendWebhook(secret, authRepo, log))
	}

	r.Route("/v1/auth", func(r chi.Router) {
		r.Mount("/", authHandler.Routes())
		if cfg.Env == "local" {
			authHandler.MountDebugRoutes(r)
			log.Warn("debug OTP-peek endpoint is mounted — local dev only, never enable this in staging/production")
		}
	})

	r.Route("/v1", func(r chi.Router) {
		r.Mount("/", chatHandler.Routes(authSvc))
	})

	r.Route("/v1/media", func(r chi.Router) {
		r.Mount("/", mediaHandler.Routes(authSvc))
	})

	r.Route("/v1/users", func(r chi.Router) {
		r.Mount("/", usersHandler.Routes(authSvc))
	})

	r.Route("/v1/friends", func(r chi.Router) {
		r.Mount("/", friendsHandler.Routes(authSvc))
	})

	r.Route("/v1/push", func(r chi.Router) {
		r.Mount("/", pushHandler.Routes(authSvc))
	})

	r.Route("/v1/notifications", func(r chi.Router) {
		r.Mount("/", inboxHandler.Routes(authSvc))
	})

	r.Route("/v1/contacts", func(r chi.Router) {
		r.Mount("/", contactsHandler.Routes(authSvc))
	})

	r.Route("/v1/stories", func(r chi.Router) {
		r.Mount("/", storiesHandler.Routes(authSvc))
	})

	r.Route("/v1/communities", func(r chi.Router) {
		r.Mount("/", communitiesHandler.Routes(authSvc))
	})

	r.Route("/v1/channels", func(r chi.Router) {
		r.Mount("/", channelsHandler.Routes(authSvc))
	})

	r.Route("/v1/ai", func(r chi.Router) {
		r.Mount("/", aiHandler.Routes(authSvc))
	})

	r.Route("/v1/admin", func(r chi.Router) {
		r.Mount("/", adminHandler.Routes(authSvc))
	})

	// WebSocket gateway lives outside /v1/rest conventions since it's a
	// long-lived connection, not a request/response call.
	r.Get("/ws", chatWSHandler.ServeWS)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTP.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// /metrics lives on its own port, deliberately never mounted on the
	// public API router above — an operator who forgets to firewall off
	// the main port still doesn't leak request-rate/latency data (which
	// can itself be sensitive, e.g. for timing-based inference) to the
	// public internet. Only bind this port to a private network / scrape
	// it from inside the cluster in staging and production.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metrics.Handler())
	metricsSrv := &http.Server{
		Addr:              ":" + cfg.Metrics.Port,
		Handler:           metricsMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("api server starting", zap.String("port", cfg.HTTP.Port), zap.String("env", cfg.Env))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	go func() {
		log.Info("metrics server starting", zap.String("port", cfg.Metrics.Port))
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("metrics server graceful shutdown failed", zap.Error(err))
	}
}

// wsAllowedOrigins builds the browser Origin allow-list for /ws: the explicit
// WS_ALLOWED_ORIGINS list (comma separated), the origin of APP_BASE_URL, and,
// only when APP_ENV=local, the usual localhost dev origins.
func wsAllowedOrigins(env, appBaseURL, explicit string) []string {
	var out []string
	for _, o := range strings.Split(explicit, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	if u, err := url.Parse(appBaseURL); err == nil && u.Scheme != "" && u.Host != "" {
		out = append(out, u.Scheme+"://"+u.Host)
	}
	if env == "local" {
		out = append(out, "http://localhost:4173", "http://127.0.0.1:4173", "http://localhost:8080", "http://localhost:5173")
	}
	return out
}
