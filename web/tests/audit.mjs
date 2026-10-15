// Responsive + accessibility smoke audit across every route and 3 breakpoints.
import { BASE, check, launch, loginDemo, summary } from "./helpers.mjs";
const S = process.env.SHOTS;
const VIEWPORTS = { mobile: { width: 360, height: 740 }, tablet: { width: 820, height: 900 }, desktop: { width: 1440, height: 900 } };
const ROUTES = ["/", "/chats", "/chats/c_u_aisha", "/friends", "/search?q=ra", "/notifications", "/profile", "/profile/u_rafi", "/settings", "/settings/privacy", "/settings/security"];

for (const [vpName, vp] of Object.entries(VIEWPORTS)) {
  console.log(`\n== ${vpName} ${vp.width}x${vp.height}`);
  const { browser, page, errors } = await launch(vp);
  await loginDemo(page);
  for (const r of ROUTES) {
    await page.goto(`${BASE}${r}`);
    await page.waitForTimeout(700);
    const res = await page.evaluate(() => {
      const out = { overflow: document.documentElement.scrollWidth > window.innerWidth + 1, unnamed: [], noAlt: 0, dupIds: [], h1: document.querySelectorAll("h1").length, tiny: [] };
      const name = (el) => (el.getAttribute("aria-label") || el.getAttribute("aria-labelledby") || el.getAttribute("title") || el.textContent || "").trim() || el.querySelector("img[alt]:not([alt=''])") || (el.labels && el.labels.length);
      document.querySelectorAll("button, a[href], input:not([type=hidden]), select, textarea").forEach((el) => {
        if (el.closest("[hidden]") || el.getAttribute("aria-hidden") === "true") return;
        if (!name(el)) out.unnamed.push(el.outerHTML.slice(0, 90));
        const r = el.getBoundingClientRect();
        if (r.width && r.height && (r.width < 24 || r.height < 24) && el.type !== "checkbox" && !el.closest(".switch") && !el.closest(".check")) out.tiny.push(`${el.tagName}.${el.className}`.slice(0, 50) + ` ${Math.round(r.width)}x${Math.round(r.height)}`);
      });
      document.querySelectorAll("img:not([alt])").forEach(() => out.noAlt++);
      const ids = {};
      document.querySelectorAll("[id]").forEach((e) => (ids[e.id] = (ids[e.id] || 0) + 1));
      out.dupIds = Object.keys(ids).filter((k) => ids[k] > 1);
      return out;
    });
    const ok = !res.overflow && res.unnamed.length === 0 && res.noAlt === 0 && res.dupIds.length === 0 && res.h1 >= 1;
    check(`${r}`, ok, JSON.stringify(res));
    if (res.tiny.length) console.log(`     (small targets: ${[...new Set(res.tiny)].slice(0, 4).join("; ")})`);
    if (S && r !== "/search?q=ra") await page.screenshot({ path: `${S}/audit-${vpName}-${r.replace(/[^a-z0-9]+/gi, "_")}.png` });
  }
  check(`${vpName}: no console/page errors`, errors.length === 0, JSON.stringify(errors));
  await browser.close();
}
summary();
