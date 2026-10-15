import { BASE, check, launch, summary } from "./helpers.mjs";

const { browser, page, errors } = await launch();
const S = process.env.SHOTS;
const shot = async (n) => S && page.screenshot({ path: `${S}/${n}.png` });

console.log("Auth guard");
await page.goto(`${BASE}/`);
await page.waitForURL(/\/login/);
check("unauthenticated / redirects to /login", page.url().includes("/login"));

console.log("Login validation");
await page.getByRole("button", { name: "Sign in" }).click();
check("empty submit shows identifier error", await page.getByText("Enter your email or username").isVisible());
check("empty submit shows password error", await page.getByText("Password is required").isVisible());
await page.getByLabel("Email or username").fill("not an email@");
await page.getByLabel("Password", { exact: true }).fill("x");
await page.getByRole("button", { name: "Sign in" }).click();
check("invalid identifier flagged", await page.getByText(/valid email|Usernames are/).first().isVisible());
await shot("login-errors");

console.log("Wrong password");
await page.getByLabel("Email or username").fill("demo");
await page.getByLabel("Password", { exact: true }).fill("wrongpass1");
await page.getByRole("button", { name: "Sign in" }).click();
await page.getByText("Incorrect email/username or password.").waitFor();
check("generic credential error", true);

console.log("Password show/hide");
const pw = page.getByLabel("Password", { exact: true });
check("password hidden by default", (await pw.getAttribute("type")) === "password");
await page.getByRole("button", { name: "Show password" }).click();
check("toggle reveals password", (await pw.getAttribute("type")) === "text");

console.log("Successful login");
await pw.fill("Demo1234");
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(`${BASE}/`);
await page.getByRole("heading", { name: /(Good|Still up).*Demo/ }).waitFor();
check("lands on home after login", true);
await page.reload();
await page.getByRole("heading", { name: /(Good|Still up).*Demo/ }).waitFor();
check("session persists on reload (remember me)", true);
await page.goto(`${BASE}/login`);
await page.waitForURL(`${BASE}/`);
check("guest routes redirect away when signed in", true);

console.log("Logout");
await page.getByRole("button", { name: "Account menu" }).click();
await page.getByRole("menuitem", { name: "Sign out" }).click();
await page.waitForURL(/\/login/);
check("logout returns to login", true);

console.log("401 handling (expired session)");
await page.evaluate(() => {
  localStorage.setItem("kikbite.session", JSON.stringify({ user: { id: "u_demo", username: "demo", displayName: "Demo User", presence: "online", createdAt: "2026-01-01T00:00:00Z" }, accessToken: "x", refreshToken: "y", expiresAt: Date.now() - 1000, remember: true }));
});
await page.goto(`${BASE}/`);
await page.waitForURL(/\/login/);
check("expired session redirects to login", true);

console.log("Register");
await page.getByRole("link", { name: "Create an account" }).click();
await page.waitForURL(/\/register/);
await page.getByRole("button", { name: "Create account" }).click();
check("register shows required errors", await page.getByText("Name is required", { exact: true }).isVisible() && await page.getByText("Username is required").isVisible());
await page.getByLabel("Full name").fill("Test Person");
await page.getByLabel("Username").fill("demo");
await page.getByLabel("Email").fill("test@example.com");
await page.getByLabel("Password", { exact: true }).fill("abc");
await page.getByLabel("Confirm password").fill("abd");
await page.getByRole("button", { name: "Create account" }).click();
check("weak password rejected", await page.getByText("Use at least 8 characters").isVisible());
check("mismatch flagged", await page.getByText("Passwords do not match").isVisible());
await page.getByLabel("Password", { exact: true }).fill("Str0ng!Passw0rd");
check("strength meter shows", await page.getByText(/Password strength: (Good|Strong)/).isVisible());
await page.getByLabel("Confirm password").fill("Str0ng!Passw0rd");
await page.getByRole("button", { name: "Create account" }).click();
await page.getByText("That username is taken").waitFor();
check("server-side duplicate username error shown on field", true);
await shot("register-errors");
await page.getByLabel("Username").fill("testperson");
await page.getByRole("button", { name: "Create account" }).click();
await page.waitForURL(`${BASE}/`);
await page.getByRole("heading", { name: /(Good|Still up).*Test/ }).waitFor();
check("register signs in and lands home", true);
await page.getByRole("button", { name: "Account menu" }).click();
await page.getByRole("menuitem", { name: "Sign out" }).click();
await page.waitForURL(/\/login/);

console.log("Forgot / reset password");
await page.getByRole("link", { name: "Forgot password?" }).click();
await page.getByLabel("Email").fill("bad");
await page.getByRole("button", { name: "Send reset link" }).click();
check("forgot validates email", await page.getByText("Enter a valid email address").isVisible());
await page.getByLabel("Email").fill("test@example.com");
await page.getByRole("button", { name: "Send reset link" }).click();
await page.getByRole("heading", { name: "Check your email" }).waitFor();
check("forgot shows neutral confirmation", true);
await shot("forgot-sent");
await page.getByRole("link", { name: "open reset link" }).click();
await page.getByRole("heading", { name: "Set a new password" }).waitFor();
await page.getByLabel("New password", { exact: true }).fill("NewPassw0rd!");
await page.getByLabel("Confirm new password").fill("NewPassw0rd!");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByRole("heading", { name: "Password updated" }).waitFor();
check("reset succeeds", true);
await page.getByRole("button", { name: "Go to sign in" }).click();
await page.getByLabel("Email or username").fill("test@example.com");
await page.getByLabel("Password", { exact: true }).fill("NewPassw0rd!");
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(`${BASE}/`);
check("login with new password + email identifier works", true);
await page.evaluate(() => localStorage.clear());
await page.goto(`${BASE}/reset-password?token=bogus`);
await page.getByLabel("New password", { exact: true }).fill("NewPassw0rd!");
await page.getByLabel("Confirm new password").fill("NewPassw0rd!");
await page.getByRole("button", { name: "Update password" }).click();
await page.getByText(/invalid or has expired/).waitFor();
check("invalid token handled", true);

check("no uncaught console/page errors", errors.length === 0, JSON.stringify(errors));
await browser.close();
summary();
