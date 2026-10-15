// Web ⇄ API contract test for settings sync across devices (liveAuth) against tests/fake-api.mjs
// (a hand-written mirror of the Go endpoints — NOT the real server).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_VERIFY_COOLDOWN_MS = "0";
const { start, log } = await import("./fake-api.mjs");
const api = await start(8096);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4178", API_PROXY: "http://localhost:8096" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4178", API = "http://localhost:8096/v1";
async function call(method, path, token, body) {
  const r = await fetch(API + path, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, json: await r.json().catch(() => null) };
}
const reg = (name, username, pw = "Str0ng!Passw0rd") => call("POST", "/auth/register", null, { name, username, email: `${username}@example.com`, password: pw, device_id: "api-" + username, platform: "web" });
const login = (id, pw, dev = "api-phone") => call("POST", "/auth/login", null, { identifier: id, password: pw, device_id: dev, platform: "web" });
await reg("Other Person", "otherp"); // owns other@… so the "taken" case exists

const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true }; });
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

console.log("Sign up");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Set Tings");
await page.getByLabel("Username").fill("settings1");
await page.getByLabel("Email").fill("settings1@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click(); // new live accounts land on the e-mail code screen
await page.waitForURL(`${BASE}/`, { waitUntil: "commit" });
await page.getByRole("heading", { name: /(Good|Still up).*Set/ }).waitFor();
const tok = (await login("settings1", "Str0ng!Passw0rd")).json.tokens.access_token;
check("a new account has no stored settings yet", (await call("GET", "/users/me/settings", tok)).json.settings === null);

console.log("Change a toggle → saved on the server");
await page.goto(`${BASE}/settings/privacy`);
const rr = page.getByRole("switch", { name: "Read receipts" });
await rr.waitFor();
check("read receipts default on", await rr.isChecked());
await rr.click({ force: true });
await page.getByLabel("Last seen & online").selectOption("friends");
await page.waitForFunction(() => true);
let stored;
for (let i = 0; i < 40; i++) { stored = (await call("GET", "/users/me/settings", tok)).json.settings; if (stored?.privacy?.readReceipts === false && stored?.privacy?.lastSeen === "friends") break; await new Promise((r) => setTimeout(r, 100)); }
check("server has the new values", stored?.privacy?.readReceipts === false && stored?.privacy?.lastSeen === "friends", JSON.stringify(stored));

console.log("A fresh browser (no local copy) gets them from the server");
await page.waitForTimeout(800); // let the queued PUTs finish before wiping the local copy
await page.evaluate(() => localStorage.clear());
await page.context().clearCookies();
const fresh = await ctx.browser().newContext();
await fresh.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true }; });
const p2 = await fresh.newPage();
await p2.goto(`${BASE}/login`);
await p2.getByLabel("Email or username").fill("settings1");
await p2.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await p2.getByRole("button", { name: "Sign in" }).click();
await p2.waitForURL(`${BASE}/`, { waitUntil: "commit" });
await p2.getByRole("heading", { name: /(Good|Still up).*Set/ }).waitFor();
await p2.goto(`${BASE}/settings/privacy`);
await p2.getByRole("switch", { name: "Read receipts" }).waitFor();
check("second device shows read receipts OFF", !(await p2.getByRole("switch", { name: "Read receipts" }).isChecked()));
check("…and last seen = friends", (await p2.getByLabel("Last seen & online").inputValue()) === "friends");
check("garbage is refused by the API", (await call("PUT", "/users/me/settings", tok, [1])).status === 400);
check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
