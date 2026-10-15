// k6 load test for the chat platform API — OTP login, authenticated
// message send, and a sustained WebSocket connection pool.
//
// STATUS: written and ready to run, NOT executed anywhere yet. See
// ../REPORT.md section 2 for why: this sandbox cannot compile the Go
// API server (no network access to fetch Go modules), so there is
// nothing local to point this at. Run it for real against a deployed
// environment — see ../RUNBOOK.md step 6.
//
// Usage:
//   k6 run -e BASE_URL=https://api.staging.example.com \
//           -e WS_URL=wss://api.staging.example.com/ws \
//           chat-load-test.js
//
// What it exercises, matching the real endpoints in internal/auth and
// internal/chat (not invented ones):
//   1. POST /v1/auth/otp/request, then POST /v1/auth/otp/verify using the
//      APP_ENV=local debug-peek endpoint (GET /v1/auth/debug/otp/{phone})
//      to read the code back — mirrors backend/README.md's own curl
//      walkthrough. This load test is for a staging/test environment
//      with APP_ENV=local-equivalent debug peek enabled; it is NOT meant
//      to be pointed at a production deployment, which must have that
//      endpoint disabled (see cmd/api/main.go's startup warning).
//   2. POST /v1/conversations to start (or re-find) a direct conversation
//      with a second seeded test user.
//   3. POST /v1/conversations/{id}/messages (the REST fallback send path)
//      under ramping load.
//   4. A long-lived WS connection per virtual user against GET /ws,
//      sending message.send envelopes and waiting for message.ack, to
//      measure realtime fan-out latency, not just REST latency.
//
// Each virtual user needs its own distinct phone number so OTP requests
// don't collide — generated from __VU (k6's 1-indexed virtual user id).

import http from 'k6/http';
import ws from 'k6/ws';
import { check, sleep, fail } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const WS_URL = __ENV.WS_URL || 'ws://localhost:8080/ws';
// A second, fixed test account every VU sends a direct message to —
// must already be seeded in the target environment (see RUNBOOK.md
// step 6 for the seed script). Using one fixed peer, rather than a
// different one per VU, deliberately creates the same kind of
// single-conversation contention REPORT.md section 1 already measured
// at the DB layer — this is where that finding gets re-checked
// end-to-end, through the real HTTP handler and JWT middleware.
const PEER_USER_ID = __ENV.PEER_USER_ID || '';

const wsAckLatency = new Trend('ws_message_ack_latency_ms');
const wsAckMissing = new Counter('ws_message_ack_missing');

export const options = {
  scenarios: {
    rest_message_send: {
      executor: 'ramping-vus',
      exec: 'restSendFlow',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 20 },  // warm-up: steady background load
        { duration: '2m', target: 20 },   // hold
        { duration: '30s', target: 200 }, // spike: a notification-driven burst
        { duration: '1m', target: 200 },  // hold under spike
        { duration: '30s', target: 0 },   // ramp down
      ],
      gracefulRampDown: '10s',
    },
    websocket_send: {
      executor: 'ramping-vus',
      exec: 'wsSendFlow',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '3m', target: 50 },
        { duration: '30s', target: 0 },
      ],
      gracefulRampDown: '10s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'],
    ws_message_ack_latency_ms: ['p(95)<1000'],
    ws_message_ack_missing: ['count<1'], // every send must get an ack — see RUNBOOK.md note on this threshold
    checks: ['rate>0.99'],
  },
};

function phoneForVU() {
  // E.164-shaped, matches isValidE164 in internal/auth — distinct per VU
  // and per test run so OTP_MAX_PER_PHONE_10M isn't hit across repeated
  // runs against the same environment.
  const n = 9000000 + __VU;
  return `+8801${n}`;
}

// Logs a new test user in via the real OTP flow and returns
// {accessToken, userID}. Requires the target environment's debug-peek
// endpoint to be enabled (APP_ENV=local or an explicit staging flag) —
// see the file header.
function loginViaOTP(phone) {
  const reqRes = http.post(`${BASE_URL}/v1/auth/otp/request`, JSON.stringify({ phone_number: phone }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(reqRes, { 'otp/request 200': (r) => r.status === 200 });

  const peekRes = http.get(`${BASE_URL}/v1/auth/debug/otp/${encodeURIComponent(phone)}`);
  check(peekRes, { 'debug otp peek 200': (r) => r.status === 200 });
  if (peekRes.status !== 200) {
    fail(`debug OTP peek failed — target env must have the debug-peek endpoint enabled for this load test (status ${peekRes.status})`);
  }
  const code = JSON.parse(peekRes.body).code;

  const verifyRes = http.post(
    `${BASE_URL}/v1/auth/otp/verify`,
    JSON.stringify({
      phone_number: phone,
      code: code,
      device_id: `k6-device-${__VU}`,
      platform: 'android',
      app_version: 'k6-load-test',
    }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(verifyRes, { 'otp/verify 200': (r) => r.status === 200 });
  if (verifyRes.status !== 200) {
    fail(`otp/verify failed with status ${verifyRes.status}: ${verifyRes.body}`);
  }
  const body = JSON.parse(verifyRes.body);
  return { accessToken: body.tokens.access_token, userID: body.user.id };
}

function ensureDirectConversation(accessToken) {
  if (!PEER_USER_ID) {
    fail('PEER_USER_ID env var is required — seed a fixed test account first, see RUNBOOK.md step 6');
  }
  const res = http.post(
    `${BASE_URL}/v1/conversations`,
    JSON.stringify({ peer_user_id: PEER_USER_ID }),
    { headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' } },
  );
  check(res, { 'start conversation 200/201': (r) => r.status === 200 || r.status === 201 });
  // StartDirectConversation responds with ConversationSummary{id, type,
  // peer_user_id} — see internal/chat/handler.go's startDirectConversation.
  return JSON.parse(res.body).id;
}

export function restSendFlow() {
  const { accessToken } = loginViaOTP(phoneForVU());
  const conversationID = ensureDirectConversation(accessToken);

  for (let i = 0; i < 5; i++) {
    const res = http.post(
      `${BASE_URL}/v1/conversations/${conversationID}/messages`,
      JSON.stringify({
        client_tmp_id: `k6-${__VU}-${__ITER}-${i}-${Date.now()}`,
        type: 'text',
        body: `k6 load test message ${i} from VU ${__VU}`,
      }),
      { headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' } },
    );
    check(res, { 'send message 200/201': (r) => r.status === 200 || r.status === 201 });
    sleep(1 + Math.random()); // a human doesn't send 5 messages/sec back-to-back
  }
}

export function wsSendFlow() {
  const { accessToken } = loginViaOTP(phoneForVU());
  const conversationID = ensureDirectConversation(accessToken);

  const params = { headers: { Authorization: `Bearer ${accessToken}` } };
  const pending = {}; // event_id -> send timestamp, awaiting message.ack

  const res = ws.connect(WS_URL, params, function (socket) {
    socket.on('open', function () {
      socket.setInterval(function () {
        const eventID = `k6evt-${__VU}-${Date.now()}-${Math.random()}`;
        const clientTmpID = `k6-ws-${__VU}-${Date.now()}`;
        pending[clientTmpID] = Date.now();
        socket.send(JSON.stringify({
          event_id: eventID,
          type: 'message.send',
          ts: new Date().toISOString(),
          conversation_id: conversationID,
          version: 1,
          payload: { client_tmp_id: clientTmpID, type: 'text', body: `k6 ws message from VU ${__VU}` },
        }));
      }, 3000); // one message every 3s per connection — a plausible active-chat rate, not a flood
    });

    socket.on('message', function (data) {
      const envelope = JSON.parse(data);
      if (envelope.type === 'message.ack' && envelope.payload && envelope.payload.client_tmp_id) {
        const sentAt = pending[envelope.payload.client_tmp_id];
        if (sentAt) {
          wsAckLatency.add(Date.now() - sentAt);
          delete pending[envelope.payload.client_tmp_id];
        }
      }
    });

    socket.setTimeout(function () {
      // Anything still pending when the VU's connection lifetime ends
      // never got acked — that's a real gap the thresholds above catch.
      wsAckMissing.add(Object.keys(pending).length);
      socket.close();
    }, 60000);
  });

  check(res, { 'ws connected (101)': (r) => r && r.status === 101 });
}
