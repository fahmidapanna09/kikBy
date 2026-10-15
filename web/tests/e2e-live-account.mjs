// Web ⇄ API contract test for change password / change e-mail / delete account (liveAuth) against tests/fake-api.mjs
// (a hand-written mirror of the Go endpoints — NOT the real server).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_VERIFY_COOLDOWN_MS = "0";
const { start, log } = await import("./fake-api.mjs");
const api = await start(8099);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4177", API_PROXY: "http://localhost:8099" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4177", API = "http://localhost:8099/v1";
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

console.log("Sign up in the browser, plus a second device via the API");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Acc Ount");
await page.getByLabel("Username").fill("account1");
await page.getByLabel("Email").fill("account1@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click(); // new live accounts land on the e-mail code screen
await page.waitForURL(`${BASE}/`, { waitUntil: "commit" });
await page.getByRole("heading", { name: /(Good|Still up).*Acc/ }).waitFor();
const phone = await login("account1", "Str0ng!Passw0rd");
check("second device is signed in", (await call("GET", "/users/me", phone.json.tokens.access_token)).status === 200);

console.log("Change password");
await page.goto(`${BASE}/settings/security`);
await page.getByLabel("Current password").fill("WrongPass123");
await page.getByLabel("New password", { exact: true }).fill("N3w!Passw0rd-x");
await page.getByLabel("Confirm new password").fill("N3w!Passw0rd-x");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText("incorrect password").first().waitFor();
check("wrong current password → inline error from the server", true);
check("…and the user is NOT signed out (422, not 401)", page.url().includes("/settings/security"));
await page.getByLabel("Current password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText("Password changed").waitFor();
check("password changed", log.passwordChanges === 1);
check("this browser stays signed in", (await page.evaluate(() => !!(localStorage.getItem("kikbite.session") || sessionStorage.getItem("kikbite.session")))));
await page.reload();
await page.getByRole("heading", { name: /Security|Change password/ }).first().waitFor();
check("reload keeps the session", page.url().includes("/settings/security"));
check("the OTHER device was signed out", (await call("GET", "/users/me", phone.json.tokens.access_token)).status === 401);
check("old password no longer works", (await login("account1", "Str0ng!Passw0rd")).status === 401);
check("new password works", (await login("account1", "N3w!Passw0rd-x")).status === 200);

console.log("Change e-mail (pending until confirmed)");
await page.goto(`${BASE}/settings/account`);
await page.getByLabel("Email", { exact: true }).fill("account1-new@example.com");
await page.getByRole("button", { name: "Change email" }).click();
await page.getByText("Enter your current password").waitFor();
check("password is required to change e-mail", true);
await page.getByLabel("Current password").fill("nope");
await page.getByRole("button", { name: "Change email" }).click();
await page.getByText("Incorrect password").waitFor();
check("wrong password rejected inline", true);
await page.getByLabel("Email", { exact: true }).fill("otherp@example.com");
await page.getByLabel("Current password").fill("N3w!Passw0rd-x");
await page.getByRole("button", { name: "Change email" }).click();
await page.getByText("an account with this email already exists").waitFor();
check("an address owned by someone else is refused", true);
await page.getByLabel("Email", { exact: true }).fill("account1-new@example.com");
await page.getByLabel("Current password").fill("N3w!Passw0rd-x");
await page.getByRole("button", { name: "Change email" }).click();
await page.getByText(/We sent a 6-digit code to account1-new@example.com/).waitFor();
check("confirmation notice shown", true);
const me1 = await call("GET", "/users/me", (await login("account1", "N3w!Passw0rd-x")).json.tokens.access_token);
check("login e-mail has NOT changed yet; pending is reported", me1.json.email === "account1@example.com" && me1.json.pending_email === "account1-new@example.com");
check("old e-mail still signs in, new one does not (yet)", (await login("account1@example.com", "N3w!Passw0rd-x")).status === 200 && (await login("account1-new@example.com", "N3w!Passw0rd-x")).status === 401);
const code = log.verifyCodes.at(-1);
check("a code was mailed for the NEW address", code && log.verifyCodes.length >= 1);
await page.goto(`${BASE}/verify-email`);
await page.getByLabel("Digit 1").click();
await page.keyboard.type(code);
await page.getByRole("heading", { name: "E-mail verified" }).waitFor();
check("code confirms the new address", true);
check("new e-mail signs in, old one no longer does", (await login("account1-new@example.com", "N3w!Passw0rd-x")).status === 200 && (await login("account1@example.com", "N3w!Passw0rd-x")).status === 401);
await page.goto(`${BASE}/settings/account`);
check("settings shows the confirmed address", (await page.getByLabel("Email", { exact: true }).inputValue()) === "account1-new@example.com");
const replay = await call("POST", "/auth/verify-email", (await login("account1-new@example.com", "N3w!Passw0rd-x")).json.tokens.access_token, { code });
check("code cannot be replayed", replay.status === 400);

console.log("Delete account");
await page.goto(`${BASE}/settings/other`);
await page.getByRole("button", { name: "Delete…" }).click();
await page.getByLabel("Type your username").fill("account1");
await page.getByLabel("Password", { exact: true }).fill("wrong-password");
await page.getByRole("button", { name: "Delete forever" }).click();
await page.getByText("incorrect password").waitFor();
check("wrong password does not delete (and does not sign out)", page.url().includes("/settings/other"));
await page.getByLabel("Password", { exact: true }).fill("N3w!Passw0rd-x");
await page.getByRole("button", { name: "Delete forever" }).click();
await page.waitForURL(/\/login/);
check("after deleting, the app returns to the login page", true);
check("the deleted account can no longer sign in", (await login("account1-new@example.com", "N3w!Passw0rd-x")).status === 401);
check("its username can be registered again", (await reg("Fresh Start", "account1", "An0ther!Passw0rd")).status === 201);

check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
