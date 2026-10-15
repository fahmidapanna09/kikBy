// Web client ⇄ API contract test: liveAuth mode against tests/fake-api.mjs (NOT the real Go server).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
import { start, log } from "./fake-api.mjs";

process.env.ACCESS_TTL_MS = "5000";
const api = await start(8099);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4174", API_PROXY: "http://localhost:8099" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4174";
const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true }; });
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

console.log("Register (live)");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Live Tester");
await page.getByLabel("Username").fill("livetester");
await page.getByLabel("Email").fill("live@example.com");
await page.getByLabel("Phone number (optional)").fill("+880 1712-345678");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click(); // new live accounts land on the e-mail code screen
await page.waitForURL(`${BASE}/`, { waitUntil: "commit" }).catch(async (e) => { console.log("URL:", page.url(), "ALERTS:", await page.locator("[role=alert],.form-error").allInnerTexts(), errors); throw e; });
await page.getByRole("heading", { name: /(Good|Still up).*Live/ }).waitFor();
check("register hits the API and lands home with server-side display name", true);

console.log("Duplicate email maps to the field");
await page.evaluate(() => localStorage.clear());
await page.evaluate(() => sessionStorage.clear());
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Other Person");
await page.getByLabel("Username").fill("otherperson");
await page.getByLabel("Email").fill("LIVE@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByText("an account with this email already exists").waitFor();
check("server 422 fields shown under the email input", true);

console.log("Wrong password does NOT log out / loop");
await page.goto(`${BASE}/login`);
await page.getByLabel("Email or username").fill("live@example.com");
await page.getByLabel("Password", { exact: true }).fill("Wrong1234");
await page.getByRole("button", { name: "Sign in" }).click();
await page.getByText(/incorrect email\/username or password/i).waitFor();
check("401 on login shows credential error and stays on /login", page.url().includes("/login"));

console.log("Login + silent refresh");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(`${BASE}/`);
const before = log.refreshCalls;
await page.waitForTimeout(7000); // access TTL is 5 s → refresh scheduled ~immediately (60 s margin > TTL)
check("access token refreshed in the background", log.refreshCalls > before, `calls=${log.refreshCalls}`);
await page.goto(`${BASE}/profile`);
await page.getByRole("heading", { name: "Live Tester" }).waitFor();
check("still signed in after token expiry; profile loads via GET /users/me", true);

console.log("Edit profile persists server-side");
await page.getByRole("button", { name: /Edit profile/ }).click();
await page.getByLabel("Display name").fill("Live Tester 2");
await page.getByRole("button", { name: "Save" }).click();
await page.getByRole("heading", { name: "Live Tester 2" }).waitFor();
await page.evaluate(() => localStorage.clear());
await page.evaluate(() => sessionStorage.clear());
await page.goto(`${BASE}/login`);
await page.getByLabel("Email or username").fill("livetester");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(`${BASE}/`);
await page.getByRole("heading", { name: /(Good|Still up).*Live/ }).waitFor();
check("new display name survives a fresh login (read from server); username login works", true);

console.log("Forgot / reset (live)");
await page.getByRole("button", { name: "Account menu" }).click();
await page.getByRole("menuitem", { name: "Sign out" }).click();
await page.waitForURL(/\/login/);
check("logout called the API and returned to login", log.logouts === 1, `logouts=${log.logouts}`);
await page.getByRole("link", { name: "Forgot password?" }).click();
await page.getByLabel("Email").fill("live@example.com");
await page.getByRole("button", { name: "Send reset link" }).click();
await page.getByRole("heading", { name: "Check your email" }).waitFor();
check("forgot shows neutral confirmation, no mock link leaked", (await page.getByText("open reset link").count()) === 0);
await page.goto(`${BASE}/reset-password?token=${log.resetLinks.at(-1)}`);
await page.getByLabel("New password", { exact: true }).fill("NewPassw0rd!");
await page.getByLabel("Confirm new password").fill("NewPassw0rd!");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByRole("heading", { name: "Password updated" }).waitFor();
await page.goto(`${BASE}/reset-password?token=${log.resetLinks.at(-1)}`);
await page.getByLabel("New password", { exact: true }).fill("Another1234");
await page.getByLabel("Confirm new password").fill("Another1234");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText(/invalid or has expired/).waitFor();
check("reset token is single-use", true);

console.log("Per-account lockout");
const lockName = "lockout1";
await fetch("http://localhost:8099/v1/auth/register", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: "Lock Out", username: lockName, email: `${lockName}@example.com`, password: "Str0ng!Passw0rd", device_id: "lk", platform: "web" }) });
const tryLogin = (pw) => fetch("http://localhost:8099/v1/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ identifier: lockName, password: pw, device_id: "lk", platform: "web" }) });
for (let i = 0; i < 10; i++) await tryLogin("wrong-" + i);
const locked = await tryLogin("Str0ng!Passw0rd");
check("after 10 failures even the right password gets 429 account_locked", locked.status === 429 && (await locked.json()).error.code === "account_locked");
check("the response tells the client how long to wait", locked.headers.get("retry-after") === "900");
await page.goto(`${BASE}/login`);
await page.getByLabel("Email or username").fill(lockName);
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Sign in" }).click();
await page.getByText(/Too many failed attempts/).waitFor({ timeout: 5000 });
check("the sign-in form shows the lockout message", true);
check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
