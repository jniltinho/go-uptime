#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# End-to-end tests of the collapsible groups of the public status pages, with agent-browser.
#
#   make build && test/e2e/status-page-groups.sh
#
# The endpoints are Push endpoints, so that the script takes a group down and brings it back on demand. The public
# payload is cached for 30 seconds, which is why two steps wait before asking the page to refresh. Screenshots in
# dist/prints/status-page-groups/ (dist/ is in .gitignore). Requires agent-browser with Chrome, curl and python3.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
PRINTS="$ROOT/dist/prints/status-page-groups"
WORK=$(mktemp -d)
PORT=${PORT:-18095}
BASE="http://127.0.0.1:$PORT"
USERNAME=admin
PASSWORD='e2e-groups-password'
PAGE_USERNAME=customer
PAGE_PASSWORD='e2e-page-password'
CACHE_SECONDS=31
SERVER_PID=""
mkdir -p "$PRINTS"

[ -x dist/go-uptime ] || { echo "dist/go-uptime not found: run make build"; exit 1; }
HASH=$(printf '%s\n' "$PASSWORD" | dist/go-uptime password hash)
PAGE_HASH=$(printf '%s\n' "$PAGE_PASSWORD" | dist/go-uptime password hash)

token_of() { printf 'e2e-push-token-%s-0123456789' "$1"; }
push_endpoint() { # group name
  printf '  - name: %s\n' "$2"
  [ -n "$1" ] && printf '    group: "%s"\n' "$1"
  printf '    token: "%s"\n' "$(token_of "$2")"
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
ui:
  logo: /logo-192x192.png
  header: "A rather long name of the monitoring service"
  # A representative custom.css: an !important rule and the override of a variable of one theme only. custom.css is
  # loaded before the stylesheet of the application, so a variable of a theme is overridden with !important (or with a
  # more specific selector): with the same specificity, the rule of the application comes later and wins.
  custom-css: |
    .app-header { outline-color: rgb(1, 2, 3) !important; }
    :root.theme-bio { --ring: 0 100% 50% !important; }
    :root.dark { --e2e-plain-override: 1; --ring: 120 100% 50%; }
external-endpoints:
CONFIG
  push_endpoint apis gateway
  push_endpoint apis search
  push_endpoint sites website
  push_endpoint sites docs
  push_endpoint "Other services" billing
  push_endpoint "__without-group__" legacy
  push_endpoint "" solo
  cat <<CONFIG
status-pages:
  rate-limit: 0
  pages:
    - slug: services
      title: "Services"
      groups: [apis, sites, "Other services", "__without-group__"]
      endpoints: [_solo]
    - slug: other
      title: "Other"
      groups: [sites]
    - slug: longtitle
      title: "Availability of every service we run for our customers"
      groups: [sites]
    - slug: compact
      title: "Compact"
      groups: [apis, sites]
      groups-collapsed: true
    - slug: private
      title: "Private"
      groups: [sites]
      auth:
        username: $PAGE_USERNAME
        password-bcrypt-base64: "$PAGE_HASH"
CONFIG
} > "$WORK/config.yaml"

browser() { agent-browser --session e2e-status-page-groups "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
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
push() { # endpoint status
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/push/$(token_of "$1")?status=$2&msg=e2e")
  [ "$code" = 200 ] || fail "the push of $1 answered $code"
}
toggle() { echo "[data-testid=\"status-group-toggle-$1\"]"; }
expanded() { js "document.querySelector('[data-testid=\"status-group-toggle-$1\"]')?.getAttribute('aria-expanded')"; }
rows() { js "document.querySelectorAll('[data-testid=\"status-group-$1\"] li').length"; }
counts() { js "document.querySelector('[data-testid=\"status-group-counts-$1\"]')?.textContent.trim()"; }
stored() { js "localStorage.getItem('go-uptime:status-page-groups') || ''"; }
# refresh makes the page fetch its payload again, as it does when the tab becomes visible, without waiting 60 seconds
refresh() {
  browser eval "Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true }); document.dispatchEvent(new Event('visibilitychange'))" >/dev/null
  browser wait 800 >/dev/null
}
open_page() { # slug
  browser open "$BASE/status/$1" >/dev/null
  browser wait "$(toggle sites)" >/dev/null || fail "the page $1 did not open"
}
set_theme() {
  browser cookies set theme "$1" --url "$BASE" >/dev/null
  browser eval "document.cookie = 'theme=$1; path=/; max-age=31536000; samesite=strict'; (() => { const themes = { dark: ['dark', '#030712'], light: ['', '#f7f9fb'], bio: ['theme-bio', '#f2f8fa'] }; const theme = themes['$1'] ? '$1' : 'light'; for (const name in themes) { if (themes[name][0]) { document.documentElement.classList.toggle(themes[name][0], name === theme) } } const meta = document.querySelector('meta[name=\"theme-color\"]'); if (meta) { meta.setAttribute('content', themes[theme][1]) } })()" >/dev/null 2>&1 || true
}

echo "==> Starting dist/go-uptime"
dist/go-uptime --config "$WORK/config.yaml" > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do curl -sf "$BASE/health" >/dev/null && break; sleep 1; done
curl -sf "$BASE/health" >/dev/null || fail "Go Uptime did not start"
# Right after the start the push endpoints may still be loading: the first push is retried
for _ in $(seq 1 20); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/push/$(token_of gateway)?status=up")" = 200 ] && break
  sleep 0.5
done
for endpoint in search website docs billing legacy solo; do push "$endpoint" up; done
# apis starts with a problem, before any payload is cached
push search down

step "Payload: groupsCollapsed of the page and the summary of each group"
python3 - "$BASE" <<'PY' || fail "the payload is not the expected one"
import json, sys, urllib.request
base = sys.argv[1]
def page(slug):
    return json.load(urllib.request.urlopen(f"{base}/api/v1/status-pages/{slug}"))
services, compact = page("services"), page("compact")
assert services["groupsCollapsed"] is False and compact["groupsCollapsed"] is True, (services["groupsCollapsed"], compact["groupsCollapsed"])
groups = {group["name"]: group for group in services["groups"]}
assert groups["apis"]["status"] == "degraded" and groups["apis"]["summary"] == {"total": 2, "up": 1, "down": 1, "pending": 0, "unknown": 0}, groups["apis"]
assert groups["sites"]["status"] == "operational" and groups["sites"]["summary"] == {"total": 2, "up": 2, "down": 0, "pending": 0, "unknown": 0}, groups["sites"]
assert set(groups) == {"apis", "sites", "Other services", "__without-group__", ""}, sorted(groups)
PY

step "Default of the page: every group expanded, header with the status and the counts"
browser open "$BASE/status/services" >/dev/null
browser set viewport 1280 900 >/dev/null
set_theme light
open_page services
expect "sites starts expanded" true "$(expanded sites)"
expect "rows of sites" 2 "$(rows sites)"
expect "counts of sites" "2 up" "$(counts sites)"
expect "counts of apis" "1 up · 1 down" "$(counts apis)"
browser screenshot "$PRINTS/01-expanded.png" >/dev/null

step "Collapsing a group removes its rows and keeps the status and the counts"
browser click "$(toggle sites)" >/dev/null
expect "sites collapsed" false "$(expanded sites)"
expect "rows of sites while collapsed" 0 "$(rows sites)"
expect "counts of sites while collapsed" "2 up" "$(counts sites)"
expect "apis untouched" true "$(expanded apis)"
expect "the panel that aria-controls points to exists" true "$(js "!!document.getElementById(document.querySelector('[data-testid=\"status-group-toggle-sites\"]').getAttribute('aria-controls'))")"
browser screenshot "$PRINTS/02-collapsed.png" >/dev/null

step "Keyboard: Enter expands, Space collapses"
browser focus "$(toggle sites)" >/dev/null
browser press Enter >/dev/null
expect "sites after Enter" true "$(expanded sites)"
expect "rows back after Enter" 2 "$(rows sites)"
expect "the accessible summary of a row is back" true "$(js "document.querySelectorAll('[data-testid=\"status-group-sites\"] [aria-live]').length === 2")"
browser press Space >/dev/null
expect "sites after Space" false "$(expanded sites)"

step "The choice is remembered across a reload, and nothing readable is stored"
STORED=$(stored)
[ -n "$STORED" ] || fail "nothing was stored"
case "$STORED" in *sites*|*services*|*apis*) fail "the storage holds a readable name: $STORED" ;; esac
browser reload >/dev/null
browser wait "$(toggle sites)" >/dev/null
expect "sites after the reload" false "$(expanded sites)"
expect "apis after the reload" true "$(expanded apis)"

step "Another page is not affected"
open_page other
expect "sites of the page other" true "$(expanded sites)"

step "The group without name, 'Other services' and '__without-group__' are three different groups"
open_page services
browser click "$(toggle outros)" >/dev/null
expect "the group without name" false "$(expanded outros)"
expect "the group called Other services" true "$(expanded 'Other services')"
expect "the group called __without-group__" true "$(expanded __without-group__)"
browser click "$(toggle outros)" >/dev/null

step "groups-collapsed: operational groups start collapsed, the one with a problem starts expanded"
open_page compact
expect "sites on compact" false "$(expanded sites)"
expect "apis on compact, degraded" true "$(expanded apis)"
browser screenshot "$PRINTS/03-collapsed-by-default.png" >/dev/null

step "Collapsing during an incident does not stick and is not remembered"
BEFORE=$(stored)
browser click "$(toggle apis)" >/dev/null
expect "apis right after the click" false "$(expanded apis)"
expect "the storage after collapsing a degraded group" "$BEFORE" "$(stored)"
refresh
expect "apis after the next payload" true "$(expanded apis)"

step "A group collapsed by the visitor opens when it fails, and closes again when it recovers"
open_page services
expect "sites, remembered as collapsed" false "$(expanded sites)"
push docs down
echo "    waiting $CACHE_SECONDS s for the cached payload to expire"
sleep "$CACHE_SECONDS"
refresh
expect "sites while degraded" true "$(expanded sites)"
expect "counts of sites while degraded" "1 up · 1 down" "$(counts sites)"
browser screenshot "$PRINTS/04-forced-open.png" >/dev/null
push docs up
echo "    waiting $CACHE_SECONDS s for the cached payload to expire"
sleep "$CACHE_SECONDS"
refresh
expect "sites after the recovery" false "$(expanded sites)"

step "Without storage the choice still holds for the visit"
open_page other
browser eval "Storage.prototype.setItem = () => { throw new Error('blocked') }; Storage.prototype.getItem = () => { throw new Error('blocked') }" >/dev/null
browser click "$(toggle sites)" >/dev/null
expect "sites on other, storage blocked" false "$(expanded sites)"
refresh
expect "sites on other after a refresh, storage blocked" false "$(expanded sites)"
browser reload >/dev/null
browser wait "$(toggle sites)" >/dev/null
expect "sites on other after a reload: nothing was remembered" true "$(expanded sites)"

step "Every group collapsed on a page without featured endpoints, then one reopened with the keyboard"
open_page compact
browser focus "$(toggle sites)" >/dev/null
expect "the overall status is still announced" true "$(js "!!document.querySelector('[role=\"status\"]')")"
browser press Enter >/dev/null
expect "sites reopened" true "$(expanded sites)"
expect "rows of sites" 2 "$(rows sites)"
browser press Enter >/dev/null

step "Page with a login of its own: the choice is remembered without its group names"
browser set credentials "$PAGE_USERNAME" "$PAGE_PASSWORD" >/dev/null
open_page private
browser click "$(toggle sites)" >/dev/null
expect "sites on private" false "$(expanded sites)"
case "$(stored)" in *sites*|*private*) fail "the storage of a page with a login holds a readable name" ;; esac
browser reload >/dev/null
browser wait "$(toggle sites)" >/dev/null
expect "sites on private after the reload" false "$(expanded sites)"

step "Dark mode and a 390 px screen"
set_theme dark
open_page services
browser set viewport 390 844 >/dev/null
browser wait 500 >/dev/null
expect "no horizontal scrolling at 390 px" true "$(js "document.documentElement.scrollWidth <= window.innerWidth")"
browser screenshot "$PRINTS/05-dark-390.png" >/dev/null
browser set viewport 1280 900 >/dev/null

step "Bio theme at 360 px: the theme selector and its open menu fit, with a logo and a long title"
set_theme bio
open_page longtitle
browser set viewport 360 780 >/dev/null
browser wait 500 >/dev/null
expect "the bio theme is the only theme class" theme-bio "$(js 'Array.from(document.documentElement.classList).filter((name) => name === "dark" || name === "theme-bio").join(" ")')"
expect "no horizontal scrolling at 360 px" true "$(js "document.documentElement.scrollWidth <= window.innerWidth")"
browser click '[data-testid="public-theme-toggle"]' >/dev/null
browser wait '[data-testid="public-theme-toggle-option-bio"]' >/dev/null || fail "the theme menu did not open"
expect "the open menu is inside the window" true "$(js "(() => { const box = document.querySelector('[data-testid=\"public-theme-toggle-menu\"]').getBoundingClientRect(); return box.left >= 0 && box.right <= window.innerWidth && box.width > 0 })()")"
expect "no horizontal scrolling with the menu open" true "$(js "document.documentElement.scrollWidth <= window.innerWidth")"
expect "the theme in use is the checked option" true "$(js "document.querySelector('[data-testid=\"public-theme-toggle-option-bio\"]').getAttribute('aria-checked')")"
browser screenshot "$PRINTS/05b-bio-360-menu.png" >/dev/null
browser press Escape >/dev/null
browser set viewport 1280 900 >/dev/null
set_theme light

step "ui.custom-css keeps working in the three themes, and can override a variable of one of them"
for theme in light dark bio; do
  set_theme "$theme"
  open_page other
  expect "the !important rule of custom.css in the $theme theme" "rgb(1, 2, 3)" "$(js "getComputedStyle(document.querySelector('.app-header')).outlineColor")"
done
expect "the variable overridden for the bio theme" "0 100% 50%" "$(js "getComputedStyle(document.documentElement).getPropertyValue('--ring').trim()")"
set_theme light
open_page other
[ "$(js "getComputedStyle(document.documentElement).getPropertyValue('--ring').trim()")" != "0 100% 50%" ] || fail "the override of the bio theme leaked into the light theme"
# The same rule as always: without !important, an override of a variable of the dark theme loses to the application
set_theme dark
open_page other
expect "a custom property of custom.css that the application does not define" 1 "$(js "getComputedStyle(document.documentElement).getPropertyValue('--e2e-plain-override').trim()")"
[ "$(js "getComputedStyle(document.documentElement).getPropertyValue('--ring').trim()")" != "120 100% 50%" ] || fail "a plain override of a theme variable is not expected to win: did the order of the stylesheets change?"
set_theme light

step "Administration: the option of the form, and the preview with the default of the page"
curl -s -o /dev/null -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/json' -d '{"slug":"managed","title":"Managed","enabled":true,"groups":["apis","sites"]}' "$BASE/api/v1/admin/status-pages"
browser open "$BASE/login" >/dev/null
browser wait '[data-testid="login-username"]' >/dev/null || fail "the login screen did not open"
browser fill '[data-testid="login-username"]' "$USERNAME" >/dev/null
browser fill '[data-testid="login-password"]' "$PASSWORD" >/dev/null
browser click '[data-testid="login-submit"]' >/dev/null
browser wait '[data-testid="logout-button"]' >/dev/null || fail "the login did not work"
# The visitor of this browser expanded nothing on "managed", but collapsed sites elsewhere: the preview ignores both
browser open "$BASE/admin/status-pages/managed/edit" >/dev/null
browser wait '[data-testid="status-page-field-groups-collapsed"]' >/dev/null || fail "the option is not in the form"
browser scrollintoview '[data-testid="status-page-field-groups-collapsed"]' >/dev/null
browser click '[data-testid="status-page-field-groups-collapsed"]' >/dev/null
browser screenshot "$PRINTS/06-form-option.png" >/dev/null
browser click '[data-testid="status-page-save"]' >/dev/null
browser wait 1500 >/dev/null
SAVED=$(curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/status-pages/managed")
case "$SAVED" in *groups-collapsed*) ;; *) fail "the saved definition has no groups-collapsed: $SAVED" ;; esac
browser click '[data-testid="status-page-preview-button"]' >/dev/null
browser wait '[data-testid="status-page-preview-group-sites"]' >/dev/null || fail "the preview did not open"
expect "sites in the preview, operational" false "$(js "document.querySelector('[data-testid=\"status-page-preview-group-sites\"] button').getAttribute('aria-expanded')")"
expect "apis in the preview, degraded" true "$(js "document.querySelector('[data-testid=\"status-page-preview-group-apis\"] button').getAttribute('aria-expanded')")"
PREVIEW_BEFORE=$(stored)
browser click '[data-testid="status-page-preview-group-sites"] button' >/dev/null
expect "sites in the preview after a click" true "$(js "document.querySelector('[data-testid=\"status-page-preview-group-sites\"] button').getAttribute('aria-expanded')")"
expect "the preview stores nothing" "$PREVIEW_BEFORE" "$(stored)"
browser screenshot "$PRINTS/07-preview.png" >/dev/null

echo "OK: $STEP steps; screenshots in $PRINTS"
