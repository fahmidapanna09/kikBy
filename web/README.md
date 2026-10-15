# KikBite Web

Production frontend for **KikBite** — *Small bites. Big conversations.*  
React 19 + TypeScript (strict) · plain CSS with design tokens · bundled with esbuild · no runtime dependencies besides React.

The UI implements the approved KikBite design system **without redesigning it**: same tokens (`src/styles/tokens.css`), same logo mark, icon style, bubble shapes, nav, states.

## Run

```bash
cd web
npm install          # only needed if you add dev deps; the app itself needs react, react-dom, esbuild, typescript
npm run build        # -> dist/ (bundle.js, styles.css, index.html)
npm run serve        # static server with SPA fallback on :4173
npm run typecheck
```

Offline mock mode (development only, never in production) has a built-in test account: **demo / Demo1234**. Register creates local accounts (stored in `localStorage`).

## ⚠️ Honest deviations from the spec

| Spec | What was built | Why |
|---|---|---|
| Next.js / Tailwind / icon lib | React + esbuild, plain CSS, own SVG icon set, own router | The build sandbox had no npm registry access (403). Only React, esbuild and TypeScript were available. Everything is ordinary React, so moving to Next.js/Vite/Tailwind later is a port, not a rewrite. |
| `@types/react` | `src/types/react-shim.d.ts` (loose React typings) | Same reason. **Delete the shim and `npm i -D @types/react @types/react-dom` in a normal environment.** Our own props/models are fully typed; React's event types are loose in the shim. |
| Virtualization | Cursor pagination (30/page) + scroll-anchored "load older" | Bounded DOM without variable-height windowing complexity. Add a windowing lib if threads get huge. |
| Login = email/username + password | Built as specified | The *approved mobile design* used phone + OTP and the Go backend currently exposes phone-OTP. `authService` is an interface — a phone/OTP implementation drops in without touching screens. |
| Real API | **Mock services only** (`src/mock`) | Contracts in `src/services/types.ts` are what the real client must satisfy. |

Not verified: behaviour against the real Go backend, real WebSocket (`webSocketClient.ts` is written but untested), real keyboards on iOS/Android devices, Safari/Firefox (tests ran in Chromium only), screen readers.

## Architecture

```
src/
  app/          App.tsx (route table + guards), ErrorBoundary
  components/
    common/     Button, Input/Checkbox/Switch, Avatar, Modal/Drawer/Confirm, Dropdown/Menu, Toast, Skeleton, EmptyState, UserRow, FriendButtons
    layout/     AppShell, Sidebar, TopBar, MobileNav
    chat/       ChatList, ConversationItem, ChatHeader, MessageList, MessageBubble, Composer, ForwardDialog, ChatDetails
  pages/        auth/*, HomePage, chats/*, FriendsPage, ProfilePage, SearchPage, NotificationsPage, settings/*, DesignSystemPage (/_ds)
  services/     contracts (types.ts), session store, http client (401 handling), config, errors, index.ts (composition point)
  mock/         in-memory backend: db/seed, per-service mocks, peer simulator, server event bus
  realtime/     events.ts (wire contract), handlers.ts (event → state), mockSocket, webSocketClient, index.ts
  state/        createStore (tiny external store), store (app state), actions, messages (optimistic send etc.), friends, drafts
  lib/          router (History API), validation, utils (sanitize, formatting), appearance, recent searches
  types/models.ts   User, Conversation, Message, Friend, AppNotification, Attachment, ApiResponse, Pagination, AuthSession, Settings
```

**Data flow:** UI → `state/*` actions → `services` (interface) → mock **or** real API.  
**Real-time:** socket → `realtime/handlers.ts` → `state/messages.ts` primitives → store → UI. Components never touch sockets; they call `sendRealtime()` (typing). Events: `message:new|update|delete|read|delivered`, `user:online|offline|typing|stop_typing`, `notification:new`. After a reconnect, conversations are re-synced via REST.

**Going live:** implement the interfaces in `services/types.ts` over `services/http.ts` (it already attaches the bearer token and maps 401 → clears session → router redirects to `/login`), return them from `services/index.ts` when `config.mock === false`, set `window.__KIKBITE_CONFIG__ = { apiBase, wsUrl, mock:false }`. Adapt `realtime/webSocketClient.ts` to the backend's frame format.

## Features checklist
Auth (login/register/forgot/reset, strength meter, show/hide, remember me, safe `?next=` redirect, session expiry) · App shell (sidebar / top bar / bottom nav / right panel, offline banner) · Home dashboard · Chats (list with filters, search, pin/mute/archive; thread with date separators, unread divider, typing, status ticks sending→sent→delivered→read, failed + retry, reply + jump-to-original, edit, delete, reactions, forward, copy, attachments with progress, drafts, long-press/right-click menu, scroll-anchoring, "N new messages" pill, keyboard-safe composer via `visualViewport`) · Profile view/edit · Friends (requests, accept/decline/cancel, remove, block/unblock) · Search (debounced, race-safe, recent searches) · Notifications · Settings (Account, Privacy, Notifications, Appearance, Chat, Security, Other) · Toasts, modals, drawers, skeletons, empty/error states everywhere.

## Security notes
- No secrets in the bundle; config is public runtime config only.
- User content is rendered only as React text nodes (escaped); URLs are linkified via an allow-list (`http(s)`), links use `rel="noopener noreferrer nofollow"`; `safeUrl()` guards `img src`/`href`; control & bidi characters stripped (`sanitizeText`).
- Open-redirect-safe `?next=`. Password-reset and login errors don't reveal whether an account exists.
- Tokens live in web storage (localStorage when "remember me", else sessionStorage) — XSS-readable. For production prefer an httpOnly refresh cookie (documented in `services/session.ts`).
- Mock password hashing is NOT secure; it exists only so the demo flows work.

## Accessibility
Semantic landmarks (nav/main/aside/header/search), one `h1` per page, real `<a>`/`<button>` (no clickable divs), labelled inputs with `aria-invalid`/`aria-describedby`, `role="alert"` errors, live regions for toasts / typing / message log, focus-visible ring, dialog focus trap + Esc + focus restore, skip link, `prefers-reduced-motion` + in-app setting. The automated audit (`tests/audit.mjs`) checks every route at 360/820/1440 px for horizontal overflow, unnamed controls, missing alt, duplicate ids, and an h1. It is a smoke test, **not** a substitute for a real WCAG audit.

## Tests
Browser tests (Playwright + system Chromium; not a project dependency):
`tests/e2e-auth|chat|friends|search-notif|settings|mobile.mjs`, `tests/audit.mjs`, all via `tests/run-all.sh`. Set `PW_ROOT` if Playwright lives elsewhere. Failure injection for QA of error states: `localStorage.setItem("kikbite.mock.fail","1")`; sending a message containing `/fail` produces a failed bubble with Retry.

## Hosting
Static site: `dist/` works on Railway (static/Caddy/`npm run serve`), Cloudflare Pages, S3+CloudFront — all need an SPA fallback to `index.html`. Domain: kikbite.com.


## Live auth + profile (hybrid mode)

`window.__KIKBITE_CONFIG__ = { liveAuth: true, apiBase: "https://api.kikbite.com/v1" }` (set in `index.html` or injected by the host)
switches **only** auth and the signed-in user's profile to the Go API (`src/api/*`). Chats, friends, notifications, settings,
change-password, delete-account and other users' profiles stay on the in-memory mock, seeded around the real user.

- Endpoints used: `POST /auth/{register,login,refresh,logout,forgot-password,reset-password}`, `GET|PATCH /users/me`, `PATCH /users/username`.
- Silent refresh with rotation (single-flight; 401 → one refresh + retry → else sign out). Wrong credentials never trigger a logout.
- Dev: `API_PROXY=http://localhost:8080 node scripts/serve.mjs` forwards `/api/*` to the Go server (strips `/api`).
- **Verified only against `tests/fake-api.mjs`**, a hand-written contract fake of the Go responses (the Go code could not be compiled in the
  build sandbox). Run the same flow against the real server before trusting it.
- Not supported live yet: changing e-mail (backend has no endpoint).

## Live friends, notifications and chat

Add `liveChat: true` next to `liveAuth: true` to put conversations, messages and the real-time socket on the Go API too:

```html
<script>window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true };</script>
```

- Friends and notifications are live whenever `liveAuth` is on (`/v1/friends`, `/v1/notifications`, plus `notification.new` over the socket).
- `liveChat`: `GET|POST /conversations`, `GET|POST /conversations/{id}/messages`, `PATCH|DELETE /messages/{id}` (`?scope=everyone`),
  `POST|DELETE /messages/{id}/reactions`, `POST /conversations/{id}/read`, `PUT /conversations/{id}/{pin,archive,mute}`.
  Sending goes over REST (idempotent on `client_tmp_id`); the socket delivers `message.new|edited|deleted|reaction|delivered|read`, typing and notifications.
- Socket auth: browsers cannot set an `Authorization` header, so the token is sent as a WebSocket **subprotocol** (`kikbite.v1`, `bearer.<token>`),
  never in the URL. The backend only accepts a browser handshake from an allow-listed `Origin`
  (`WS_ALLOWED_ORIGINS`, `APP_BASE_URL`, and the localhost dev origins when `APP_ENV=local`). `scripts/serve.mjs` proxies `/ws` too.
- **Attachments (live with `liveChat`)**: `POST /media/presign-upload` → browser `PUT` straight to storage (progress via XHR; the `Content-Type`
  must equal the declared one because it is part of the signature) → `POST /media/{id}/confirm` (server HEADs the object, checks real type + size)
  → send with `attachment_media_ids`. Messages carry metadata only (`attachments: [{id,kind,mime_type,size_bytes,name}]`); the bubble fetches a signed
  link per id (`GET /media/{id}/download-url`, cached 10 min, links live 15 min). Types/limits mirror the backend allow-list (images 15 MB, video 200 MB,
  audio 25 MB, documents 100 MB; env `MEDIA_MAX_*`). **Storage CORS**: MinIO allows browsers by default; on AWS S3 add a bucket CORS rule for your web
  origin (`PUT`, `GET`, header `Content-Type`). Video and audio play inline (a file the browser cannot decode falls back to a download chip). Images load a server-made thumbnail when the worker has produced one (`GET /media/{id}/download-url?variant=thumb`) and open the full file on click. **Voice notes**: mic button in the composer (MediaRecorder; uploaded with kind `voice`). **Forwarding** a message with files re-attaches the same file (no copy) — the server allows it when the sender can already see the file in one of their conversations. Not done: video poster frames.
- **Groups (live with `liveChat`)**: "New group" in the chat list (name + friends), member list, rename / add / remove (owner & admins), leave. Members and names
  update live through `group.updated|member_added|member_removed`.
- **History ticks**: messages loaded from the server carry `status` (sent / delivered / read) for your own messages, so ticks survive a reload.
- **Settings sync**: `GET|PUT /users/me/settings` stores the whole settings object per account (read receipts, last seen, notifications, appearance…) so a second
  device gets it. The server stores the choice; it does not yet *enforce* every privacy toggle (only what the backend already did for last-seen).
- Not live yet: display-name search (the backend searches usernames), avatars.
- **Verified only against `tests/fake-api.mjs`** (REST + WebSocket mirror of the Go chat module) with two real browsers. The Go side was
  checked with `gofmt`, isolated tests and the SQL run on PostgreSQL 16, but not compiled here: run `go build ./... && go test ./...` first.

## E-mail verification (live)
With `liveAuth`, a new account is sent straight to `/verify-email` (sign-in required): six boxes for the **6-digit code** e-mailed to the address
(valid **10 minutes**, 5 wrong guesses burn it, paste and one-time-code autofill supported, resend after 60 s). "Do this later" leaves the screen;
until verified a banner ("Verify your e-mail address" – **Enter code** / **Resend code**) stays at the top. Changing the sign-in e-mail uses the same screen.
Not enforced: nothing is blocked while unverified. Mail is sent by the Go backend through Resend (see `backend/docs/EMAIL_AUTH.md`).
Verified against `tests/fake-api.mjs` (`tests/e2e-live-verify.mjs`), not the real server or the real Resend API.

## Live account management
With `liveAuth`: Settings → Security changes the password (other devices are signed out), Settings → Account changes the e-mail (needs the password; the new address
must be confirmed through the emailed link before sign-in switches to it), and Settings → Other deletes the account (soft delete + anonymise, see `backend/docs/EMAIL_AUTH.md`).
Verified against `tests/fake-api.mjs` (`tests/e2e-live-account.mjs`), not the real Go server.

## Voice, video and group calls (live chat)
With `liveChat` the chat header shows **voice** and **video** call buttons (also in groups). WebRTC mesh, max 4 people, signaling over the `/ws` socket;
see `backend/docs/CALLS.md` for the protocol, ICE/TURN setup and limits. Code: `src/calls/` (engine `callClient.ts`, store, ringtone) and
`src/components/call/CallOverlay.tsx` (incoming card + call screen). In a video call you can answer **audio only** or turn your camera off; the others still
see your name/avatar. Needs HTTPS or localhost for camera/microphone. Verified with real WebRTC between three browsers (`tests/e2e-live-calls.mjs`, 51 checks,
also with `CSP=1`) against `tests/fake-api.mjs` — not against the real Go server, and not over real networks.

## Production serving
`scripts/serve.mjs` is also the production server (see `Dockerfile`, and `backend/docs/DEPLOY.md`): security headers always; strict CSP when
`NODE_ENV=production` or `CSP=1` (set `MEDIA_ORIGIN` to the storage host); `/config.js` generated from `KIKBITE_LIVE=1` / `KIKBITE_CONFIG`
(never overrides a config the page already set); gzip + ETag; `/healthz`; `API_PROXY` forwards `/api/*` and `/ws`. `Permissions-Policy` allows camera/microphone for the page itself (`self`) — earlier it blocked them, which would have broken voice notes and calls in a real browser. The live-chat and attachments suites also pass with `CSP=1`.
