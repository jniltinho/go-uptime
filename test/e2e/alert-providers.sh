#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# End-to-end tests of the alerting providers managed through the web (Notifications tab), with agent-browser.
#
#   test/e2e/alert-providers.sh
#
# Starts the locally built Go Uptime (temporary SQLite, basic auth, administration, a Slack provider in the file) and a
# local HTTP receiver, configures a Custom webhook through the form, sends a test that triggers and resolves, attaches
# the provider to a Push endpoint, makes it fail and recover and checks that the receiver got both notifications, and
# that the secret of the provider is never answered nor logged. Screenshots go to dist/prints/alert-providers/ (dist/
# is in .gitignore). Requires agent-browser with Chrome installed and python3.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18095}
RECEIVER_PORT=${E2E_RECEIVER_PORT:-18096}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/alert-providers"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Go Uptime decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'
SECRET='e2e-webhook-secret-7f3a'
PUSH_TOKEN='tok_e2e_alerts_0123456789abcdef'

for command in agent-browser python3 curl; do
  command -v "$command" >/dev/null || { echo "$command not found"; exit 1; }
done
for port in "$PORT" "$RECEIVER_PORT"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "port $port is already in use: stop the process using it or set E2E_PORT and E2E_RECEIVER_PORT"
    exit 1
  fi
done
mkdir -p "$PRINTS"

echo "==> Building"
make -s build

# Receiver of the notifications: one line per request, with the path and the body
cat > "$WORK/receiver.py" <<'PYTHON'
import http.server, sys
class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0))).decode(errors="replace")
        with open(sys.argv[2], "a") as log:
            log.write(f"{self.path} {body}\n")
        self.send_response(200)
        self.end_headers()
    def log_message(self, *args):
        pass
http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
PYTHON
touch "$WORK/received.log"
python3 -I "$WORK/receiver.py" "$RECEIVER_PORT" "$WORK/received.log" &
RECEIVER_PID=$!

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
alerting:
  slack:
    webhook-url: "https://hooks.slack.com/services/T000/B000/file-secret"
endpoints:
  - name: health
    group: core
    url: $BASE/health
    interval: 1m
    conditions:
      - "[STATUS] == 200"
CONFIG

GO_UPTIME_CONFIG_PATH="$WORK/config.yaml" dist/go-uptime > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
browser() { agent-browser --session e2e-alert-providers "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  kill "$SERVER_PID" "$RECEIVER_PID" >/dev/null 2>&1 || true
  wait "$SERVER_PID" "$RECEIVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

for _ in $(seq 1 60); do
  curl -sf "$BASE/health" >/dev/null && break
  sleep 1
done
curl -sf "$BASE/health" >/dev/null || { echo "Go Uptime did not start"; cat "$WORK/go-uptime.log"; exit 1; }

STEP=0
step() {
  STEP=$((STEP + 1))
  echo "==> $STEP. $*"
}
fail() {
  echo "FAILED: $*"
  browser screenshot --full "$PRINTS/error.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/go-uptime.log"
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
wait_for() {
  browser wait "$(testid "$1")" >/dev/null || fail "element $1 did not appear"
}
api() { # method path [body]
  curl -s -u "$USERNAME:$PASSWORD" -X "$1" -H 'Content-Type: application/yaml' ${3:+--data-binary "$3"} "$BASE$2"
}
# Waits until the receiver has at least $1 notifications
wait_received() {
  for _ in $(seq 1 30); do
    [ "$(wc -l < "$WORK/received.log")" -ge "$1" ] && return 0
    sleep 0.5
  done
  fail "expected $1 notifications, got: $(cat "$WORK/received.log")"
}
# A push right after the start may answer 404 until the managed endpoints are loaded
push() {
  for _ in $(seq 1 10); do
    curl -s "$BASE/api/push/$PUSH_TOKEN?status=$1&msg=e2e" | grep -q '"ok":true' && return 0
    sleep 1
  done
  fail "the push was not accepted"
}

step "Sign in through the login screen and open the Notifications tab"
browser set viewport 1280 900 >/dev/null
browser open "$BASE/login" >/dev/null
wait_for login-username
browser fill "$(testid login-username)" "$USERNAME" >/dev/null
browser fill "$(testid login-password)" "$PASSWORD" >/dev/null
browser click "$(testid login-submit)" >/dev/null
wait_for logout-button
browser open "$BASE/admin/alert-providers" >/dev/null
wait_for alert-provider-row-slack
browser screenshot "$PRINTS/01-list.png" >/dev/null

step "The provider of the file is read-only, with its webhook masked"
api GET /api/v1/admin/alert-providers/slack | grep -q '"source":"yaml"' || fail "slack should come from the file"
api GET /api/v1/admin/alert-providers/slack | grep -q 'file-secret' && fail "the webhook of the file was answered"
[ "$(curl -s -o /dev/null -w '%{http_code}' -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/json' -d '{"type":"slack","webhook-url":"https://x"}' "$BASE/api/v1/admin/alert-providers")" = 409 ] || fail "creating a type of the file should answer 409"

step "Add a Custom webhook through the picker and the form"
browser click "$(testid alert-provider-add)" >/dev/null
wait_for alert-provider-picker
browser fill "$(testid alert-provider-search)" "custom" >/dev/null
browser click "$(testid alert-provider-pick-custom)" >/dev/null
wait_for alert-provider-field-url
browser fill "$(testid alert-provider-field-url)" "http://127.0.0.1:$RECEIVER_PORT/hook?token=$SECRET" >/dev/null
browser fill "$(testid alert-provider-field-method)" "POST" >/dev/null
browser fill "$(testid alert-provider-field-body)" '[ALERT_TRIGGERED_OR_RESOLVED] [ENDPOINT_NAME]' >/dev/null
browser fill "$(testid alert-provider-failure-threshold)" "2" >/dev/null
browser click "$(testid alert-provider-send-on-resolved) button" >/dev/null
browser click "$(testid alert-provider-send-on-resolved) [role=option]:nth-child(2)" >/dev/null
browser screenshot --full "$PRINTS/02-form.png" >/dev/null

step "Send test: the notification and its resolution reach the receiver, nothing is stored"
browser click "$(testid alert-provider-send-test)" >/dev/null
wait_for alert-provider-test-result
wait_received 2
grep -q 'TRIGGERED Go Uptime test' "$WORK/received.log" || fail "the test notification did not arrive"
grep -q 'RESOLVED Go Uptime test' "$WORK/received.log" || fail "the resolution of the test did not arrive"
api GET /api/v1/admin/alert-providers/custom | grep -q '"source":"none"' || fail "a test must not store the provider"

step "Create: the provider is listed as Web and valid, its URL masked"
browser click "$(testid admin-save)" >/dev/null
wait_for alert-provider-configured-url
detail=$(api GET /api/v1/admin/alert-providers/custom)
echo "$detail" | grep -q '"source":"web"' || fail "the provider was not created"
echo "$detail" | grep -q "$SECRET" && fail "the URL of the provider was answered"
browser screenshot --full "$PRINTS/03-edit.png" >/dev/null

step "A Push endpoint with a custom alert notifies when it fails and when it recovers"
: > "$WORK/received.log"
api POST /api/v1/admin/endpoints "$(printf 'type: push\nname: nightly\ngroup: jobs\ntoken: %s\nheartbeat:\n  interval: 1h\nalerts:\n  - type: custom\n' "$PUSH_TOKEN")" | grep -q '"key":"jobs_nightly"' || fail "the endpoint was not created"
push down
push down
wait_received 1
grep -q 'TRIGGERED nightly' "$WORK/received.log" || fail "the alert did not trigger after the inherited 2 failures"
push up
push up
wait_received 2
grep -q 'RESOLVED nightly' "$WORK/received.log" || fail "the alert was not resolved"

step "A provider in use cannot be removed"
browser open "$BASE/admin/alert-providers" >/dev/null
wait_for alert-provider-remove-custom
browser click "$(testid alert-provider-remove-custom)" >/dev/null
wait_for alert-provider-in-use
browser screenshot "$PRINTS/04-in-use.png" >/dev/null

step "The secret never reached the log"
grep -q "$SECRET" "$WORK/go-uptime.log" && fail "the secret of the provider was logged"

echo "OK: alerting providers"
