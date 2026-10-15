// Web ⇄ API contract test for voice / video / GROUP calls with THREE real browser sessions and REAL WebRTC between them
// (Chromium, loopback host candidates), signaling through tests/fake-api.mjs (a hand-written mirror of the Go call service —
// NOT the real server). Camera/mic are synthetic streams (the sandbox has no devices) so everything after getUserMedia is real:
// RTCPeerConnection offer/answer/ICE over the relayed signaling, tracks flowing, mute/camera toggles, hang-up, decline, missed,
// busy, group join/leave, a late join through the "call in progress" banner, and simultaneous joins (offer glare).
import { chromium, check, summary } from "./helpers.mjs";
import { spawn } from "node:child_process";
process.env.ACCESS_TTL_MS = "600000";
process.env.FAKE_WS_ORIGINS = "http://localhost:4181";
process.env.FAKE_RING_MS = "4000";
const { start, callsDb } = await import("./fake-api.mjs");
const api = await start(8101);
const web = spawn("node", ["scripts/serve.mjs"], { env: { ...process.env, PORT: "4181", API_PROXY: "http://localhost:8101" }, stdio: "ignore" });
await new Promise((r) => setTimeout(r, 800));
const BASE = "http://localhost:4181", API = "http://localhost:8101/v1", PW = "Str0ng!Passw0rd";
const call = async (method, path, token, body) => {
  const r = await fetch(API + path, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, json: await r.json().catch(() => null) };
};

const browser = await chromium.launch({ executablePath: "/opt/pw-browsers/chromium", args: ["--no-sandbox", "--disable-features=WebRtcHideLocalIpsWithMdns", "--autoplay-policy=no-user-gesture-required"] });
const initScript = () => {
  window.__KIKBITE_CONFIG__ = { liveAuth: true, liveChat: true };
  window.__pcs = [];
  const Orig = window.RTCPeerConnection;
  window.RTCPeerConnection = class extends Orig { constructor(...a) { super(...a); window.__pcs.push(this); } };
  window.__gum = [];
  navigator.mediaDevices.getUserMedia = async (c) => {
    window.__gum.push(JSON.stringify(c));
    const s = new MediaStream();
    if (c.audio) { const ac = new AudioContext(); const dst = ac.createMediaStreamDestination(); const o = ac.createOscillator(); o.connect(dst); o.start(); await ac.resume().catch(() => {}); dst.stream.getAudioTracks().forEach((t) => s.addTrack(t)); }
    if (c.video) { const cv = document.createElement("canvas"); cv.width = 320; cv.height = 240; const g = cv.getContext("2d"); let n = 0; setInterval(() => { g.fillStyle = `hsl(${(n += 17) % 360},80%,50%)`; g.fillRect(0, 0, 320, 240); }, 50); cv.captureStream(15).getVideoTracks().forEach((t) => s.addTrack(t)); }
    return s;
  };
};
async function newUser() {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 } });
  await ctx.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort());
  await ctx.addInitScript(initScript);
  const page = await ctx.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => m.type() === "error" && !/Failed to load resource/.test(m.text()) && errors.push(m.text()));
  return { ctx, page, errors };
}
process.on("exit", () => web.kill()); // never leave the web server behind when an assertion throws
const finish = async () => { await browser.close(); api.close(); web.kill(); summary(); };

const U = { alex: await newUser(), bella: await newUser(), cara: await newUser() };
const names = { alex: "Alex Live", bella: "Bella Khan", cara: "Cara Dev" };
const ids = {}, tokens = {};
for (const u of Object.keys(U)) {
  const r = await call("POST", "/auth/register", null, { name: names[u], username: u, email: `${u}@example.com`, password: PW, device_id: "t-" + u, platform: "web" });
  const l = await call("POST", "/auth/login", null, { identifier: u, password: PW, device_id: "t-" + u, platform: "web" });
  ids[u] = l.json.user.id; tokens[u] = l.json.tokens.access_token; void r;
}
for (const [x, y] of [["alex", "bella"], ["alex", "cara"], ["cara", "bella"]]) { await call("POST", "/friends/requests", tokens[x], { user_id: ids[y] }); await call("POST", `/friends/${ids[x]}/accept`, tokens[y]); }
const dmAB = (await call("POST", "/conversations", tokens.alex, { peer_user_id: ids.bella })).json.id;
const dmCB = (await call("POST", "/conversations", tokens.cara, { peer_user_id: ids.bella })).json.id;
const group = (await call("POST", "/groups", tokens.alex, { name: "Trip Crew", member_user_ids: [ids.bella, ids.cara] })).json.conversation_id;

async function login(u) {
  const p = U[u].page;
  await p.goto(`${BASE}/login`);
  await p.getByLabel("Email or username").fill(u);
  await p.getByLabel("Password", { exact: true }).fill(PW);
  await p.getByRole("button", { name: "Sign in" }).click();
  await p.waitForURL(`${BASE}/`, { waitUntil: "commit" });
  await p.getByRole("heading", { name: new RegExp(`(Good|Still up).*${names[u].split(" ")[0]}`) }).waitFor();
}
for (const u of Object.keys(U)) await login(u);
const open = async (u, convId) => { await U[u].page.goto(`${BASE}/chats/${convId}`); await U[u].page.locator(".chat-header").waitFor(); };
// every page sits in its chat; the realtime socket must be open before anybody calls
await open("alex", dmAB); await open("bella", dmAB); await open("cara", dmCB);

const P = (u) => U[u].page;
const screen = (u) => P(u).getByRole("dialog", { name: /call with/ });
const incoming = (u) => P(u).getByRole("alertdialog");
const connected = (u) => P(u).locator(".call-status", { hasText: /^\d+:\d\d$/ }).waitFor({ timeout: 15000 });
const gone = async (u) => { await P(u).locator(".call-screen").waitFor({ state: "detached", timeout: 8000 }); };
const dump = (u) => P(u).evaluate(() => ({ tiles: [...document.querySelectorAll("figure.call-tile[data-user]")].map((f) => f.dataset.user.slice(0, 4) + ":" + f.dataset.state), vids: [...document.querySelectorAll("video[data-remote]")].map((v) => v.videoWidth + "/" + v.readyState), pcs: window.__pcs.map((pc) => pc.connectionState + "/" + pc.signalingState + "/" + pc.iceConnectionState) }));
const remotesWithVideo = (u, n) => remotesWithVideo0(u, n).catch(async (e) => { console.log("  DEBUG", u, JSON.stringify(await dump(u))); throw e; });
const remotesWithVideo0 = (u, n) => P(u).waitForFunction((n) => [...document.querySelectorAll("video[data-remote]")].filter((v) => v.videoWidth > 0 && v.readyState >= 2).length === n, n, { timeout: 15000 });
const remoteTiles = (u) => P(u).locator("video[data-remote]").count();
const inboundBytes = (u, kind) => P(u).evaluate(async (kind) => { let t = 0; for (const pc of window.__pcs) for (const r of (await pc.getStats()).values()) if (r.type === "inbound-rtp" && r.kind === kind) t += r.bytesReceived || 0; return t; }, kind);
const history = async (conv) => (await call("GET", `/conversations/${conv}/calls`, tokens.alex)).json.calls;
const waitFor = async (fn, ms = 8000) => { const t0 = Date.now(); while (Date.now() - t0 < ms) { if (await fn()) return true; await new Promise((r) => setTimeout(r, 100)); } return false; };

console.log("Call buttons");
check("a direct chat has voice and video call buttons", (await P("alex").getByRole("button", { name: "Voice call" }).isVisible()) && (await P("alex").getByRole("button", { name: "Video call" }).isVisible()));

console.log("1:1 voice call");
await P("alex").getByRole("button", { name: "Voice call" }).click();
await screen("alex").waitFor();
check("the caller sees a ringing call screen", /Ringing/.test(await screen("alex").innerText()) || /Calling/.test(await screen("alex").innerText()));
await incoming("bella").waitFor();
const inText = await incoming("bella").innerText();
check("the callee sees who is calling", /Alex Live/.test(inText) && /voice call/i.test(inText), inText);
check("a voice call offers no audio-only button", (await incoming("bella").getByRole("button", { name: "Answer with audio only" }).count()) === 0);
check("the second device of the callee also dialled?  no — only members other than the caller ring", (await P("alex").getByRole("alertdialog").count()) === 0);
check("the caller's call buttons are disabled while ringing", await P("alex").getByRole("button", { name: "Voice call" }).isDisabled());
await incoming("bella").getByRole("button", { name: "Accept call" }).click();
await connected("alex"); await connected("bella");
check("both sides reach a connected call with a running timer", true);
check("only a microphone was requested for a voice call", (await P("bella").evaluate(() => window.__gum)).every((c) => !JSON.parse(c).video));
await waitFor(async () => (await inboundBytes("alex", "audio")) > 0 && (await inboundBytes("bella", "audio")) > 0, 10000);
check("audio RTP actually flows both ways", (await inboundBytes("alex", "audio")) > 0 && (await inboundBytes("bella", "audio")) > 0);
check("a voice call has no camera button", (await screen("alex").getByRole("button", { name: /camera/i }).count()) === 0);
const mute = screen("alex").getByRole("button", { name: "Mute microphone" });
await mute.click();
check("muting flips the button state", (await screen("alex").getByRole("button", { name: "Unmute microphone" }).getAttribute("aria-pressed")) === "true");
check("and disables the local audio track", await P("alex").evaluate(() => [...document.querySelectorAll("video[data-local]")].every((v) => v.srcObject.getAudioTracks().every((t) => !t.enabled))));
await screen("alex").getByRole("button", { name: "End call" }).click();
await gone("alex"); await gone("bella");
check("hanging up ends the call for both people", true);
await waitFor(async () => (await history(dmAB))[0]?.status === "ended");
check("the call is recorded as ended in the history", (await history(dmAB))[0]?.status === "ended" && (await history(dmAB))[0]?.kind === "voice");

console.log("1:1 video call");
await P("alex").getByRole("button", { name: "Video call" }).click();
await incoming("bella").waitFor();
check("a video call offers an audio-only answer", await incoming("bella").getByRole("button", { name: "Answer with audio only" }).isVisible());
await incoming("bella").getByRole("button", { name: "Accept call" }).click();
await connected("alex"); await connected("bella");
await remotesWithVideo("alex", 1); await remotesWithVideo("bella", 1);
check("video frames arrive on both sides", true);
check("the camera was requested for a video call", (await P("alex").evaluate(() => window.__gum)).some((c) => JSON.parse(c).video));
await screen("bella").getByRole("button", { name: "Turn camera off" }).click();
check("the camera button toggles", (await screen("bella").getByRole("button", { name: "Turn camera on" }).getAttribute("aria-pressed")) === "true");
await screen("bella").getByRole("button", { name: "End call" }).click();
await gone("alex"); await gone("bella");

console.log("Video caller + audio-only callee (mixed call)");
await P("alex").getByRole("button", { name: "Video call" }).click();
await incoming("bella").waitFor();
await incoming("bella").getByRole("button", { name: "Answer with audio only" }).click();
await connected("alex"); await connected("bella");
await remotesWithVideo("bella", 1); // Bella (audio only) still SEES Alex's video
check("the audio-only person still sees the other's video", true);
await waitFor(async () => (await inboundBytes("alex", "audio")) > 0);
check("Alex hears Bella", (await inboundBytes("alex", "audio")) > 0);
check("Alex receives no video from Bella (audio only)", await P("alex").evaluate(() => [...document.querySelectorAll("video[data-remote]")].every((v) => !v.srcObject || v.srcObject.getVideoTracks().length === 0)));
check("Bella's tile on Alex's screen shows an avatar, not a black box", (await P("alex").locator(".call-tile.audio-only:not(.local)").count()) === 1);
check("Bella requested audio only", (await P("bella").evaluate(() => window.__gum)).some((c) => !JSON.parse(c).video));
await screen("alex").getByRole("button", { name: "End call" }).click();
await gone("alex"); await gone("bella");

console.log("Decline");
await P("alex").getByRole("button", { name: "Voice call" }).click();
await incoming("bella").waitFor();
await incoming("bella").getByRole("button", { name: "Decline call" }).click();
await gone("alex");
check("the caller's screen closes when the callee declines", true);
check("the caller is told it was declined", await P("alex").getByText("Call declined.").isVisible());
check("recorded as declined", await waitFor(async () => (await history(dmAB))[0]?.status === "declined"));

console.log("Cancel while ringing / missed");
await P("alex").getByRole("button", { name: "Voice call" }).click();
await incoming("bella").waitFor();
await screen("alex").getByRole("button", { name: "End call" }).click();
await incoming("bella").waitFor({ state: "detached", timeout: 6000 });
check("hanging up while ringing stops the callee's ringing", true);
check("recorded as cancelled", await waitFor(async () => (await history(dmAB))[0]?.status === "cancelled"));
check("the callee is told about the missed call", await P("bella").getByText(/Missed voice call from Alex/).isVisible());
await P("alex").getByRole("button", { name: "Voice call" }).click();
await incoming("bella").waitFor();
await incoming("bella").waitFor({ state: "detached", timeout: 9000 });
await gone("alex");
check("an unanswered call times out on both sides", true);
check("recorded as missed", await waitFor(async () => (await history(dmAB))[0]?.status === "missed"));

console.log("Busy: a new direct call while already in a call is auto-declined");
await P("alex").getByRole("button", { name: "Voice call" }).click();
await incoming("bella").getByRole("button", { name: "Accept call" }).click();
await connected("alex"); await connected("bella");
await P("cara").getByRole("button", { name: "Voice call" }).click();
await P("cara").locator(".call-screen").waitFor({ state: "detached", timeout: 8000 });
check("Cara is told Bella (already in a call) declined", await P("cara").getByText("Call declined.").isVisible());
check("Bella's call with Alex is unaffected", (await screen("bella").count()) === 1);
await screen("alex").getByRole("button", { name: "End call" }).click();
await gone("alex"); await gone("bella");

console.log("Group video call, simultaneous joins (offer glare)");
await open("alex", group); await open("bella", group); await open("cara", group);
check("a group chat offers group call buttons", await P("alex").getByRole("button", { name: "Start group video call" }).isVisible());
process.env.FAKE_JOIN_DELAY_MS = "150"; // both joiners then see each other in the participant list, so they offer each other at once
await P("alex").getByRole("button", { name: "Start group video call" }).click();
await incoming("bella").waitFor(); await incoming("cara").waitFor();
check("everyone else in the group rings", /group video call/i.test(await incoming("cara").innerText()));
await Promise.all([incoming("bella").getByRole("button", { name: "Accept call" }).click(), incoming("cara").getByRole("button", { name: "Accept call" }).click()]);
await Promise.all([connected("alex"), connected("bella"), connected("cara")]);
await Promise.all([remotesWithVideo("alex", 2), remotesWithVideo("bella", 2), remotesWithVideo("cara", 2)]);
check("all three see two remote videos each (full mesh, glare resolved)", true);
check("the header shows 3 people", /3/.test(await P("alex").locator(".call-count").innerText()));
process.env.FAKE_JOIN_DELAY_MS = "0";
await screen("bella").getByRole("button", { name: "End call" }).click();
await gone("bella");
await waitFor(async () => (await remoteTiles("alex")) === 1 && (await remoteTiles("cara")) === 1);
check("when one person leaves, the call continues for the others", (await remoteTiles("alex")) === 1 && (await remoteTiles("cara")) === 1 && (await screen("alex").count()) === 1);
check("and Bella's chat now offers to rejoin", await P("bella").getByText(/Video call in progress/).isVisible());
await P("bella").getByRole("button", { name: "Join", exact: true }).click();
await connected("bella");
await Promise.all([remotesWithVideo("alex", 2), remotesWithVideo("bella", 2), remotesWithVideo("cara", 2)]);
check("rejoining through the banner rebuilds the mesh", true);
for (const u of ["alex", "bella", "cara"]) await screen(u).getByRole("button", { name: "End call" }).click();
await Promise.all([gone("alex"), gone("bella"), gone("cara")]);
check("the group call ends when the last person leaves", await waitFor(async () => (await history(group))[0]?.status === "ended"));

console.log("Group voice call: decline one ring, then join from the banner");
await P("alex").getByRole("button", { name: "Start group voice call" }).click();
await incoming("bella").waitFor(); await incoming("cara").waitFor();
await incoming("bella").getByRole("button", { name: "Accept call" }).click();
await connected("alex");
await incoming("cara").getByRole("button", { name: "Decline call" }).click();
await incoming("cara").waitFor({ state: "detached" });
check("declining a group ring does not end the call for the others", (await screen("alex").count()) === 1 && (await screen("bella").count()) === 1);
check("Cara can still join from the chat banner", await P("cara").getByText(/Voice call in progress/).isVisible());
check("a voice banner has no audio-only variant", (await P("cara").getByRole("button", { name: "Join with audio only" }).count()) === 0);
await P("cara").getByRole("button", { name: "Join", exact: true }).click();
await connected("cara");
await waitFor(async () => (await remoteTiles("cara")) === 2 && (await remoteTiles("alex")) === 2);
check("a late joiner is connected to everybody", (await remoteTiles("cara")) === 2 && (await remoteTiles("alex")) === 2);
await waitFor(async () => (await inboundBytes("cara", "audio")) > 0);
check("the late joiner hears the others", (await inboundBytes("cara", "audio")) > 0);
for (const u of ["alex", "bella", "cara"]) await screen(u).getByRole("button", { name: "End call" }).click();
await Promise.all([gone("alex"), gone("bella"), gone("cara")]);

console.log("Closing the tab drops the user from the call");
await open("alex", dmAB); await open("bella", dmAB);
await P("alex").getByRole("button", { name: "Voice call" }).click();
await incoming("bella").getByRole("button", { name: "Accept call" }).click();
await connected("alex"); await connected("bella");
await U.bella.page.close();
await gone("alex");
check("if one side disappears the other's call ends", true);
check("no live calls are left behind", [...callsDb.values()].every((c) => c.status !== "ringing" && c.status !== "active"));

console.log("Quality");
for (const [u, x] of Object.entries(U)) check(`${u}: no console errors`, x.errors.length === 0, x.errors.join(" | "));
await finish();
