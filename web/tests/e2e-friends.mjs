import { BASE, check, launch, loginDemo, summary } from "./helpers.mjs";
const S = process.env.SHOTS;
const { browser, page, errors } = await launch({ width: 1280, height: 860 });
await loginDemo(page);

console.log("Friends");
await page.goto(`${BASE}/friends`);
await page.getByText("Aisha Rahman").first().waitFor();
check("friends tab lists friends", (await page.locator(".user-row").count()) === 6);
await page.getByLabel("Filter friends").fill("rafi");
check("filter by username", (await page.locator(".user-row").count()) === 1);
await page.getByLabel("Filter friends").fill("zzz");
check("no-match state", await page.getByText("No friends match").isVisible());
await page.getByLabel("Filter friends").fill("");

await page.getByRole("tab", { name: /Requests/ }).click();
check("incoming + outgoing sections", await page.getByText(/Received · 2/).isVisible() && await page.getByText(/Sent · 1/).isVisible());
await page.getByRole("button", { name: "Accept" }).first().click();
await page.getByText(/are now friends/).waitFor();
check("accept moves to friends + toast", await page.getByText(/Received · 1/).isVisible());
await page.getByRole("button", { name: "Decline" }).first().click();
await page.getByText("Request declined").waitFor();
check("decline removes request", await page.getByText(/Received/).count() === 0);
await page.getByRole("button", { name: "Cancel request" }).click();
await page.getByText("No pending requests").waitFor();
check("cancel outgoing → empty state", true);

await page.getByRole("tab", { name: "Blocked" }).click();
check("blocked list", await page.getByText("Imran Hossain").isVisible());
await page.getByRole("button", { name: "Unblock" }).click();
await page.getByText("No blocked users").waitFor();
check("unblock works", true);

await page.getByRole("tab", { name: "Friends" }).click();
await page.getByRole("button", { name: "Remove Nabila Ahmed" }).click();
check("remove asks confirmation", await page.getByRole("dialog", { name: "Remove friend?" }).isVisible());
await page.getByRole("dialog").getByRole("button", { name: "Remove" }).click();
await page.getByText(/removed from friends/).waitFor();
check("friend removed", (await page.locator(".user-row", { hasText: "Nabila Ahmed" }).count()) === 0);
if (S) await page.screenshot({ path: `${S}/friends.png` });

console.log("Profile (others)");
await page.goto(`${BASE}/profile/u_aisha`);
await page.getByRole("heading", { name: "Aisha Rahman" }).waitFor();
check("shows handle + status", await page.getByText(/@aisha/).isVisible());
await page.getByRole("button", { name: "Message", exact: false }).first().click();
await page.waitForURL(/\/chats\/c_u_aisha/);
check("message button opens direct chat", true);
await page.goto(`${BASE}/profile/u_zubair`);
await page.getByRole("button", { name: "Add friend" }).click();
await page.getByRole("button", { name: "Cancel request" }).waitFor();
check("add friend → pending", true);
await page.getByRole("button", { name: /Block/ }).first().click();
await page.getByRole("button", { name: "Unblock" }).waitFor();
check("block from profile", true);
await page.goto(`${BASE}/profile/nobody`);
await page.getByText("Profile not found").waitFor();
check("404 profile", true);

console.log("Profile (me) edit");
await page.goto(`${BASE}/profile`);
await page.getByRole("button", { name: "Edit profile" }).click();
await page.getByLabel("Display name").fill("");
await page.getByLabel("Username").fill("ab");
await page.getByRole("button", { name: "Save" }).click();
check("validation errors inline", await page.getByText("Name is required").isVisible() && await page.getByText(/3–24 characters/).isVisible());
await page.getByLabel("Display name").fill("Demo Renamed");
await page.getByLabel("Username").fill("demo");
await page.getByLabel("Bio").fill("Hello <script>alert(1)</script>");
await page.getByRole("button", { name: "Save" }).click();
await page.getByText("Profile updated").waitFor();
check("profile saved", true);
check("name updated in profile", await page.getByRole("heading", { name: "Demo Renamed" }).isVisible());
check("bio shown as text (no script injection)", await page.getByText("<script>alert(1)</script>", { exact: false }).count() >= 1);
if (S) await page.screenshot({ path: `${S}/profile.png` });

check("no console/page errors", errors.length === 0, JSON.stringify(errors));
await browser.close();
summary();
