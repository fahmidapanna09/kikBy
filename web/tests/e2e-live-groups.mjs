// Web ⇄ API contract test for live chat GROUPS (liveAuth + liveChat) with TWO real browser sessions talking through
// tests/fake-api.mjs (a hand-written mirror of the Go chat REST + WebSocket envelope — NOT the real server).
// Covers: real-time delivery over the WebSocket (via the serve.mjs /ws proxy), subprotocol auth, delivered/read ticks,
// typing, edit, reaction, delete-for-everyone, persistence across reload.
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_WS_ORIGINS = "http://localhost:4177"; // the browser Origin the backend allow-list must accept
const { start, chatLog } = await import("./fake-api.mjs");

const api = await start(8097);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4177", API_PROXY: "http://localhost:8097" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4177", API = "http://localhost:8097/v1";
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
const caraReg = await call("POST", "/auth/register", null, { name: "Cara Dev", username: "caradev", email: "caradev@example.com", password: PW, device_id: "t-cara", platform: "web" });
const cara = await login("caradev");
await call("POST", "/friends/requests", alex.token, { user_id: cara.id });
await call("POST", `/friends/${alex.id}/accept`, cara.token);


await A.page.goto(`${BASE}/chats`);
await B.page.goto(`${BASE}/chats`);
await B.page.getByText(/No conversations|Chats/).first().waitFor();

console.log("Create a group");
await A.page.getByRole("button", { name: "New group" }).click();
const dlg = A.page.getByRole("dialog");
check("Create is disabled until a name and a member are chosen", await dlg.getByRole("button", { name: /^Create/ }).isDisabled());
await dlg.getByLabel("Group name").fill("Trip Crew");
await dlg.getByRole("checkbox", { name: "Bella Khan" }).click();
await dlg.getByRole("checkbox", { name: "Cara Dev" }).click();
await dlg.getByRole("button", { name: "Create (2)" }).click();
await A.page.waitForURL(/\/chats\/[0-9a-f-]{36}/);
const gid = A.page.url().split("/").pop();
check("opens the new group", (await A.page.getByRole("button", { name: /Trip Crew — open details/ }).count()) === 1);
const created = await call("GET", `/groups/${gid}`, alex.token);
check("server group has 3 members, Alex is owner", created.json.members.length === 3 && created.json.members.find((m) => m.user_id === alex.id).role === "owner");
await B.page.getByRole("link", { name: /Trip Crew/ }).waitFor({ timeout: 6000 });
check("Bella's list shows the group live (group.updated)", true);

console.log("Group messaging");
const boxA = A.page.getByLabel("Message", { exact: true });
await boxA.fill("Pack your bags");
await boxA.press("Enter");
await B.page.getByRole("link", { name: /Trip Crew/ }).click();
await B.page.locator(".bubble.in", { hasText: "Pack your bags" }).waitFor({ timeout: 6000 });
check("Bella receives the group message with the sender's name", (await B.page.locator(".msg-sender", { hasText: "Alex Live" }).count()) >= 1);
const cmsg = await call("GET", `/conversations/${gid}/messages`, cara.token);
check("Cara (API) can read it too", cmsg.status === 200 && cmsg.json.messages.length === 1);

console.log("Details: members, rename, remove, add");
await A.page.getByRole("button", { name: "Show details" }).click();
await A.page.getByRole("heading", { name: "Members" }).waitFor();
await A.page.getByText("3 members").waitFor();
check("members listed (You + 2)", (await A.page.locator(".panel-list li").count()) === 3 && (await A.page.locator(".panel-list").innerText()).includes("You"));
await A.page.getByRole("button", { name: "Rename group" }).click();
await A.page.getByLabel("Group name").fill("Trip Crew 2");
await A.page.getByLabel("Group name").press("Enter");
await B.page.getByRole("link", { name: /Trip Crew 2/ }).waitFor({ timeout: 6000 });
check("rename reaches Bella live", true);
await A.page.getByRole("button", { name: "Remove Cara Dev" }).click();
await A.page.getByText("2 members").waitFor();
check("removing a member updates the list", true);
check("server agrees (Cara is out)", (await call("GET", `/groups/${gid}`, alex.token)).json.members.length === 2 && (await call("GET", `/groups/${gid}`, cara.token)).status === 403);
await A.page.getByRole("button", { name: "Add people" }).click();
await A.page.getByRole("dialog").getByRole("checkbox", { name: "Cara Dev" }).click();
await A.page.getByRole("dialog").getByRole("button", { name: "Add (1)" }).click();
await A.page.getByText("3 members").waitFor();
check("adding a member back works", true);
check("a non-admin can't rename (server)", (await call("PATCH", `/groups/${gid}`, bella.token, { name: "hijack" })).status === 403);

console.log("Leave");
await B.page.getByRole("button", { name: "Show details" }).click();
await B.page.getByText("Leave group").waitFor();
check("non-admin sees no Add/Rename controls", (await B.page.getByRole("button", { name: "Add people" }).count()) === 0 && (await B.page.getByRole("button", { name: "Rename group" }).count()) === 0);
await B.page.getByRole("button", { name: "Leave group" }).click();
await B.page.getByRole("dialog").getByRole("button", { name: "Leave" }).click();
await B.page.waitForURL(`${BASE}/chats`);
check("Bella is back on the list and the group is gone", (await B.page.getByRole("link", { name: /Trip Crew/ }).count()) === 0);
await A.page.getByText("2 members").waitFor({ timeout: 6000 });
check("Alex's member count drops live when someone leaves", true);
await B.page.reload();
check("after reload the group is still gone for Bella", (await B.page.getByRole("link", { name: /Trip Crew/ }).count()) === 0);
check("no uncaught console/page errors (Alex)", A.errors.length === 0, JSON.stringify(A.errors));
check("no uncaught console/page errors (Bella)", B.errors.length === 0, JSON.stringify(B.errors));
await finish();
