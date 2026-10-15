// Voice-note recorder (MediaRecorder) in mock mode, using Chromium's fake microphone.
import { chromium, check, summary, BASE, loginDemo } from "./helpers.mjs";
const browser = await chromium.launch({ executablePath: "/opt/pw-browsers/chromium", args: ["--no-sandbox", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 } });
// The sandbox has no audio device (even Chromium's fake one is refused), so feed MediaRecorder a synthetic WebAudio stream:
// everything after getUserMedia (recording, encoding, upload, playback element) is the real code path.
await ctx.addInitScript(() => {
  navigator.mediaDevices.getUserMedia = async () => {
    const ac = new AudioContext(); const dst = ac.createMediaStreamDestination(); const osc = ac.createOscillator(); osc.connect(dst); osc.start();
    return dst.stream;
  };
});
await ctx.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort());
const page = await ctx.newPage();
const errors = [];
page.on("pageerror", (e) => errors.push(e.message));
page.on("console", (m) => m.type() === "error" && !/Failed to load resource/.test(m.text()) && errors.push(m.text()));
await loginDemo(page);
await page.goto(`${BASE}/chats`);
await page.locator(".conv-item").first().click();
await page.getByLabel("Message", { exact: true }).waitFor();

console.log("Record");
const mic = page.getByRole("button", { name: "Record voice message" });
check("mic button shows when the box is empty", await mic.isVisible());
await mic.click();
await page.getByRole("status", { name: "Recording voice message" }).waitFor();
check("recording indicator + timer", /Recording 0:0\d/.test(await page.getByRole("status", { name: "Recording voice message" }).innerText()));
await page.waitForTimeout(1300);
await page.getByRole("button", { name: "Finish recording" }).click();
await page.locator(".pending", { hasText: /voice-.*\.(webm|ogg|m4a)/ }).waitFor({ timeout: 6000 });
await page.waitForFunction(() => !document.querySelector(".pending .spinner"), null, { timeout: 8000 });
check("the recording is queued as an attachment and uploads", (await page.locator(".pending.err").count()) === 0);
await page.getByRole("button", { name: "Send message" }).click();
await page.locator(".bubble.out audio[controls]").waitFor({ timeout: 6000 });
check("it is sent as an audio message with a player", true);

console.log("Cancel");
await page.getByRole("button", { name: "Record voice message" }).click();
await page.getByRole("status", { name: "Recording voice message" }).waitFor();
await page.getByRole("button", { name: "Cancel recording" }).click();
await page.waitForTimeout(400);
check("cancelling leaves nothing queued", (await page.locator(".pending").count()) === 0 && (await page.getByRole("status", { name: "Recording voice message" }).count()) === 0);
await page.getByLabel("Message", { exact: true }).fill("typing hides the mic");
check("the mic button hides once text is typed", (await page.getByRole("button", { name: "Record voice message" }).count()) === 0);

console.log("Denied microphone");
const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 860 } });
await ctx2.addInitScript(() => { navigator.mediaDevices.getUserMedia = () => Promise.reject(new DOMException("denied", "NotAllowedError")); });
await ctx2.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort());
const p2 = await ctx2.newPage();
await loginDemo(p2);
await p2.goto(`${BASE}/chats`);
await p2.locator(".conv-item").first().click();
await p2.getByRole("button", { name: "Record voice message" }).click();
await p2.getByText(/Couldn't use the microphone/).waitFor({ timeout: 4000 });
check("a denied microphone shows a clear message and nothing is stuck", (await p2.getByRole("status", { name: "Recording voice message" }).count()) === 0);
check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await browser.close();
summary();
