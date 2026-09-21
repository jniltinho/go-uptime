#!/usr/bin/env bash
# End-to-end tests of status-pages.maximum-endpoints-per-page, with agent-browser.
#
#   make build && test/e2e/status-page-limit.sh
#
# The limit is an access rule as much as a display rule: an endpoint beyond the cut is answered 404 by every route of
# the page. The script starts with a limit of 3 on a page that selects 5 endpoints, then lowers the limit to 2 with
# Go Uptime running, which takes up to 30 seconds to notice the change of its configuration file. Screenshots in
# dist/prints/status-page-limit/ (dist/ is in .gitignore). Requires agent-browser with Chrome, curl and python3.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
PRINTS="$ROOT/dist/prints/status-page-limit"
WORK=$(mktemp -d)
PORT=${PORT:-18097}
BASE="http://127.0.0.1:$PORT"
USERNAME=admin
PASSWORD='e2e-limit-password'
SERVER_PID=""
STREAM_PID=""
mkdir -p "$PRINTS"

[ -x dist/go-uptime ] || { echo "dist/go-uptime not found: run make build"; exit 1; }
HASH=$(printf '%s\n' "$PASSWORD" | dist/go-uptime password hash)

token_of() { printf 'e2e-push-token-%s-0123456789' "$1"; }
ENDPOINTS="alpha bravo charlie delta echo"
write_config() { # limit ("" leaves the option out)
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
external-endpoints:
CONFIG
    for endpoint in $ENDPOINTS; do
      printf '  - name: %s\n    group: services\n    token: "%s"\n' "$endpoint" "$(token_of "$endpoint")"
    done
    echo "status-pages:"
    echo "  rate-limit: 0"
    [ -n "$1" ] && echo "  maximum-endpoints-per-page: $1"
    cat <<CONFIG
  pages:
    - slug: services
      title: "Services"
      groups: [services]
CONFIG
  } > "${2:-$WORK/config.yaml}"
}

browser() { agent-browser --session e2e-status-page-limit "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  for pid in "$STREAM_PID" "$SERVER_PID"; do
    if [ -n "$pid" ]; then
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -rf "$WORK"
}
trap cleanup EXIT

STEP=0
step() { STEP=$((STEP + 1)); echo "==> $STEP. $*"; }
fail() {
  echo "FAILED: $*"
  browser screenshot --full "$PRINTS/error.png" >/dev/null 2>&1 || true
  tail -15 "$WORK/go-uptime.log"
  exit 1
}
js() { browser eval "$*" 2>/dev/null | tr -d '"'; }
expect() { # description expected actual
  [ "$2" = "$3" ] || fail "$1: expected '$2', got '$3'"
}
code_of() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
admin_api() { curl -s -u "$USERNAME:$PASSWORD" -H 'X-Requested-With: XMLHttpRequest' "$@"; }
# routes_of lists the routes of the page that serve one endpoint
routes_of() { # key
  local prefix="$BASE/api/v1/status-pages/services/endpoints/$1"
  echo "$prefix" "$prefix/response-time-chart?period=24h" "$prefix/health/badge.svg" "$prefix/response-times/24h/badge.svg"
}
expect_routes() { # key code
  local route
  for route in $(routes_of "$1"); do
    expect "GET $route" "$2" "$(code_of "$route")"
  done
  expect "HEAD of the stream of $1" "$2" "$(code_of -I "$BASE/api/v1/status-pages/services/endpoints/$1/events")"
}

step "go-uptime config validate refuses a limit out of bounds or that is not an integer"
for invalid in 0 -1 1001 2.5 many; do
  write_config "$invalid" "$WORK/invalid.yaml"
  if dist/go-uptime config validate --config "$WORK/invalid.yaml" >/dev/null 2>&1; then
    fail "config validate accepted maximum-endpoints-per-page: $invalid"
  fi
done
for valid in "" 1 400 1000; do
  write_config "$valid" "$WORK/valid.yaml"
  dist/go-uptime config validate --config "$WORK/valid.yaml" >/dev/null 2>&1 || fail "config validate refused maximum-endpoints-per-page: '$valid'"
done

write_config 3
echo "==> Starting dist/go-uptime"
dist/go-uptime --config "$WORK/config.yaml" > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do curl -sf "$BASE/health" >/dev/null && break; sleep 1; done
curl -sf "$BASE/health" >/dev/null || fail "Go Uptime did not start"
# Right after the start the push endpoints may still be loading: the first push is retried
for _ in $(seq 1 20); do
  [ "$(code_of "$BASE/api/push/$(token_of alpha)?status=up")" = 200 ] && break
  sleep 0.5
done
for endpoint in bravo charlie delta echo; do
  expect "push of $endpoint" 200 "$(code_of "$BASE/api/push/$(token_of "$endpoint")?status=up")"
done

step "Payload: the first 3 endpoints in display order, truncated, with the total of what is shown"
python3 - "$BASE" 3 alpha,bravo,charlie <<'PY' || fail "the payload is not the expected one"
import json, sys, urllib.request
base, total, names = sys.argv[1], int(sys.argv[2]), sys.argv[3].split(",")
page = json.load(urllib.request.urlopen(f"{base}/api/v1/status-pages/services"))
shown = [endpoint["name"] for group in page["groups"] for endpoint in group["endpoints"]]
assert page["truncated"] is True and page["summary"]["total"] == total and shown == names, (page["truncated"], page["summary"], shown)
PY

step "Routes of the page: 200 within the cut, the 404 of the status pages beyond it"
expect_routes services_charlie 200
expect_routes services_delta 404
expect_routes services_echo 404
# The HTML is the single page application either way: the 404 is the one of the API
expect "HTML of the details beyond the cut" 200 "$(code_of "$BASE/status/services/endpoints/services_delta")"
# The dashboard, behind the login of the administration, still knows the endpoint
expect "endpoint beyond the cut in the API of the dashboard" 200 "$(code_of -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/services_delta/statuses")"

step "Public page: the notice has the number of the payload"
browser open "$BASE/status/services" >/dev/null
browser set viewport 1280 900 >/dev/null
browser wait '[data-testid="status-page-truncated"]' >/dev/null || fail "the notice of the truncated page did not show up"
expect "notice" "Showing the first 3 services." "$(js "document.querySelector('[data-testid=\"status-page-truncated\"]').textContent.trim()")"
expect "rows of the page" 3 "$(js "document.querySelectorAll('[data-testid=\"status-group-services\"] li').length")"
browser screenshot "$PRINTS/01-public-truncated.png" >/dev/null

step "Details page of an endpoint beyond the cut: not found"
browser open "$BASE/status/services/endpoints/services_delta" >/dev/null
browser wait '[data-testid="status-page-not-found"]' >/dev/null || fail "the details page of an endpoint beyond the cut did not answer not found"
browser open "$BASE/status/services/endpoints/services_charlie" >/dev/null
browser wait '[data-testid="status-endpoint-details"]' >/dev/null || fail "the details page of an endpoint within the cut did not open"

step "Administration API: truncated in the listing, the warning in the validation"
python3 - "$BASE" "$USERNAME" "$PASSWORD" <<'PY' || fail "the administration API is not the expected one"
import base64, json, sys, urllib.request
base, username, password = sys.argv[1:4]
headers = {"Authorization": "Basic " + base64.b64encode(f"{username}:{password}".encode()).decode(), "X-Requested-With": "XMLHttpRequest"}
def call(path, data=None, content_type=None):
    request = urllib.request.Request(base + path, data=data, headers=dict(headers, **({"Content-Type": content_type} if content_type else {})))
    return json.load(urllib.request.urlopen(request))
item = [page for page in call("/api/v1/admin/status-pages")["statusPages"] if page["slug"] == "services"][0]
assert item["endpoints"] == 3 and item["truncated"] is True, item
validation = call("/api/v1/admin/status-pages/validate", b"slug: team\ntitle: Team\ngroups: [services]\n", "application/yaml")
assert validation["endpoints"] == 3 and validation["warnings"] == [{"type": "truncated", "value": "3"}], validation
validation = call("/api/v1/admin/status-pages/validate", b"slug: team\ntitle: Team\nendpoints: [services_echo, services_delta]\n", "application/yaml")
assert validation["endpoints"] == 2 and validation["warnings"] == [], validation
created = call("/api/v1/admin/status-pages", b"slug: team\ntitle: Team\ngroups: [services]\nenabled: true\n", "application/yaml")
assert created["truncated"] is True and created["endpoints"] == 3, created
PY

step "Administration: the listing marks the truncated pages, the form warns and the preview is truncated"
browser open "$BASE/login" >/dev/null
browser wait '[data-testid="login-username"]' >/dev/null || fail "the login screen did not open"
browser fill '[data-testid="login-username"]' "$USERNAME" >/dev/null
browser fill '[data-testid="login-password"]' "$PASSWORD" >/dev/null
browser click '[data-testid="login-submit"]' >/dev/null
browser wait '[data-testid="logout-button"]' >/dev/null || fail "the login did not work"
browser open "$BASE/admin/status-pages" >/dev/null
browser wait '[data-testid="status-page-truncated-services"]' >/dev/null || fail "the listing does not mark the truncated page"
expect "count of the truncated page" "3+" "$(js "document.querySelector('[data-testid=\"status-page-truncated-team\"]').firstChild.textContent.trim()")"
browser screenshot "$PRINTS/02-admin-listing.png" >/dev/null
browser open "$BASE/admin/status-pages/team/edit" >/dev/null
browser wait '[data-testid="status-page-validate"]' >/dev/null || fail "the form did not open"
browser click '[data-testid="status-page-validate"]' >/dev/null
browser wait '[data-testid="status-page-validation"]' >/dev/null || fail "the validation did not warn"
case "$(js "document.querySelector('[data-testid=\"status-page-validation\"]').textContent")" in
  *"maximum-endpoints-per-page (3)"*"only the first 3 are shown"*) ;;
  *) fail "the warning of the validation does not cite the limit" ;;
esac
browser screenshot "$PRINTS/03-admin-validation.png" >/dev/null
browser click '[data-testid="status-page-preview-button"]' >/dev/null
browser wait '[data-testid="status-page-preview-truncated"]' >/dev/null || fail "the preview does not show the notice"
expect "notice of the preview" "Showing the first 3 services." "$(js "document.querySelector('[data-testid=\"status-page-preview-truncated\"]').textContent.trim()")"
browser screenshot "$PRINTS/04-admin-preview.png" >/dev/null

step "Limit lowered to 2 with Go Uptime running: an open stream ends, and everything follows the new limit"
curl -sN -H 'Accept: text/event-stream' "$BASE/api/v1/status-pages/services/endpoints/services_charlie/events" > "$WORK/stream.log" 2>&1 &
STREAM_PID=$!
sleep 1
kill -0 "$STREAM_PID" 2>/dev/null || fail "the stream of an endpoint within the cut did not stay open"
write_config 2
for _ in $(seq 1 50); do
  [ "$(code_of "$BASE/api/v1/status-pages/services/endpoints/services_charlie")" = 404 ] && break
  sleep 1
done
expect_routes services_bravo 200
expect_routes services_charlie 404
for _ in $(seq 1 10); do kill -0 "$STREAM_PID" 2>/dev/null || break; sleep 1; done
if kill -0 "$STREAM_PID" 2>/dev/null; then fail "the stream opened under the previous limit is still open"; fi
STREAM_PID=""
python3 - "$BASE" 2 alpha,bravo <<'PY' || fail "the payload did not follow the new limit"
import json, sys, urllib.request
base, total, names = sys.argv[1], int(sys.argv[2]), sys.argv[3].split(",")
page = json.load(urllib.request.urlopen(f"{base}/api/v1/status-pages/services"))
shown = [endpoint["name"] for group in page["groups"] for endpoint in group["endpoints"]]
assert page["truncated"] is True and page["summary"]["total"] == total and shown == names, (page["summary"], shown)
PY
expect "count of the administration" "2 True" "$(admin_api "$BASE/api/v1/admin/status-pages" | python3 -c 'import json,sys; item=[p for p in json.load(sys.stdin)["statusPages"] if p["slug"]=="team"][0]; print(item["endpoints"], item["truncated"])')"
browser open "$BASE/status/services" >/dev/null
browser wait '[data-testid="status-page-truncated"]' >/dev/null || fail "the notice did not show up after the reload"
expect "notice after the reload" "Showing the first 2 services." "$(js "document.querySelector('[data-testid=\"status-page-truncated\"]').textContent.trim()")"
browser screenshot "$PRINTS/05-public-after-reload.png" >/dev/null

step "Limit removed: the default of 400 shows everything, without notice"
write_config ""
for _ in $(seq 1 50); do
  [ "$(code_of "$BASE/api/v1/status-pages/services/endpoints/services_echo")" = 200 ] && break
  sleep 1
done
expect_routes services_echo 200
browser open "$BASE/status/services" >/dev/null
browser wait '[data-testid="status-group-services"]' >/dev/null || fail "the page did not open"
expect "rows of the page" 5 "$(js "document.querySelectorAll('[data-testid=\"status-group-services\"] li').length")"
expect "notice without truncation" 0 "$(js "document.querySelectorAll('[data-testid=\"status-page-truncated\"]').length")"

echo "OK: $STEP steps. Screenshots in $PRINTS"
