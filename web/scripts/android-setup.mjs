// Helper for the Capacitor Android shell. Run through npm scripts (see MOBILE.md):
//   node scripts/android-setup.mjs patch   -> after `npx cap add android`: permissions + KikBite icons
//   node scripts/android-setup.mjs sync    -> checks KIKBITE_URL, runs `npx cap sync android`, then patches again
// Everything here is idempotent: running it twice changes nothing the second time.
import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const androidDir = join(root, "android");
const resDir = join(androidDir, "app", "src", "main", "res");
const manifestPath = join(androidDir, "app", "src", "main", "AndroidManifest.xml");
const brand = "#4f46e5";

function fail(msg) {
  console.error(`\n✖ ${msg}\n`);
  process.exit(1);
}

function requireAndroidProject() {
  if (!existsSync(manifestPath)) fail("android/ project not found. Run `npm run app:init` first (it runs `npx cap add android`).");
}

/** Camera, microphone and network-state permissions for WebRTC calls (INTERNET is already in Capacitor's template). */
export function patchManifest(xml) {
  const lines = [];
  const need = (tag, name, line) => {
    if (!new RegExp(`<${tag}[^>]*android:name="${name.replace(/\./g, "\\.")}"`).test(xml)) lines.push(`    ${line}`);
  };
  need("uses-permission", "android.permission.CAMERA", '<uses-permission android:name="android.permission.CAMERA" />');
  need("uses-permission", "android.permission.RECORD_AUDIO", '<uses-permission android:name="android.permission.RECORD_AUDIO" />');
  need("uses-permission", "android.permission.MODIFY_AUDIO_SETTINGS", '<uses-permission android:name="android.permission.MODIFY_AUDIO_SETTINGS" />');
  need("uses-permission", "android.permission.ACCESS_NETWORK_STATE", '<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />');
  need("uses-permission", "android.permission.POST_NOTIFICATIONS", '<uses-permission android:name="android.permission.POST_NOTIFICATIONS" />');
  need("uses-permission", "android.permission.VIBRATE", '<uses-permission android:name="android.permission.VIBRATE" />');
  need("uses-feature", "android.hardware.camera", '<uses-feature android:name="android.hardware.camera" android:required="false" />');
  need("uses-feature", "android.hardware.microphone", '<uses-feature android:name="android.hardware.microphone" android:required="false" />');
  if (!lines.length) return xml;
  if (!xml.includes("</manifest>")) throw new Error("AndroidManifest.xml has no </manifest> tag");
  return xml.replace("</manifest>", `\n    <!-- KikBite: calls + notifications -->\n${lines.join("\n")}\n</manifest>`);
}

function copyDir(src, dst) {
  mkdirSync(dst, { recursive: true });
  for (const name of readdirSync(src)) {
    const s = join(src, name);
    const d = join(dst, name);
    if (statSync(s).isDirectory()) copyDir(s, d);
    else copyFileSync(s, d);
  }
}

function patchAll() {
  requireAndroidProject();
  const before = readFileSync(manifestPath, "utf8");
  const after = patchManifest(before);
  if (after !== before) {
    writeFileSync(manifestPath, after);
    console.log("✔ AndroidManifest.xml: camera / microphone / notification permissions added");
  } else {
    console.log("✔ AndroidManifest.xml: permissions already present");
  }

  const icons = join(root, "mobile", "icons");
  if (existsSync(icons)) {
    copyDir(icons, resDir);
    console.log("✔ KikBite launcher icons installed");
  }
  mkdirSync(join(resDir, "values"), { recursive: true });
  writeFileSync(
    join(resDir, "values", "ic_launcher_background.xml"),
    `<?xml version="1.0" encoding="utf-8"?>\n<resources>\n    <color name="ic_launcher_background">${brand}</color>\n</resources>\n`,
  );
}

function run(cmd) {
  // shell:true so `npx` resolves on Windows too; the command string is fixed, never user input
  const r = spawnSync(cmd, { cwd: root, stdio: "inherit", shell: true, env: process.env });
  if (r.status !== 0) fail(`Command failed: ${cmd}`);
}

const mode = process.argv[2] || "patch";
if (mode === "patch") {
  patchAll();
} else if (mode === "sync") {
  const raw = (process.env.KIKBITE_URL || "").trim();
  if (!raw) fail('KIKBITE_URL is not set. PowerShell example:\n    $env:KIKBITE_URL="https://app.your-domain.com"\n    npm run app:sync');
  let u;
  try {
    u = new URL(raw);
  } catch {
    fail(`KIKBITE_URL is not a valid URL: ${raw}`);
  }
  if (u.protocol !== "https:" && process.env.ALLOW_HTTP !== "1") {
    fail("KIKBITE_URL must start with https:// (camera and microphone only work on secure pages). For a plain-http test without calls set ALLOW_HTTP=1.");
  }
  if (!existsSync(join(root, "dist", "index.html"))) run("npm run build");
  requireAndroidProject();
  console.log(`App will open: ${u.origin}`);
  run("npx cap sync android");
  patchAll();
  console.log("\n✔ Done. Open Android Studio with `npm run app:open`, or build an APK with `npm run app:apk`.");
} else {
  fail(`Unknown mode "${mode}". Use "patch" or "sync".`);
}
