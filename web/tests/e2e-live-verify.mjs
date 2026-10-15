// Web ⇄ API contract test for e-mail verification by 6-digit code (liveAuth) against tests/fake-api.mjs (mirror of the Go endpoints).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_VERIFY_COOLDOWN_MS = "4000"; // short, so the cooldown is observable without a 60 s wait
const { start, log } = await import("./fake-api.mjs");
const api = await start(8099);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4176", API_PROXY: "http://localhost:8099" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4176";
const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true }; });
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

const typeCode = async (code) => { await page.getByLabel("Digit 1").click(); await page.keyboard.type(code); };
const register = async (name, username, email) => {
  await page.goto(`${BASE}/register`);
  await page.getByLabel("Full name").fill(name);
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
  await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
  await page.getByRole("button", { name: "Create account" }).click();
};

console.log("Register → code screen");
await register("Vera Verify", "vera", "vera@example.com");
await page.getByRole("heading", { name: "Verify your e-mail" }).waitFor();
check("new live account lands on the code screen", page.url().endsWith("/verify-email"));
check("screen names the address and the 10-minute lifetime", (await page.getByText(/vera@example\.com\. It expires in 10 minutes/).count()) === 1);
check("registration queued one 6-digit code", log.verifyCodes.length === 1 && /^\d{6}$/.test(log.verifyCodes[0]));

console.log("Wrong code, then resend + cooldown");
await typeCode(log.verifyCodes[0] === "123456" ? "654321" : "123456");
await page.getByText(/code isn't right/).waitFor();
check("wrong code is rejected with a clear message", true);
await page.getByRole("button", { name: /resend in \d+s|send a new code/ }).waitFor();
await page.getByRole("button", { name: "send a new code" }).click();
await page.getByText(/wait a minute/).waitFor();
check("immediate resend is refused by the cooldown", log.verifyCodes.length === 1);
await page.waitForTimeout(4200);
await page.getByRole("button", { name: "send a new code" }).click();
await page.getByText(/New code sent to vera@example.com/).waitFor();
check("resend after the cooldown succeeds", log.verifyCodes.length === 2);

console.log("Old code is dead, newest code works");
const [oldCode, newCode] = log.verifyCodes;
await typeCode(oldCode === newCode ? "000000" : oldCode);
if (oldCode !== newCode) { await page.getByText(/code isn't right/).waitFor(); check("superseded code is rejected", true); }
await typeCode(newCode);
await page.getByRole("heading", { name: "E-mail verified" }).waitFor();
check("current code verifies the address", true);
await page.getByRole("button", { name: "Continue to KikBite" }).click();
await page.getByRole("heading", { name: /(Good|Still up).*Vera/ }).waitFor();
check("banner is gone after verifying", (await page.getByText(/Verify your e-mail address/).count()) === 0);
await page.reload();
await page.getByRole("heading", { name: /(Good|Still up).*Vera/ }).waitFor();
check("still verified after reload (flag comes from GET /users/me)", (await page.getByText(/Verify your e-mail address/).count()) === 0);

console.log("Pasting a code");
await page.evaluate(() => localStorage.clear());
await register("Paste Person", "paste", "paste@example.com");
await page.getByRole("heading", { name: "Verify your e-mail" }).waitFor();
await page.getByLabel("Digit 1").fill(log.verifyCodes.at(-1));
await page.getByRole("heading", { name: "E-mail verified" }).waitFor();
check("a pasted 6-digit code fills all boxes and auto-submits", true);

console.log("Too many wrong guesses burn the code");
await page.evaluate(() => localStorage.clear());
await register("Burn Person", "burn", "burn@example.com");
await page.getByRole("heading", { name: "Verify your e-mail" }).waitFor();
const real = log.verifyCodes.at(-1);
const wrong = real === "111111" ? "222222" : "111111";
for (let i = 0; i < 5; i++) {
  await typeCode(wrong);
  await page.getByText(i < 4 ? /code isn't right/ : /too many wrong codes/).waitFor();
}
check("5th wrong guess burns the code", true);
await typeCode(real);
await page.getByText(/expired|request a new one/).waitFor();
check("the burned code no longer works, even when correct", (await page.getByRole("heading", { name: "E-mail verified" }).count()) === 0);

console.log("Banner + signed-out access");
await page.getByRole("button", { name: "Do this later" }).click();
await page.getByRole("heading", { name: /(Good|Still up).*Burn/ }).waitFor();
await page.getByText(/Verify your e-mail address \(burn@example.com\)/).waitFor();
check("banner offers an Enter code action", await page.getByRole("button", { name: "Enter code" }).isVisible());
await page.getByRole("button", { name: "Enter code" }).click();
await page.getByRole("heading", { name: "Verify your e-mail" }).waitFor();
await page.evaluate(() => localStorage.clear());
await page.goto(`${BASE}/verify-email`);
await page.getByRole("heading", { name: /Sign in|Welcome/i }).first().waitFor();
check("signed-out visitors are sent to sign in (no public verify endpoint)", page.url().includes("/login"));

check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
