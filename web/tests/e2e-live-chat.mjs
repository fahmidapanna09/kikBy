// Web ⇄ API contract test for live chat (liveAuth + liveChat) with TWO real browser sessions talking through
// tests/fake-api.mjs (a hand-written mirror of the Go chat REST + WebSocket envelope — NOT the real server).
// Covers: real-time delivery over the WebSocket (via the serve.mjs /ws proxy), subprotocol auth, delivered/read ticks,
// typing, edit, reaction, delete-for-everyone, persistence across reload.
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_WS_ORIGINS = "http://localhost:4175"; // the browser Origin the backend allow-list must accept
const { start, chatLog } = await import("./fake-api.mjs");

const api = await start(8099);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4175", API_PROXY: "http://localhost:8099" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4175", API = "http://localhost:8099/v1";
async function call(method, path, token, body) {
  const r = await fetch(API + path, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, json: await r.json().catch(() => null) };
}
const PW = "Str0ng!Passw0rd";
const cfg = (ctx) => ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true }; });

const A = await launch({ width: 1280, height: 860 });
const B = { ...(await (async () => { const ctx = await A.browser.newContext({ viewport: { width: 1280, height: 860 } }); await ctx.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort()); const page = await ctx.newPage(); const errors = []; page.on("pageerror", (e) => errors.push(e.message)); page.on("console", (m) => m.type() === "error" && !/Failed to load resource/.test(m.text()) && errors.push(m.text())); return { ctx, page, errors }; })()) };
await cfg(A.ctx); await cfg(B.ctx);
const finish = async () => { await A.browser.close(); api.close(); web.kill(); summary(); };

async function register(page, name, username) {
  await page.goto(`${BASE}/register`);
  await page.getByLabel("Full name").fill(name);
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Email").fill(`${username}@example.com`);
  await page.getByLabel("Password", { exact: true }).fill(PW);
  await page.getByLabel("Confirm password").fill(PW);
  await page.getByRole("button", { name: "Create account" }).click();
  await page.getByRole("button", { name: "Do this later" }).click(); // new live accounts land on the e-mail code screen
  await page.waitForURL(`${BASE}/`, { waitUntil: "commit" });
  await page.getByRole("heading", { name: new RegExp(`(Good|Still up).*${name.split(" ")[0]}`) }).waitFor();
}
console.log("Two users sign up in two browsers");
await register(A.page, "Alex Live", "alexlive");
await register(B.page, "Bella Khan", "bella");
const login = async (u) => { const r = await call("POST", "/auth/login", null, { identifier: u, password: PW, device_id: "t-" + u, platform: "web" }); return { token: r.json.tokens.access_token, id: r.json.user.id }; };
const alex = await login("alexlive"), bella = await login("bella");
await call("POST", "/friends/requests", alex.token, { user_id: bella.id });
await call("POST", `/friends/${alex.id}/accept`, bella.token);

console.log("Socket handshake");
await B.page.goto(`${BASE}/chats`);
await B.page.getByText(/No conversations|Chats/).first().waitFor();
await new Promise((r) => setTimeout(r, 600));
check("both browsers opened an authenticated socket (none rejected)", chatLog.wsOrigins.length >= 2 && chatLog.wsRejected === 0, JSON.stringify({ o: chatLog.wsOrigins, rej: chatLog.wsRejected }));
check("token travels as a subprotocol, server picks kikbite.v1", chatLog.wsProtocols.every((p) => p[0] === "kikbite.v1" && /^bearer\./.test(p[1])));
check("handshake carries the page Origin for the allow-list", chatLog.wsOrigins.every((o) => o === BASE));

console.log("Alex opens a chat with Bella and sends");
await A.page.goto(`${BASE}/friends`);
await A.page.getByRole("button", { name: "Message Bella Khan" }).first().click();
await A.page.waitForURL(/\/chats\/[0-9a-f-]{36}/);
const boxA = A.page.getByLabel("Message", { exact: true });
await boxA.fill("Hello Bella <3");
await boxA.press("Enter");
await A.page.locator(".bubble.out", { hasText: "Hello Bella" }).waitFor();
check("optimistic bubble shown to the sender", true);

console.log("Bella receives it in real time (no reload)");
await B.page.getByRole("link", { name: /Alex Live/ }).waitFor({ timeout: 6000 });
check("new conversation appears in Bella's list", true);
check("unread badge = 1", (await B.page.locator(".conv-item .badge").first().innerText()).trim() === "1");
check("preview shows the message", (await B.page.locator(".conv-item", { hasText: "Alex Live" }).innerText()).includes("Hello Bella"));
await A.page.locator('.bubble.out [aria-label="Delivered"]').first().waitFor({ timeout: 6000 });
check("sender gets the 'delivered' tick (device ack over the socket)", true);

await B.page.getByRole("link", { name: /Alex Live/ }).click();
await B.page.locator(".bubble.in", { hasText: "Hello Bella" }).waitFor();
check("history loaded from GET /conversations/{id}/messages", true);
await A.page.locator('.bubble.out [aria-label="Read"]').first().waitFor({ timeout: 6000 });
check("sender gets the 'read' tick when Bella opens the chat", true);
check("badge cleared for Bella", (await B.page.locator('.conv-item[aria-current="page"] .badge').count()) === 0);

console.log("Typing + reply in real time");
const boxB = B.page.getByLabel("Message", { exact: true });
await boxB.click();
await boxB.pressSequentially("Hi Alex, hel", { delay: 40 });
await A.page.locator(".typing-bubble").waitFor({ timeout: 6000 });
check("Alex sees Bella typing", true);
await boxB.fill("Hi Alex!");
await boxB.press("Enter");
await A.page.locator(".bubble.in", { hasText: "Hi Alex!" }).waitFor({ timeout: 6000 });
check("Bella's reply arrives live", true);
await A.page.waitForFunction(() => document.querySelectorAll(".typing-bubble").length === 0, null, { timeout: 8000 });
check("typing clears once the message lands", true);

console.log("Edit / react / delete propagate");
await A.page.waitForTimeout(900);
await A.page.locator(".bubble.out", { hasText: "Hello Bella" }).click({ button: "right" });
await A.page.getByRole("menuitem", { name: "Edit" }).click();
await boxA.fill("Hello Bella EDITED");
await boxA.press("Enter");
await B.page.locator(".bubble.in", { hasText: "Hello Bella EDITED" }).waitFor({ timeout: 6000 });
check("edit reaches Bella (message.edited)", true);
check("edited label shown to Bella", (await B.page.locator(".bubble.in", { hasText: "Hello Bella EDITED" }).getByText("edited", { exact: true }).count()) === 1);

await B.page.waitForTimeout(900);
await B.page.locator(".bubble.in", { hasText: "Hello Bella EDITED" }).click({ button: "right" });
await B.page.getByRole("group", { name: "React" }).getByRole("button", { name: "React ❤️" }).click();
await A.page.locator(".react-chip", { hasText: "❤️" }).waitFor({ timeout: 6000 });
check("Bella's reaction shows on Alex's message (message.reaction)", true);
await B.page.locator(".react-chip.mine", { hasText: "❤️" }).click();
await A.page.waitForFunction(() => ![...document.querySelectorAll(".react-chip")].some((e) => e.textContent.includes("❤️")), null, { timeout: 6000 });
check("removing the reaction propagates", true);

await A.page.waitForTimeout(900);
await A.page.locator(".bubble.out", { hasText: "Hello Bella EDITED" }).click({ button: "right" });
await A.page.getByRole("menuitem", { name: "Delete" }).click();
await A.page.getByRole("dialog").getByRole("button", { name: "Delete" }).click();
await B.page.getByText("This message was deleted").first().waitFor({ timeout: 6000 });
check("delete-for-everyone tombstones on Bella's side", true);
const sv = await call("GET", `/conversations/${(await call("GET", "/conversations", alex.token)).json.conversations[0].id}/messages`, bella.token);
check("server never returns the deleted body", !JSON.stringify(sv.json).includes("Hello Bella EDITED"));

console.log("Persistence + errors");
await A.page.reload();
await A.page.locator(".bubble.in", { hasText: "Hi Alex!" }).waitFor();
check("after reload the thread comes from the server", (await A.page.getByText("This message was deleted").count()) >= 1);
const boxA2 = A.page.getByLabel("Message", { exact: true });
await boxA2.fill("one more");
await boxA2.press("Enter");
await B.page.locator(".bubble.in", { hasText: "one more" }).waitFor({ timeout: 6000 });
check("socket reconnects after a reload and keeps delivering", true);
await A.page.reload();
await A.page.locator(".bubble.out", { hasText: "one more" }).waitFor();
check("history shows delivered/read ticks for my own messages after a reload", (await A.page.locator('.bubble.out [aria-label="Read"], .bubble.out [aria-label="Delivered"]').count()) >= 1);
const dup = await call("POST", `/conversations/${(await call("GET", "/conversations", alex.token)).json.conversations[0].id}/messages`, alex.token, { client_tmp_id: "same", body: "x", type: "text" });
const dup2 = await call("POST", `/conversations/${dup.json.conversation_id}/messages`, alex.token, { client_tmp_id: "same", body: "x", type: "text" });
check("retried client_tmp_id is idempotent (same message id)", dup.json.id === dup2.json.id);
check("no uncaught console/page errors (Alex)", A.errors.length === 0, JSON.stringify(A.errors));
check("no uncaught console/page errors (Bella)", B.errors.length === 0, JSON.stringify(B.errors));
await finish();
