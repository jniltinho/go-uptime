#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# End-to-end tests of the expiration of the TLS certificate, with agent-browser.
#
#   test/e2e/certificate.sh
#
# Starts a local HTTPS server with a self-signed certificate valid for 73 days and the locally built Go Uptime (temporary
# SQLite, basic auth, administration enabled), checks the discreet line on the endpoint details page of the dashboard,
# a status page of the configuration file without the option, and a status page created through the form with
# "Show certificate expiration", and saves screenshots in dist/prints/certificate/ (dist/ is in .gitignore). Requires
# agent-browser with Chrome installed, openssl and python3.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18093}
TLS_PORT=${E2E_TLS_PORT:-18443}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/certificate"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Go Uptime decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'

for command in agent-browser openssl python3; do
  command -v "$command" >/dev/null || { echo "$command not found"; exit 1; }
done
for port in "$PORT" "$TLS_PORT"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "port $port is already in use: stop the process using it or set E2E_PORT/E2E_TLS_PORT"
    exit 1
  fi
done
mkdir -p "$PRINTS"

echo "==> Building"
make -s build

openssl req -x509 -newkey rsa:2048 -nodes -days 73 -subj "/CN=127.0.0.1" \
  -keyout "$WORK/key.pem" -out "$WORK/cert.pem" >/dev/null 2>&1
cat > "$WORK/https.py" <<PYTHON
import http.server, ssl
server = http.server.HTTPServer(("127.0.0.1", $TLS_PORT), http.server.SimpleHTTPRequestHandler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain("$WORK/cert.pem", "$WORK/key.pem")
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
PYTHON
# Started directly, so that HTTPS_PID is the PID of python and the cleanup stops the server
python3 "$WORK/https.py" >/dev/null 2>&1 &
HTTPS_PID=$!

cat > "$WORK/config.yaml" <<CONFIG
web:
  address: 127.0.0.1
  port: $PORT
storage:
  type: sqlite
  path: "$WORK/go-uptime.db"
security:
  basic:
    username: $USERNAME
    password-bcrypt-base64: "$PASSWORD_HASH"
admin:
  enabled: true
endpoints:
  - name: site
    group: web
    url: https://127.0.0.1:$TLS_PORT/
    interval: 5s
    client:
      insecure: true
    conditions:
      - "[STATUS] == 200"
  - name: plain
    group: web
    url: $BASE/health
    interval: 5s
    conditions:
      - "[STATUS] == 200"
status-pages:
  rate-limit: 0
  pages:
    - slug: web
      title: "Web"
      groups: [web]
CONFIG

GO_UPTIME_CONFIG_PATH="$WORK/config.yaml" dist/go-uptime > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
admin() { agent-browser --session e2e-certificate-admin "$@"; }
public() { agent-browser --session e2e-certificate-public "$@"; }
cleanup() {
  admin close >/dev/null 2>&1 || true
  public close >/dev/null 2>&1 || true
  kill "$SERVER_PID" "$HTTPS_PID" >/dev/null 2>&1 || true
  wait "$SERVER_PID" "$HTTPS_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

for _ in $(seq 1 60); do
  curl -sf "$BASE/health" >/dev/null && break
  sleep 1
done
curl -sf "$BASE/health" >/dev/null || { echo "Go Uptime did not start"; cat "$WORK/go-uptime.log"; exit 1; }

# Fork: the theme is chosen by the theme cookie, like the theme selector, because the operating system preference is not
# followed (dark by default, see ui.dark-mode). It also applies the theme to the page that is already open.
set_theme() {
  local session=$1 theme=$2
  "$session" cookies set theme "$theme" --url "$BASE" >/dev/null
  "$session" eval "document.cookie = 'theme=$theme; path=/; max-age=31536000; samesite=strict'; (() => { const themes = { dark: ['dark', '#030712'], light: ['', '#f7f9fb'], bio: ['theme-bio', '#f2f8fa'] }; const theme = themes['$theme'] ? '$theme' : 'light'; for (const name in themes) { if (themes[name][0]) { document.documentElement.classList.toggle(themes[name][0], name === theme) } } const meta = document.querySelector('meta[name=\"theme-color\"]'); if (meta) { meta.setAttribute('content', themes[theme][1]) } })()" >/dev/null 2>&1 || true
}

STEP=0
step() {
  STEP=$((STEP + 1))
  echo "==> $STEP. $*"
}
fail() {
  echo "FAILED: $*"
  admin screenshot --full "$PRINTS/error-admin.png" >/dev/null 2>&1 || true
  public screenshot --full "$PRINTS/error-public.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/go-uptime.log"
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
count() {
  "$1" eval "document.querySelectorAll('[data-testid=\"$2\"]').length" 2>/dev/null | tr -d '"'
}
EXPIRES='Certificate expires in 7[23] days'

step "Protected status API with the certificate expiration of the HTTPS check"
for _ in $(seq 1 30); do
  curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/web_site/statuses" | grep -q '"certificateExpiration"' && break
  sleep 1
done
curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/web_site/statuses" | grep -q '"certificateExpiration"' || fail "the HTTPS check has no certificate expiration"
curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/web_plain/statuses" | grep -q '"certificateExpiration"' && fail "the HTTP check should not have a certificate expiration"

step "Dashboard: discreet line below the name, only for the endpoint with TLS"
admin set viewport 1280 900 >/dev/null
# Signs in through the login screen of security.basic
login_screen() {
  admin open "$BASE/login" >/dev/null
  admin wait "$(testid login-username)" >/dev/null || fail "the login screen did not open"
  admin fill "$(testid login-username)" "$USERNAME" >/dev/null
  admin fill "$(testid login-password)" "$PASSWORD" >/dev/null
  admin click "$(testid login-submit)" >/dev/null
  admin wait "$(testid logout-button)" >/dev/null || fail "the login did not work"
}
login_screen
admin open "$BASE/endpoints/web_site" >/dev/null
admin wait "$(testid endpoint-certificate-expiration)" >/dev/null || fail "the dashboard did not show the certificate expiration"
admin get text "$(testid endpoint-certificate-expiration)" | grep -Eq "$EXPIRES · " || fail "unexpected certificate line: $(admin get text "$(testid endpoint-certificate-expiration)")"
admin eval "document.querySelector('[data-testid=\"endpoint-certificate-expiration\"]').className" | grep -q 'text-xs' || fail "the certificate line should be discreet"
admin screenshot "$PRINTS/01-dashboard.png" >/dev/null
admin open "$BASE/endpoints/web_plain" >/dev/null
admin wait 3000 >/dev/null
[ "$(count admin endpoint-certificate-expiration)" = 0 ] || fail "the endpoint without TLS should not show the certificate line"

step "Status page of the configuration file without the option"
[ "$(curl -s "$BASE/api/v1/status-pages/web" | grep -c certificateExpiresInDays)" = 0 ] || fail "the page without the option published the certificate expiration"
public set viewport 1280 900 >/dev/null
public open "$BASE/status/web" >/dev/null
public wait "$(testid status-endpoint-site)" >/dev/null || fail "the web page did not show site"
[ "$(count public status-endpoint-certificate-site)" = 0 ] || fail "the page without the option showed the certificate line"

step "Status page created through the form with Show certificate expiration"
admin open "$BASE/admin/status-pages/new" >/dev/null
admin wait "$(testid status-page-field-slug)" >/dev/null || fail "the form did not open"
admin fill "$(testid status-page-field-slug)" "secure" >/dev/null
admin fill "$(testid status-page-field-title)" "Secure" >/dev/null
admin click "$(testid status-page-group-web)" >/dev/null
admin click "$(testid status-page-field-show-certificate-expiration)" >/dev/null
admin click "$(testid status-page-save)" >/dev/null
admin wait --text "Status page created" >/dev/null || fail "the page was not created"
[ "$(admin eval "document.querySelector('[data-testid=\"status-page-field-show-certificate-expiration\"]').checked")" = true ] || fail "the option was not saved"
admin click "$(testid status-page-field-enabled)" >/dev/null
admin click "$(testid status-page-save)" >/dev/null
admin wait --text "saved and published" >/dev/null || fail "the page was not published"
admin screenshot --full "$PRINTS/02-form.png" >/dev/null
curl -s "$BASE/api/v1/status-pages/secure" | grep -Eq '"certificateExpiresInDays":7[23]' || fail "the public API did not publish the days"

step "Public page and details page with the discreet line"
public open "$BASE/status/secure" >/dev/null
public wait "$(testid status-endpoint-certificate-site)" >/dev/null || fail "the public page did not show the certificate line"
public get text "$(testid status-endpoint-certificate-site)" | grep -Eq "$EXPIRES" || fail "unexpected public certificate line"
[ "$(count public status-endpoint-certificate-plain)" = 0 ] || fail "the endpoint without TLS showed the certificate line"
public screenshot "$PRINTS/03-status-page.png" >/dev/null
public open "$BASE/status/secure/endpoints/web_site" >/dev/null
public wait "$(testid status-endpoint-certificate)" >/dev/null || fail "the details page did not show the certificate line"
public screenshot "$PRINTS/04-details.png" >/dev/null
set_theme public dark
public open "$BASE/status/secure" >/dev/null
public wait "$(testid status-endpoint-certificate-site)" >/dev/null
public screenshot "$PRINTS/05-status-page-dark.png" >/dev/null

echo "==> OK: certificate expiration end-to-end tests passed (screenshots in $PRINTS)"
