// Shared Playwright helpers. Playwright is NOT a project dependency (npm was unavailable when this was
// written); it is resolved from a global install: NODE_PATH or /opt/npm-tools/node_modules.
import { createRequire } from "node:module";
const require = createRequire(process.env.PW_ROOT || "/opt/npm-tools/node_modules/");
export const { chromium } = require("playwright");
export const BASE = process.env.BASE_URL || "http://localhost:4173";

export async function launch(viewport = { width: 1280, height: 860 }) {
  const browser = await chromium.launch({ executablePath: "/opt/pw-browsers/chromium", args: ["--no-sandbox"] }).catch(() => chromium.launch({ args: ["--no-sandbox"] }));
  const ctx = await browser.newContext({ viewport });
  // Google Fonts are unreachable from the sandbox; abort quickly instead of hanging.
  await ctx.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort());
  const page = await ctx.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => m.type() === "error" && !/Failed to load resource/.test(m.text()) && errors.push(m.text()));
  return { browser, ctx, page, errors };
}

let passed = 0, failed = 0;
export function check(name, cond, extra = "") {
  if (cond) { passed++; console.log(`  ✓ ${name}`); } else { failed++; console.log(`  ✗ ${name} ${extra}`); }
}
export function summary() {
  console.log(`\n${passed} passed, ${failed} failed`);
  process.exit(failed ? 1 : 0);
}

export async function loginDemo(page, user = "demo", pw = "Demo1234") {
  await page.goto(`${BASE}/login`);
  await page.getByLabel("Email or username").fill(user);
  await page.getByLabel("Password", { exact: true }).fill(pw);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(`${BASE}/`);
}
