# k6 load test suite

**Not executed anywhere yet** — see `../REPORT.md`. This sandbox has no
way to build or run the Go API server, so there is no live endpoint to
point k6 at. This is the ready-to-run suite for whoever deploys Phase 12's
Terraform (or any other environment with a real running API).

## Install k6
```bash
# see https://k6.io/docs/get-started/installation/ for your platform
```

## Prerequisites on the target environment
1. The API must be reachable at `BASE_URL` (REST) and `WS_URL` (WebSocket).
2. `APP_ENV` must be `local` (or another value that mounts the debug OTP-peek
   route) — **this load test logs in through the real OTP flow and reads
   the code back via `GET /v1/auth/debug/otp/{phone}`**, the same endpoint
   `backend/README.md`'s Phase 1 curl walkthrough uses. **Never point this
   at a real production deployment** — that route must not exist there
   (`cmd/api/main.go` only mounts it when `cfg.Env == "local"` and logs a
   loud warning when it does).
3. A fixed second test account must already exist to act as every virtual
   user's message-recipient peer — seed one (see `../RUNBOOK.md` step 6)
   and pass its user ID as `PEER_USER_ID`.

## Run
```bash
k6 run \
  -e BASE_URL=https://staging.example.com \
  -e WS_URL=wss://staging.example.com/ws \
  -e PEER_USER_ID=<seeded-user-uuid> \
  chat-load-test.js
```

## What it measures
- `http_req_duration` — REST latency for OTP login + message send, under
  a ramp from 20 to 200 concurrent virtual users (a steady-state phase
  plus a spike, per the roadmap's load-test intent).
- `ws_message_ack_latency_ms` — time from sending a `message.send` WS
  frame to receiving its `message.ack`, with 50 concurrent long-lived
  WS connections each sending roughly one message every 3 seconds.
- `ws_message_ack_missing` — a message sent over WS that *never* got
  acked within its connection's lifetime. This should be exactly 0; the
  architecture doc's backpressure design says message events are never
  silently dropped (only typing/presence are), so any non-zero count
  here is a real bug to chase, not an acceptable loss rate to tune a
  threshold around.

## A known limitation of this script, disclosed up front
Every virtual user messages the *same* fixed `PEER_USER_ID` — deliberately,
to stress the same single-conversation contention pattern
`../REPORT.md` section 1 already measured at the database layer alone.
This is useful for finding the worst-case ceiling, but it is **not** a
realistic traffic shape (real users are spread across many conversations).
Run a second pass with `PEER_USER_ID` randomized per VU (swap in a pool of
seeded peers) to also get a best-case/typical-case number — that variant
is not included here to keep this one script's intent unambiguous.
