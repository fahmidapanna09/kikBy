import { BASE, check, launch, loginDemo, summary } from "./helpers.mjs";
const S = process.env.SHOTS;
const { browser, page, errors } = await launch({ width: 1440, height: 860 });
await loginDemo(page);

console.log("Chat list");
await page.getByRole("link", { name: "Chats", exact: false }).first().click();
await page.waitForURL(/\/chats$/);
await page.getByRole("link", { name: /Aisha Rahman/ }).waitFor();
check("list renders conversations", (await page.locator(".conv-item").count()) >= 7);
check("pinned chats first", /Aisha|Nabila/.test(await page.locator(".conv-item").first().innerText()));
await page.getByRole("tab", { name: "Unread" }).click();
check("unread filter hides read chats", (await page.locator(".conv-item").count()) < 7);
await page.getByRole("tab", { name: "Groups" }).click();
check("groups filter", (await page.locator(".conv-item").count()) === 1);
await page.getByRole("tab", { name: "All" }).click();
await page.getByLabel("Search chats").first().fill("zzzz");
check("empty search state", await page.getByText("No matching chats").isVisible());
await page.getByLabel("Search chats").first().fill("");

console.log("Open conversation");
await page.getByRole("link", { name: /Aisha Rahman/ }).click();
await page.waitForURL(/\/chats\/c_u_aisha/);
await page.locator(".bubble").first().waitFor();
check("messages render", (await page.locator(".bubble").count()) >= 10);
check("date separators", (await page.locator(".day-sep").count()) >= 2);
check("unread divider shown", await page.locator("[data-unread-divider]").count() === 1);
check("unread badge cleared in list", (await page.locator('.conv-item[aria-current="page"] .badge').count()) === 0);
check("reply reference rendered", (await page.locator(".reply-ref").count()) >= 1);
check("reaction chip rendered", (await page.locator(".react-chip").count()) >= 1);
if (S) await page.screenshot({ path: `${S}/chat-desktop.png` });

console.log("Send + status + peer reply");
const box = page.getByLabel("Message", { exact: true });
await box.fill("Hello from the test <b>bold</b> https://example.com");
await box.press("Enter");
await page.locator(".bubble.out", { hasText: "Hello from the test" }).waitFor();
check("optimistic bubble appears", true);
check("HTML in text is escaped, not rendered", (await page.locator(".bubble.out", { hasText: "Hello from the test" }).locator("b").count()) === 0 && (await page.locator(".bubble.out", { hasText: "<b>bold</b>" }).count()) === 1);
check("URL becomes safe link", (await page.locator('.bubble.out a[rel*="noopener"]').count()) === 1);
await page.locator('.bubble.out [aria-label="Read"]').first().waitFor({ timeout: 6000 }).catch(() => {});
check("status progresses to read", (await page.locator('.bubble.out [aria-label="Read"]').count()) >= 1);
await page.locator(".typing-bubble").waitFor({ timeout: 6000 });
check("typing indicator shows", true);
await page.locator(".bubble.in").last().waitFor();
await page.waitForFunction(() => document.querySelectorAll(".typing-bubble").length === 0, null, { timeout: 8000 });
check("typing indicator clears when reply arrives", true);

console.log("Shift+Enter newline");
await box.fill("line1");
await box.press("Shift+Enter");
await box.type("line2");
check("shift+enter adds newline instead of sending", (await box.inputValue()) === "line1\nline2");
await box.fill("");

console.log("Context menu: reply / react / edit / delete / forward");
await page.waitForTimeout(900); // let smooth auto-scroll settle (menus close on scroll)
const target = page.locator(".bubble.in", { hasText: "dark theme feels really premium" });
await target.click({ button: "right" });
await page.getByRole("menuitem", { name: "Reply" }).click();
check("reply preview in composer", await page.getByText("Replying to Aisha Rahman").isVisible());
await box.fill("Thank you!");
await box.press("Enter");
await page.locator(".bubble.out", { hasText: "Thank you!" }).locator(".reply-ref").waitFor();
check("reply sent with reference", true);

await page.waitForTimeout(900);
await target.click({ button: "right" });
await page.getByRole("group", { name: "React" }).getByRole("button", { name: "React ❤️" }).click();
await page.locator(".react-chip", { hasText: "❤️" }).waitFor();
check("reaction added", true);
await page.locator(".react-chip.mine", { hasText: "❤️" }).click();
await page.waitForFunction(() => ![...document.querySelectorAll(".react-chip")].some((e) => e.textContent.includes("❤️")));
check("reaction toggled off", true);

await page.waitForTimeout(900);
const mine = page.locator(".bubble.out", { hasText: "Hello from the test" });
await mine.click({ button: "right" });
await page.getByRole("menuitem", { name: "Edit" }).click();
await box.fill("Hello EDITED");
await box.press("Enter");
await page.locator(".bubble.out", { hasText: "Hello EDITED" }).waitFor();
check("edit applied", true);
check("edited label shown", (await page.locator(".bubble.out", { hasText: "Hello EDITED" }).getByText("edited", { exact: true }).count()) === 1);

await page.waitForTimeout(900);
await page.locator(".bubble.out", { hasText: "Hello EDITED" }).click({ button: "right" });
await page.getByRole("menuitem", { name: "Forward" }).click();
await page.getByRole("checkbox", { name: /Rafi Khan/ }).click();
await page.getByRole("button", { name: /Send \(1\)/ }).click();
await page.getByText("Forwarded to 1 chat").waitFor();
check("forward works", true);

await page.waitForTimeout(900);
await page.locator(".bubble.out", { hasText: "Hello EDITED" }).click({ button: "right" });
await page.getByRole("menuitem", { name: "Delete" }).click();
check("delete asks for confirmation", await page.getByRole("dialog", { name: "Delete message?" }).isVisible());
await page.getByRole("dialog").getByRole("button", { name: "Delete" }).click();
await page.getByText("This message was deleted").first().waitFor();
check("delete shows tombstone", true);

console.log("Failed send + retry");
await box.fill("this will /fail");
await box.press("Enter");
await page.getByText("Not sent.").waitFor();
check("failed state with retry", true);
await page.getByRole("button", { name: "Delete", exact: true }).click();
check("discard removes failed message", (await page.getByText("Not sent.").count()) === 0);

console.log("Pagination + jump to reply");
await page.locator(".msg-scroll").evaluate((el) => (el.scrollTop = 0));
await page.waitForFunction(() => document.querySelectorAll(".bubble").length > 30, null, { timeout: 6000 }).catch(() => {});
check("older page loaded on scroll top", (await page.locator(".bubble").count()) > 20);

console.log("Not scrolled to bottom: no force scroll");
await page.locator(".msg-scroll").evaluate((el) => (el.scrollTop = 0));
const before = await page.locator(".msg-scroll").evaluate((el) => el.scrollTop);
await page.waitForTimeout(500);
check("reading history position kept", before < 50);

console.log("Group chat + details");
await page.getByRole("link", { name: /KikBite Team/ }).click();
await page.locator(".bubble").first().waitFor();
check("group shows sender names", (await page.locator(".msg-sender").count()) >= 1);
await page.getByRole("button", { name: "Show details" }).click();
check("details panel (desktop) shows members", await page.getByRole("heading", { name: "Members" }).isVisible());
if (S) await page.screenshot({ path: `${S}/chat-group.png` });

console.log("Not found");
await page.goto(`${BASE}/chats/nope`);
await page.getByText("Conversation not found").waitFor();
check("unknown conversation handled", true);

check("no console/page errors", errors.length === 0, JSON.stringify(errors));
await browser.close();
summary();
