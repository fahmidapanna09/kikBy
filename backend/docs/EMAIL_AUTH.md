# Email + password accounts (migrations 0012, 0015, 0016)

Phone verification is no longer required. Sign-up is e-mail + username + password; a phone number is an
**optional, unverified** contact field.

## Endpoints (`/v1/auth`)
| Method | Path | Notes |
|---|---|---|
| POST | `/register` | `{name, username, email, password, phone?, device_id, platform}` → 201 `{user, tokens, is_new_user}`; 422 `{error:{code:"validation", fields:{...}}}` |
| POST | `/login` | `{identifier (email or username), password, device_id, platform}` → 200; 401 `invalid_credentials` (same answer for unknown user / wrong password) |
| POST | `/forgot-password` | `{email}` → always 202 (no account enumeration) |
| POST | `/reset-password` | `{token, password}` → 200; 400 `invalid_token`; revokes all sessions |
| POST | `/verify-email` | **auth required**. `{code}` (6 digits) → 200 `{status:"verified"}`; 400 `invalid_code` / `code_expired`; 429 `too_many_attempts` (5th wrong guess burns the code) |
| POST | `/verify-email/resend` | **auth required**. Sends a new 6-digit code. 202 `sent`, 200 `already_verified`, 429 `too_many_requests` (60 s cooldown), 502 `mail_failed` |
| POST | `/change-password` | **auth**. `{current_password, new_password}` → 200; 422 `fields.current` ("incorrect password") / `fields.new_password`. Signs out every *other* device |
| POST | `/change-email` | **auth**. `{password, new_email}` → 202 `verification_sent`; 422 `fields.email` / `fields.password`; 502 `mail_failed`. The login e-mail only switches after the 6-digit code sent to the **new** address is entered |
| POST | `/delete-account` | **auth**. `{password}` → 200; 422 `fields.password`. Soft-delete + anonymise (below) |
| PATCH | `/v1/users/me` | now accepts `contact_phone` (E.164, `""` removes) |

Existing phone/OTP endpoints still work for legacy accounts.

## Design decisions
- Passwords: Argon2id (same parameters as OTP hashing), self-describing encoded string.
- The optional phone lives in `users.contact_phone`, **not** `phone_number`/`phone_number_hash`, because it is unverified: using the
  verified columns would let anyone squat another person's number, block their real OTP sign-up, or poison contact-sync matches.
- Reset tokens: 32 random bytes, only the SHA-256 is stored, 30-minute TTL, single use (atomic `UPDATE … WHERE used_at IS NULL`).
- Per-IP limits: register/login 10/min, forgot 5/min, reset 10/min (plus the global limit).

## E-mail delivery: Resend
Mail goes through the Resend HTTP API (`POST https://api.resend.com/emails`, standard library only, no SDK).

1. Create an API key: https://resend.com/api-keys  → `RESEND_API_KEY`.
2. Add and verify your sending domain (DNS records): https://resend.com/domains → `MAIL_FROM="KikBite <noreply@kikbite.com>"`.
   Until a domain is verified, Resend only lets you send from `onboarding@resend.dev` **to your own Resend account address**
   (handy for a first test; the backend falls back to that sender if `MAIL_FROM` is empty and logs a warning).
3. Put both in `backend/.env` (never in git/zips), set `APP_BASE_URL` to where the web app is served, then
   `docker compose up -d --force-recreate api`.

Provider selection at start-up: `RESEND_API_KEY` set → Resend; else `APP_ENV=local` → console (links logged); else `NoopMailer`
(every send fails loudly in the logs). A mail failure never fails sign-up: the verification mail is sent in the background and the
person can press **Resend code** in the app. Each send carries an `Idempotency-Key` and gets one automatic retry on network
errors / 429 / 5xx, so a retry cannot deliver a duplicate. The API key is never logged or included in errors.

## E-mail verification (migrations 0015, 0021)
- A **6-digit code** is e-mailed (valid **10 minutes**) and entered in the app on `/verify-email`; there is no link to click.
- The code is random (crypto/rand) and stored only as a salted Argon2id hash in `email_verification_tokens.token_hash`.
- `POST /v1/auth/verify-email` needs a session: the code is compared with the newest open code of **that user only**. Each code allows
  **5 wrong guesses** (`attempts`); the 5th burns it and a new one must be requested (`/verify-email/resend`, 60 s cooldown). The row is
  locked (`FOR UPDATE`), so parallel guesses cannot dodge the counter. Per-IP rate limits apply on top.
- Checking the code and setting `users.email_verified_at` happen in one transaction; a code minted for an old address cannot verify a changed one.
- Requesting a new code closes older open ones. Following a password-reset link also marks the address verified (it proves the mailbox).
- `email_verified` is returned by login/register (`user`) and `GET /users/me`. **Verification is not enforced**: unverified users can use the app;
  new accounts are sent straight to the code screen and the app shows a banner until verified. Making it a gate (e.g. block chat until verified)
  is a one-line policy change when you want it.
- Local dev with `ConsoleMailer` (no `RESEND_API_KEY`): the code is printed in the API log.

## Account management (migration 0016)
- Wrong confirmation passwords return **422**, never 401: the web client treats 401 as "session expired" and would sign the person out over a typo.
- **Change e-mail**: needs the current password; the new address is stored in `users.pending_email`, and a 6-digit code (10 min) bound to it is mailed there.
  Entering the code swaps `email` and sets `email_verified_at` in one transaction; `/users/me` reports `pending_email` meanwhile. A second change request closes the first code; if someone
  else took the address in between, the unique index rejects the swap and the code is treated as expired. Shares the 60 s resend cooldown (no inbox spam), plus per-IP limits.
- **Change password**: needs the current password, revokes all sessions except the calling device's.
- **Delete account**: needs the password. The row is kept (`deleted_at`, `status='deleted'`) so old messages keep a valid sender, shown as "Deleted user"; e-mail/username are replaced by unique
  placeholders (so the originals can be registered again), and password hash, phone numbers, avatar, about, sessions, devices, push tokens, friendships, notifications and pending tokens are removed.
  Messages the person sent remain visible to the other participants. Already-issued access tokens stay valid until they expire (short TTL); refresh is dead immediately.
  This is a soft delete, not a GDPR-style hard erase: add a purge job if you need one.

## Not done / caveats
- No Resend webhooks (bounces/complaints are not tracked), and nothing is queued: if Resend is down both attempts fail and the user presses Resend.
- No "your e-mail was changed" notice to the OLD address, and no e-mail on password change — worth adding before a public launch.
- Per-account login lockout is not implemented (only per-IP limits).
- The Android client still speaks phone/OTP and does not show the verification state.

## Security notices & lockout
- **Notices** (`internal/auth/notice.go`): after a password change, a completed password reset, and when an e-mail change is *requested*, the account's current address
  gets a short notice (no link, no secret; the new address is masked as `n***@new.com`). Sent asynchronously; a mailer that doesn't implement
  `SecurityNotifier` simply skips them. A final notice goes to the old address when the swap completes.
- **Per-account lockout** (`lockout.go`): 10 failed sign-ins in 15 minutes → `429 account_locked` (+ `Retry-After: 900`), even for the right password.
  Attempts made while locked are not counted, so an attacker cannot extend the lock forever; a successful sign-in or a completed password reset clears the count.
  Trade-offs: it can be used to annoy a known account for 15 minutes (the per-IP limit still applies), and the 429 reveals that the account exists.

## Bounce / complaint webhooks
`POST /v1/webhooks/resend` (mounted only when `RESEND_WEBHOOK_SECRET` is set). In Resend → Webhooks add `https://YOUR-DOMAIN/api/v1/webhooks/resend`
with events `email.bounced` and `email.complained`. Requests are verified with Resend's Svix signature (HMAC-SHA256, ±5 min); anything unsigned gets 401.
A **permanent** bounce or a spam complaint adds the address to `email_suppressions` (migration 0019) and `SuppressingMailer` then skips sending to it
(verification, reset and notices). Soft bounces do not suppress. The check fails open, so a database hiccup never blocks a password-reset mail.
To un-suppress an address: `DELETE FROM email_suppressions WHERE email = '...'`.
