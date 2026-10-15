/** A tiny WebAudio ringtone (no audio assets). Browsers may keep it silent until the user has interacted with the page. */
let ctx: AudioContext | null = null;
let timer: ReturnType<typeof setInterval> | null = null;

function beep(freq: number, at: number, len: number, gain: number) {
  if (!ctx) return;
  const osc = ctx.createOscillator();
  const g = ctx.createGain();
  osc.frequency.value = freq;
  g.gain.setValueAtTime(0, ctx.currentTime + at);
  g.gain.linearRampToValueAtTime(gain, ctx.currentTime + at + 0.02);
  g.gain.linearRampToValueAtTime(0, ctx.currentTime + at + len);
  osc.connect(g).connect(ctx.destination);
  osc.start(ctx.currentTime + at);
  osc.stop(ctx.currentTime + at + len + 0.05);
}

export function startRingtone(kind: "incoming" | "outgoing") {
  stopRingtone();
  try {
    const AC = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) return;
    ctx = new AC();
    void ctx.resume().catch(() => undefined);
    const pattern = () => {
      if (kind === "incoming") {
        beep(880, 0, 0.3, 0.12);
        beep(660, 0.4, 0.3, 0.12);
      } else {
        beep(440, 0, 0.9, 0.06);
      }
    };
    pattern();
    timer = setInterval(pattern, kind === "incoming" ? 2500 : 3000);
  } catch {
    /* audio is a nicety, never a failure */
  }
}

export function stopRingtone() {
  if (timer) clearInterval(timer);
  timer = null;
  void ctx?.close().catch(() => undefined);
  ctx = null;
}
