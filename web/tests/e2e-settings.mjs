import { BASE, check, launch, loginDemo, summary } from "./helpers.mjs";
const S = process.env.SHOTS;
const { browser, page, errors } = await launch({ width: 1280, height: 860 });
const nav = () => page.getByRole("navigation", { name: "Settings sections" });
await loginDemo(page);

console.log("Settings navigation");
await page.goto(`${BASE}/settings`);
await page.waitForURL(/\/settings\/account/);
check("desktop /settings → account", true);
for (const t of ["Privacy", "Notifications", "Appearance", "Chat", "Security", "Other", "Account"]) {
  await nav().getByRole("link", { name: t, exact: true }).click();
  await page.getByRole("heading", { name: t, level: 2 }).waitFor();
}
check("all 7 sections reachable", true);

console.log("Account: email");
await page.getByRole("textbox", { name: "Email" }).fill("nope");
await page.getByRole("button", { name: "Save email" }).click();
check("invalid email rejected", await page.getByText("Enter a valid email address").isVisible());
await page.getByRole("textbox", { name: "Email" }).fill("demo2@kikbite.com");
await page.getByRole("button", { name: "Save email" }).click();
await page.getByText("Email updated").waitFor();
check("email saved", true);

console.log("Privacy toggles persist");
await nav().getByRole("link", { name: "Privacy", exact: true }).click();
const rr = page.getByRole("switch", { name: "Read receipts" });
check("read receipts default on", await rr.isChecked());
await rr.click({ force: true });
await page.getByLabel("Last seen & online").selectOption("friends");
await page.reload();
await page.getByRole("switch", { name: "Read receipts" }).waitFor();
check("toggle persisted after reload", !(await page.getByRole("switch", { name: "Read receipts" }).isChecked()));
check("select persisted after reload", (await page.getByLabel("Last seen & online").inputValue()) === "friends");

console.log("Appearance applies live");
await nav().getByRole("link", { name: "Appearance", exact: true }).click();
await page.getByLabel("Text size").selectOption("120");
check("font scale applied to <html>", (await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--font-scale").trim())) === "1.2");
await page.getByLabel("Density").selectOption("compact");
check("density attribute", (await page.evaluate(() => document.documentElement.dataset.density)) === "compact");
await page.getByRole("switch", { name: "Reduce motion" }).click({ force: true });
check("reduce motion attribute", (await page.evaluate(() => document.documentElement.dataset.reduceMotion)) === "true");
if (S) await page.screenshot({ path: `${S}/settings-appearance.png` });
await page.getByLabel("Text size").selectOption("100");

console.log("Chat setting changes behaviour: Enter sends off");
await nav().getByRole("link", { name: "Chat", exact: true }).click();
await page.getByRole("switch", { name: "Enter sends message" }).click({ force: true });
await page.goto(`${BASE}/chats/c_u_nabila`);
const box = page.getByLabel("Message", { exact: true });
await box.fill("no-send-on-enter");
await box.press("Enter");
await page.waitForTimeout(500);
check("Enter no longer sends", (await page.locator(".bubble.out", { hasText: "no-send-on-enter" }).count()) === 0);
await box.press("Control+Enter");
await page.locator(".bubble.out", { hasText: "no-send-on-enter" }).waitFor();
check("Ctrl+Enter sends", true);

console.log("Security: change password");
await page.goto(`${BASE}/settings/security`);
await page.getByLabel("Current password").fill("wrong");
await page.getByLabel("New password", { exact: true }).fill("Newpass123");
await page.getByLabel("Confirm new password").fill("Newpass123");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText("Current password is incorrect").first().waitFor();
check("wrong current password rejected inline", true);
await page.getByLabel("Current password").fill("Demo1234");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText("Password changed").waitFor();
check("password changed", true);

console.log("Other: delete account");
await page.goto(`${BASE}/settings/other`);
await page.getByRole("button", { name: "Delete…" }).click();
const del = page.getByRole("button", { name: "Delete forever" });
check("delete disabled until confirmed", await del.isDisabled());
await page.getByLabel("Type your username").fill("demo");
await page.getByLabel("Password", { exact: true }).fill("Newpass123");
await del.click();
await page.waitForURL(/\/login/);
check("account deleted → login", true);
await page.getByLabel("Email or username").fill("demo");
await page.getByLabel("Password", { exact: true }).fill("Newpass123");
await page.getByRole("button", { name: "Sign in" }).click();
await page.getByText("Incorrect email/username or password.").waitFor();
check("deleted account cannot sign in", true);

console.log("Mobile settings");
await page.setViewportSize({ width: 390, height: 800 });
await loginDemo(page, "testperson", "Str0ng!Passw0rd").catch(() => {});
check("no console/page errors", errors.length === 0, JSON.stringify(errors));
await browser.close();
summary();
