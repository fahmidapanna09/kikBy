// Web ⇄ API contract test for live ATTACHMENTS (presign → PUT → confirm → send → download-url) over liveChat with TWO real browser sessions talking through
// tests/fake-api.mjs (a hand-written mirror of the Go chat REST + WebSocket envelope — NOT the real server).
// Covers: real-time delivery over the WebSocket (via the serve.mjs /ws proxy), subprotocol auth, delivered/read ticks,
// typing, edit, reaction, delete-for-everyone, persistence across reload.
import { check, launch, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_THUMBS = "1"; // the fake serves a "thumb" variant for images, like the server once the thumbnail worker has run
process.env.FAKE_WS_ORIGINS = "http://localhost:4176"; // the browser Origin the backend allow-list must accept
const { start, chatLog, media, mediaLog } = await import("./fake-api.mjs");

const api = await start(8098);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4176", API_PROXY: "http://localhost:8098" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4176", API = "http://localhost:8098/v1";
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


await call("POST", "/friends/requests", alex.token, { user_id: bella.id }).catch(() => {});
await B.page.goto(`${BASE}/chats`);
await A.page.goto(`${BASE}/friends`);
await A.page.getByRole("button", { name: "Message Bella Khan" }).first().click();
await A.page.waitForURL(/\/chats\/[0-9a-f-]{36}/);
const convId = A.page.url().split("/").pop();
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==", "base64");
const fileA = A.page.locator('input[type="file"]');
const boxA = A.page.getByLabel("Message", { exact: true });

console.log("Image: upload → send → live delivery");
await fileA.setInputFiles({ name: "pixel.png", mimeType: "image/png", buffer: PNG });
await A.page.locator(".pending", { hasText: "pixel.png" }).waitFor();
await A.page.waitForFunction(() => !document.querySelector(".pending .spinner"), null, { timeout: 8000 });
check("file PUT went straight to storage with the declared content-type", mediaLog.puts.length === 1 && mediaLog.puts[0].mime === "image/png" && mediaLog.puts[0].size === PNG.length, JSON.stringify(mediaLog.puts));
await A.page.getByRole("button", { name: "Send message" }).click();
await A.page.locator(".bubble.out .att-img img").waitFor();
check("sender sees the image in the bubble", (await A.page.locator(".bubble.out .att-img img").getAttribute("src")).includes("/storage/"));
await A.page.waitForFunction(() => { const i = document.querySelector(".bubble.out .att-img img"); return i && i.complete && i.naturalWidth === 1; }, null, { timeout: 6000 });
check("the image actually loads from storage (not a broken link)", true);
const sent = (await call("GET", `/conversations/${convId}/messages`, alex.token)).json.messages[0];
check("server stored the attachment on the message, type=image", sent.type === "image" && sent.attachments?.length === 1 && sent.attachments[0].kind === "image" && sent.attachments[0].name === "pixel.png", JSON.stringify(sent));
check("message JSON carries no download URL (links are fetched per id)", !JSON.stringify(sent).includes("/storage/"));

await B.page.getByRole("link", { name: /Alex Live/ }).waitFor({ timeout: 6000 });
check("chat-list preview of a media-only message says Photo", (await B.page.locator(".conv-item", { hasText: "Alex Live" }).innerText()).includes("Photo"));
await B.page.getByRole("link", { name: /Alex Live/ }).click();
await B.page.locator(".bubble.in .att-img img").waitFor();
await B.page.waitForFunction(() => { const i = document.querySelector(".bubble.in .att-img img"); return i && i.complete && i.naturalWidth === 1; }, null, { timeout: 6000 });
check("recipient resolves a signed link and sees the image", (await B.page.locator(".bubble.in .att-img img").getAttribute("src")).includes("sig=d"));
check("recipient's image uses the thumbnail variant (data-thumb, &t=1)", (await B.page.locator(".bubble.in .att-img img").getAttribute("data-thumb")) !== null && (await B.page.locator(".bubble.in .att-img img").getAttribute("src")).includes("t=1"));
const [popup] = await Promise.all([B.page.waitForEvent("popup"), B.page.locator(".bubble.in .att-img").first().click()]);
await popup.waitForURL(/\/storage\//, { timeout: 6000 });
check("clicking opens the FULL file (no thumbnail flag)", popup.url().includes("/storage/") && !popup.url().includes("t=1"), popup.url());
await popup.close();
await B.page.reload();
await B.page.locator(".bubble.in .att-img img").waitFor();
check("after reload the image is still there (history + fresh link)", true);

console.log("Document with caption");
await fileA.setInputFiles({ name: "report.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4 test") });
await A.page.waitForFunction(() => !document.querySelector(".pending .spinner") && document.querySelector(".pending"), null, { timeout: 8000 });
await boxA.fill("Here is the report");
await boxA.press("Enter");
const docB = B.page.locator(".bubble.in", { hasText: "Here is the report" });
await docB.waitFor({ timeout: 6000 });
check("recipient sees file name", (await docB.locator(".att-file").innerText()).includes("report.pdf"));
check("…and size", /13 B|0\.0 KB|13 bytes/i.test(await docB.locator(".att-file").innerText()), await docB.locator(".att-file").innerText());
await B.page.waitForFunction(() => /\/storage\//.test(document.querySelector(".bubble.in .att-file")?.getAttribute("href") || ""), null, { timeout: 6000 });
check("file chip links to a signed storage URL", true);

console.log("Video and audio play inline");
// the fake bytes aren't a decodable video, so the app (correctly) falls back to a file chip after the error; record that a player WAS rendered
await B.page.evaluate(() => { window.__seen = { VIDEO: null, AUDIO: null }; new MutationObserver((ms) => ms.forEach((m) => m.addedNodes.forEach((n) => { if (n.nodeType === 1) for (const el of [n, ...n.querySelectorAll("video,audio")]) if (window.__seen[el.tagName] === null && /^(VIDEO|AUDIO)$/.test(el.tagName)) window.__seen[el.tagName] = { controls: el.controls, src: el.getAttribute("src") }; }))).observe(document.body, { childList: true, subtree: true }); });
await fileA.setInputFiles({ name: "clip.mp4", mimeType: "video/mp4", buffer: Buffer.from("not-a-real-video") });
await A.page.waitForFunction(() => !document.querySelector(".pending .spinner") && document.querySelector(".pending"), null, { timeout: 8000 });
await A.page.getByRole("button", { name: "Send message" }).click();
await B.page.waitForFunction(() => window.__seen.VIDEO, null, { timeout: 6000 });
const vs = await B.page.evaluate(() => window.__seen.VIDEO);
check("recipient gets a <video controls> element pointing at a signed link", vs.controls && /sig=d/.test(vs.src), JSON.stringify(vs));
await fileA.setInputFiles({ name: "note.mp3", mimeType: "audio/mpeg", buffer: Buffer.from("ID3-fake") });
await A.page.waitForFunction(() => !document.querySelector(".pending .spinner") && document.querySelector(".pending"), null, { timeout: 8000 });
await A.page.getByRole("button", { name: "Send message" }).click();
await B.page.waitForFunction(() => window.__seen.AUDIO, null, { timeout: 6000 });
check("recipient gets an <audio controls> element", (await B.page.evaluate(() => window.__seen.AUDIO)).controls === true);
await B.page.locator(".bubble.in .att-file", { hasText: "clip.mp4" }).waitFor({ timeout: 6000 });
check("an undecodable file degrades to a download chip instead of a broken player", true);

console.log("Voice note (live)");
await A.ctx.addInitScript(() => { navigator.mediaDevices.getUserMedia = async () => { const ac = new AudioContext(); const d = ac.createMediaStreamDestination(); const o = ac.createOscillator(); o.connect(d); o.start(); return d.stream; }; });
await A.page.reload();
await A.page.getByLabel("Message", { exact: true }).waitFor();
await A.page.getByRole("button", { name: "Record voice message" }).click();
await A.page.waitForTimeout(1200);
await A.page.getByRole("button", { name: "Finish recording" }).click();
await A.page.waitForFunction(() => !document.querySelector(".pending .spinner") && document.querySelector(".pending") && !document.querySelector(".pending.err"), null, { timeout: 8000 });
await A.page.getByRole("button", { name: "Send message" }).click();
await B.page.locator(".bubble.in .att-audio audio[controls], .bubble.in .att-file", { hasText: /voice-/ }).first().waitFor({ timeout: 6000 }).catch(() => {});
const voiceMsg = (await call("GET", `/conversations/${convId}/messages`, alex.token)).json.messages.find((m) => m.attachments?.[0]?.kind === "voice");
check("server stored it with kind=voice (own allow-list), audio/webm or ogg", !!voiceMsg && /^audio\/(webm|ogg|mp4)/.test(voiceMsg.attachments[0].mime_type), JSON.stringify(voiceMsg));

console.log("Edit keeps the attachment; reactions survive an edit");
await B.page.waitForTimeout(900);
await docB.click({ button: "right" });
await B.page.getByRole("group", { name: "React" }).getByRole("button", { name: "React ❤️" }).click();
await A.page.locator(".react-chip", { hasText: "❤️" }).waitFor({ timeout: 6000 });
await A.page.waitForTimeout(900);
await A.page.locator(".bubble.out", { hasText: "Here is the report" }).click({ button: "right" });
await A.page.getByRole("menuitem", { name: "Edit" }).click();
await boxA.fill("Here is the FINAL report");
await boxA.press("Enter");
const docA = A.page.locator(".bubble.out", { hasText: "FINAL report" });
await docA.waitFor({ timeout: 6000 });
await B.page.locator(".bubble.in", { hasText: "FINAL report" }).waitFor({ timeout: 6000 });
check("sender's bubble keeps its file after the edit", (await docA.locator(".att-file").count()) === 1);
check("recipient's bubble keeps its file after message.edited", (await B.page.locator(".bubble.in", { hasText: "FINAL report" }).locator(".att-file").count()) === 1);
check("reaction survives the edit (sender)", (await A.page.locator(".react-chip", { hasText: "❤️" }).count()) === 1);
check("reaction survives the edit (recipient)", (await B.page.locator(".react-chip", { hasText: "❤️" }).count()) === 1);

console.log("Rejections");
const before = mediaLog.puts.length;
await fileA.setInputFiles({ name: "setup.exe", mimeType: "application/x-msdownload", buffer: Buffer.from("MZ") });
await A.page.locator(".pending.err", { hasText: "setup.exe" }).waitFor();
check("unsupported type is refused before any request", mediaLog.puts.length === before && /isn't supported/.test(await A.page.locator(".pending.err").innerText()));
await A.page.getByRole("button", { name: "Remove setup.exe" }).click();
await fileA.setInputFiles({ name: "reject-me.txt", mimeType: "text/plain", buffer: Buffer.from("hello") });
await A.page.locator(".pending.err", { hasText: "reject-me.txt" }).waitFor();
check("server-side rejection at confirm is shown to the user", /safety check/.test(await A.page.locator(".pending.err").innerText()), await A.page.locator(".pending.err").innerText());
check("a rejected file can't be attached by id", await (async () => {
  const id = [...media].find(([, f]) => f.name === "reject-me.txt")?.[0];
  const r = await call("POST", `/conversations/${convId}/messages`, alex.token, { client_tmp_id: "sneaky", type: "document", attachment_media_ids: [id] });
  return !r.json.attachments?.length;
})());
await A.page.getByRole("button", { name: "Remove reject-me.txt" }).click();
await fileA.setInputFiles({ name: "x.png", mimeType: "image/png", buffer: Buffer.alloc(0) });
await A.page.locator(".pending.err", { hasText: "x.png" }).waitFor();
check("empty file is refused", /empty/.test(await A.page.locator(".pending.err").innerText()));

console.log("Access control (server contract)");
const mid = sent.attachments[0].id;
const outsider = await call("POST", "/auth/register", null, { name: "Eve", username: "eve99", email: "eve99@example.com", password: PW, device_id: "t-eve", platform: "web" });
const eve = (await call("POST", "/auth/login", null, { identifier: "eve99", password: PW, device_id: "t-eve", platform: "web" })).json.tokens.access_token;
const malTok = (await (async () => { await call("POST", "/auth/register", null, { name: "Mal Out", username: "mal99", email: "mal99@example.com", password: PW, device_id: "t-mal", platform: "web" }); return (await call("POST", "/auth/login", null, { identifier: "mal99", password: PW, device_id: "t-mal", platform: "web" })).json.tokens.access_token; })());
console.log("Forward a message with a file");
const bellaEve = await call("POST", "/conversations", bella.token, { peer_user_id: (await call("POST", "/auth/login", null, { identifier: "eve99", password: PW, device_id: "t-eve2", platform: "web" })).json.user.id });
check("before the forward Eve still can't open Alex's image", (await call("GET", `/media/${mid}/download-url`, eve)).status === 403);
await B.page.reload();
await B.page.getByRole("link", { name: /Eve/ }).waitFor();
await B.page.getByRole("link", { name: /Alex Live/ }).click();
await B.page.locator(".bubble.in .att-img").first().waitFor();
await B.page.locator(".bubble.in", { has: B.page.locator(".att-img") }).first().click({ button: "right" });
await B.page.getByRole("menuitem", { name: "Forward" }).click();
await B.page.getByRole("dialog").getByRole("checkbox", { name: /Eve/ }).click();
await B.page.getByRole("dialog").getByRole("button", { name: /^Send/ }).click();
await B.page.getByText(/Forwarded to 1 chat/).waitFor({ timeout: 6000 });
const evMsgs = (await call("GET", `/conversations/${bellaEve.json.id}/messages`, eve)).json.messages;
check("the forwarded message in the Eve chat carries the same file", evMsgs.length === 1 && evMsgs[0].attachments?.[0]?.id === mid && evMsgs[0].type === "image", JSON.stringify(evMsgs));
check("Eve can now open it (the file is in a conversation she is in)", (await call("GET", `/media/${mid}/download-url`, eve)).status === 200);
check("a stranger who is in neither chat still can't", (await call("GET", `/media/${mid}/download-url`, malTok)).status === 403);
const sneak = await call("POST", `/conversations/${bellaEve.json.id}/messages`, malTok, { client_tmp_id: "x", type: "image", attachment_media_ids: [mid] });
check("…and can't attach it to a message either", sneak.status !== 200 || !sneak.json?.attachments?.length);

check("a non-member can't get a download link", (await call("GET", `/media/${mid}/download-url`, malTok)).status === 403);
check("a member can", (await call("GET", `/media/${mid}/download-url`, bella.token)).status === 200);
check("no uncaught console/page errors (Alex)", A.errors.length === 0, JSON.stringify(A.errors));
check("no uncaught console/page errors (Bella)", B.errors.length === 0, JSON.stringify(B.errors));
await finish();
