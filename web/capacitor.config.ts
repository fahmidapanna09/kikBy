// Capacitor wraps the deployed KikBite web app in a native Android shell (an installable APK).
//
// The app loads your HTTPS site (KIKBITE_URL) inside the shell, so the web app, the API proxy and the
// WebSocket stay same-origin (no CORS changes) and every web release reaches the app without a rebuild.
// Set KIKBITE_URL when you run `npm run app:sync` — see MOBILE.md.
declare const process: { env: Record<string, string | undefined> };

const url = (process.env.KIKBITE_URL || "").trim().replace(/\/+$/, "");

const config = {
  appId: "com.kikbite.app",
  appName: "KikBite",
  webDir: "dist",
  backgroundColor: "#070B16",
  server: url
    ? { url, cleartext: url.startsWith("http://"), androidScheme: "https" }
    : { androidScheme: "https" },
  plugins: {
    LocalNotifications: { smallIcon: "ic_stat_kikbite", iconColor: "#4f46e5" },
  },
  android: {
    allowMixedContent: false,
    backgroundColor: "#070B16",
  },
};

export default config;
