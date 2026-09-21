#!/usr/bin/env bash
# Retakes the screenshots of docs/screenshots, so that they all come from the same version and the same data.
#
#   make build && docs/screenshots/capture.sh
#
# Starts dist/go-uptime with a temporary SQLite, registers endpoints, a status page and push keys through the administration
# API (9 endpoints in all, so that the dashboard is exactly three rows), lets the history fill for HISTORY_SECONDS
# (default 210) and captures every screen at 1280x900 with agent-browser. The endpoints check real hosts every 5 seconds
# while it runs. Requires agent-browser with Chrome and curl.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
OUT="$ROOT/docs/screenshots"
WORK=$(mktemp -d)
PORT=${PORT:-18093}
BASE="http://127.0.0.1:$PORT"
USERNAME=admin
PASSWORD='screenshots-password'
HISTORY_SECONDS=${HISTORY_SECONDS:-210}
PUSH_TOKEN=keSDu7G855jvVat1xWiY2Gk4CkL1End5
SERVER_PID=""
PUSHER_PID=""

browser() { agent-browser --session go-uptime-screenshots "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  [ -n "$PUSHER_PID" ] && kill "$PUSHER_PID" >/dev/null 2>&1 || true
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

[ -x "$ROOT/dist/go-uptime" ] || { echo "dist/go-uptime not found: run make build"; exit 1; }
VERSION=$("$ROOT/dist/go-uptime" version | awk '{print $2}')
HASH=$(printf '%s\n' "$PASSWORD" | "$ROOT/dist/go-uptime" password hash)

endpoint() { # group name url [extra condition]
  printf '  - name: %s\n    group: %s\n    url: "%s"\n    interval: 5s\n    conditions:\n      - "[STATUS] == 200"\n      - "[RESPONSE_TIME] < 2000"\n' "$2" "$1" "$3"
  [ -n "${4:-}" ] && printf '      - "%s"\n' "$4"
  return 0
}
{
  cat <<CONFIG
web:
  address: 127.0.0.1
  port: $PORT
storage:
  type: sqlite
  path: "$WORK/data.db"
security:
  basic:
    username: $USERNAME
    password-bcrypt-base64: "$HASH"
admin:
  enabled: true
status-pages:
  enabled: true
ui:
  logo: /logo-192x192.png
endpoints:
CONFIG
  endpoint apis api-gateway https://api.cloudflare.com/client/v4/ips
  endpoint apis search https://duckduckgo.com "[CERTIFICATE_EXPIRATION] > 48h"
  endpoint sites website https://example.org "[CERTIFICATE_EXPIRATION] > 48h"
  endpoint sites docs https://www.wikipedia.org "[CERTIFICATE_EXPIRATION] > 48h"
  cat <<CONFIG
  - name: legacy
    group: apis
    url: "http://127.0.0.1:9"
    interval: 5s
    conditions:
      - "[STATUS] == 200"
  - name: dns
    group: infra
    url: "1.1.1.1"
    interval: 5s
    dns:
      query-name: "example.org"
      query-type: "A"
    conditions:
      - "[DNS_RCODE] == NOERROR"
  - name: gateway-ping
    group: infra
    url: "tcp://1.1.1.1:443"
    interval: 5s
    conditions:
      - "[CONNECTED] == true"
CONFIG
} > "$WORK/config.yaml"

echo "==> Starting dist/go-uptime $VERSION"
"$ROOT/dist/go-uptime" --config "$WORK/config.yaml" > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do curl -sf "$BASE/health" >/dev/null && break; sleep 1; done
curl -sf "$BASE/health" >/dev/null || { echo "Go Uptime did not start"; cat "$WORK/go-uptime.log"; exit 1; }

api() { # method path content-type body
  local code
  for _ in $(seq 1 10); do
    code=$(curl -s -o "$WORK/response.json" -w '%{http_code}' -u "$USERNAME:$PASSWORD" -X "$1" -H "Content-Type: $3" --data-binary "$4" "$BASE$2")
    [ "$code" != 503 ] && break
    sleep 1
  done
  case "$code" in 200 | 201) ;; *) echo "$1 $2 answered $code: $(cat "$WORK/response.json")"; exit 1 ;; esac
}

echo "==> Registering through the administration: endpoints, push key and status page"
api POST /api/v1/admin/endpoints application/yaml "$(printf 'type: push\nname: nightly-backup\ngroup: jobs\ntoken: %s\nheartbeat:\n  interval: 1m\n' "$PUSH_TOKEN")"
api POST /api/v1/admin/endpoints application/yaml "$(printf 'name: checkout\ngroup: shop\nurl: "https://www.iana.org"\ninterval: 5s\nconditions:\n  - "[STATUS] == 200"\n  - "[CERTIFICATE_EXPIRATION] > 48h"\n')"
api POST /api/v1/admin/push-keys application/json '{"name":"deploy-scripts"}'
api POST /api/v1/admin/push-keys application/json '{"name":"cron-jobs"}'
api POST /api/v1/admin/status-pages application/json '{"slug":"services","title":"Services","description":"Availability of the services we run for our customers.","enabled":true,"groups":["sites","apis","shop"],"featured":["sites_website","shop_checkout"],"show-certificate-expiration":true}'
api POST /api/v1/admin/status-pages application/json '{"slug":"internal","title":"Internal tools","enabled":false,"groups":["infra","jobs"]}'

# A push every 5 seconds, with a response time that varies like the one of a real job
(
  while true; do
    curl -s -o /dev/null "$BASE/api/push/$PUSH_TOKEN?status=up&msg=OK&ping=$((250 + RANDOM % 140))" || true
    sleep 5
  done
) &
PUSHER_PID=$!

echo "==> Letting the history fill for $HISTORY_SECONDS seconds"
sleep "$HISTORY_SECONDS"

set_theme() {
  browser cookies set theme "$1" --url "$BASE" >/dev/null
  browser eval "document.cookie = 'theme=$1; path=/; max-age=31536000; samesite=strict'; (() => { const themes = { dark: ['dark', '#030712'], light: ['', '#f7f9fb'], bio: ['theme-bio', '#f2f8fa'] }; const theme = themes['$1'] ? '$1' : 'light'; for (const name in themes) { if (themes[name][0]) { document.documentElement.classList.toggle(themes[name][0], name === theme) } } const meta = document.querySelector('meta[name=\"theme-color\"]'); if (meta) { meta.setAttribute('content', themes[theme][1]) } })()" >/dev/null 2>&1 || true
}
capture() { # name url [selector to wait for]
  browser open "$BASE$2" >/dev/null
  [ -n "${3:-}" ] && browser wait "$3" >/dev/null
  browser wait 1500 >/dev/null
  browser screenshot "$OUT/$1.png" >/dev/null
  echo "    $1.png"
}

echo "==> Capturing at 1280x900"
browser open "$BASE/login" >/dev/null
browser set viewport 1280 900 >/dev/null
set_theme dark
capture login /login '[data-testid="login-username"]'
browser fill '[data-testid="login-username"]' "$USERNAME" >/dev/null
browser fill '[data-testid="login-password"]' "$PASSWORD" >/dev/null
browser click '[data-testid="login-submit"]' >/dev/null
browser wait '[data-testid="logout-button"]' >/dev/null

capture dashboard /
capture endpoint-details /endpoints/sites_website
capture admin-endpoints /admin
capture admin-status-pages /admin/status-pages
capture admin-status-page-form /admin/status-pages/services/edit
capture admin-push-keys /admin/push-keys
capture admin-backup /admin/backup
capture status-page-dark /status/services
set_theme light
capture status-page /status/services
capture status-page-endpoint /status/services/endpoints/sites_website
set_theme bio
capture status-page-bio /status/services
capture dashboard-bio /

echo "OK: 13 screenshots of $VERSION in $OUT"
