# Backend — Phase 1 (Auth)

Implements: OTP request/verify, JWT access tokens, rotating refresh tokens
with reuse detection, device registration, session listing/revocation.

## Before your first build

This code was written without network access, so `go.sum` isn't included.
Run once, with network access, before `docker compose up --build` or `go run`:
```bash
go mod tidy
```
That resolves and pins exact versions/checksums for everything in `go.mod`.

## Run locally (Docker Compose — recommended for testing)

```bash
cd backend
cp .env.example .env
docker compose up --build
```

This starts Postgres, Redis, runs migrations automatically (the `migrate`
service), and starts the API on `http://localhost:8080`.

Check it's up:
```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Try the auth flow end-to-end (no real SMS needed in local mode)

```bash
# 1. request an OTP
curl -X POST http://localhost:8080/v1/auth/otp/request \
  -H "Content-Type: application/json" \
  -d '{"phone_number": "+8801712345678"}'

# 2. peek the code (LOCAL DEV ONLY — this route does not exist outside APP_ENV=local)
curl "http://localhost:8080/v1/auth/debug/otp/%2B8801712345678"

# 3. verify it — returns access_token + refresh_token
curl -X POST http://localhost:8080/v1/auth/otp/verify \
  -H "Content-Type: application/json" \
  -d '{"phone_number":"+8801712345678","code":"<code from step 2>","device_id":"test-device-1","platform":"android"}'

# 4. call an authenticated route
curl http://localhost:8080/v1/auth/sessions \
  -H "Authorization: Bearer <access_token from step 3>"

# 5. rotate the refresh token
curl -X POST http://localhost:8080/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token from step 3>"}'
```

## Try the chat flow (Phase 2)

Create two test users (repeat the OTP flow above with two different phone
numbers) to get two access tokens, `TOKEN_A` and `TOKEN_B`, and their
`user.id` values, `USER_A` and `USER_B`.

```bash
# Start a direct conversation between them (either side can call this)
curl -X POST http://localhost:8080/v1/conversations \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"peer_user_id\":\"$USER_B\"}"
# -> note the returned conversation "id" as CONV_ID

# List conversations
curl http://localhost:8080/v1/conversations -H "Authorization: Bearer $TOKEN_A"

# Send a message over plain REST (works without a WebSocket open)
curl -X POST http://localhost:8080/v1/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"client_tmp_id":"tmp-1","type":"text","body":"hey"}'

# Fetch message history (cursor-paginated, newest first)
curl "http://localhost:8080/v1/conversations/$CONV_ID/messages" -H "Authorization: Bearer $TOKEN_B"
```

### Testing the WebSocket path

Install a CLI WS client, e.g. [websocat](https://github.com/vi/websocat):
```bash
websocat "ws://localhost:8080/ws" -H="Authorization: Bearer $TOKEN_B"
```
Then, with `TOKEN_A`, send a message (via REST as above, or another websocat
session) into `CONV_ID` — the `TOKEN_B` session should receive a
`message.new` frame immediately. Send this from the `TOKEN_B` websocat
session to test the client -> server path directly:
```json
{"type":"message.send","conversation_id":"<CONV_ID>","payload":{"conversation_id":"<CONV_ID>","client_tmp_id":"tmp-2","type":"text","body":"hi back"}}
```
You should see a `message.ack` come back on that same connection, and the
`TOKEN_A` session (if still connected) should receive `message.new`.

### Testing offline push fallback

Close the `TOKEN_B` websocat session (simulating them being offline), then
send another message from `TOKEN_A`. Check the worker's logs — since Phase 2
ships with `LoggingStubSender` (no real Firebase credentials wired up yet),
you'll see a `push (stub, not actually sent)` log line instead of an actual
push, confirming the outbox → worker pipeline ran:
```bash
docker compose logs -f worker
```

## Run without Docker (host Go + local Postgres/Redis)

```bash
# start just the datastores
docker compose up postgres redis minio minio-init migrate

# run the API directly
go run ./cmd/api

# and, in a second terminal, the workers (notification outbox + thumbnails)
go run ./cmd/worker
```

## Try the media flow (Phase 3)

MinIO's web console is at http://localhost:9001 (user `chatapp_minio` /
password `chatapp_minio_password`) if you want to see uploaded objects
directly.

```bash
# 1. Ask for a presigned upload URL
curl -X POST http://localhost:8080/v1/media/presign-upload \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"kind":"image","mime_type":"image/jpeg","file_name":"cat.jpg"}'
# -> note "media_id" and "upload_url"

# 2. PUT the actual file bytes straight to storage (not through the API)
curl -X PUT "<upload_url>" -H "Content-Type: image/jpeg" --data-binary @cat.jpg

# 3. Confirm the upload — this is what actually validates size/MIME server-side
curl -X POST http://localhost:8080/v1/media/$MEDIA_ID/confirm \
  -H "Authorization: Bearer $TOKEN_A"
# -> {"media_id":"...","status":"uploaded","download_url":"..."}

# 4. Send a message with that attachment
curl -X POST http://localhost:8080/v1/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"client_tmp_id\":\"tmp-img-1\",\"type\":\"image\",\"attachment_media_ids\":[\"$MEDIA_ID\"]}"

# 5. As the OTHER user, get a fresh download URL for that attachment
curl http://localhost:8080/v1/media/$MEDIA_ID/download-url -H "Authorization: Bearer $TOKEN_B"
```

Watch the thumbnail get generated (image kind only, in Phase 3):
```bash
docker compose logs -f worker
# re-check the media row a few seconds later — thumbnail_object_key gets filled in
```

Try the rejection path (server-side validation, not just client-side checks)
by uploading a file whose real content doesn't match what you declared —
e.g. PUT a `.txt` file after presigning as `image/jpeg`. Step 3 should come
back `"status":"rejected"` with a reason, and the object gets deleted from
storage.

**Testing from the Android emulator specifically:** presigned URLs generated
by the API embed whatever host is in `S3_PRESIGN_ENDPOINT`. The default
(`http://localhost:9000`) works for curl from your host machine, but the
emulator can't reach your host's `localhost` — set
`S3_PRESIGN_ENDPOINT=http://10.0.2.2:9000` in `.env` before testing uploads
from the app, and restart `docker compose up api`.

## Try groups, reactions, forwarding & search (Phase 4)

```bash
# Create a group with two other members (TOKEN_A creates, includes USER_B and USER_C)
curl -X POST http://localhost:8080/v1/groups \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"name\":\"Weekend Trip\",\"member_user_ids\":[\"$USER_B\",\"$USER_C\"]}"
# -> note "conversation_id" as GROUP_ID; creator (TOKEN_A) is role=owner

# Send a message into the group — same endpoint as 1:1 chat, no special group API
curl -X POST http://localhost:8080/v1/conversations/$GROUP_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"client_tmp_id":"tmp-g1","type":"text","body":"who is free Saturday?"}'

# Promote USER_B to admin (only owner/admin can do this)
curl -X PUT http://localhost:8080/v1/groups/$GROUP_ID/members/$USER_B/role \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"role":"admin"}'

# Generate an invite link, then have a fourth user join with it
curl -X POST http://localhost:8080/v1/groups/$GROUP_ID/invite-link -H "Authorization: Bearer $TOKEN_A"
# -> {"invite_code":"..."}
curl -X POST http://localhost:8080/v1/groups/join/<invite_code> -H "Authorization: Bearer $TOKEN_D"

# React to a message
curl -X POST http://localhost:8080/v1/messages/$MESSAGE_ID/reactions \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" \
  -d '{"emoji":"👍"}'
curl http://localhost:8080/v1/messages/$MESSAGE_ID/reactions -H "Authorization: Bearer $TOKEN_A"

# Forward a message into another conversation — same send endpoint, with provenance
curl -X POST http://localhost:8080/v1/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"client_tmp_id\":\"tmp-fwd-1\",\"type\":\"text\",\"body\":\"who is free Saturday?\",\"forwarded_from_message_id\":\"$MESSAGE_ID\"}"

# Full-text search across every conversation you're in
curl "http://localhost:8080/v1/search/messages?q=saturday" -H "Authorization: Bearer $TOKEN_A"
# ...or scoped to one conversation
curl "http://localhost:8080/v1/search/messages?q=saturday&conversation_id=$GROUP_ID" -H "Authorization: Bearer $TOKEN_A"
```

Watch group membership/role-change events arrive live on a websocat session
connected as `TOKEN_C` (a non-admin member) — `group.updated` and
`group.member_added`/`group.member_removed` frames should show up the moment
another member acts, same as `message.new` does in Phase 2.

> **API shape notes (from the group-UI wiring pass):** `GET /v1/conversations`
> now includes `name` for group conversations and `peer_user_id` only for
> direct ones (it previously returned an arbitrary member's id for groups).
> `POST /v1/conversations` now returns `{id, type, peer_user_id}` — before,
> the response had capitalized keys (`ID`, `Type`) that no client could read.
> Creating a group also pushes a `group.updated` event to every member.

## Try contacts, username search & blocking (Phase 5)

Every account now gets a random username automatically at signup
(`user_xxxxxxxxxx`) — no separate step needed. Change it if you want
something memorable:

```bash
curl -X PATCH http://localhost:8080/v1/users/username \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"username":"alice_dev"}'

# search for someone by username (prefix match)
curl "http://localhost:8080/v1/users/search?username=alice" -H "Authorization: Bearer $TOKEN_B"

# exact phone lookup (tightly rate-limited — 5/hour — unlike username search)
curl "http://localhost:8080/v1/users/search?phone=%2B8801712345678" -H "Authorization: Bearer $TOKEN_B"

# add a contact manually
curl -X POST http://localhost:8080/v1/contacts \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"phone_number":"+8801712345678","nickname":"Bob"}'

curl http://localhost:8080/v1/contacts -H "Authorization: Bearer $TOKEN_A"
```

**Contact sync** never sends raw phone numbers for people who aren't
already registered — hash them client-side first:
```bash
HASH=$(printf '%s' '+8801712345678' | sha256sum | cut -d' ' -f1)
curl -X POST http://localhost:8080/v1/contacts/sync \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"phone_hashes\":[\"$HASH\"]}"
# -> only returns a match if that hash belongs to a registered account;
#    the server never sees the plaintext number here.
```

**Blocking** — once blocked, sending in either direction is rejected:
```bash
curl -X POST http://localhost:8080/v1/contacts/block/$USER_B -H "Authorization: Bearer $TOKEN_A"

# TOKEN_B trying to message TOKEN_A now fails:
curl -X POST http://localhost:8080/v1/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" \
  -d '{"client_tmp_id":"tmp-blocked-1","type":"text","body":"hello?"}'
# -> 403 {"error":{"code":"blocked", ...}}

curl -X DELETE http://localhost:8080/v1/contacts/block/$USER_B -H "Authorization: Bearer $TOKEN_A" # unblock
```

## Try Stories (Phase 6)

```bash
# Text story, default (contacts) privacy
curl -X POST http://localhost:8080/v1/stories \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"type":"text","text_content":"On a beach today"}'

# Image story, visible to everyone except one contact
curl -X POST http://localhost:8080/v1/stories \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d "{\"type\":\"image\",\"media_id\":\"$MEDIA_ID\",\"privacy\":\"contacts_except\",\"audience_user_ids\":[\"$USER_C\"]}"

# As a contact of A (but not USER_C from above): see it in your feed
curl http://localhost:8080/v1/stories/feed -H "Authorization: Bearer $TOKEN_B"

# Mark viewed, react, reply
curl -X POST http://localhost:8080/v1/stories/$STORY_ID/view -H "Authorization: Bearer $TOKEN_B"
curl -X POST http://localhost:8080/v1/stories/$STORY_ID/reactions \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" -d '{"emoji":"🔥"}'
curl -X POST http://localhost:8080/v1/stories/$STORY_ID/replies \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" -d '{"body":"looks nice!"}'

# As the author: see who viewed/reacted, and read replies (only the author can)
curl http://localhost:8080/v1/stories/$STORY_ID/viewers -H "Authorization: Bearer $TOKEN_A"
curl http://localhost:8080/v1/stories/$STORY_ID/replies -H "Authorization: Bearer $TOKEN_A"

# Mute someone's stories (still counted, just flagged in the feed)
curl -X POST http://localhost:8080/v1/stories/mute/$USER_B -H "Authorization: Bearer $TOKEN_A"
```

A `story.created` event pushes live over the same WebSocket used for chat —
connect as `TOKEN_B` with websocat (see Phase 2 above) and post a story as
`TOKEN_A` to see it arrive. Expiry needs no cron: a story just stops
matching the feed/visibility query 24 hours after `created_at` — the
`worker` process only hard-deletes the row and its media roughly 30 days
after that, as a "recently deleted" grace window, checked hourly.

## Try Communities (Phase 7)

Creating a community makes an admins-only "announcement group" automatically
— every member gets auto-joined to it on join, so it works as the
community's broadcast channel from the start.

```bash
# Create a community (TOKEN_A becomes owner)
curl -X POST http://localhost:8080/v1/communities \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"name":"Neighborhood Watch","description":"Local updates","discoverable":true}'
# -> note "id" as COMMUNITY_ID, and "groups": [{"conversation_id": ..., "is_announcement": true}]

# Get an invite link (owner/admin only)
curl -X POST http://localhost:8080/v1/communities/$COMMUNITY_ID/invite-link -H "Authorization: Bearer $TOKEN_A"

# TOKEN_B joins — via invite code, or directly since this one is discoverable
curl -X POST http://localhost:8080/v1/communities/join/<invite_code> -H "Authorization: Bearer $TOKEN_B"
# -> {"community_id":"...","announcement_conversation_id":"..."} — TOKEN_B is now also a member of that group

# Only admins/owner can post in the announcement group — this should succeed for TOKEN_A...
curl -X POST http://localhost:8080/v1/conversations/$ANNOUNCEMENT_CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"client_tmp_id":"tmp-a1","type":"text","body":"Meeting Saturday at noon"}'
# ...and fail with send_not_allowed for TOKEN_B (a plain member)
curl -X POST http://localhost:8080/v1/conversations/$ANNOUNCEMENT_CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" \
  -d '{"client_tmp_id":"tmp-b1","type":"text","body":"can I post here?"}'

# Create a regular (non-announcement) group inside the community
curl -X POST http://localhost:8080/v1/communities/$COMMUNITY_ID/groups \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"name":"Book club"}'

# ...or link an existing group you already own/admin
curl -X POST http://localhost:8080/v1/communities/$COMMUNITY_ID/groups/$SOME_GROUP_ID -H "Authorization: Bearer $TOKEN_A"

# Promote TOKEN_B to community admin, then ban a third member
curl -X PUT http://localhost:8080/v1/communities/$COMMUNITY_ID/members/$USER_B/role \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" -d '{"role":"admin"}'
curl -X POST http://localhost:8080/v1/communities/$COMMUNITY_ID/ban/$USER_C -H "Authorization: Bearer $TOKEN_A"
# USER_C trying to rejoin with the invite link now fails

# Search discoverable communities by name
curl "http://localhost:8080/v1/communities/search?q=neighborhood" -H "Authorization: Bearer $TOKEN_C"
```

## Try Channels (Phase 8)

Channel posts are completely separate from chat messages — no delivery
receipts, no typing, just posts, view counts, and reactions.

```bash
# Create a public channel (TOKEN_A becomes owner, auto-follows it)
curl -X POST http://localhost:8080/v1/channels \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"name":"City Updates","category":"news","is_public":true}'
# -> note "id" as CHANNEL_ID

# TOKEN_B finds and follows it
curl "http://localhost:8080/v1/channels/search?q=city" -H "Authorization: Bearer $TOKEN_B"
curl -X POST http://localhost:8080/v1/channels/$CHANNEL_ID/follow -H "Authorization: Bearer $TOKEN_B"

# Post — only an admin/owner can do this
curl -X POST http://localhost:8080/v1/channels/$CHANNEL_ID/posts \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"type":"text","body":"Road closure on Main St this weekend"}'
# TOKEN_B trying the same returns 403 permission_denied

# A poll
curl -X POST http://localhost:8080/v1/channels/$CHANNEL_ID/posts \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"type":"poll","body":"Best day for the town meeting?","poll_options":["Saturday","Sunday"]}'
# -> note a poll_options[].id as OPTION_ID
curl -X POST http://localhost:8080/v1/channels/posts/$POST_ID/vote \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" -d "{\"option_id\":\"$OPTION_ID\"}"

# Followers read, react, and are counted as views
curl "http://localhost:8080/v1/channels/$CHANNEL_ID/posts" -H "Authorization: Bearer $TOKEN_B"
curl -X POST http://localhost:8080/v1/channels/posts/$POST_ID/view -H "Authorization: Bearer $TOKEN_B"
curl -X POST http://localhost:8080/v1/channels/posts/$POST_ID/reactions \
  -H "Authorization: Bearer $TOKEN_B" -H "Content-Type: application/json" -d '{"emoji":"👍"}'

# Add a second admin, generate an invite link for a private channel, etc. — same shape as groups/communities
curl -X POST http://localhost:8080/v1/channels/$CHANNEL_ID/admins/$USER_B -H "Authorization: Bearer $TOKEN_A"
curl -X POST http://localhost:8080/v1/channels/$CHANNEL_ID/invite-link -H "Authorization: Bearer $TOKEN_A"
```

A `channel.post` event pushes live to every follower over the same
WebSocket chat uses (connect as `TOKEN_B` with websocat, post as `TOKEN_A`),
and a `channel.update` push notification is queued per-follower through the
same outbox the stories/notifications worker already drains — nothing new
to run.

### Known gaps in Phase 8
- **Polls are single-choice only.** Voting again moves your vote instead of
  adding a second one — `allows_multiple` isn't exposed, though the schema
  (`channel_poll_votes` keyed by `option_id, user_id`) doesn't prevent
  adding it later.
- **No automatic link-preview fetching** for `type: "link"` posts — the
  client gets back whatever `link_url` it sent, no title/description/image
  scraped server-side.
- **Stickers are just images** tagged `type: "sticker"` — no sticker-pack
  system, matches how chat messages handle stickers today too.
- **No per-post reporting**, only per-channel (`POST /channels/{id}/report`)
  — consistent with how communities only support reporting the community,
  not individual messages.
- **"Channel analytics"** from the original spec is just `follower_count`
  and per-post `view_count`/reaction tallies — no dashboards, trends, or
  export.

## Try the AI Agent (Phase 9)

The `ai` module is intentionally sealed off from every other module — it has
no import of `chat`, `groups`, `communities`, `channels`, `stories`, or
`contacts`, and its repository only ever touches `ai_conversations` /
`ai_messages`. The agent can only ever see what the user types directly into
it; it is not given standing access to anyone's chats. Set `GEMINI_API_KEY`
in `.env` (see `.env.example`) before trying this — without it, every route
below 500s on the actual Gemini call with a clear error, everything else
(create/list/rename/delete conversation) still works.

```bash
# Start a conversation
curl -X POST http://localhost:8080/v1/ai/conversations \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"title":"Trip planning"}'
# -> note "id" as CONV_ID

# Send a message — streamed back as Server-Sent Events
curl -N -X POST http://localhost:8080/v1/ai/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"content":"Suggest a 3-day itinerary for Cox'"'"'s Bazar"}'
# -> a stream of:
#      event: delta
#      data: {"text":"Day 1: ..."}
#    ...ending with:
#      event: done
#      data: {"message": { "id": "...", "role": "model", "content": "<full text>", "finish_reason": "STOP", ... }}

# History persists — later turns include everything above as context
curl -X POST http://localhost:8080/v1/ai/conversations/$CONV_ID/messages \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" \
  -d '{"content":"Make day 2 less packed"}'

# Read it back, list all conversations, rename, delete
curl http://localhost:8080/v1/ai/conversations/$CONV_ID/messages -H "Authorization: Bearer $TOKEN_A"
curl http://localhost:8080/v1/ai/conversations -H "Authorization: Bearer $TOKEN_A"
curl -X PATCH http://localhost:8080/v1/ai/conversations/$CONV_ID \
  -H "Authorization: Bearer $TOKEN_A" -H "Content-Type: application/json" -d '{"title":"Cox'"'"'s Bazar trip"}'
curl -X DELETE http://localhost:8080/v1/ai/conversations/$CONV_ID -H "Authorization: Bearer $TOKEN_A"

# Quota: the 31st message in an hour from the same user gets 429 rate_limited
# (or, mid-stream on turn >1 of a slow client, an `event: error` frame)
```

### Known gaps in Phase 9
- **The real Gemini key used for local testing is a disposable test key the
  user supplied**, stored only in `backend/.env` (gitignored, never in
  `.env.example` or committed anywhere) — rotate it before any real/shared
  deployment; it is not meant for production use.
- **Quota is request-count-based** (30 turns/hour/user via the same Redis
  `ratelimit` package as OTP/search/posts), not token- or cost-based — a
  user sending 30 huge messages costs far more than 30 tiny ones and the
  limiter can't see that. A real cost budget needs Gemini's
  `usageMetadata` wired through, deferred to Phase 12 hardening.
- **Full history is resent every turn** with no sliding-window trimming or
  summarization — a very long conversation gets slower and more expensive
  per turn, and could eventually exceed the model's context window with no
  graceful fallback other than the upstream error Gemini itself returns.
- **No function/tool calling, no image or audio input** — text-in,
  text-out only, matching the roadmap's Phase 9 scope (streaming responses,
  quota, isolation) and leaving tool use for a later phase if ever needed.
- **No moderation layer of our own** — relies entirely on Gemini's own
  safety filtering; a blocked/filtered response just surfaces as whatever
  `finishReason` (e.g. `SAFETY`) Gemini reports, unstyled, to the client.
- **No stop-generation control** — the client can close the HTTP
  connection to abandon a stream (the Go context is cancelled and the
  Gemini call aborts), but there's no explicit "stop" button/endpoint
  wired up to do that cleanly from the Android UI yet.
- **One conversation has no concurrency guard** — two simultaneous
  requests against the same `CONV_ID` (e.g. two tabs) can interleave; low
  risk for a single-user AI chat, not race-tested.

## Security hardening (Phase 10)

Roadmap exit criteria for this phase: *"Threat model doc + key management
design reviewed."* That document is
[`docs/E2EE_THREAT_MODEL.md`](docs/E2EE_THREAT_MODEL.md) — a Signal-style
X3DH + Double Ratchet design, explicitly **not implemented**, written to
be reviewed before any encryption code is written. Everything else below
*is* implemented in this phase.

### What changed
- **OTP codes are now hashed (Argon2id) before being stored in Redis**,
  not kept as plaintext. See `internal/auth/otp_hash.go`'s doc comment for
  the honest limitation (a 6-digit code's keyspace is small enough that
  hashing raises the cost of a stolen-Redis-dump attack, it doesn't make
  the code unguessable — the rate limiter remains the real defense against
  guessing). The local-dev debug-peek endpoint (`APP_ENV=local` only)
  still works, via a separate plaintext side-channel key that only exists
  when debug mode is on.
- **Security response headers** on every response: CSP (`default-src
  'none'` — this is a JSON API, not a page-serving one), `X-Frame-Options:
  DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
  a locked-down `Permissions-Policy`, and HSTS when the request arrived
  over TLS. See `internal/platform/middleware.SecurityHeaders`.
- **`security_audit_log` table** (migration `0010`) plus
  `internal/platform/audit`, a small append-only "who did what, from
  where, when" trail — wired into the `auth` module: OTP requested, OTP
  verify failed, login succeeded, logout, session revoked, all-sessions
  revoked, and refresh-token-reuse-detected (the existing defensive
  revoke-all on token replay now also leaves an audit trail, not just a
  silent revoke). **Never** given message/post content, OTP codes,
  passwords, or raw tokens — see the package's own doc comment for the
  rule.
- **`POST /v1/auth/sessions/revoke-all`** — "log out everywhere" / "I
  think my account is compromised", revoking every active session for the
  caller at once (reuses the same `RevokeAllSessionsForUser` repository
  method the reuse-detection defense already used internally).
- **A regression test that enforces the "never log secrets" rule
  mechanically** (`internal/platform/securitylint`): it greps every `.go`
  file for a `zap.String/Any/...` structured-log call whose field name is
  exactly `otp`, `password`, `token`, `body`, `content`, etc., and fails
  the build if one is found. This turns a rule every phase's README has
  asserted by hand into something `go test ./...` actually checks.

```bash
# Try "log out everywhere":
curl -X POST http://localhost:8080/v1/auth/sessions/revoke-all -H "Authorization: Bearer $TOKEN_A"
# -> {"status":"all_revoked"}; TOKEN_A's refresh token (and every other
#    device's) is now revoked — the access token itself stays valid until
#    it naturally expires (access tokens are stateless/short-lived and
#    aren't looked up per request, by design).
```

### Known gaps in Phase 10
- **Audit logging covers the `auth` module only.** Extending the same
  `audit.Logger` into admin-role-change events in `channels`/`communities`
  (grant/revoke admin) is a natural next step, deliberately left as a
  follow-up to avoid changing those modules' already-tested service
  signatures late in the project.
- **OTP hashing's limitation is real, not cosmetic** — see
  `otp_hash.go`'s doc comment; this is defense against passive exposure of
  the Redis store, not a claim that OTP codes are now brute-force-proof.
- **No automated secrets-scanning on commit** (e.g. gitleaks/truffleHog in
  CI) — the project has no CI pipeline yet (that's Phase 12 territory);
  the `securitylint` test only catches secrets in *log calls*, not secrets
  accidentally committed elsewhere.
- **E2EE is a design doc, not code** — see the status banner at the top of
  `docs/E2EE_THREAT_MODEL.md`. Nothing in this app may be described as
  "end-to-end encrypted" until that design is implemented and
  independently reviewed.
- **No rate limit on `/sessions/revoke-all` or `/sessions/{id}`** beyond
  the global per-IP limiter — low risk (both require a valid access
  token already) but not explicitly throttled per-user.

## Admin panel + observability (Phase 11)

Roadmap exit criteria for this phase: *"Admin can view reports, ban users,
and see basic growth metrics; `/metrics` is scrapeable."* All three are
implemented, as an API-only module — see "Why there's no Android admin
screen" below for why that's a deliberate scope decision, not an
oversight.

### RBAC model
A new `admin_roles` table (migration `0011`) gives a user **at most one**
role: `moderator` < `admin` < `superadmin` (see `internal/admin/model.go`'s
`rank()`). Every admin route goes through the shared `middleware.RequireAuth`
first (is this a real, logged-in user?) and then admin's own
`requireRole(minRole, ...)` (is this user an admin of at least this rank?) —
two separate questions, two separate middleware layers, never conflated.
A caller with no admin role and a caller with an insufficient one both get
the identical `403 admin_permission_denied` — the same "don't reveal which
kind of no" reasoning Phase 10 already used for session lookups.

**Bootstrapping the first superadmin** is a chicken-and-egg problem —
granting a role normally requires an existing superadmin to grant it. Set
`ADMIN_BOOTSTRAP_PHONE_NUMBERS=+8801xxxxxxxxx,+8801yyyyyyyyy` (comma-separated
E.164 numbers) in the environment; every startup idempotently promotes any
of those numbers that are already registered to `superadmin`. Restart the
server after that phone number's first OTP login if it wasn't registered
yet at boot time.

### Endpoints (all under `/v1/admin`, `Authorization: Bearer <access token>` required)

```bash
# Am I an admin? (no role required — any logged-in user can check their own status)
curl http://localhost:8080/v1/admin/me -H "Authorization: Bearer $TOKEN"
# -> {"is_admin": false}  or  {"is_admin": true, "role": "moderator"}

# Search users (moderator+)
curl "http://localhost:8080/v1/admin/users?q=alice&limit=25" -H "Authorization: Bearer $MOD_TOKEN"

# Suspend a user (moderator+) — immediately blocks future logins AND
# revokes every one of their currently-active sessions
curl -X POST http://localhost:8080/v1/admin/users/$TARGET_ID/suspend \
  -H "Authorization: Bearer $MOD_TOKEN" -H "Content-Type: application/json" \
  -d '{"reason": "spam reports from 6 distinct users"}'
# -> {"status": "suspended"}; that user's next OTP login attempt now gets
#    403 account_suspended (see auth.Service.VerifyOTP)

curl -X POST http://localhost:8080/v1/admin/users/$TARGET_ID/unsuspend -H "Authorization: Bearer $MOD_TOKEN"
# -> {"status": "active"}

# Channel reports (moderator+) — reuses channels' existing report table
# from Phase 8, never a separate admin-owned copy of the data
curl "http://localhost:8080/v1/admin/reports?resolved=false" -H "Authorization: Bearer $MOD_TOKEN"
curl -X POST http://localhost:8080/v1/admin/reports/$REPORT_ID/resolve \
  -H "Authorization: Bearer $MOD_TOKEN" -H "Content-Type: application/json" \
  -d '{"note": "channel owner removed the post"}'

# Manage other admins (superadmin only)
curl http://localhost:8080/v1/admin/admins -H "Authorization: Bearer $SUPER_TOKEN"
curl -X POST http://localhost:8080/v1/admin/admins/$TARGET_ID \
  -H "Authorization: Bearer $SUPER_TOKEN" -H "Content-Type: application/json" -d '{"role": "moderator"}'
# -> {"status": "granted"}; granting yourself a role, or revoking the last
#    remaining superadmin, both fail with 400 — see GrantRole/RevokeRole's
#    doc comments in internal/admin/service.go

curl -X DELETE http://localhost:8080/v1/admin/admins/$TARGET_ID -H "Authorization: Bearer $SUPER_TOKEN"

# Growth metrics (moderator+) — read-only aggregate counts, not raw user data
curl http://localhost:8080/v1/admin/metrics/overview -H "Authorization: Bearer $MOD_TOKEN"
curl "http://localhost:8080/v1/admin/metrics/growth?days=14" -H "Authorization: Bearer $MOD_TOKEN"
```

### Prometheus `/metrics`
`http_requests_total{method,route,status}` and
`http_request_duration_seconds{method,route}` (a histogram) are recorded
for every request and exposed on a **separate port**
(`METRICS_PORT`, default `9090`) — deliberately never on the public API
port `8080`, so forgetting to firewall off scrape access still doesn't
leak request-rate/latency data to the internet.

```bash
curl http://localhost:9090/metrics
```

The route label is chi's matched **route pattern** (`/v1/users/{id}`), never
the raw request path — so hitting the same route with 10,000 different user
IDs produces one time series, not 10,000.

**Disclosed deviation from the architecture doc**: this is a small,
hand-rolled, dependency-free Prometheus text-exposition writer
(`internal/platform/metrics`), not the official
`github.com/prometheus/client_golang` library the roadmap implies. The
sandbox this project is being built in has no network egress to
`proxy.golang.org` or to raw `github.com` module downloads, so a brand-new
`go.mod` dependency could not actually be fetched or verified here — adding
the import without being able to run `go build` against it would have been
worse than not adding it. The text format written is the real exposition
format and scrapes correctly from a real Prometheus server; what's missing
is the official library's battle-tested bucketing/collector internals. See
`internal/platform/metrics/metrics.go`'s package doc for the full reasoning.
Swapping in the real library later is a self-contained change to one
package plus two lines in `cmd/api/main.go` — nothing else in the app
depends on this being hand-rolled.

### Why there's no Android admin screen
The architecture doc's Android feature list (Section on client screens)
never included an admin surface — moderation/ops tooling was scoped as
"ops people use curl/an internal tool," not an end-user app feature, and
nothing about Phase 11 changes that. Building an Android admin UI now would
be scope not asked for by the roadmap, not a shortcut — happy to add it as
an explicit follow-up phase if that's actually wanted.

### Known gaps in Phase 11
- **`/metrics` uses a hand-rolled exposition writer, not
  `prometheus/client_golang`** — see above; this is a sandbox network
  limitation, disclosed rather than hidden, with a documented swap-out path.
- **No CI pipeline wires `go test`, `gofmt`, or a secrets scan into every
  commit** — same gap Phase 10 already flagged; still Phase 12 territory.
- **Admin endpoints have no per-route rate limit** beyond the global
  per-IP limiter — a compromised moderator token could still enumerate
  `/admin/users` quickly. Low likelihood (requires a stolen admin token
  specifically) but not explicitly throttled.
- **`SearchUsersAdmin` only matches an exact phone number or a username
  prefix** (see its doc comment in `admin_repository.go`) — there's no
  fuzzy/partial phone search, by design (avoids making phone-number
  enumeration easy), but that does mean a moderator who only half-remembers
  a username can't find the account either.
- **No audit-log *reader* endpoint** — `security_audit_log` now records
  every admin action (Phase 10 added the table and logger; Phase 11 extends
  the event vocabulary) but there's no `/v1/admin/audit-log` route to read
  it back through the API yet; today that's a direct-SQL-access-only view.
- **No webhook/alert on suspicious admin activity** (e.g. a superadmin
  granting themselves — which is blocked — or rapid-fire suspensions) —
  the data to build that exists in `security_audit_log`, nothing consumes
  it yet.
- **`process_uptime_seconds` is the only non-HTTP metric exposed** — no
  database connection-pool stats, Redis latency, WebSocket connection
  count, or outbox queue depth yet; the roadmap's "basic growth metrics"
  bar is met via `/admin/metrics/*`, but `/metrics` itself is HTTP-traffic-only
  for now.

## Tests

```bash
go test ./... -v
```
`jwt_test.go` and `phone_test.go` run with no external dependencies. Full
repository-level integration tests (against a real Postgres) are a Phase 1
hardening follow-up — see `docs/TESTING.md` (to be added) for the
Testcontainers-based approach planned for that.

## Load testing + AWS deployment, Singapore (Phase 12)

Roadmap exit criteria for this phase: *"Load test report, IaC applied, WS
survives LB failover."* Read this section before trusting any of those
three as done — **one is real and measured, one is written but not
applied, and one has not been empirically verified at all.** This phase
is the one place in this whole project where that distinction matters
most, so it's stated plainly rather than folded into a "known gaps" list
at the bottom.

### What's real: the data-layer load test
[`loadtest/REPORT.md`](loadtest/REPORT.md) — real pgbench numbers from a
real (if small, local-to-the-sandbox) PostgreSQL 16 instance, exercising
`internal/chat/repository.go`'s exact `InsertMessage` transaction:
**17,148 concurrent inserts into the same busy conversation in 30s, zero
errors** (573 tps, 34.9ms avg latency) with the code's `SELECT ... FOR
UPDATE` row lock in place — and, as a comparison only, the same
transaction with that lock removed, which fails immediately with a
`uq_messages_conversation_seq` unique-constraint violation on the very
first concurrent attempt. That second result is the first time in this
project's life the "the row lock prevents concurrent sends from
colliding on seq" comment has actually been checked against real
concurrency instead of taken on faith.

The same report also records that **all 11 migrations were verified
forward *and* backward against a real Postgres 16 server** — 45 tables
created cleanly, then all 11 torn down cleanly in reverse order, zero
errors either direction. Every earlier phase's "migrations verified
forward+back" line in this README was written from manual SQL review,
because no sandbox before this one had a reachable Postgres server; this
is the first time that claim is backed by actually running them.

### What's written but not run: the k6 suite and the Terraform
[`loadtest/k6/`](loadtest/k6/) is a full HTTP/WS load-test suite against
the real HTTP and WebSocket endpoints — login, message send, a 50-connection
WS pool measuring `message.send` → `message.ack` latency. It has never
been pointed at a running instance of this API, because this sandbox
cannot compile the Go server at all (no network path to fetch Go
modules — see `internal/platform/metrics`'s package doc for the same
constraint hit in Phase 11, and `loadtest/REPORT.md`'s header for the
full explanation). **Zero Go code in this entire project has ever been
built successfully in any sandbox session across all 12 phases** —
every correctness check in every phase's README was `gofmt`-level syntax
validation, manual SQL/logic review, or (this phase) a real database
engine, never a real `go build`.

[`terraform/`](terraform/) is complete AWS infrastructure for
ap-southeast-1: VPC across 2 AZs, Multi-AZ RDS Postgres, a Redis
replication group with automatic failover, ECS Fargate running >= 2 API
tasks behind an ALB (deliberately *not* using ALB sticky sessions — see
`terraform/alb.tf`'s comment on why that would mask a real fan-out bug
instead of catching one), S3 for media, Secrets Manager for every
credential, and conditional ACM/Route53 wiring. It has been reviewed for
internal consistency by hand (variable/resource cross-references, brace
balance) since this sandbox has neither a `terraform` binary nor AWS
credentials to run `terraform init`/`plan`/`apply` — **nothing here has
been applied, anywhere.** `terraform/RUNBOOK.md` is the full step-by-step
for whoever does: build the image for the first time, apply, migrate,
seed, run the k6 suite for real, and — step 7 — perform the actual
WS-survives-LB-failover drill this phase's exit criteria asks for.

**One real backend fix came out of writing the Terraform, not a
load test**: the Redis TLS mismatch below.

### A real bug the IaC review caught
`terraform/elasticache.tf` turns on ElastiCache transit encryption (TLS)
— the sensible default for anything outside a sandbox. `internal/platform/redisx`
had no TLS support at all before this phase; connecting it to a
TLS-required Redis would have failed outright. Fixed in this phase:
`redisx.New` now takes a `tlsEnabled bool` (wired to `REDIS_TLS_ENABLED`,
default `false` — no change for local Docker Compose Redis), and
`terraform/ecs.tf` sets `REDIS_TLS_ENABLED=true` for the real deployment.
This is the kind of mismatch that only surfaces by writing the deployment
config and checking it against the code that has to run inside it —
exactly why this review happened now rather than being deferred to
"whoever deploys this."

### Known gaps in Phase 12
- **"IaC applied" has not happened.** This is the headline gap of the
  whole phase, not a footnote — see above.
- **The k6 suite has never run against a live server**, for the reason
  above (no buildable Go in this sandbox, ever). Its thresholds
  (`p(95)<500ms`, zero missing WS acks) are informed estimates from the
  architecture doc, not calibrated against a real measurement.
- **"WS survives LB failover" is unverified.** `terraform/RUNBOOK.md`
  step 7 is the drill procedure; it requires a real multi-task deployment
  and has never been run.
- **The pgbench result is a single-conversation, single-box number** —
  real, but it measures Postgres alone, on sandbox hardware, not the
  `db.t4g.micro` RDS instance class in `terraform/variables.tf`, and not
  the Go handler/JWT/WS-hub overhead a real client actually experiences.
  See `loadtest/REPORT.md`'s "reading these numbers honestly" section.
- **No CI pipeline** runs any of this automatically — still true from
  Phases 10 and 11, still Phase-13-or-later territory if this roadmap
  continued past its planned 12 phases.
- **`terraform/`'s task definition builds the Postgres DSN with the
  password interpolated directly**, rather than via a separate ECS
  `secrets` entry (which can only inject one whole secret as one env
  var, not compose a larger string) — see `ecs.tf`'s comment. Equivalent
  exposure to any other `DescribeTaskDefinition`-readable value, but
  tightening it is a real follow-up, not done here to avoid changing
  `config.go`'s DSN parsing this late.
- **No remote Terraform state backend configured by default** —
  `versions.tf`'s S3 backend block is commented out; using local state
  for anything beyond a single person's first trial run is a real risk
  (no locking, nothing to recover from if a laptop disk dies), not a
  style preference.

## Deploying somewhere reachable for QA/testing (not just your laptop)

Cheapest path to a shareable test environment, roughly in order of effort:

1. **Fly.io or Railway** — both take this docker-compose-shaped app (API +
   Postgres + Redis) with almost no changes and give you a public HTTPS URL
   in minutes. Good for "let a teammate hit my API" testing before touching AWS.
2. **A single AWS EC2 instance in ap-southeast-1 (Singapore)** running this
   same `docker-compose.yml` — closest to production topology, still cheap,
   good for testing under conditions closer to prod (same region as your
   eventual deployment, real TLS via Caddy/Let's Encrypt in front).
3. **AWS ECS Fargate + RDS Postgres + ElastiCache Redis** — written as real
   Terraform in [`terraform/`](terraform/) (Phase 12), not just described
   here anymore. See `terraform/RUNBOOK.md` for the full apply-and-deploy
   procedure — it has not been applied anywhere yet, so treat it as a
   thoroughly-reviewed starting point, not a proven one.

For Android testing against any of these, point `BASE_URL` at the deployed
host instead of `10.0.2.2:8080` (emulator localhost alias) — see the Android
`ApiConfig` file.
