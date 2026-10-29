// Online / offline dots end to end (liveAuth + liveChat, against tests/fake-api.mjs which mirrors the Go presence fan-out:
// broadcast on first connect / last disconnect, plus a snapshot of who is online when a client connects).
// Alex is the browser; Bella is a raw WebSocket "phone" that goes online and offline.
import { createRequire } from "node:module";
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
const { WebSocket } = createRequire(process.env.PW_ROOT || "/opt/npm-tools/node_modules/")("ws");
process.env.ACCESS_TTL_MS = "600000";
const { start } = await import("./fake-api.mjs");
const api = await start(8097);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4177", API_PROXY: "http://localhost:8097" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4177", API = "http://localhost:8097/v1";
async function call(method, path, token, body) {
  const r = await fetch(API + path, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, json: await r.json().catch(() => null) };
}
const reg = await call("POST", "/auth/register", null, { name: "Bella Khan", username: "bella", email: "bella@example.com", password: "Str0ng!Passw0rd", device_id: "api-bella", platform: "web" });
const bella = { id: reg.json.user.id, token: reg.json.tokens.access_token };
const phone = { ws: null };
const goOnline = () => new Promise((res) => { phone.ws = new WebSocket("ws://localhost:8097/ws", ["kikbite.v1", `bearer.${bella.token}`]); phone.ws.on("open", res); });
const goOffline = () => new Promise((res) => { phone.ws.on("close", res); phone.ws.close(); });

const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true }; });
const finish = async () => { try { phone.ws?.close(); } catch {} await browser.close(); api.close(); web.kill(); summary(); };

console.log("Alex signs up and becomes friends with Bella");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Alex Live");
await page.getByLabel("Username").fill("alexlive");
await page.getByLabel("Email").fill("alex@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click();
await page.waitForURL(`${BASE}/`, { waitUntil: "commit" });
await page.getByRole("heading", { name: /(Good|Still up).*Alex/ }).waitFor();
await page.goto(`${BASE}/search`);
await page.getByLabel("Search people and chats").fill("bel");
await page.getByRole("button", { name: "Follow", exact: true }).first().click();
await page.getByText(/You are following/).waitFor();
const alexId = (await call("GET", "/friends", bella.token)).json.friends.find((x) => x.status === "incoming")?.user.id;
await call("POST", `/friends/${alexId}/accept`, bella.token);
await page.goto(`${BASE}/`);
const panel = page.locator(".panel-title", { hasText: "Online now" });
await panel.waitFor();
const count = async () => (await panel.locator(".badge").innerText()).trim();
await page.waitForTimeout(600);
check("Bella is offline: Online now shows 0", (await count()) === "0", await count());

console.log("Bella's phone connects while Alex is looking (live update)");
await goOnline();
await page.waitForFunction(() => /Online now\s*1/.test(document.body.innerText), null, { timeout: 5000 });
check("Online now shows 1 without any reload", true);
check("Bella is listed", (await page.getByText("Bella Khan").count()) > 0);

console.log("Bella's phone goes away (live update)");
await goOffline();
await page.waitForFunction(() => /Online now\s*0/.test(document.body.innerText), null, { timeout: 5000 });
check("Online now drops back to 0", true);

console.log("Alex reloads while Bella is offline: he is told when she was last active");
await page.reload();
await page.getByRole("heading", { name: /(Good|Still up).*Alex/ }).waitFor();
await page.goto(`${BASE}/friends`);
await page.getByText(/active (just now|\d+ min ago)/).first().waitFor({ timeout: 5000 });
check("an offline friend shows 'active … ago' (not just 'offline')", true);
await page.goto(`${BASE}/`);

console.log("Bella is already online when Alex opens the app (snapshot on connect)");
await goOnline();
await page.reload();
await page.getByRole("heading", { name: /(Good|Still up).*Alex/ }).waitFor();
await page.waitForFunction(() => /Online now\s*1/.test(document.body.innerText), null, { timeout: 5000 });
check("the app shows Bella online right after opening, with no later event", true);

console.log("The chat header agrees");
const chat = await call("POST", "/conversations", bella.token, { type: "direct", member_ids: [alexId] }).catch(() => null);
await page.goto(`${BASE}/friends`);
await page.waitForTimeout(800);
const onlineDot = await page.locator("[data-presence='online'], .presence.online, .avatar .dot.online").count();
check("a green dot is rendered for her on the Friends page", onlineDot > 0, `dots=${onlineDot}`);
await page.getByRole("button", { name: /Message/ }).first().click().catch(() => undefined);

check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
