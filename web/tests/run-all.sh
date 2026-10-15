#!/usr/bin/env bash
# Builds, typechecks, serves dist/ and runs every browser test. Requires Playwright + Chromium (see README).
set -e
cd "$(dirname "$0")/.."
node_modules/.bin/tsc --noEmit
node scripts/build.mjs >/dev/null
PORT=${PORT:-4173}
node scripts/serve.mjs & SERVER=$!
trap "kill $SERVER 2>/dev/null" EXIT
sleep 1
for t in e2e-auth e2e-live-auth e2e-live-social e2e-live-chat e2e-live-attachments e2e-live-groups e2e-live-verify e2e-live-account e2e-live-settings e2e-live-avatar e2e-live-calls e2e-phone e2e-voice e2e-chat e2e-friends e2e-search-notif e2e-settings e2e-mobile audit; do
  echo "=== $t"; BASE_URL=http://localhost:$PORT node tests/$t.mjs | tail -3
done
