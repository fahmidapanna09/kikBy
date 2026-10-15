# KikBite Android app (APK)

The app is a native Android shell (Capacitor) that opens your **deployed HTTPS site**. All features work
the same as on the web, including voice / video / group calls. A web release reaches the app without
rebuilding it. Only changing the site address needs a new APK.

## Requirements
- The site must already be online on **HTTPS** (Railway or any host). Camera and microphone are blocked on http.
- Node 20+ and either Android Studio (recent, includes JDK 21) or the GitHub Actions route below.

## One-time setup (in `web/`)
```powershell
npm install @capacitor/core@7 @capacitor/android@7
npm install -D @capacitor/cli@7
npm run app:init
```
`app:init` builds the web app, creates the `web/android/` project, adds the camera / microphone permissions
and installs the KikBite icons.

## Build the APK
```powershell
$env:KIKBITE_URL="https://app.your-domain.com"   # your real site address
npm run app:sync
npm run app:apk
```
The file is `web/android/app/build/outputs/apk/debug/app-debug.apk`. Send it to friends (WhatsApp, Drive).
On their phone: open the file and allow "Install unknown apps" for that app when Android asks.

Prefer the IDE? `npm run app:open` opens the project in Android Studio -> Build -> Build APK(s).

## Without Android Studio: GitHub Actions
1. Push the repo to GitHub.
2. Repo -> Settings -> Secrets and variables -> Actions -> **Variables** -> New variable `KIKBITE_URL` = your site address.
3. Actions tab -> "Android APK" -> Run workflow. Download `kikbite-debug-apk` from the run page.

## Notes
- `app-debug.apk` is a test build (fine for a survey). A Play Store release needs a signed release build.
- iPhone is not covered: it needs a Mac, Xcode and an Apple developer account.
- The site must allow the app: set `APP_BASE_URL` (and `WS_ALLOWED_ORIGINS` if you use another host) on the API.
- Alerts while the app is closed (push for messages / incoming calls) are not built yet.
