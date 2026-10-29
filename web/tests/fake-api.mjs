// Contract fake of the Go /v1 auth + users endpoints (response shapes copied from backend/internal/auth|users).
// NOT the real backend: it exists so the web client's HTTP layer, refresh rotation and error mapping can be tested
// in a sandbox that cannot compile the Go code. Access tokens live only ACCESS_TTL_MS so refresh is exercised.
import { createServer } from "node:http";
import { randomUUID, createHash } from "node:crypto";
import { createRequire } from "node:module";
const { WebSocketServer } = createRequire(process.env.PW_ROOT || "/opt/npm-tools/node_modules/")("ws");

export const ACCESS_TTL_MS = Number(process.env.ACCESS_TTL_MS || 4000);
const users = new Map(); // id -> {id,email,username,pw,display_name,about,contact_phone}
const access = new Map(); // token -> {userId, exp}
const refresh = new Map(); // token -> {userId, used}
const friendships = []; // {a (requester), b (addressee), status, since}
const blocks = new Set(); // "blocker>blocked"
const notifs = []; // {id,user_id,kind,title,body,actor_id,read,created_at}
const resets = new Map(); // userId -> {code, attempts, used}  (6-digit code, 5 wrong guesses burn it)
export const db = { users };
export const log = { refreshCalls: 0, resetCodes: [], logouts: 0, verifyCodes: [] };
const verifyTokens = new Map(); // tokenHash -> {userId, email, used, exp, created}
const VERIFY_TTL_MS = Number(process.env.FAKE_VERIFY_TTL_MS ?? 600000);
const VERIFY_COOLDOWN_MS = Number(process.env.FAKE_VERIFY_COOLDOWN_MS ?? 60000);
function sendVerify(u) {
  const code = String(Math.floor(Math.random() * 1e6)).padStart(6, "0");
  for (const t of verifyTokens.values()) if (t.userId === u.id) t.used = true;
  verifyTokens.set(randomUUID(), { userId: u.id, email: u.pending || u.email, code, attempts: 0, used: false, exp: Date.now() + VERIFY_TTL_MS, created: Date.now() });
  log.verifyCodes.push(code);
}
// mirrors auth.normalizeContactPhone (Go): strips spaces/dashes/parentheses, adds "+" if missing
const normPhone = (s) => { const t = String(s).trim().replace(/[\s\-()]/g, ""); return t.startsWith("+") ? t : "+" + t; };
const sha = (s) => createHash("sha256").update(s).digest("hex");

const json = (res, status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body)); };
const err = (res, status, code, message, fields) => json(res, status, { error: { code, message, request_id: "r1", ...(fields ? { fields } : {}) } });
const pwErr = (p) => (p.length < 8 ? "use at least 8 characters" : !/[A-Za-z]/.test(p) || !/\d/.test(p) ? "include at least one letter and one number" : "");

function mint(userId, device = "") {
  const a = "at_" + randomUUID(), r = "rt_" + randomUUID();
  access.set(a, { userId, exp: Date.now() + ACCESS_TTL_MS, device });
  refresh.set(r, { userId, used: false, device });
  return { access_token: a, refresh_token: r, access_expires_at: new Date(Date.now() + ACCESS_TTL_MS).toISOString(), refresh_expires_at: new Date(Date.now() + 864e5).toISOString() };
}
const me = (u) => ({ user_id: u.id, username: u.username, email: u.email, contact_phone: u.contact_phone || undefined, display_name: u.display_name, about: u.about || undefined, email_verified: !!u.verified, pending_email: u.pending || undefined, last_seen_visibility: "everyone", avatar_media_id: u.avatar || undefined });

const lite = (u) => ({ id: u.id, username: u.username, display_name: u.display_name || u.username, about: u.about || undefined });
const pair = (x, y) => friendships.find((f) => (f.a === x && f.b === y) || (f.a === y && f.b === x));
const notify = (to, kind, title, body, actor) => notifs.push({ id: randomUUID(), user_id: to, kind, title, body, actor_id: actor, read: false, created_at: new Date().toISOString() });
function relationsOf(meId, only) {
  const out = [];
  for (const f of friendships) {
    if (f.a !== meId && f.b !== meId) continue;
    const other = users.get(f.a === meId ? f.b : f.a);
    if (!other || (only && other.id !== only)) continue;
    out.push({ user: lite(other), status: f.status === "accepted" ? "friends" : f.a === meId ? "outgoing" : "incoming", since: f.since });
  }
  for (const k of blocks) {
    const [x, y] = k.split(">");
    if (x === meId && (!only || only === y) && users.get(y)) out.push({ user: lite(users.get(y)), status: "blocked", since: new Date().toISOString() });
  }
  return out;
}


// ---- chat (mirrors backend/internal/chat: REST shapes + websocket Envelope) ----
const convs = new Map(); // id -> {id, type, members:[ids], state:{[uid]:{pinned_at,archived_at,muted_until,last_read_seq}}, msgs:[...] , seq}
const sockets = new Map(); // userId -> Set<ws>
export const chatLog = { wsOrigins: [], wsProtocols: [], wsRejected: 0, sends: 0 };
const ALLOWED_ORIGINS = (process.env.FAKE_WS_ORIGINS || "").split(",").filter(Boolean);
const ts = () => new Date().toISOString();
const env = (type, conversation_id, payload) => JSON.stringify({ event_id: randomUUID(), type, ts: ts(), conversation_id, version: 1, payload });
const pushTo = (uid, frame) => sockets.get(uid)?.forEach((w) => w.readyState === 1 && w.send(frame));
const pubMsg = (m) => ({ id: m.id, conversation_id: m.conversation_id, seq: m.seq, sender_id: m.sender_id, client_tmp_id: m.client_tmp_id, type: m.type, body: m.deleted_at ? undefined : m.body, reply_to_id: m.reply_to_id, created_at: m.created_at, edited_at: m.edited_at, deleted_at: m.deleted_at, forwarded_from_message_id: m.forwarded_from_message_id, ...(m.deleted_at || !m.attachments?.length ? {} : { attachment_ids: m.attachments, attachments: m.attachments.map((id) => { const f = media.get(id); return { id, kind: f.kind, mime_type: f.mime, size_bytes: f.bytes.length, name: f.name }; }) }) });
// the real edit path returns the bare message row (no attachments / reactions)
const pubBare = (m) => ({ ...pubMsg(m), attachment_ids: undefined, attachments: undefined });
export const media = new Map(); // id -> {owner, kind, mime, name, status, bytes}
export const mediaLog = { puts: [], downloads: 0, variants: [] };
const MIMES = { "image/jpeg": "image", "image/png": "image", "image/webp": "image", "image/gif": "image", "video/mp4": "video", "video/webm": "video", "audio/mpeg": "audio", "application/pdf": "document", "text/plain": "document", "application/zip": "document" };
const reactionsOf = (m) => { const by = new Map(); for (const [uid, e] of Object.entries(m.reacts)) by.set(e, [...(by.get(e) || []), uid]); return [...by].map(([emoji, user_ids]) => ({ emoji, user_ids })); };
const directKey = (a, b) => [a, b].sort().join(":");
const memberOf = (c, uid) => c && c.members.includes(uid);
export const notifyPush = (uid, type, conv, payload) => pushTo(uid, env(type, conv, payload));
const summary = (c, uid) => {
  const last = c.msgs[c.msgs.length - 1], st = c.state[uid];
  const peer = c.type === "direct" ? c.members.find((m) => m !== uid) : undefined;
  return {
    id: c.id, type: c.type, peer_user_id: peer, name: c.name, last_message: last ? { ...pubMsg(last), body: last.deleted_at ? undefined : last.body } : undefined,
    unread_count: c.msgs.filter((m) => m.seq > st.last_read_seq && m.sender_id !== uid && !m.deleted_at).length,
    pinned_at: st.pinned_at, archived_at: st.archived_at, muted_until: st.muted_until,
  };
};

// ---- calls (mirrors backend/internal/chat/call_service.go: same events, same state machine, in memory) ----
export const callsDb = new Map(); // id -> {id, conversation_id, initiator_id, kind, status, started_at, answered_at, ended_at, end_reason, joined:Set}
export const callLog = { signals: 0, started: 0 };
const RING_MS = Number(process.env.FAKE_RING_MS || 45000);
const joinDelay = () => Number(process.env.FAKE_JOIN_DELAY_MS || 0); // >0 widens the "read who is in the call" window so simultaneous joins see each other
const MAX_CALL = 4;
const callPub = (c) => ({ id: c.id, conversation_id: c.conversation_id, initiator_id: c.initiator_id, kind: c.kind, status: c.status, started_at: c.started_at, answered_at: c.answered_at, ended_at: c.ended_at, end_reason: c.end_reason });
const liveCall = (conv) => [...callsDb.values()].find((c) => c.conversation_id === conv && (c.status === "ringing" || c.status === "active"));
const wsErr = (uid, code, message) => pushTo(uid, env("error", "", { code, message }));
function endCall(c, status, reason) {
  if (c.status !== "ringing" && c.status !== "active") return;
  c.status = status; c.end_reason = reason; c.ended_at = ts(); c.joined.clear();
  for (const m of convs.get(c.conversation_id).members) pushTo(m, env("call.ended", c.conversation_id, { call: callPub(c) }));
}
function joinCall(uid, c) {
  const conv = convs.get(c.conversation_id);
  if (!memberOf(conv, uid)) return wsErr(uid, "not_a_member", "you are not a member of this conversation");
  if (c.status !== "ringing" && c.status !== "active") return wsErr(uid, "call_ended", "this call has already ended");
  const others = [...c.joined].filter((u) => u !== uid);
  if (others.length + 1 > MAX_CALL) return wsErr(uid, "call_full", "this call is full");
  c.joined.add(uid);
  if (c.status === "ringing" && uid !== c.initiator_id) { c.status = "active"; c.answered_at = ts(); }
  const finish = () => {
    const before = [...c.joined].filter((u) => u !== uid);
    pushTo(uid, env("call.joined", c.conversation_id, { call: callPub(c), participants: before, ice_servers: [] }));
    for (const u of before) pushTo(u, env("call.participant_joined", c.conversation_id, { call_id: c.id, user_id: uid }));
  };
  if (joinDelay()) setTimeout(finish, joinDelay()); else finish();
}
function leaveCall(uid, c) {
  if (c.status !== "ringing" && c.status !== "active") return;
  if (!c.joined.delete(uid)) return;
  const conv = convs.get(c.conversation_id);
  if (c.joined.size === 0 || conv.type === "direct") return endCall(c, c.answered_at ? "ended" : "cancelled", "hangup");
  for (const u of c.joined) pushTo(u, env("call.participant_left", c.conversation_id, { call_id: c.id, user_id: uid }));
}
function fakeCall(uid, e) {
  const p = e.payload || {};
  if (e.type === "call.start") {
    const conv = convs.get(e.conversation_id);
    if (!memberOf(conv, uid)) return wsErr(uid, "not_a_member", "you are not a member of this conversation");
    if (p.kind !== "voice" && p.kind !== "video") return wsErr(uid, "call_invalid", "invalid call request");
    if (conv.type === "direct") { const peer = conv.members.find((m) => m !== uid); if (blocks.has(uid + ">" + peer) || blocks.has(peer + ">" + uid)) return wsErr(uid, "call_blocked", "calling is not allowed between these users"); }
    const live = liveCall(conv.id);
    if (live) return joinCall(uid, live);
    const c = { id: randomUUID(), conversation_id: conv.id, initiator_id: uid, kind: p.kind, status: "ringing", started_at: ts(), joined: new Set([uid]) };
    callsDb.set(c.id, c); callLog.started++;
    for (const m of conv.members) if (m !== uid) pushTo(m, env("call.incoming", conv.id, { call: callPub(c), from_user_id: uid }));
    pushTo(uid, env("call.joined", conv.id, { call: callPub(c), participants: [], ice_servers: [] }));
    setTimeout(() => { if (c.status === "ringing") endCall(c, "missed", "timeout"); }, RING_MS);
    return;
  }
  const c = callsDb.get(p.call_id);
  if (!c) return wsErr(uid, "call_not_found", "call not found");
  if (e.type === "call.join") return joinCall(uid, c);
  if (e.type === "call.leave") return leaveCall(uid, c);
  if (e.type === "call.reject") {
    const conv = convs.get(c.conversation_id);
    if (!memberOf(conv, uid)) return wsErr(uid, "not_a_member", "you are not a member of this conversation");
    if (c.status !== "ringing" && c.status !== "active") return;
    if (c.joined.has(uid)) return leaveCall(uid, c);
    if (conv.type === "direct") return endCall(c, "declined", "declined");
    return pushTo(uid, env("call.dismissed", conv.id, { call_id: c.id }));
  }
  if (e.type === "call.signal") {
    if (c.status !== "ringing" && c.status !== "active") return wsErr(uid, "call_ended", "this call has already ended");
    if (!p.to_user_id || p.to_user_id === uid || p.data == null || !c.joined.has(uid) || !c.joined.has(p.to_user_id)) return wsErr(uid, "call_invalid", "invalid call request");
    callLog.signals++;
    pushTo(p.to_user_id, env("call.signal", c.conversation_id, { call_id: c.id, from_user_id: uid, data: p.data }));
  }
}
function dropUserCalls(uid) { if (sockets.get(uid)?.size) return; for (const c of callsDb.values()) if (c.joined.has(uid)) leaveCall(uid, c); }

function chatRoute(req, res, path, body, authed, helpers) {
  const { json, err } = helpers;
  const seg = path.split("/").filter(Boolean); // v1, conversations, :id, messages
  const url = new URL(req.url, "http://x");
  if (seg[1] === "conversations" && seg[3] === "calls" && req.method === "GET") {
    const conv = convs.get(seg[2]);
    if (!memberOf(conv, authed.id)) return err(res, 403, "not_a_member", "you are not a member of this conversation");
    const lim = Math.min(Number(url.searchParams.get("limit")) || 20, 50);
    return json(res, 200, { calls: [...callsDb.values()].filter((c) => c.conversation_id === conv.id).sort((a, b) => +new Date(b.started_at) - +new Date(a.started_at)).slice(0, lim).map(callPub) });
  }
  if (path === "/v1/conversations") {
    if (req.method === "GET") return json(res, 200, { conversations: [...convs.values()].filter((c) => memberOf(c, authed.id)).sort((a, b) => (b.msgs.at(-1)?.seq ? +new Date(b.msgs.at(-1).created_at) : 0) - (a.msgs.at(-1)?.seq ? +new Date(a.msgs.at(-1).created_at) : 0)).map((c) => summary(c, authed.id)) });
    if (req.method === "POST") {
      if (!body.peer_user_id) return err(res, 400, "invalid_body", "peer_user_id is required");
      if (body.peer_user_id === authed.id) return err(res, 400, "invalid_peer", "cannot start a conversation with yourself");
      if (blocks.has(authed.id + ">" + body.peer_user_id) || blocks.has(body.peer_user_id + ">" + authed.id)) return err(res, 403, "blocked", "you can't message this user");
      const key = directKey(authed.id, body.peer_user_id);
      let c = [...convs.values()].find((x) => x.key === key);
      if (!c) { c = { id: randomUUID(), type: "direct", key, members: [authed.id, body.peer_user_id], state: {}, msgs: [] }; for (const m of c.members) c.state[m] = { last_read_seq: 0 }; convs.set(c.id, c); }
      return json(res, 200, { id: c.id, type: "direct", peer_user_id: body.peer_user_id, unread_count: 0 });
    }
  }
  if (seg[1] === "groups") {
    const groupJson = (g) => ({ conversation_id: g.id, name: g.name, members: g.members.map((u) => ({ user_id: u, role: g.roles[u] || "member" })) });
    const pushAll = (g, type, payload, extra = []) => [...new Set([...g.members, ...extra])].forEach((u) => pushTo(u, env(type, g.id, payload)));
    if (!seg[2] && req.method === "POST") {
      if (!String(body.name || "").trim()) return err(res, 400, "invalid_body", "name is required");
      const members = [...new Set([authed.id, ...(body.member_user_ids || [])])];
      const g = { id: randomUUID(), type: "group", name: String(body.name).trim(), members, roles: { [authed.id]: "owner" }, state: {}, msgs: [] };
      for (const m of members) g.state[m] = { last_read_seq: 0 };
      convs.set(g.id, g);
      pushAll(g, "group.updated", { group: groupJson(g) });
      return json(res, 200, groupJson(g));
    }
    const g = convs.get(seg[2]);
    if (!g || g.type !== "group") return err(res, 404, "not_found", "group not found");
    if (!memberOf(g, authed.id)) return err(res, 403, "not_a_member", "you are not a member of this group");
    const admin = ["owner", "admin"].includes(g.roles[authed.id]);
    if (!seg[3] && req.method === "GET") return json(res, 200, groupJson(g));
    if (!seg[3] && req.method === "PATCH") {
      if (!admin) return err(res, 403, "permission_denied", "you don't have permission to do that");
      if (typeof body.name === "string" && body.name.trim()) g.name = body.name.trim();
      pushAll(g, "group.updated", { group: groupJson(g) });
      return json(res, 200, groupJson(g));
    }
    if (seg[3] === "members" && req.method === "POST") {
      if (!admin) return err(res, 403, "permission_denied", "you don't have permission to do that");
      const add = (body.user_ids || []).filter((u) => !g.members.includes(u));
      for (const u of add) { g.members.push(u); g.state[u] = { last_read_seq: g.msgs.length }; }
      pushAll(g, "group.member_added", { members: add.map((user_id) => ({ user_id, role: "member" })), added_by: authed.id });
      return json(res, 200, { status: "ok" });
    }
    if (seg[3] === "members" && seg[4] && req.method === "DELETE") {
      if (seg[4] !== authed.id && !admin) return err(res, 403, "permission_denied", "you don't have permission to do that");
      if (g.roles[seg[4]] === "owner" && seg[4] !== authed.id) return err(res, 403, "permission_denied", "you don't have permission to do that");
      const before = [...g.members];
      g.members = g.members.filter((u) => u !== seg[4]);
      for (const u of before) pushTo(u, env("group.member_removed", g.id, { user_id: seg[4], removed_by: authed.id }));
      return json(res, 200, { status: "ok" });
    }
    return err(res, 404, "not_found", "no such route");
  }
  if (seg[1] === "conversations" && seg[2]) {
    const c = convs.get(seg[2]);
    if (!memberOf(c, authed.id)) return err(res, 403, "not_a_member", "you are not a member of this conversation");
    const others = c.members.filter((m) => m !== authed.id);
    if (seg[3] === "messages" && req.method === "GET") {
      const before = Number(url.searchParams.get("cursor") || 0);
      const page = c.msgs.filter((m) => (!before || m.seq < before) && !m.hidden_for?.includes(authed.id)).sort((a, b) => b.seq - a.seq).slice(0, 50);
      const statusOf = (m) => { if (m.sender_id !== authed.id || m.deleted_at) return undefined; const oth = c.members.filter((u) => u !== authed.id); const rd = oth.filter((u) => c.state[u].last_read_seq >= m.seq).length; return oth.length && rd === oth.length ? "read" : rd || m.delivered_to?.length ? "delivered" : "sent"; };
      return json(res, 200, { messages: page.map((m) => ({ ...pubMsg(m), status: statusOf(m), reactions: m.deleted_at ? undefined : reactionsOf(m) })), next_cursor: page.length === 50 ? String(page.at(-1).seq) : undefined });
    }
    if (seg[3] === "messages" && req.method === "POST") {
      if (!body.client_tmp_id) return err(res, 400, "invalid_body", "client_tmp_id is required");
      let m = c.msgs.find((x) => x.sender_id === authed.id && x.client_tmp_id === body.client_tmp_id);
      if (m) return json(res, 200, pubMsg(m)); // idempotent retry
      chatLog.sends++;
      m = { id: randomUUID(), conversation_id: c.id, seq: c.msgs.length + 1, sender_id: authed.id, client_tmp_id: body.client_tmp_id, type: body.type || "text", body: body.body, reply_to_id: body.reply_to_id, forwarded_from_message_id: body.forwarded_from_message_id, created_at: ts(), reacts: {}, attachments: (body.attachment_media_ids || []).filter((id) => media.get(id)?.status === "uploaded" && (media.get(id).owner === authed.id || [...convs.values()].some((cv) => memberOf(cv, authed.id) && cv.msgs.some((x) => !x.deleted_at && x.attachments?.includes(id))))) };
      c.msgs.push(m);
      for (const o of others) pushTo(o, env("message.new", c.id, pubMsg(m)));
      return json(res, 200, pubMsg(m));
    }
    if (seg[3] === "read" && req.method === "POST") {
      const m = c.msgs.find((x) => x.id === body.message_id);
      if (!m) return err(res, 404, "not_found", "message not found");
      c.state[authed.id].last_read_seq = Math.max(c.state[authed.id].last_read_seq, m.seq);
      for (const o of others) pushTo(o, env("message.read", c.id, { message_id: m.id, user_id: authed.id }));
      return json(res, 200, { status: "ok" });
    }
    if (["pin", "archive", "mute"].includes(seg[3]) && req.method === "PUT") {
      const st = c.state[authed.id];
      if (seg[3] === "pin") st.pinned_at = body.value ? st.pinned_at || ts() : undefined;
      if (seg[3] === "archive") st.archived_at = body.value ? st.archived_at || ts() : undefined;
      if (seg[3] === "mute") st.muted_until = body.until || undefined;
      return json(res, 200, { status: "ok" });
    }
  }
  if (seg[1] === "messages" && seg[2]) {
    const c = [...convs.values()].find((x) => x.msgs.some((m) => m.id === seg[2]));
    const m = c?.msgs.find((x) => x.id === seg[2]);
    if (!m) return err(res, 404, "not_found", "message not found");
    if (!memberOf(c, authed.id)) return err(res, 403, "not_a_member", "you are not a member of this conversation");
    const all = () => c.members.forEach((u) => 0);
    if (req.method === "PATCH" && !seg[3]) {
      if (m.sender_id !== authed.id) return err(res, 403, "not_sender", "only the sender can do that");
      if (m.deleted_at) return err(res, 409, "message_deleted", "this message was deleted");
      if (!String(body.body || "").trim()) return err(res, 400, "invalid_body", "message text is empty or too long");
      m.body = String(body.body).trim(); m.edited_at = ts();
      for (const u of c.members) pushTo(u, env("message.edited", c.id, { message: pubBare(m) }));
      return json(res, 200, pubBare(m));
    }
    if (req.method === "DELETE" && !seg[3]) {
      if (url.searchParams.get("scope") === "everyone") {
        if (m.sender_id !== authed.id) return err(res, 403, "not_sender", "only the sender can do that");
        m.deleted_at = ts(); m.body = undefined;
        for (const u of c.members) pushTo(u, env("message.deleted", c.id, { message_id: m.id }));
      } else (m.hidden_for ||= []).push(authed.id);
      return json(res, 200, { status: "ok" });
    }
    if (seg[3] === "reactions" && (req.method === "POST" || req.method === "DELETE")) {
      if (req.method === "POST") m.reacts[authed.id] = body.emoji; else delete m.reacts[authed.id];
      for (const u of c.members) pushTo(u, env("message.reaction", c.id, { message_id: m.id, reactions: reactionsOf(m) }));
      return json(res, 200, { status: "ok" });
    }
    void all;
  }
  return null;
}
function attachWs(srv) {
  // Presence — mirrors backend/internal/chat/ws_handler.go: peers = accepted friends ∪ people sharing a conversation.
  const peersOf = (uid) => {
    const out = new Set();
    for (const f of friendships) if (f.status === "accepted" && (f.a === uid || f.b === uid)) out.add(f.a === uid ? f.b : f.a);
    for (const c of convs.values()) if (c.members.includes(uid)) for (const m of c.members) if (m !== uid) out.add(m);
    return [...out];
  };
  const lastSeen = new Map();
  const isOnline = (uid) => (sockets.get(uid)?.size ?? 0) > 0;
  const wss = new WebSocketServer({ noServer: true, handleProtocols: (set) => (set.has("kikbite.v1") ? "kikbite.v1" : false) });
  srv.on("upgrade", (req, socket, head) => {
    const protos = String(req.headers["sec-websocket-protocol"] || "").split(",").map((x) => x.trim());
    const tok = protos.find((x) => x.startsWith("bearer."))?.slice(7);
    const origin = req.headers.origin;
    chatLog.wsOrigins.push(origin); chatLog.wsProtocols.push(protos);
    const sess = access.get(tok);
    const originOk = !origin || ALLOWED_ORIGINS.length === 0 || ALLOWED_ORIGINS.includes(origin);
    if (!sess || sess.exp < Date.now() || !originOk || req.url !== "/ws") { chatLog.wsRejected++; socket.write("HTTP/1.1 401 Unauthorized\r\n\r\n"); return socket.destroy(); }
    wss.handleUpgrade(req, socket, head, (ws) => {
      const uid = sess.userId;
      if (!sockets.has(uid)) sockets.set(uid, new Set());
      const first = sockets.get(uid).size === 0;
      sockets.get(uid).add(ws);
      if (first) for (const p of peersOf(uid)) pushTo(p, env("presence.update", "", { user_id: uid, online: true }));
      for (const p of peersOf(uid)) {
        if (isOnline(p)) ws.send(env("presence.update", "", { user_id: p, online: true })); // snapshot on connect
        else if (lastSeen.has(p)) ws.send(env("presence.update", "", { user_id: p, online: false, last_seen: lastSeen.get(p) }));
      }
      ws.on("close", () => {
        sockets.get(uid)?.delete(ws); dropUserCalls(uid);
        if (!isOnline(uid)) { const at = new Date().toISOString(); lastSeen.set(uid, at); for (const p of peersOf(uid)) pushTo(p, env("presence.update", "", { user_id: uid, online: false, last_seen: at })); }
      });
      ws.on("message", (raw) => {
        let e; try { e = JSON.parse(String(raw)); } catch { return; }
        if (typeof e.type === "string" && e.type.startsWith("call.")) return fakeCall(uid, e);
        const c = convs.get(e.conversation_id);
        if (!memberOf(c, uid)) return;
        const rest = c.members.filter((m) => m !== uid);
        if (e.type === "typing.start" || e.type === "typing.stop") for (const o of rest) pushTo(o, env(e.type, c.id, { user_id: uid }));
        if (e.type === "message.delivered") { const dm = c.msgs.find((x) => x.id === e.payload.message_id); if (dm) (dm.delivered_to ||= []).push(uid); }
        if (e.type === "message.delivered") for (const o of rest) pushTo(o, env("message.delivered", c.id, { message_id: e.payload.message_id, user_id: uid }));
        if (e.type === "message.read") {
          const m = c.msgs.find((x) => x.id === e.payload.message_id);
          if (m) c.state[uid].last_read_seq = Math.max(c.state[uid].last_read_seq, m.seq);
          for (const o of rest) pushTo(o, env("message.read", c.id, { message_id: e.payload.message_id, user_id: uid }));
        }
      });
    });
  });
}

export function start(port) {
  const srv = createServer(async (req, res) => {
    if (req.url.startsWith("/storage/")) { // stands in for the S3/MinIO presigned URL host (cross-origin from the page)
      const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Methods": "PUT, GET, OPTIONS", "Access-Control-Allow-Headers": "content-type" };
      if (req.method === "OPTIONS") { res.writeHead(204, cors); return res.end(); }
      const f = media.get(req.url.split("?")[0].split("/")[2]);
      if (!f) { res.writeHead(404, cors); return res.end(); }
      if (req.method === "PUT") {
        const parts = []; for await (const c of req) parts.push(c);
        if (req.headers["content-type"] !== f.mime) { res.writeHead(403, cors); return res.end("SignatureDoesNotMatch"); } // Content-Type is part of the signature
        f.bytes = Buffer.concat(parts); mediaLog.puts.push({ mime: req.headers["content-type"], size: f.bytes.length });
        res.writeHead(200, cors); return res.end();
      }
      res.writeHead(200, { ...cors, "Content-Type": f.mime }); return res.end(f.bytes);
    }
    let raw = ""; for await (const c of req) raw += c;
    let body = {}; try { body = raw ? JSON.parse(raw) : {}; } catch { return err(res, 400, "invalid_body", "malformed request body"); }
    const path = req.url.split("?")[0];
    const bearer = (req.headers.authorization || "").replace(/^Bearer /, "");
    const sess = access.get(bearer);
    const authed = sess && sess.exp > Date.now() ? users.get(sess.userId) : null; // deleted accounts are removed from `users`

    if (req.method === "POST" && path === "/v1/auth/register") {
      const f = {};
      const email = String(body.email || "").trim().toLowerCase(), username = String(body.username || "").trim().toLowerCase();
      if (String(body.name || "").trim().length < 2) f.name = "name is too short";
      if (!/^[a-z0-9_]{3,24}$/.test(username)) f.username = "3–24 characters: letters, numbers or _";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(email)) f.email = "enter a valid email address";
      if (pwErr(body.password || "")) f.password = pwErr(body.password || "");
      if (body.phone && !/^\+[1-9]\d{6,14}$/.test(normPhone(body.phone))) f.phone = "enter a valid phone number (7–15 digits)";
      if (Object.keys(f).length) return err(res, 422, "validation", "please fix the highlighted fields", f);
      if ([...users.values()].some((u) => u.email === email)) return err(res, 422, "validation", "please fix the highlighted fields", { email: "an account with this email already exists" });
      if ([...users.values()].some((u) => u.username === username)) return err(res, 422, "validation", "please fix the highlighted fields", { username: "that username is taken" });
      const u = { id: randomUUID(), email, username, pw: body.password, display_name: body.name, contact_phone: body.phone ? normPhone(body.phone) : undefined };
      users.set(u.id, u);
      sendVerify(u);
      return json(res, 201, { user: { id: u.id, email, username, email_verified: false }, tokens: mint(u.id, body.device_id), is_new_user: true });
    }
    if (req.method === "POST" && path === "/v1/auth/login") {
      const id = String(body.identifier || "").trim().toLowerCase();
      const u = [...users.values()].find((x) => x.email === id || x.username === id);
      if (u && (u.fails || 0) >= 10) { res.setHeader("Retry-After", "900"); return err(res, 429, "account_locked", "Too many failed attempts. Try again in 15 minutes, or reset your password."); } // mirrors auth.lockoutMaxFailures
      if (!u || u.pw !== body.password) { if (u) u.fails = (u.fails || 0) + 1; return err(res, 401, "invalid_credentials", "incorrect email/username or password"); }
      u.fails = 0;
      return json(res, 200, { user: { id: u.id, email: u.email, username: u.username, email_verified: !!u.verified }, tokens: mint(u.id, body.device_id), is_new_user: false });
    }
    if (req.method === "POST" && path === "/v1/auth/verify-email") {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      const digits = String(body.code || "").replace(/[\s-]/g, "");
      if (!/^\d{6}$/.test(digits)) return err(res, 400, "invalid_code", "that code isn't right — check it and try again");
      const t = [...verifyTokens.values()].filter((x) => x.userId === authed.id && !x.used && x.exp > Date.now()).sort((a, b) => b.created - a.created)[0];
      if (!t) return err(res, 400, "code_expired", "this code has expired — request a new one");
      if (t.code !== digits) {
        t.attempts++;
        if (t.attempts >= 5) { t.used = true; return err(res, 429, "too_many_attempts", "too many wrong codes — request a new one"); }
        return err(res, 400, "invalid_code", "that code isn't right — check it and try again");
      }
      const isPending = authed.pending === t.email;
      if (isPending && [...users.values()].some((x) => x.id !== authed.id && x.email === t.email)) { t.used = true; return err(res, 400, "code_expired", "this code has expired — request a new one"); }
      t.used = true; authed.verified = true;
      if (isPending) { authed.email = t.email; authed.pending = undefined; }
      return json(res, 200, { status: "verified" });
    }
    if (req.method === "POST" && path === "/v1/auth/verify-email/resend") {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      if (authed.verified && !authed.pending) return json(res, 200, { status: "already_verified" });
      const last = [...verifyTokens.values()].filter((t) => t.userId === authed.id).sort((a, b) => b.created - a.created)[0];
      if (last && Date.now() - last.created < VERIFY_COOLDOWN_MS) return err(res, 429, "too_many_requests", "a verification e-mail was just sent — wait a minute before asking again");
      sendVerify(authed);
      return json(res, 202, { status: "sent" });
    }
    if (req.method === "POST" && ["/v1/auth/change-password", "/v1/auth/change-email", "/v1/auth/delete-account"].includes(path)) {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      const bad = (fields) => err(res, 422, "validation", "please fix the highlighted fields", fields);
      if (path === "/v1/auth/change-password") {
        if (pwErr(body.new_password || "")) return bad({ new_password: pwErr(body.new_password || "") });
        if (body.new_password === body.current_password) return bad({ new_password: "choose a password you haven't used just now" });
        if (authed.pw !== body.current_password) return bad({ current: "incorrect password" });
        authed.pw = body.new_password;
        const keep = sess.device;
        for (const [k, v] of access) if (v.userId === authed.id && v.device !== keep) access.delete(k);
        for (const v of refresh.values()) if (v.userId === authed.id && v.device !== keep) v.used = true;
        log.passwordChanges = (log.passwordChanges || 0) + 1;
        return json(res, 200, { status: "password_changed" });
      }
      if (path === "/v1/auth/change-email") {
        const ne = String(body.new_email || "").trim().toLowerCase();
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(ne)) return bad({ email: "enter a valid email address" });
        if (authed.pw !== body.password) return bad({ password: "incorrect password" });
        if (ne === authed.email) return bad({ email: "that is already your email address" });
        if ([...users.values()].some((x) => x.email === ne)) return bad({ email: "an account with this email already exists" });
        authed.pending = ne;
        sendVerify(authed);
        return json(res, 202, { status: "verification_sent" });
      }
      if (authed.pw !== body.password) return bad({ password: "incorrect password" });
      users.delete(authed.id); // anonymised + freed, like the Go soft delete
      for (const [k, v] of access) if (v.userId === authed.id) access.delete(k);
      for (const v of refresh.values()) if (v.userId === authed.id) v.used = true;
      return json(res, 200, { status: "deleted" });
    }
    if (req.method === "POST" && path === "/v1/auth/refresh") {
      log.refreshCalls++;
      const r = refresh.get(body.refresh_token);
      if (!r) return err(res, 401, "session_expired", "session expired or revoked, please log in again");
      if (r.used) { for (const [k, v] of refresh) if (v.userId === r.userId) v.used = true; return err(res, 401, "token_reuse_detected", "security check failed, please log in again"); }
      r.used = true;
      return json(res, 200, mint(r.userId, r.device));
    }
    if (req.method === "POST" && path === "/v1/auth/logout") {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      log.logouts++;
      const r = refresh.get(body.refresh_token); if (r) r.used = true;
      return json(res, 200, { status: "logged_out" });
    }
    if (req.method === "POST" && path === "/v1/auth/forgot-password") {
      const u = [...users.values()].find((x) => x.email === String(body.email || "").trim().toLowerCase());
      if (u) { const code = String(100000 + Math.floor(Math.random() * 900000)); resets.set(u.id, { code, attempts: 0, used: false }); log.resetCodes.push(code); }
      return json(res, 202, { status: "if_account_exists_email_sent" });
    }
    if (req.method === "POST" && path === "/v1/auth/reset-password") {
      if (pwErr(body.password || "")) return err(res, 422, "validation", "please fix the highlighted fields", { password: pwErr(body.password) });
      const u = [...users.values()].find((x) => x.email === String(body.email || "").trim().toLowerCase());
      const t = u && resets.get(u.id);
      if (!t || t.used) return err(res, 400, "invalid_code", "That code is wrong or has expired.");
      if (String(body.code || "").replace(/[\s-]/g, "") !== t.code) {
        if (++t.attempts >= 5) { t.used = true; return err(res, 429, "too_many_attempts", "Too many wrong codes. Request a new code and try again."); }
        return err(res, 400, "invalid_code", "That code is wrong or has expired.");
      }
      t.used = true; u.pw = body.password; u.verified = true; /* the code proves the mailbox */
      for (const v of refresh.values()) if (v.userId === u.id) v.used = true;
      return json(res, 200, { status: "password_updated" });
    }
    { // GET /v1/users/{id}/avatar is PUBLIC (an <img> cannot send a bearer token): 302 to a signed link, or 404
      const m = req.method === "GET" && path.match(/^\/v1\/users\/([^/]+)\/avatar$/);
      if (m) {
        const u = users.get(m[1]);
        if (!u?.avatar) { res.writeHead(404, { "Cache-Control": "public, max-age=120" }); return res.end("404 page not found"); }
        res.writeHead(302, { Location: `http://localhost:${port}/storage/${u.avatar}?sig=a${++mediaLog.downloads}`, "Cache-Control": "private, max-age=300" }); return res.end();
      }
    }
    if (path.startsWith("/v1/users")) {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      if (req.method === "GET" && path === "/v1/users/search") {
        const q = (new URL(req.url, "http://x").searchParams.get("username") || "").toLowerCase();
        const hits = [...users.values()].filter((u) => u.id !== authed.id && u.username.startsWith(q) && !blocks.has(u.id + ">" + authed.id));
        return json(res, 200, { users: hits.slice(0, 20).map((u) => ({ user_id: u.id, username: u.username, display_name: u.display_name, about: u.about || undefined, avatar_media_id: u.avatar || undefined })) });
      }
      if (req.method === "GET" && /^\/v1\/users\/[0-9a-f-]{36}$/.test(path)) {
        const u = users.get(path.split("/")[3]);
        if (!u) return err(res, 404, "not_found", "user not found");
        return json(res, 200, { user_id: u.id, username: u.username, display_name: u.display_name, about: u.about || undefined, avatar_media_id: u.avatar || undefined });
      }
      if (path === "/v1/users/me/settings") {
        if (req.method === "GET") return json(res, 200, { settings: authed.settings ?? null });
        if (req.method === "PUT") { if (!body || Array.isArray(body) || typeof body !== "object") return err(res, 400, "invalid_body", "settings must be a JSON object under 8 KB"); authed.settings = body; return json(res, 200, { settings: body }); }
      }
      if (req.method === "GET" && path === "/v1/users/me") return json(res, 200, me(authed));
      if (req.method === "PATCH" && path === "/v1/users/me") {
        if (body.contact_phone && !/^\+[1-9]\d{6,14}$/.test(body.contact_phone)) return err(res, 400, "invalid_update", "users: contact_phone must be in E.164 format");
        if (body.display_name !== undefined) authed.display_name = body.display_name;
        if (body.about !== undefined) authed.about = body.about;
        if (body.avatar_media_id !== undefined) {
          if (body.avatar_media_id === "") authed.avatar = undefined;
          else {
            const f = media.get(body.avatar_media_id);
            if (!f || f.owner !== authed.id || f.status !== "uploaded" || f.kind !== "image" || !["image/jpeg", "image/png", "image/webp"].includes(f.mime)) return err(res, 400, "invalid_update", "users: profile picture must be an uploaded JPEG, PNG or WebP image of yours");
            authed.avatar = body.avatar_media_id;
          }
        }
        if (body.contact_phone !== undefined) authed.contact_phone = body.contact_phone || undefined;
        return json(res, 200, me(authed));
      }
      if (req.method === "PATCH" && path === "/v1/users/username") {
        if ([...users.values()].some((x) => x.id !== authed.id && x.username === body.username)) return err(res, 409, "username_taken", "that username is already taken");
        authed.username = body.username; return json(res, 200, me(authed));
      }
    }
    if (path.startsWith("/v1/media")) {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      const seg = path.split("/").filter(Boolean); // v1, media, presign-upload | :id, confirm | download-url
      if (req.method === "POST" && seg[2] === "presign-upload") {
        if (!(body.kind === "voice" ? ["audio/webm", "audio/ogg", "audio/mp4", "audio/aac", "audio/amr"].includes(body.mime_type) : MIMES[body.mime_type] === body.kind)) return err(res, 422, "mime_not_allowed", "this file type isn't allowed");
        const id = randomUUID();
        media.set(id, { owner: authed.id, kind: body.kind, mime: body.mime_type, name: String(body.file_name || "").slice(0, 120), status: "pending_upload", bytes: Buffer.alloc(0) });
        return json(res, 200, { media_id: id, upload_url: `http://localhost:${port}/storage/${id}`, expires_in_seconds: 900 });
      }
      const f = media.get(seg[2]);
      if (!f) return err(res, 404, "not_found", "media not found");
      if (req.method === "POST" && seg[3] === "confirm") {
        if (f.owner !== authed.id) return err(res, 403, "forbidden", "not authorized");
        if (!f.bytes.length) return err(res, 409, "not_uploaded", "the file hasn't been uploaded yet");
        if (/reject/i.test(f.name)) { f.status = "rejected"; return json(res, 200, { media_id: seg[2], status: "rejected", reason: "file failed the safety check" }); }
        f.status = "uploaded";
        return json(res, 200, { media_id: seg[2], status: "uploaded", download_url: `http://localhost:${port}/storage/${seg[2]}?sig=c${++mediaLog.downloads}` });
      }
      if (req.method === "GET" && seg[3] === "download-url") {
        const visible = [...convs.values()].some((c) => memberOf(c, authed.id) && c.msgs.some((m) => !m.deleted_at && m.attachments?.includes(seg[2])));
        if (f.owner !== authed.id && !visible) return err(res, 403, "forbidden", "not authorized");
        if (f.status !== "uploaded") return err(res, 409, "not_uploaded", "the file hasn't been uploaded yet");
        const wantThumb = new URL(req.url, "http://x").searchParams.get("variant") === "thumb" && f.kind === "image" && process.env.FAKE_THUMBS === "1";
        mediaLog.variants.push(wantThumb ? "thumb" : "full");
        return json(res, 200, { download_url: `http://localhost:${port}/storage/${seg[2]}?sig=d${++mediaLog.downloads}${wantThumb ? "&t=1" : ""}`, expires_in_seconds: 900, variant: wantThumb ? "thumb" : "full" });
      }
      return err(res, 404, "not_found", "no such route");
    }
    if (path.startsWith("/v1/conversations") || path.startsWith("/v1/messages") || path.startsWith("/v1/groups")) {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      if (chatRoute(req, res, path, body, authed, { json, err }) !== null) return; // handled (returns undefined after responding)
      return err(res, 404, "not_found", "no such route");
    }
    // ---- friends (mirrors backend/internal/friends) ----
    if (path.startsWith("/v1/friends") || path.startsWith("/v1/notifications")) {
      if (!authed) return err(res, 401, "unauthorized", "missing or invalid token");
      const seg = path.split("/").filter(Boolean); // v1, friends, ...
      const other = seg[2] && seg[2] !== "requests" ? seg[2] : seg[3];
      if (path.startsWith("/v1/friends")) {
        if (req.method === "GET" && seg.length === 2) return json(res, 200, { friends: relationsOf(authed.id) });
        if (req.method === "POST" && path === "/v1/friends/requests") {
          const t = users.get(body.user_id);
          if (!/^[0-9a-f-]{36}$/.test(String(body.user_id))) return err(res, 400, "invalid_user", "friends: invalid user id");
          if (body.user_id === authed.id) return err(res, 400, "invalid_user", "friends: cannot befriend yourself");
          if (!t || blocks.has(t.id + ">" + authed.id)) return err(res, 404, "not_found", "not found");
          if (blocks.has(authed.id + ">" + t.id)) return err(res, 409, "blocked", "unblock this user first");
          const ex = pair(authed.id, t.id);
          if (ex && ex.status === "pending" && ex.b === authed.id) { ex.status = "accepted"; ex.since = new Date().toISOString(); notify(t.id, "friend_accept", authed.display_name + " followed you back", "You are now friends. You can chat and see each other's status.", authed.id); }
          else if (!ex) { friendships.push({ a: authed.id, b: t.id, status: "pending" }); notify(t.id, "friend_request", authed.display_name + " started following you", "Follow back to become friends.", authed.id); }
          return json(res, 200, relationsOf(authed.id, t.id)[0]);
        }
        if (req.method === "POST" && seg[3] === "accept") {
          const f = pair(authed.id, other);
          if (!f || f.status !== "pending" || f.b !== authed.id) return err(res, 404, "not_found", "not found");
          f.status = "accepted"; f.since = new Date().toISOString();
          notify(other, "friend_accept", authed.display_name + " followed you back", "You are now friends. You can chat and see each other's status.", authed.id);
          return json(res, 200, relationsOf(authed.id, other)[0]);
        }
        if (req.method === "DELETE" && seg[2] === "requests") {
          const f = pair(authed.id, other);
          if (!f || f.status !== "pending") return err(res, 404, "not_found", "not found");
          friendships.splice(friendships.indexOf(f), 1);
          return json(res, 200, { status: "removed" });
        }
        if (req.method === "PUT" && seg[3] === "block") { blocks.add(authed.id + ">" + other); const f = pair(authed.id, other); if (f) friendships.splice(friendships.indexOf(f), 1); return json(res, 200, { status: "blocked" }); }
        if (req.method === "DELETE" && seg[3] === "block") { blocks.delete(authed.id + ">" + other); return json(res, 200, { status: "unblocked" }); }
        if (req.method === "DELETE" && seg.length === 3) {
          const f = pair(authed.id, other);
          if (!f || f.status !== "accepted") return err(res, 404, "not_found", "not found");
          // unfollow: they keep following me → pending other → me
          f.a = other; f.b = authed.id; f.status = "pending"; delete f.since;
          return json(res, 200, { status: "removed" });
        }
      } else {
        const mine = notifs.filter((n) => n.user_id === authed.id);
        if (req.method === "GET" && seg.length === 2) return json(res, 200, { notifications: [...mine].reverse(), unread_count: mine.filter((n) => !n.read).length });
        if (req.method === "POST" && seg[2] === "read-all") { mine.forEach((n) => (n.read = true)); return json(res, 200, { status: "all_read" }); }
        if (req.method === "POST" && seg[3] === "read") { const n = mine.find((x) => x.id === seg[2]); if (!n) return err(res, 404, "not_found", "notification not found"); n.read = true; return json(res, 200, { status: "read" }); }
      }
    }
    err(res, 404, "not_found", "no such route");
  });
  attachWs(srv);
  return new Promise((ok) => srv.listen(port, () => ok(srv)));
}
if (import.meta.url === `file://${process.argv[1]}`) start(Number(process.env.FAKE_PORT || 8099)).then(() => console.log("fake api on", process.env.FAKE_PORT || 8099));
