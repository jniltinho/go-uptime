#!/usr/bin/env bash
# End-to-end tests of the login screen of security.basic, with agent-browser.
#
#   test/e2e/login.sh
#
# Starts the locally built Gatus (temporary SQLite, basic auth, administration, a suite and a status page), goes through
# the login screen (redirections, refused redirects, wrong password, logout and limit of failed logins), checks that the
# public status pages open without login and that curl -u keeps working, and saves screenshots in dist/prints/login/
# (dist/ is in .gitignore). Requires agent-browser with Chrome installed and python3.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18094}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/login"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Gatus decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'

for command in agent-browser python3 curl; do
  command -v "$command" >/dev/null || { echo "$command not found"; exit 1; }
done
if (exec 3<>"/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then
  echo "port $PORT is already in use: stop the process using it or set E2E_PORT"
  exit 1
fi
mkdir -p "$PRINTS"

echo "==> Building"
make -s build

cat > "$WORK/config.yaml" <<CONFIG
web:
  address: 127.0.0.1
  port: $PORT
storage:
  type: sqlite
  path: "$WORK/gatus.db"
security:
  basic:
    username: $USERNAME
    password-bcrypt-base64: "$PASSWORD_HASH"
admin:
  enabled: true
endpoints:
  - name: health
    group: core
    url: $BASE/health
    interval: 5s
    conditions:
      - "[STATUS] == 200"
suites:
  - name: checkout
    group: core
    interval: 5s
    endpoints:
      - name: cart
        url: $BASE/health
        conditions:
          - "[STATUS] == 200"
status-pages:
  rate-limit: 0
  pages:
    - slug: services
      title: "Services"
      groups: [core]
CONFIG

GATUS_CONFIG_PATH="$WORK/config.yaml" dist/gatus > "$WORK/gatus.log" 2>&1 &
GATUS_PID=$!
browser() { agent-browser --session e2e-login "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  kill "$GATUS_PID" >/dev/null 2>&1 || true
  wait "$GATUS_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

for _ in $(seq 1 60); do
  curl -sf "$BASE/health" >/dev/null && break
  sleep 1
done
curl -sf "$BASE/health" >/dev/null || { echo "Gatus did not start"; cat "$WORK/gatus.log"; exit 1; }

STEP=0
step() {
  STEP=$((STEP + 1))
  echo "==> $STEP. $*"
}
fail() {
  echo "FAILED: $*"
  browser screenshot --full "$PRINTS/error.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/gatus.log"
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
js() {
  browser eval "$1" 2>/dev/null | tr -d '"'
}
wait_for() {
  browser wait "$(testid "$1")" >/dev/null || fail "element $1 did not appear"
}
expect_location() {
  local location
  location=$(js 'location.pathname + location.search + location.hash')
  [ "$location" = "$1" ] || fail "expected the location $1, got $location"
}
http_status() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}
login_json() {
  printf '{"username":"%s","password":"%s"}' "$USERNAME" "$1"
}
sign_in() {
  browser fill "$(testid login-username)" "$USERNAME" >/dev/null
  browser fill "$(testid login-password)" "$1" >/dev/null
  browser click "$(testid login-submit)" >/dev/null
}

step "API without credentials: 401 with the Basic challenge for curl, without it for the frontend, and curl -u works"
headers=$(curl -s -D - -o /dev/null "$BASE/api/v1/endpoints/statuses")
echo "$headers" | grep -q '^HTTP/1.1 401' || fail "expected 401 without credentials"
echo "$headers" | grep -qi '^WWW-Authenticate: Basic' || fail "expected the Basic challenge for curl"
headers=$(curl -s -D - -o /dev/null -H 'X-Requested-With: XMLHttpRequest' "$BASE/api/v1/endpoints/statuses")
echo "$headers" | grep -q '^HTTP/1.1 401' || fail "expected 401 for the frontend without session"
echo "$headers" | grep -qi '^WWW-Authenticate' && fail "the frontend should not receive the Basic challenge"
[ "$(http_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/statuses")" = 200 ] || fail "curl -u should keep working"

step "Sessions through the API: login, cookie on a protected route, logout and old token refused"
[ "$(http_status -c "$WORK/session.txt" -H 'Content-Type: application/json' -d "$(login_json "$PASSWORD")" "$BASE/api/v1/auth/login")" = 204 ] || fail "expected 204 for the login through the API"
grep -q go_uptime_session "$WORK/session.txt" || fail "the login did not set the session cookie"
[ "$(http_status -b "$WORK/session.txt" "$BASE/api/v1/endpoints/statuses")" = 200 ] || fail "the session cookie should authenticate the protected routes"
[ "$(http_status -b "$WORK/session.txt" -c "$WORK/after-logout.txt" -X POST "$BASE/api/v1/auth/logout")" = 204 ] || fail "expected 204 for the logout through the API"
[ "$(http_status -b "$WORK/session.txt" "$BASE/api/v1/endpoints/statuses")" = 401 ] || fail "the token of a closed session should be refused"
[ "$(http_status -b "$WORK/session.txt" -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/statuses")" = 200 ] || fail "an old session cookie should not block curl -u"

step "Administration without session: login screen at 10% of the top, without the dashboard header nor the native dialog"
browser set viewport 1280 900 >/dev/null
# Fork: the operating system prefers the light theme, which is not followed: without a theme cookie, the theme is dark
browser set media light >/dev/null
browser open "$BASE/admin" >/dev/null
wait_for login-card
[ "$(js 'location.pathname')" = /login ] || fail "expected the login screen, got $(js 'location.pathname')"
[ "$(js 'new URLSearchParams(location.search).get("redirect")')" = /admin ] || fail "expected redirect=/admin, got $(js 'location.search')"
[ "$(js 'document.querySelector("header") === null && document.querySelector("[data-testid=admin-link]") === null')" = true ] || fail "the login screen should not show the dashboard header"
[ "$(js 'Math.abs(document.querySelector("[data-testid=login-card]").getBoundingClientRect().top - innerHeight * 0.10) < 2')" = true ] || fail "the card should start at 10% of the height of the window"
[ "$(js 'document.querySelector("[data-testid=login-title]").textContent.trim()')" = Status ] || fail "expected the default header Status on the login screen, got $(js 'document.querySelector("[data-testid=login-title]").textContent')"
# Without ui.logo the logo embedded in the binary is shown (ui.logo: none hides it)
[ "$(js 'new URL(document.querySelector("[data-testid=login-card] img").src).pathname')" = "/logo-192x192.png" ] || fail "the login screen should show the embedded logo without ui.logo"
[ "$(js 'document.querySelector("[data-testid=login-card] img").naturalWidth > 0')" = true ] || fail "the embedded logo did not load"
[ "$(js 'document.documentElement.classList.contains("dark")')" = true ] || fail "without a theme cookie, the login screen should be dark even with the operating system in light mode"
[ "$(js 'document.documentElement.dataset.defaultTheme')" = dark ] || fail "the HTML should have the default theme of ui.dark-mode"
[ "$(js 'document.querySelector("meta[name=theme-color]").content')" = "#030712" ] || fail "the theme color should follow the dark theme"
browser screenshot "$PRINTS/01-login-dark.png" >/dev/null

step "Theme selector: light login screen, kept after a reload"
theme_classes() { js 'Array.from(document.documentElement.classList).filter((name) => name === "dark" || name === "theme-bio").join(" ")'; }
theme_color() { js 'document.querySelector("meta[name=theme-color]").content'; }
choose_theme() {
  browser click "$(testid login-theme-toggle)" >/dev/null
  browser wait "$(testid "login-theme-toggle-option-$1")" >/dev/null || fail "the theme menu did not open"
  browser click "$(testid "login-theme-toggle-option-$1")" >/dev/null
}
[ "$(js "document.querySelector('[data-testid=\"login-theme-toggle-option-dark\"]').getAttribute('aria-checked')")" = true ] || fail "the theme in use should be the checked option"
choose_theme light
[ -z "$(theme_classes)" ] || fail "the theme selector did not switch to the light theme: $(theme_classes)"
[ "$(theme_color)" = "#f7f9fb" ] || fail "the theme color should follow the light theme"
[ "$(js "document.querySelector('[data-testid=\"login-theme-toggle\"]').getAttribute('aria-expanded')")" = false ] || fail "the menu should close after a choice"
# Waits for the color transitions and moves the mouse away from the selector before the screenshot
browser mouse move 0 450 >/dev/null 2>&1 || true
browser wait 500 >/dev/null
browser screenshot "$PRINTS/02-login-light.png" >/dev/null
browser reload >/dev/null
wait_for login-card
[ -z "$(theme_classes)" ] || fail "the light theme chosen with the selector should be kept after a reload"

step "Theme selector with the keyboard: the bio theme, one theme class at a time, kept after a reload"
browser focus "$(testid login-theme-toggle)" >/dev/null
browser press Enter >/dev/null
browser wait "$(testid login-theme-toggle-option-bio)" >/dev/null || fail "Enter did not open the theme menu"
[ "$(js 'document.activeElement.getAttribute("data-testid")')" = login-theme-toggle-option-light ] || fail "the menu should open on the theme in use, got $(js 'document.activeElement.getAttribute("data-testid")')"
browser press Escape >/dev/null
[ "$(js 'document.activeElement.getAttribute("data-testid")')" = login-theme-toggle ] || fail "Escape should close the menu and give the focus back to the selector"
[ -z "$(theme_classes)" ] || fail "Escape should not change the theme"
browser press Enter >/dev/null
browser press End >/dev/null
[ "$(js 'document.activeElement.getAttribute("data-testid")')" = login-theme-toggle-option-bio ] || fail "End should go to the last theme"
browser press Enter >/dev/null
[ "$(theme_classes)" = theme-bio ] || fail "the bio theme should be the only theme class, got: $(theme_classes)"
[ "$(theme_color)" = "#f2f8fa" ] || fail "the theme color should follow the bio theme"
browser mouse move 0 450 >/dev/null 2>&1 || true
browser wait 500 >/dev/null
browser screenshot "$PRINTS/02b-login-bio.png" >/dev/null
browser reload >/dev/null
wait_for login-card
[ "$(theme_classes)" = theme-bio ] || fail "the bio theme should be kept after a reload, got: $(theme_classes)"
# The HTML already comes with the theme, before any script runs
RAW=$(curl -s -H 'Cookie: theme=bio' "$BASE/login" | grep -o '<html[^>]*>')
case "$RAW" in *'class="theme-bio"'*) ;; *) fail "the HTML of the server should already have the bio theme: $RAW" ;; esac
RAW=$(curl -s -H 'Cookie: theme=blue' "$BASE/login" | grep -o '<html[^>]*>')
case "$RAW" in *'class="dark"'*) ;; *) fail "an invalid theme cookie should fall back to the default theme: $RAW" ;; esac
choose_theme dark
[ "$(theme_classes)" = dark ] || fail "from bio to dark, dark should be the only theme class, got: $(theme_classes)"

step "Wrong password: generic message and still on the login screen"
sign_in "wrong-password"
wait_for login-error
browser get text "$(testid login-error)" | grep -q 'Invalid username or password' || fail "unexpected error: $(browser get text "$(testid login-error)")"
[ "$(js 'location.pathname')" = /login ] || fail "a wrong password should stay on the login screen"
browser screenshot "$PRINTS/03-wrong-password.png" >/dev/null

step "Login with redirect to /admin/status-pages, without going back to the login screen"
browser open "$BASE/login?redirect=%2Fadmin%2Fstatus-pages" >/dev/null
wait_for login-username
sign_in "$PASSWORD"
wait_for logout-button
expect_location /admin/status-pages
wait_for status-page-row-config-services
browser screenshot "$PRINTS/04-after-login.png" >/dev/null

step "Dashboard, endpoint details and suite details with the session"
browser open "$BASE/" >/dev/null
wait_for logout-button
wait_for admin-link
[ "$(js 'document.querySelector("header h1").textContent.trim()')" = Status ] || fail "expected the default header Status on the dashboard, got $(js 'document.querySelector("header h1").textContent')"
[ "$(js 'new URL(document.querySelector("header img").src).pathname')" = "/logo-192x192.png" ] || fail "the dashboard header should show the embedded logo without ui.logo"
[ "$(js '!document.body.innerText.includes("Gatus") && document.querySelector("#social, a[href*=\"github.com\"], a[href*=\"gatus.io\"]") === null')" = true ] || fail "the dashboard should not show the Gatus name, the GitHub link nor the Powered by footer"
[ "$(js 'document.title')" = "Health Dashboard | Status" ] || fail "expected the default title, got $(js 'document.title')"
browser open "$BASE/endpoints/core_health" >/dev/null
wait_for recent-checks-card
expect_location /endpoints/core_health
browser open "$BASE/suites/core_checkout" >/dev/null
browser wait --text "checkout" >/dev/null || fail "the suite details page did not open"
wait_for logout-button
expect_location /suites/core_checkout
browser screenshot "$PRINTS/05-suite-details.png" >/dev/null

step "Refused redirects open the dashboard"
for value in '//site-malicioso.exemplo' '/%2F%2Fsite-malicioso.exemplo' '/%252F%252Fsite-malicioso.exemplo' '/\site-malicioso.exemplo' 'https://site-malicioso.exemplo' '/%ZZ' '/login'; do
  encoded=$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$value")
  browser open "$BASE/login?redirect=$encoded" >/dev/null
  wait_for logout-button
  expect_location /
done

step "Logout from the header: login screen, and the dashboard asks for a new login"
browser click "$(testid logout-button)" >/dev/null
wait_for login-card
expect_location /login
browser open "$BASE/" >/dev/null
wait_for login-card
[ "$(js 'new URLSearchParams(location.search).get("redirect")')" = / ] || fail "expected redirect=/ after the logout, got $(js 'location.search')"
browser screenshot "$PRINTS/06-after-logout.png" >/dev/null

step "Public status page without login"
browser open "$BASE/status/services" >/dev/null
wait_for public-layout
expect_location /status/services
[ "$(js 'document.querySelector("[data-testid=login-card]") === null')" = true ] || fail "the public status page should not ask for a login"
[ "$(js 'new URL(document.querySelector("[data-testid=public-layout] header img").src).pathname === "/logo-192x192.png" && document.querySelector("[data-testid=public-layout] header").innerText.includes("Status")')" = true ] || fail "the public header should show the embedded logo and Status without ui.logo"
browser screenshot "$PRINTS/07-public-status-page.png" >/dev/null

step "Limit of failed logins: 429 with Retry-After, also with the right password, and message on the login screen"
blocked=""
for _ in $(seq 1 11); do
  code=$(http_status -H 'Content-Type: application/json' -d "$(login_json wrong-password)" "$BASE/api/v1/auth/login")
  if [ "$code" = 429 ]; then
    blocked=1
    break
  fi
  [ "$code" = 401 ] || fail "expected 401 for a wrong password, got $code"
done
[ -n "$blocked" ] || fail "expected 429 after 10 failed logins"
curl -s -D - -o /dev/null -H 'Content-Type: application/json' -d "$(login_json "$PASSWORD")" "$BASE/api/v1/auth/login" | grep -qi '^Retry-After: ' || fail "expected Retry-After for the right password from a blocked address"
[ "$(http_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/statuses")" = 429 ] || fail "expected 429 for curl -u from a blocked address"
browser open "$BASE/login" >/dev/null
wait_for login-username
sign_in "$PASSWORD"
wait_for login-error
browser get text "$(testid login-error)" | grep -q 'Too many failed attempts' || fail "unexpected error: $(browser get text "$(testid login-error)")"
browser screenshot "$PRINTS/08-too-many-attempts.png" >/dev/null

echo "==> OK: login screen end-to-end tests passed (screenshots in $PRINTS)"
