# Voice, video and group calls (prototype)

WebRTC **mesh**: every participant connects directly to every other participant, so audio/video never touches the API.
The API only (1) keeps call state and history in Postgres (`calls`, `call_participants`, migration 0020) and (2) relays
WebRTC offers/answers/ICE candidates over the existing `/ws` socket. Code: `internal/chat/call_*.go`.

## What works
- 1:1 voice and video calls; group calls in any group chat (voice or video), **max 4 people** (`maxCallParticipants`).
- Mixed calls: in a video call someone can answer **audio only** (they still see/hear everybody) or switch their camera off.
  A call's kind (voice/video) is fixed when it starts; a voice call cannot be upgraded to video.
- Decline (1:1 ends the call; in a group it only stops ringing for you), cancel while ringing, 45 s ring timeout → *missed*,
  "busy" (a new 1:1 call while you are in a call is declined automatically), late join / rejoin from a "call in progress" banner,
  one tab per user, dropped automatically when a user's last WebSocket closes.
- History: `GET /v1/conversations/{id}/calls?limit=20` → `{calls:[{id,kind,status,started_at,answered_at,ended_at,end_reason,...}]}`.

## WebSocket events
Client → server: `call.start {kind}` (envelope `conversation_id`), `call.join {call_id}`, `call.leave {call_id}`, `call.reject {call_id}`,
`call.signal {call_id, to_user_id, data}` (`data` = `{type:"offer"|"answer",sdp}` or `{type:"candidate",candidate}`; opaque to the server).
Server → client: `call.incoming {call, from_user_id}`, `call.joined {call, participants, ice_servers}` (to whoever started/joined),
`call.participant_joined|left {call_id, user_id}`, `call.signal {call_id, from_user_id, data}`, `call.ended {call}`, `call.dismissed {call_id}`,
and the usual `error` frame with codes `call_full`, `call_ended`, `call_blocked`, `call_not_found`, `call_invalid`, `not_a_member`, `calls_disabled`.
Whoever **joins** offers to everybody already in the call; if two people join at the same moment the web client resolves the double offer with
the "perfect negotiation" rule (lower user id is polite). A second `call.start` in a conversation that already has a live call becomes a join.

## ICE servers (STUN / TURN)
Works out of the box with Google's public STUN. A **TURN relay** is what makes calls work across strict NATs (mobile data ↔ Wi-Fi, some offices);
it is the only part of calls that needs a server or money. Configure through env (see `.env.example`):

| Variable | Meaning |
|---|---|
| `STUN_URLS` | comma list, default `stun:stun.l.google.com:19302` |
| `TURN_URLS` | e.g. `turn:turn.example.com:3478,turns:turn.example.com:5349` |
| `TURN_SECRET` | your coturn `static-auth-secret`; the API mints short-lived per-user credentials (`TURN_TTL`, default 1h) |
| `TURN_USERNAME` + `TURN_CREDENTIAL` | alternative: static credentials from a hosted TURN provider |

Minimal coturn (`/etc/turnserver.conf`): `listening-port=3478`, `fingerprint`, `use-auth-secret`, `static-auth-secret=<same as TURN_SECRET>`,
`realm=turn.example.com`, `no-cli`, plus open UDP 3478 and the relay port range on the firewall.

## Limits and honest gaps
- Mesh cost grows with every person (each uploads a stream to each other person) — hence the cap of 4. Bigger calls need an SFU (e.g. LiveKit); the
  signaling contract above would then only carry join/leave.
- Browsers need **HTTPS or `localhost`** for camera/microphone. Testing from a phone against `http://<LAN-ip>` will not work.
- `Permissions-Policy` must allow `camera=(self), microphone=(self)` (the bundled web server does).
- No push notification for incoming calls when the app/tab is closed yet (needs FCM/Web Push); no screen sharing; no call-history UI in the chat list;
  call state is per-user across API nodes through the database, but the 45 s ring timer lives on the node that handled `call.start`.
- Verified: state machine with unit tests (`call_service_test.go`, race detector), SQL on PostgreSQL 16, and the web client with real WebRTC between
  three browsers against `web/tests/fake-api.mjs` (a mirror of this service) — **not** against the real Go server and never across real networks/TURN.
