# Deploying KikBite (Railway: web + api + worker + migrate, with Postgres, Redis and external object storage)

Nothing here was run against a real Railway project from the build sandbox — rehearse on a staging project first.

## Services
| Service | Source | Notes |
|---|---|---|
| postgres | Railway Postgres plugin | |
| redis | Railway Redis plugin | |
| api | `backend/` Dockerfile, `CMD_PATH=./cmd/api` | can stay private: the web service proxies to it over the private network |
| worker | `backend/` Dockerfile, `CMD_PATH=./cmd/worker` | same variables as api |
| migrate | one-off job (`migrate/migrate` image) | `-path migrations -database $POSTGRES_DSN up` after every deploy that adds a migration (0014–0018 are new) |
| web | `web/` Dockerfile | the only public service; custom domain + HTTPS automatic |

## Object storage
Railway has no S3. Use Cloudflare R2, Backblaze B2 or AWS S3 with a **private** bucket. Browsers upload straight to it, so add a **CORS rule**:
origin `https://YOUR-DOMAIN`, methods `PUT, GET`, allowed header `Content-Type`. Set `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`,
`S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_USE_PATH_STYLE` (R2/B2: true) and leave `S3_PRESIGN_ENDPOINT` empty.
Set `MEDIA_ORIGIN` on the **web** service to the bucket origin — the CSP allows uploads/images only from there.

## Variables
api / worker (generate every secret fresh, never reuse local test values): `APP_ENV=production`, `POSTGRES_DSN`, `REDIS_ADDR`,
the JWT/signing secrets from `.env.example`, `APP_BASE_URL=https://YOUR-DOMAIN`, `WS_ALLOWED_ORIGINS=https://YOUR-DOMAIN`,
`RESEND_API_KEY`, `MAIL_FROM="KikBite <noreply@YOUR-DOMAIN>"` (verify the domain in Resend; before that only `onboarding@resend.dev` → your own address works),
`GEMINI_API_KEY` (a NEW key), and the S3 variables.

web: `KIKBITE_LIVE=1`, `API_PROXY=http://<api private host>:8080`, `MEDIA_ORIGIN=<bucket origin>`.
`/config.js` is generated from these, so index.html needs no editing (`KIKBITE_CONFIG='{"apiBase":"/api/v1"}'` adds extra keys).

## Before going public
- [ ] `go build ./... && go test ./...` pass on your machine (the sandbox never compiled the Go code)
- [ ] HTTPS only; `APP_ENV` is not `local` (local mode logs e-mail links and allows localhost origins)
- [ ] fresh Gemini key + secrets, Resend domain verified, bucket private with the CORS rule
- [ ] register → e-mail arrives → link verifies; send a photo; reload; try on a phone
- [ ] Safari + Firefox pass (only Chromium was tested), incl. WebSocket on mobile networks
- [ ] Postgres backups; alert on `/healthz`

## Optional
`RESEND_WEBHOOK_SECRET` + a Resend webhook to `https://YOUR-DOMAIN/api/v1/webhooks/resend` (events `email.bounced`, `email.complained`) — see EMAIL_AUTH.md.
Image thumbnails need the **worker** service running (it generates them); until it has, the web falls back to the full image.

Calls: set `TURN_URLS` + (`TURN_SECRET` or `TURN_USERNAME`/`TURN_CREDENTIAL`) so calls work across strict NATs; without them only STUN is used — see CALLS.md.

## Not done
Web push notifications (incl. incoming-call alerts when the app is closed); call history UI; screen sharing; calls over 4 people; video poster frames; Safari/Firefox verification; real-traffic rate-limit tuning.
