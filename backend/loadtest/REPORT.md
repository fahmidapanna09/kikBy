# Phase 12 load test report

**What this is, honestly:** this sandbox cannot compile or run the Go API
server — outbound network access is restricted to a small allowlist that
excludes `proxy.golang.org` and even plain `github.com` module downloads
(see `internal/platform/metrics`'s package doc for the same constraint
hit in Phase 11), so `go build` has not succeeded here even once across
this whole project. That rules out a real end-to-end HTTP/WebSocket load
test against the actual API binary in this environment.

What *is* available here: real PostgreSQL 16 and Redis 7 server binaries,
installed and running locally (no Docker daemon available either, so this
is bare-metal-in-the-sandbox, not the project's own `docker-compose.yml`).
That's enough to genuinely, empirically load-test the **data layer** —
the actual SQL this project's hottest code path runs — rather than
inventing numbers. Section 1 below is real, measured output from this
sandbox. Section 2 is the HTTP/WS load test suite that should be run for
real once Phase 12's Terraform is applied somewhere with a buildable Go
toolchain (see `../terraform/` and `RUNBOOK.md`) — it is written and
ready, but **not executed anywhere, and its numbers are not in this
report**, because running it against nothing would mean fabricating a
result.

---

## 1. Real result: concurrent message-insert load test (pgbench)

### What's being tested
`internal/chat/repository.go`'s `InsertMessage` — the single hottest
write path in the whole app (every 1:1 and group message send). Its
transaction: an idempotency pre-check, `SELECT ... FOR UPDATE` on the
conversation row, `MAX(seq)+1`, the `INSERT`, then a conversation bump.
The two pgbench scripts in `pgbench/` replicate that transaction
statement-for-statement against a real Postgres instance — not a mock,
not a simulation.

**Setup:** one Postgres 16 database with all 11 migrations applied
(see "Migration verification" below), 20 seeded users, and **one single
busy group conversation** that every virtual client sends into — the
worst-case contention pattern (many devices posting into the same group
simultaneously), not an easy best-case spread across many conversations.

### Scenario A — the real code, as written (with the row lock)
```
pgbench -c 20 -j 4 -T 30 -f pgbench/message_insert_with_lock.sql
```
```
number of transactions actually processed: 17148
number of failed transactions: 0 (0.000%)
latency average = 34.865 ms
tps = 573.637311 (without initial connection time)
```
**Zero errors across 17,148 concurrent inserts into the same conversation.**
The `seq` column was never once double-assigned — the row lock does
exactly what its doc comment claims.

### Scenario B — the same transaction with the lock removed (comparison only)
This is **not** a proposal to change the code — it exists only to show
what the lock in Scenario A is actually preventing, with evidence instead
of a comment.
```
pgbench -c 20 -j 4 -T 15 -f pgbench/message_insert_without_lock.sql
```
```
pgbench: error: client 0 script 0 aborted in command 3 query 0:
  ERROR:  duplicate key value violates unique constraint "uq_messages_conversation_seq"
pgbench: error: client 1 script 0 aborted ... (same error)
... (all 20 clients, same error, every one on its first transaction)
```
Every single one of 20 concurrent virtual clients hit a `seq` collision
on essentially its *first* attempt the moment the lock was removed — two
clients both read the same `MAX(seq)`, both tried to insert the same
value, and the **`uq_messages_conversation_seq` unique constraint**
(migration `0002`) caught it and aborted the second writer rather than
silently corrupting message ordering. This is the "durable backstop"
Phase 2's design intended, and this is the first time in this project it's
actually been exercised under real concurrency instead of just asserted
in a comment. No data corruption occurred in either scenario — Scenario B
just demonstrates the lock is load-bearing, not decorative.

### Reading these numbers honestly
- **573 tps on one unpartitioned conversation row** is a sandbox-container
  number (shared vCPU, local Postgres, no network hop) — it is not a
  production capacity estimate for the AWS RDS instance class in
  `terraform/`. Treat it as a *shape-of-the-bottleneck* finding, not a
  capacity number: the row lock serializes all writers to *one*
  conversation, so this is a per-conversation ceiling, not a whole-system
  ceiling — a busy server with 10,000 different active conversations
  would see roughly 10,000× this before Postgres itself becomes the
  bottleneck, since those transactions don't contend with each other at
  all (different rows, different locks).
- **This only measures the database.** The real per-request latency a
  client sees also includes the Go handler, JWT verification, the WS
  hub fan-out, and the network — none of which exist in this number.
  Section 2's k6 suite is what measures that; it has not been run.

### Migration verification (forward + back, real Postgres)
All 11 `migrations/*.up.sql` were applied in order against a fresh
database — zero errors, 45 tables created. All 11 `*.down.sql` were then
applied in *reverse* order — zero errors, 0 tables remaining (full clean
teardown). This is the first time in this project's life that's been
verified against a real database engine rather than read by eye; every
earlier phase's "migrations verified forward+back" claim relied on manual
SQL review because no Postgres server was reachable from that sandbox.

---

## 2. Written, not executed: the k6 HTTP/WS suite

See `k6/` and its own `README.md`. It covers OTP login, authenticated
message send, and a sustained WebSocket connection pool, with ramping
stages matching the roadmap's intent (steady background load + a spike).
**It requires a deployed, reachable API** (`BASE_URL`/`WS_URL` env vars)
and has not been pointed at one from this sandbox. Running it for real is
step 6 of `../RUNBOOK.md`.

## 3. Not done here: AWS deployment / WS-survives-LB-failover drill
No AWS credentials exist in this session, so `terraform/` (section
below) was written but never `apply`'d, nothing was deployed, and the
roadmap's "WS survives LB failover" exit criterion has **not** been
empirically verified — it can't be, without a real multi-task ALB
deployment to fail over. `RUNBOOK.md` step 7 is the drill procedure for
whoever runs this for real.
