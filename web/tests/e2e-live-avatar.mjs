// Web ⇄ API contract test for profile pictures (liveAuth) against tests/fake-api.mjs (a mirror of the Go endpoints).
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
const { start } = await import("./fake-api.mjs");
const api = await start(8095);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4179", API_PROXY: "http://localhost:8095" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4179", API = "http://localhost:8095/v1";
async function call(method, path, token, body) {
  const r = await fetch(API + path, { method, redirect: "manual", headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, location: r.headers.get("location"), json: await r.json().catch(() => null) };
}
const login = (id) => call("POST", "/auth/login", null, { identifier: id, password: "Str0ng!Passw0rd", device_id: "api", platform: "web" });
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

const { browser, ctx, page, errors } = await launch();
await ctx.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true }; });
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

console.log("Sign up and open the profile editor");
await page.goto(`${BASE}/register`);
await page.getByLabel("Full name").fill("Ava Tar");
await page.getByLabel("Username").fill("avatar1");
await page.getByLabel("Email").fill("avatar1@example.com");
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByRole("button", { name: "Do this later" }).click();
await page.getByRole("heading", { name: /(Good|Still up).*Ava/ }).waitFor();
await page.goto(`${BASE}/profile`);
await page.getByRole("button", { name: "Edit profile" }).click();
check("no picture yet: the editor shows initials", (await page.locator(".avatar-edit img").count()) === 0);

console.log("Upload a picture");
await page.locator('input[type="file"]').setInputFiles({ name: "me.png", mimeType: "image/png", buffer: PNG });
await page.locator(".avatar-edit img").waitFor();
check("instant local preview after picking a file", true);
await page.getByRole("button", { name: "Save" }).click();
await page.getByRole("heading", { name: "Edit profile" }).waitFor({ state: "hidden", timeout: 6000 }); // the dialog closes once the save succeeded
const t = (await login("avatar1")).json.tokens.access_token;
const me = (await call("GET", "/users/me", t)).json;
check("PATCH /users/me stored avatar_media_id", !!me.avatar_media_id);
check("the stored file is a downscaled JPEG (always an allowed type)", (await import("./fake-api.mjs")).media.get(me.avatar_media_id)?.mime === "image/jpeg");
const src = await page.locator("aside img, nav img, header img").first().getAttribute("src").catch(() => null);
check("app shows the picture through the public avatar link", !!src && src.includes(`/users/${me.user_id}/avatar`) && src.includes(`v=${me.avatar_media_id}`), String(src));
await page.waitForFunction(() => [...document.images].some((i) => i.src.includes("/avatar") && i.complete && i.naturalWidth > 0), null, { timeout: 5000 });
check("the image really loads (302 → signed storage link)", true);

console.log("Public link + safety");
const pub = await call("GET", `/users/${me.user_id}/avatar`, null);
check("avatar link needs no token and redirects", pub.status === 302 && /\/storage\//.test(pub.location ?? ""));
check("user without a picture → 404", (await call("GET", `/users/${"0".repeat(8)}-0000-0000-0000-000000000000/avatar`, null)).status === 404);
await call("POST", "/auth/register", null, { name: "Mallory", username: "mallory1", email: "mallory1@example.com", password: "Str0ng!Passw0rd", device_id: "m", platform: "web" });
const mt = (await login("mallory1")).json.tokens.access_token;
const steal = await call("PATCH", "/users/me", mt, { avatar_media_id: me.avatar_media_id });
check("someone else's upload cannot be set as my picture", steal.status === 400);
check("neither can a made-up media id", (await call("PATCH", "/users/me", mt, { avatar_media_id: "11111111-1111-1111-1111-111111111111" })).status === 400);

console.log("Another user sees it; removing it");
const other = await ctx.newPage();
await other.addInitScript(() => { window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true }; });
await page.reload();
await page.getByRole("button", { name: "Edit profile" }).click();
await page.getByRole("button", { name: "Remove" }).click();
await page.getByRole("button", { name: "Save" }).click();
await page.getByRole("heading", { name: "Edit profile" }).waitFor({ state: "hidden", timeout: 6000 });
const me2 = (await call("GET", "/users/me", t)).json;
check("Remove clears avatar_media_id on the server", !me2.avatar_media_id);
check("avatar link is 404 again", (await call("GET", `/users/${me.user_id}/avatar`, null)).status === 404);
await other.close();

check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await finish();
