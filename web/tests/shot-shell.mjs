import { BASE, launch } from "./helpers.mjs";
const out = process.env.SHOTS || "/tmp";
for (const [name, vp] of [["desktop", { width: 1440, height: 860 }], ["tablet", { width: 900, height: 800 }], ["mobile", { width: 390, height: 800 }]]) {
  const { browser, page, errors } = await launch(vp);
  await page.goto(`${BASE}/login`);
  await page.getByLabel("Email or username").fill("demo");
  await page.getByLabel("Password", { exact: true }).fill("Demo1234");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByText("Recent conversations").waitFor();
  await page.getByText("Aisha Rahman").first().waitFor();
  await page.waitForTimeout(300);
  await page.screenshot({ path: `${out}/home-${name}.png` });
  console.log(name, errors.length ? errors : "ok");
  await browser.close();
}
