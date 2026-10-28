package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/media"
	"github.com/yourorg/chatplatform/internal/notifications"
	"github.com/yourorg/chatplatform/internal/platform/config"
	"github.com/yourorg/chatplatform/internal/platform/db"
	"github.com/yourorg/chatplatform/internal/platform/logger"
	"github.com/yourorg/chatplatform/internal/stories"
)

func main() {
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
		log.Fatal("worker: failed to connect to postgres", zap.Error(err))
	}
	defer pgPool.Close()

	outbox := notifications.NewOutbox(pgPool)
	tokens := notifications.NewTokenRepository(pgPool)
	// Real Firebase push when FCM_SERVICE_ACCOUNT_JSON (the service-account key, raw JSON or base64) is set;
	// otherwise pushes are only logged.
	var sender notifications.PushSender = notifications.LoggingStubSender{Log: log}
	if raw := os.Getenv("FCM_SERVICE_ACCOUNT_JSON"); raw != "" {
		fcm, err := notifications.NewFCMSenderFromJSON(raw)
		if err != nil {
			log.Fatal("worker: FCM_SERVICE_ACCOUNT_JSON is set but invalid", zap.Error(err))
		}
		sender = fcm
		log.Info("push: Firebase Cloud Messaging enabled")
	} else {
		log.Warn("push: FCM_SERVICE_ACCOUNT_JSON is not set - push notifications are only logged, not sent")
	}
	notificationWorker := notifications.NewWorker(outbox, tokens, sender, log)

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
		log.Fatal("worker: failed to init s3 client", zap.Error(err))
	}
	mediaRepo := media.NewRepository(pgPool)
	thumbnailWorker := media.NewThumbnailWorker(mediaRepo, s3Client, log)

	// mediaSvc (not mediaRepo) is what implements DeleteMedia — it needs S3
	// access to remove the object, not just the DB row. storyMedia/limiter
	// dependencies aren't needed for cleanup, so this instance is built with
	// nil for those — DeleteMedia doesn't touch either.
	mediaSvcForCleanup := media.NewService(mediaRepo, s3Client, nil, nil, media.Limits{}, 0)
	storiesCleanup := stories.NewCleanupWorker(pgPool, mediaSvcForCleanup, log)

	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("workers starting: notification outbox + thumbnail generation + stories cleanup")
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); notificationWorker.Run(runCtx) }()
	go func() { defer wg.Done(); thumbnailWorker.Run(runCtx) }()
	go func() { defer wg.Done(); storiesCleanup.Run(runCtx) }()
	wg.Wait()
	log.Info("workers stopped")
}
