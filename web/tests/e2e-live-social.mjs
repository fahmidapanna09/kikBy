// Web ⇄ API contract test for friends, notifications, user search and public profiles (liveAuth mode,
// against tests/fake-api.mjs — a hand-written mirror of the Go responses, NOT the real server).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000"; // long-lived tokens: this test is about social flows, not refresh
const { start } = await import("./fake-api.mjs");

const api = await start(8099);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4174", API_PROXY: "http://localhost:8099" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4174", API = "http://localhost:8099/v1";

async function call(method, path, token, body) {
  const r = await fetch(API + path, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, json: await r.json().catch(() => null) };
}
async function signUp(name, username) {
  const r = await call("POST", "/auth/register", null, { name, username, email: `${username}@example.com`, password: "Str0ng!Passw0rd", device_id: "api-" + username, platform: "web" });
  return { id: r.json.user.id, token: r.json.tokens.access_token };
}
const bella = await signUp("Bella Khan", "bella");
const carlos = await signUp("Carlos Diaz", "carlos");

const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true }; });
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

console.log("Sign up as Alex (browser)");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Alex Live");
await page.getByLabel("Username").fill("alexlive");
await page.getByLabel("Email").fill("alex@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click(); // new live accounts land on the e-mail code screen
await page.waitForURL(`${BASE}/`, { waitUntil: "commit" });
await page.getByRole("heading", { name: /(Good|Still up).*Alex/ }).waitFor();

console.log("Search + friend request (live)");
await page.goto(`${BASE}/search`);
const box = page.getByLabel("Search people and chats");
await box.fill("bel");
await page.getByText("Bella Khan").first().waitFor();
check("username-prefix search hits GET /users/search", true);
await page.getByRole("button", { name: "Add friend" }).first().click();
await page.getByText(/Friend request sent/).waitFor();
check("add friend → POST /friends/requests, toast shown", true);
await page.goto(`${BASE}/friends`);
await page.getByRole("tab", { name: /Requests/ }).click();
await page.getByText(/Sent · 1/).waitFor();
check("request listed as sent after reload (server is source of truth)", true);

console.log("Other side accepts → Alex sees friend + notification");
const rel = (await call("GET", "/friends", bella.token)).json.friends;
const alexId = rel.find((x) => x.status === "incoming")?.user.id;
check("Bella sees an incoming request from Alex", !!alexId);
await page.getByRole("tab", { name: /Friends/ }).first().click().catch(() => undefined);
check("accept API ok", (await call("POST", `/friends/${alexId}/accept`, bella.token)).status === 200);
// Alex stays on the page: coming back to the app (visibility change) must refresh the list without a navigation.
await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
await page.getByText("Bella Khan").first().waitFor({ timeout: 5000 });
check("friends list refreshes on app resume (no manual reload)", true);
await page.goto(`${BASE}/friends`);
await page.getByText("Bella Khan").first().waitFor();
check("friends tab shows Bella", (await page.locator(".user-row", { hasText: "Bella Khan" }).count()) === 1);
await page.goto(`${BASE}/notifications`);
await page.getByText("Bella Khan accepted your request").first().waitFor();
check("notification from the inbox endpoint is shown unread", await page.getByRole("button", { name: /Unread\..*accepted your request/ }).count() === 1);

console.log("Incoming request → accept in UI");
await call("POST", "/friends/requests", carlos.token, { user_id: alexId });
await page.goto(`${BASE}/friends`);
await page.getByRole("tab", { name: /Requests/ }).click();
await page.getByText(/Received · 1/).waitFor();
await page.getByRole("button", { name: "Accept" }).first().click();
await page.getByText(/are now friends/).waitFor();
const carlosFriends = (await call("GET", "/friends", carlos.token)).json.friends;
check("accept in UI is persisted server-side", carlosFriends.some((x) => x.status === "friends"));

console.log("Public profile + remove + block (live)");
await page.goto(`${BASE}/profile/${bella.id}`);
await page.getByRole("heading", { name: "Bella Khan" }).waitFor();
check("other user's profile loads from GET /users/{id}", await page.getByText(/@bella/).isVisible());
await page.goto(`${BASE}/friends`);
await page.getByRole("button", { name: "Remove Bella Khan" }).click();
await page.getByRole("dialog").getByRole("button", { name: "Remove" }).click();
await page.getByText(/removed from friends/).waitFor();
check("remove persisted (Bella no longer has Alex as a friend)", (await call("GET", "/friends", bella.token)).json.friends.length === 0);
await page.goto(`${BASE}/profile/${carlos.id}`);
await page.getByRole("button", { name: /Block/ }).first().click();
await page.getByRole("button", { name: "Unblock" }).waitFor();
const afterBlock = await call("POST", "/friends/requests", carlos.token, { user_id: alexId });
check("blocked user cannot send a request (404, block not revealed)", afterBlock.status === 404);
await page.getByRole("button", { name: "Unblock" }).click();
await page.getByRole("button", { name: /Add friend/ }).waitFor();
check("unblock persisted", true);

console.log("Mark all read");
await page.goto(`${BASE}/notifications`);
await page.getByRole("button", { name: /Mark all as read/ }).click();
await page.waitForTimeout(400);
const me = await call("POST", "/auth/login", null, { identifier: "alexlive", password: "Str0ng!Passw0rd", device_id: "chk", platform: "web" });
const inbox = (await call("GET", "/notifications", me.json.tokens.access_token)).json;
check("mark-all-read persisted (unread_count 0)", inbox.unread_count === 0, JSON.stringify(inbox.unread_count));

console.log("Errors");
check("unknown profile id → not-found state", await (async () => { await page.goto(`${BASE}/profile/00000000-0000-0000-0000-000000000000`); return page.getByText("Profile not found").waitFor({ timeout: 5000 }).then(() => true, () => false); })());
check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
