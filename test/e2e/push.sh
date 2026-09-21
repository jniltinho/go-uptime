#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# End-to-end tests of the push monitoring and of its administration screens, with agent-browser.
#
#   test/e2e/push.sh
#
# Starts the locally built Go Uptime (temporary SQLite, basic auth, administration enabled), creates a global push key, a
# push endpoint and an active endpoint that accepts push through the administration screens, sends pushes with curl in
# the format of the Uptime Kuma and saves screenshots in dist/prints/push/ (dist/ is in .gitignore). Requires
# agent-browser with Chrome installed.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18092}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/push"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Go Uptime decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'
YAML_KEY='e2e-yaml-global-key-0123456789'
YAML_ENDPOINT_TOKEN='e2e-health-token'

command -v agent-browser >/dev/null || { echo "agent-browser not found"; exit 1; }
mkdir -p "$PRINTS"

STORAGE_TYPE=${E2E_STORAGE_TYPE:-sqlite}
STORAGE_PATH=${E2E_STORAGE_PATH:-$WORK/go-uptime.db}

echo "==> Building"
make -s build

cat > "$WORK/config.yaml" <<CONFIG
web:
  address: 127.0.0.1
  port: $PORT
storage:
  type: $STORAGE_TYPE
  path: "$STORAGE_PATH"
security:
  basic:
    username: $USERNAME
    password-bcrypt-base64: "$PASSWORD_HASH"
admin:
  enabled: true
push:
  keys:
    - name: yaml-key
      token: $YAML_KEY
  endpoints:
    - key: core_health
      token: $YAML_ENDPOINT_TOKEN
endpoints:
  - name: health
    group: core
    url: $BASE/health
    interval: 5s
    conditions:
      - "[STATUS] == 200"
# Fork: without pushes, the heartbeat records a pending result before each failure
external-endpoints:
  - name: late
    group: jobs
    token: late-job-token-000000000000000000
    heartbeat:
      interval: 10s
      retries: 1
CONFIG

GO_UPTIME_CONFIG_PATH="$WORK/config.yaml" dist/go-uptime > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
# Fork: --hide-scrollbars false because headless Chromium hides the native scrollbars by default, and the thin
# scrollbar of the theme is measured in this script
admin() { agent-browser --session e2e-push-admin --hide-scrollbars false "$@"; }
cleanup() {
  admin close >/dev/null 2>&1 || true
  kill "$SERVER_PID" >/dev/null 2>&1 || true
  wait "$SERVER_PID" 2>/dev/null || true
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
  admin screenshot --full "$PRINTS/error.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/go-uptime.log"
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
push() {
  curl -s "$@"
}
authenticated() {
  curl -s -u "$USERNAME:$PASSWORD" "$@"
}

step "Push to an endpoint of the configuration file, with its token and with the global key of the file"
push "$BASE/api/push/$YAML_ENDPOINT_TOKEN?status=up&msg=OK&ping=12" | grep -q '"ok":true' || fail "push with the token of the endpoint was not accepted"
push "$BASE/api/push/$YAML_KEY/core_health?status=down&msg=from-yaml-key" | grep -q '"ok":true' || fail "push with the global key of the file was not accepted"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/push/wrong-token-000")" = 404 ] || fail "expected 404 for an unknown token"

step "Push keys tab: the key of the file is read-only and a key is created through the web"
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
admin open "$BASE/admin/push-keys" >/dev/null
admin wait "$(testid push-key-row-config-yaml-key)" >/dev/null || fail "the key of the file is not listed"
admin fill "$(testid push-key-name)" "akamai" >/dev/null
admin click "$(testid push-key-create)" >/dev/null
admin wait "$(testid push-key-token)" >/dev/null || fail "the created key was not shown"
GLOBAL_KEY=$(admin get value "$(testid push-key-token)")
[ ${#GLOBAL_KEY} -eq 32 ] || fail "the created key does not have 32 characters"
admin wait "$(testid push-key-row-admin-akamai)" >/dev/null || fail "the created key is not listed"
[ "$(admin eval "document.documentElement.scrollHeight <= window.innerHeight + 1" | tr -d '"')" = true ] || fail "the push keys page should fit the window, with the scroll inside its table"
admin screenshot "$PRINTS/01-push-keys.png" >/dev/null
authenticated "$BASE/api/v1/admin/push-keys" | grep -q "$GLOBAL_KEY" && fail "the list of keys exposed the token"

step "New push endpoint: type Push, generated token, push URL and no Test button"
admin open "$BASE/admin/endpoints/new" >/dev/null
admin wait "$(testid admin-field-type)" >/dev/null
admin click "$(testid admin-field-type) button" >/dev/null
admin click "$(testid admin-type-push)" >/dev/null
admin wait "$(testid admin-push-url)" >/dev/null || fail "the push URL was not shown"
admin fill "$(testid admin-field-name)" "backup" >/dev/null
admin click "$(testid admin-field-group-select) button" >/dev/null
admin click "$(testid admin-group-new)" >/dev/null
admin fill "$(testid admin-field-group)" "jobs" >/dev/null
admin fill "$(testid admin-field-heartbeat)" "10m" >/dev/null
PUSH_TOKEN=$(admin get value "$(testid admin-field-push-token)")
[ ${#PUSH_TOKEN} -eq 32 ] || fail "the push token was not generated"
admin get value "$(testid admin-push-url)" | grep -q "/api/push/$PUSH_TOKEN?status=up&msg=OK&ping=" || fail "unexpected push URL"
[ "$(admin eval "document.querySelectorAll('[data-testid=\"admin-test\"]').length")" = 0 ] || fail "the Test button was shown for a push endpoint"
admin click "$(testid admin-mode-yaml)" >/dev/null
admin get value "$(testid admin-yaml)" | grep -q "type: push" || fail "the YAML does not contain type: push"
admin get value "$(testid admin-yaml)" | grep -q "url:" && fail "the YAML of a push endpoint contains url"
admin click "$(testid admin-mode-form)" >/dev/null
admin wait "$(testid admin-push-url)" >/dev/null
admin screenshot --full "$PRINTS/02-push-endpoint-form.png" >/dev/null
admin click "$(testid admin-save)" >/dev/null
admin wait "$(testid admin-row-jobs_backup)" >/dev/null || fail "the push endpoint was not created"

step "Pushes to the new endpoint, with its token and with the created global key"
push "$BASE/api/push/$PUSH_TOKEN?status=up&msg=backup%20ok&ping=250" | grep -q '"ok":true' || fail "push with the token was not accepted"
push -X POST "$BASE/api/push/$GLOBAL_KEY/jobs_backup?status=down&msg=disk%20full" | grep -q '"ok":true' || fail "push with the global key was not accepted"
authenticated "$BASE/api/v1/endpoints/jobs_backup/statuses" | grep -q "disk full" || fail "the message of the push is not in the history"

step "Active endpoint with Accept push"
admin open "$BASE/admin/endpoints/new" >/dev/null
admin wait "$(testid admin-field-name)" >/dev/null
admin fill "$(testid admin-field-name)" "cdn" >/dev/null
admin fill "$(testid admin-field-url)" "$BASE/health" >/dev/null
admin fill "$(testid admin-field-interval)" "5s" >/dev/null
admin click "$(testid admin-field-accept-push)" >/dev/null
admin wait "$(testid admin-push-url)" >/dev/null || fail "the push URL of the active endpoint was not shown"
ACTIVE_TOKEN=$(admin get value "$(testid admin-field-push-token)")
[ ${#ACTIVE_TOKEN} -eq 32 ] || fail "the push token of the active endpoint was not generated"
admin screenshot --full "$PRINTS/03-active-endpoint-accept-push.png" >/dev/null
admin click "$(testid admin-save)" >/dev/null
admin wait "$(testid admin-accepts-push-_cdn)" >/dev/null || fail "the list does not mark the active endpoint that accepts push"
admin wait "$(testid admin-accepts-push-core_health)" >/dev/null || fail "the list does not mark the endpoint of the file that accepts push"
[ "$(admin eval "document.documentElement.scrollHeight <= window.innerHeight + 1" | tr -d '"')" = true ] || fail "the endpoints page should fit the window, with the scroll inside its table"
admin screenshot "$PRINTS/04-endpoints.png" >/dev/null
push "$BASE/api/push/$ACTIVE_TOKEN?status=up&msg=akamai" | grep -q '"ok":true' || fail "push to the active endpoint was not accepted"
push "$BASE/api/push/$GLOBAL_KEY/_cdn?status=up&msg=akamai-global" | grep -q '"ok":true' || fail "push with the global key to the active endpoint was not accepted"

step "Editing: the token is shown, the type cannot become active and the push URL is kept"
admin open "$BASE/admin/endpoints/jobs_backup/edit" >/dev/null
admin wait "$(testid admin-push-toggle)" >/dev/null || fail "the push block was not shown when editing"
[ "$(admin eval "document.querySelectorAll('[data-testid=\"admin-push-url\"]').length" | tr -d '"')" = 0 ] || fail "the push block should start collapsed when editing"
admin get text "$(testid admin-push-summary)" | grep -q "Token …${PUSH_TOKEN: -4}" || fail "the collapsed push block does not summarize the token"
admin click "$(testid admin-push-toggle)" >/dev/null
admin wait "$(testid admin-push-url)" >/dev/null || fail "the push URL was not shown when editing"
[ "$(admin eval "Math.abs(document.querySelector('[data-testid=\"admin-generate-push-token\"]').getBoundingClientRect().height - document.querySelector('[data-testid=\"admin-field-push-token\"]').getBoundingClientRect().height)" | tr -d '"')" = 0 ] || fail "the Generate token button does not have the height of the input"
[ "$(admin eval "Math.abs(document.querySelector('[data-testid=\"admin-copy-push-url\"]').getBoundingClientRect().height - document.querySelector('[data-testid=\"admin-push-url\"]').getBoundingClientRect().height)" | tr -d '"')" = 0 ] || fail "the Copy button does not have the height of the push URL"
[ "$(admin get value "$(testid admin-field-push-token)")" = "$PUSH_TOKEN" ] || fail "the token shown when editing is different"
admin click "$(testid admin-field-type) button" >/dev/null
[ "$(admin eval "document.querySelectorAll('[data-testid=\"admin-type-http\"]').length")" = 0 ] || fail "an active type was offered for a push endpoint"
admin screenshot --full "$PRINTS/05-push-endpoint-edit.png" >/dev/null

step "Push endpoint with the token of an Uptime Kuma monitor and the Recent checks table"
KUMA_TOKEN='keSDu7G855jvVat1xWiY2Gk4CkL1End5'
admin open "$BASE/admin/endpoints/new" >/dev/null
admin wait "$(testid admin-field-type)" >/dev/null
admin click "$(testid admin-field-type) button" >/dev/null
admin click "$(testid admin-type-push)" >/dev/null
admin wait "$(testid admin-field-push-token)" >/dev/null
admin fill "$(testid admin-field-name)" "kuma-backup" >/dev/null
admin fill "$(testid admin-field-push-token)" "$KUMA_TOKEN" >/dev/null
admin get value "$(testid admin-push-url)" | grep -q "/api/push/$KUMA_TOKEN?status=up&msg=OK&ping=" || fail "the push URL does not use the pasted token"
admin click "$(testid admin-save)" >/dev/null
admin wait "$(testid admin-row-_kuma-backup)" >/dev/null || fail "the push endpoint with the pasted token was not created"
for message in "status=up&msg=Backup OK" "status=down&msg=Falha no backup: disco cheio" "status=up&msg=Backup recuperado"; do
  push -G "$BASE/api/push/$KUMA_TOKEN" --data-urlencode "${message%%&*}" --data-urlencode "${message#*&}" | grep -q '"ok":true' || fail "push '$message' was not accepted"
  sleep 0.2
done
selector_count() {
  admin eval "document.querySelectorAll('[data-testid=\"$1\"]').length" 2>/dev/null | tr -d '"'
}
admin open "$BASE/endpoints/jobs_backup" >/dev/null
admin wait "$(testid response-time-trend)" >/dev/null || fail "the Response Time Trend chart is not shown"
admin wait "$(testid recent-checks-card)" >/dev/null
top_of() {
  admin eval "Math.round(document.querySelector('[data-testid=\"$1\"]').getBoundingClientRect().top + window.scrollY)" 2>/dev/null | tr -d '"'
}
# Same order as the monitor page of the Uptime Kuma: heartbeat bars, numbers, chart and table of checks
BARS_TOP=$(top_of recent-checks-card)
CHART_TOP=$(top_of response-time-trend)
TABLE_TOP=$(top_of checks-table-card)
[ "$BARS_TOP" -lt "$CHART_TOP" ] && [ "$CHART_TOP" -lt "$TABLE_TOP" ] || fail "expected the bars, the chart and the table of checks in this order ($BARS_TOP, $CHART_TOP, $TABLE_TOP)"
[ "$(selector_count recent-checks-table)" = 0 ] || fail "the checks table should start collapsed"
admin wait 2000 >/dev/null
admin screenshot --full "$PRINTS/06-kuma-order.png" >/dev/null
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid recent-checks-toggle)" >/dev/null || fail "the toggle of the checks table is not shown"
[ "$(selector_count recent-checks-table)" = 0 ] || fail "the checks table should start collapsed"
admin click "$(testid recent-checks-toggle)" >/dev/null
admin wait "$(testid recent-check-2)" >/dev/null || fail "the Recent checks table does not have the 3 pushes"
recent_message() {
  admin get text "$(testid "recent-check-$1") $(testid recent-check-message)"
}
[ "$(recent_message 0)" = "Backup recuperado" ] || fail "unexpected most recent check: $(recent_message 0)"
[ "$(recent_message 1)" = "Falha no backup: disco cheio" ] || fail "unexpected second check: $(recent_message 1)"
[ "$(recent_message 2)" = "Backup OK" ] || fail "unexpected third check: $(recent_message 2)"
admin get text "$(testid recent-check-1)" | grep -q "Down" || fail "the failure is not shown as Down"
admin get text "$(testid recent-check-1)" | grep -q "Push" || fail "the origin is not shown as Push"
admin scrollintoview "$(testid recent-checks-table)" >/dev/null
admin screenshot "$PRINTS/06-recent-checks.png" >/dev/null
set_theme admin dark
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid recent-check-2)" >/dev/null
admin scrollintoview "$(testid recent-checks-table)" >/dev/null
admin screenshot "$PRINTS/07-recent-checks-dark.png" >/dev/null
admin open "$BASE/admin/push-keys" >/dev/null
admin wait "$(testid push-keys-table)" >/dev/null
admin screenshot --full "$PRINTS/08-push-keys-dark.png" >/dev/null
set_theme admin light

step "Pending: status=pending in yellow, retries of the heartbeat and panel of numbers with Ping"
push "$BASE/api/push/$KUMA_TOKEN?status=down&msg=Queda&ping=90" | grep -q '"ok":true' || fail "the down push was not accepted"
sleep 1
push "$BASE/api/push/$KUMA_TOKEN?status=up&msg=Voltou&ping=40" | grep -q '"ok":true' || fail "the up push was not accepted"
push "$BASE/api/push/$KUMA_TOKEN?status=pending&msg=Backup%20em%20andamento" | grep -q '"ok":true' || fail "the pending push was not accepted"
authenticated "$BASE/api/v1/endpoints/_kuma-backup/statuses" | python3 -c '
import json, sys
status = json.load(sys.stdin)
last = status["results"][-1]
sys.exit(0 if last.get("pending") and not last["success"] and last.get("message") == "Backup em andamento" and status.get("push") is True else 1)
' || fail "the pending push is not a pending result of a push endpoint in the API"
# The heartbeat of jobs_late ran without pushes since the start: Pending, then failures, and a single UNHEALTHY event
for _ in $(seq 1 30); do
  authenticated "$BASE/api/v1/endpoints/jobs_late/statuses" | python3 -c '
import json, sys
status = json.load(sys.stdin)
results = status["results"]
sys.exit(0 if len(results) >= 2 and results[0].get("pending") and not results[1].get("pending") and not results[1]["success"] else 1)
' && break
  sleep 1
done
authenticated "$BASE/api/v1/endpoints/jobs_late/statuses" | python3 -c '
import json, sys
status = json.load(sys.stdin)
results, events = status["results"], [event["type"] for event in status["events"]]
ok = len(results) >= 2 and results[0].get("pending") and results[0].get("message", "").startswith("heartbeat: no update received within") and not results[0].get("errors")
ok = ok and not results[1].get("pending") and not results[1]["success"] and events == ["START", "UNHEALTHY"]
sys.exit(0 if ok else 1)
' || fail "expected Pending, then a failure with a single UNHEALTHY event for the heartbeat with retries"
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid details-summary)" >/dev/null || fail "the panel of numbers is not shown"
admin get text "$(testid details-summary)" | grep -q "Ping (Current)" || fail "the panel of a push endpoint should show Ping"
[ "$(top_of recent-checks-card)" -lt "$(top_of details-summary)" ] && [ "$(top_of details-summary)" -lt "$(top_of response-time-trend)" ] || fail "expected the panel of numbers between the bars and the chart"
[ "$(selector_count recent-checks-table)" = 0 ] && admin click "$(testid recent-checks-toggle)" >/dev/null
admin wait "$(testid recent-check-0)" >/dev/null
admin get text "$(testid recent-check-0)" | grep -q "Pending" || fail "the pending push is not shown as Pending"
[ "$(admin eval "document.querySelector('[data-testid=\"recent-check-0\"]').innerHTML.includes('yellow')" 2>/dev/null | tr -d '"')" = true ] || fail "the Pending badge is not yellow"
admin wait 2000 >/dev/null
admin screenshot --full "$PRINTS/09-pending-light.png" >/dev/null
set_theme admin dark
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid details-summary)" >/dev/null
admin wait 2000 >/dev/null
admin screenshot --full "$PRINTS/10-pending-dark.png" >/dev/null
admin open "$BASE/" >/dev/null
admin wait "$(testid dashboard-summary-pending)" >/dev/null || fail "the dashboard summary does not count the pending endpoint"
admin screenshot "$PRINTS/11-dashboard-pending-dark.png" >/dev/null
set_theme admin light

step "Real time: a pending push shows up on the details page without reloading it"
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid details-summary)" >/dev/null
# The events of the dashboard are collapsed by default, like the Checks table
admin wait "$(testid events-toggle)" >/dev/null || fail "the events are not shown on the details page"
[ "$(selector_count events-list)" = 0 ] || fail "the events of the dashboard should be collapsed by default"
admin click "$(testid events-toggle)" >/dev/null
admin wait "$(testid events-list)" >/dev/null || fail "the events of the dashboard did not expand"
admin scrollintoview "$(testid events-list)" >/dev/null
admin screenshot "$PRINTS/12-events-expanded.png" >/dev/null
admin click "$(testid events-toggle)" >/dev/null
admin scrollintoview "$(testid details-summary)" >/dev/null
[ "$(selector_count recent-checks-table)" = 0 ] && admin click "$(testid recent-checks-toggle)" >/dev/null
admin wait "$(testid recent-check-0)" >/dev/null
# The marker only survives if the page is not reloaded
admin eval "window.__e2eNoReload = true" >/dev/null
admin wait 1500 >/dev/null
push "$BASE/api/push/$KUMA_TOKEN?status=pending&msg=Realtime%20pending" | grep -q '"ok":true' || fail "the real-time pending push was not accepted"
shown=false
for _ in $(seq 1 10); do
  if admin get text "$(testid recent-check-0)" 2>/dev/null | grep -q "Realtime pending"; then
    shown=true
    break
  fi
  sleep 0.5
done
[ "$shown" = true ] || fail "the pending push did not show up on the details page within 5 seconds"
[ "$(admin eval "window.__e2eNoReload === true" 2>/dev/null | tr -d '"')" = true ] || fail "the details page was reloaded"
admin get text "$(testid recent-check-0)" | grep -q "Pending" || fail "the real-time push is not shown as Pending"
# Chart in the format of the Uptime Kuma: the Down push is a red column and the Pending pushes are yellow columns
chart_data() {
  admin eval "document.querySelector('[data-testid=\"response-time-chart\"]').dataset.$1" 2>/dev/null | tr -d '"'
}
for _ in $(seq 1 10); do
  [ "$(chart_data pendingColumns)" -ge 2 ] && break
  sleep 0.5
done
[ "$(chart_data period)" = recent ] || fail "the chart did not open in Recent"
[ "$(chart_data pendingColumns)" -ge 2 ] || fail "the pending pushes are not yellow columns of the chart"
[ "$(chart_data downColumns)" -ge 1 ] || fail "the down push is not a red column of the chart"
[ "$(chart_data linePoints)" -ge 1 ] || fail "the chart has no line point"
admin wait 2000 >/dev/null
admin screenshot --full "$PRINTS/12-realtime-pending.png" >/dev/null
admin screenshot "$PRINTS/12-realtime-pending-viewport.png" >/dev/null

step "Chart periods: 24h and 1w with the aggregates, remembered after a reload"
admin network requests --clear >/dev/null 2>&1 || true
admin eval "(() => { const select = document.querySelector('[data-testid=\"response-time-chart-period\"]'); select.value = '24h'; select.dispatchEvent(new Event('change')) })()" >/dev/null
admin wait "[data-testid=\"response-time-chart\"][data-period=\"24h\"]" >/dev/null || fail "the chart did not load the 24h period"
# The down, up and pending pushes share a minute, which is a yellow (mixed) column
[ "$(( $(chart_data downColumns) + $(chart_data pendingColumns) ))" -ge 1 ] || fail "the minute of the down and pending pushes is not a column"
admin eval "(() => { const select = document.querySelector('[data-testid=\"response-time-chart-period\"]'); select.value = '1w'; select.dispatchEvent(new Event('change')) })()" >/dev/null
admin wait "[data-testid=\"response-time-chart\"][data-period=\"1w\"]" >/dev/null || fail "the chart did not load the 1w period"
requests=$(admin network requests 2>/dev/null)
grep -q "/api/v1/endpoints/_kuma-backup/response-time-chart?period=24h" <<<"$requests" || fail "the 24h chart was not requested"
grep -q "/api/v1/endpoints/_kuma-backup/response-time-chart?period=1w" <<<"$requests" || fail "the 1w chart was not requested"
authenticated "$BASE/api/v1/endpoints/_kuma-backup/response-time-chart?period=1w" | python3 -c '
import json, sys
chart = json.load(sys.stdin)
buckets = chart["buckets"]
sys.exit(0 if chart["bucketSeconds"] == 3600 and buckets and sum(b["pending"] for b in buckets) >= 2 and sum(b["down"] for b in buckets) >= 1 else 1)
' || fail "the 1w chart does not have the pending and down pushes"
admin reload >/dev/null
admin wait "[data-testid=\"response-time-chart\"][data-period=\"1w\"]" >/dev/null || fail "the period of the chart was not remembered after a reload"
admin wait 2000 >/dev/null
admin screenshot "$PRINTS/13-chart-1w.png" >/dev/null
# Fork: legend of the series of the dashboard, with the Pending column of this endpoint
LEGEND=$(admin eval "Array.from(document.querySelectorAll('[data-testid=\"response-time-chart-legend\"] [data-series]')).map((item) => item.dataset.series).join(',')" 2>/dev/null | tr -d '"')
case "$LEGEND" in
  average,minimum,maximum*pending) ;;
  *) fail "unexpected legend of the 1w chart: $LEGEND" ;;
esac
# The legend does not change the height of the chart and is not inside the element that carries it
[ "$(admin eval "document.querySelector('[data-testid=\"response-time-chart\"]').offsetHeight" 2>/dev/null | tr -d '"')" = 250 ] || fail "the chart lost the height of the Uptime Kuma"
[ "$(admin eval "document.querySelector('[data-testid=\"response-time-chart\"]').contains(document.querySelector('[data-testid=\"response-time-chart-legend\"]'))" 2>/dev/null | tr -d '"')" = "false" ] || fail "the legend should be outside of the element with the height of the chart"
set_theme admin dark
admin eval "(() => { const select = document.querySelector('[data-testid=\"response-time-chart-period\"]'); select.value = 'recent'; select.dispatchEvent(new Event('change')) })()" >/dev/null
admin wait "[data-testid=\"response-time-chart\"][data-period=\"recent\"]" >/dev/null
admin wait 2000 >/dev/null
admin screenshot "$PRINTS/14-chart-recent-dark.png" >/dev/null
set_theme admin light

step "Thin scrollbar: 10 px, square and in the colors of the theme"
# The rules are inside @media not all and (pointer: coarse): on a touch screen the browser keeps the scrollbar of the
# system. Headless Chromium has no pointing device at all and reports pointer: none, so it is styled like a desktop.
[ "$(admin eval "matchMedia('(pointer: coarse)').matches" | tr -d '"')" = "false" ] || fail "the browser reports a coarse pointer, the rules of the scrollbar do not apply"
thumb_color() {
  admin eval "getComputedStyle(document.documentElement).getPropertyValue('--scrollbar-thumb').trim()" 2>/dev/null | tr -d '"'
}
# The scrollbar only exists where the content overflows, so the window is made short enough for the panel of the list
admin open "$BASE/admin" >/dev/null
admin wait "$(testid admin-list-scroll)" >/dev/null || fail "the list of endpoints did not open"
admin set viewport 1280 300 >/dev/null
vertical_scrollbar() {
  admin eval "(() => { const el = document.querySelector('[data-testid=\"$1\"]'); if (!el) { return 'missing' } if (el.scrollHeight <= el.clientHeight) { return 'no-overflow' } const style = getComputedStyle(el); const borders = parseFloat(style.borderLeftWidth) + parseFloat(style.borderRightWidth); return el.offsetWidth - el.clientWidth - borders })()" 2>/dev/null | tr -d '"'
}
VERTICAL=$(vertical_scrollbar admin-list-scroll)
# 10 px is the value of --scrollbar-size; 11 px would mean that Chromium fell back to scrollbar-width: thin
case "$VERTICAL" in
  9 | 10) ;;
  *) fail "expected a vertical scrollbar of 9 to 10 px in the list of endpoints, got: $VERTICAL" ;;
esac
[ "$(thumb_color)" = "215.4 16.3% 46.9%" ] || fail "unexpected color of the thumb in the light theme: $(thumb_color)"
admin screenshot "$PRINTS/15-scrollbar-light.png" >/dev/null
set_theme admin dark
admin open "$BASE/admin" >/dev/null
admin wait "$(testid admin-list-scroll)" >/dev/null
[ "$(thumb_color)" = "215 20.2% 65.1%" ] || fail "unexpected color of the thumb in the dark theme: $(thumb_color)"
admin screenshot "$PRINTS/16-scrollbar-dark.png" >/dev/null
set_theme admin light
# Horizontal scrollbar: the table of checks in a narrow window
admin set viewport 420 900 >/dev/null
admin open "$BASE/endpoints/_kuma-backup" >/dev/null
admin wait "$(testid recent-checks-toggle)" >/dev/null
[ "$(selector_count recent-checks-table)" = 0 ] && admin click "$(testid recent-checks-toggle)" >/dev/null
admin wait "$(testid recent-check-2)" >/dev/null || fail "the table of checks did not open"
# offsetWidth/offsetHeight include the borders of the element, which are discounted to get the scrollbar alone
HORIZONTAL=$(admin eval "(() => { const el = document.querySelector('[data-testid=\"recent-checks-table\"]'); if (!el) { return 'missing' } if (el.scrollWidth <= el.clientWidth) { return 'no-overflow' } const style = getComputedStyle(el); const borders = parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth); return el.offsetHeight - el.clientHeight - borders })()" 2>/dev/null | tr -d '"')
case "$HORIZONTAL" in
  9 | 10) ;;
  *) fail "expected a horizontal scrollbar of 9 to 10 px in the table of checks, got: $HORIZONTAL" ;;
esac
admin screenshot "$PRINTS/17-scrollbar-horizontal.png" >/dev/null
# Back to the window of the rest of the script
admin set viewport 1280 900 >/dev/null

step "Admin lists: rows of the same height and actions as icons"
# Fork: measured at 1000 and 900 px, where the Type column is tight — at 1280 px the push badge fits and the defect hides
for width in 1000 900; do
  admin set viewport "$width" 800 >/dev/null
  admin open "$BASE/admin" >/dev/null
  admin wait "$(testid admin-accepts-push-core_health)" >/dev/null || fail "the badge of the endpoint that receives push is missing at $width px"
  ROWS=$(admin eval "(() => { const rows = Array.from(document.querySelectorAll('[data-testid^=\"admin-row-\"]')).map((row) => Math.round(row.getBoundingClientRect().height)); return JSON.stringify({ same: new Set(rows).size === 1, tallest: Math.max(...rows), count: rows.length }) })()" 2>/dev/null | tr -d '\\"')
  grep -q "same:true" <<<"$ROWS" || fail "the rows of the list of endpoints have different heights at $width px: $ROWS"
  TALLEST=$(sed -n 's/.*tallest:\([0-9]*\).*/\1/p' <<<"$ROWS")
  [ "$TALLEST" -le 32 ] || fail "the rows of the list of endpoints are ${TALLEST} px tall at $width px, more than the 32 px of the requirement"
done
admin set viewport 1280 900 >/dev/null
admin open "$BASE/admin" >/dev/null
admin wait "$(testid admin-table)" >/dev/null
# The actions are icons: accessible name with the action and the name of the endpoint, and no visible text
ACTIONS=$(admin eval "(() => { const ids = ['admin-open-core_health', 'admin-toggle-jobs_backup', 'admin-remove-jobs_backup']; return JSON.stringify(ids.map((id) => { const el = document.querySelector('[data-testid=\"' + id + '\"]'); if (!el) { return id + ':missing' } const label = el.getAttribute('aria-label') || ''; return (label.length > 0 && el.textContent.trim() === '') ? 'ok' : id + ':' + label + '/' + el.textContent.trim() })) })()" 2>/dev/null | tr -d '\\"')
grep -q "ok,ok,ok" <<<"$ACTIONS" || fail "the actions of the list are not icons with an accessible name: $ACTIONS"
grep -q "health" <<<"$(admin eval "document.querySelector('[data-testid=\"admin-open-core_health\"]').getAttribute('aria-label')" 2>/dev/null)" || fail "the accessible name of the action does not have the name of the endpoint"

step "Push keys list without horizontal scrolling"
# Fork: the same widths as the other lists; the key created through the web is the row with the Revoke action
for width in 1100 900 820; do
  admin set viewport "$width" 800 >/dev/null
  admin open "$BASE/admin/push-keys" >/dev/null
  admin wait "$(testid push-keys-table)" >/dev/null || fail "the list of push keys did not open at $width px"
  FITS=$(admin eval "(() => { const panel = document.querySelector('[data-testid=\"admin-list-scroll\"]'); const action = document.querySelector('[data-testid=\"push-key-revoke-akamai\"]'); return panel.scrollWidth <= panel.clientWidth && document.documentElement.scrollWidth <= innerWidth + 1 && Boolean(action) && action.getBoundingClientRect().right <= innerWidth + 1 })()" 2>/dev/null | tr -d '"')
  [ "$FITS" = "true" ] || fail "the list of push keys has horizontal scrolling or actions out of the window at $width px"
done
for width in 700 390 360; do
  admin set viewport "$width" 800 >/dev/null
  admin open "$BASE/admin/push-keys" >/dev/null
  admin wait "$(testid push-key-card-admin-akamai)" >/dev/null || fail "the list of push keys is not in cards at $width px"
  CARDS=$(admin eval "(() => { const table = document.querySelector('[data-testid=\"push-keys-table\"]'); return (!table || getComputedStyle(table).display === 'none') && document.documentElement.scrollWidth <= innerWidth + 1 && Boolean(document.querySelector('[data-testid=\"push-key-revoke-akamai\"]')) })()" 2>/dev/null | tr -d '"')
  [ "$CARDS" = "true" ] || fail "the cards of the push keys at $width px still have a table, horizontal scrolling or no actions"
done
admin set viewport 1280 900 >/dev/null
admin open "$BASE/admin/push-keys" >/dev/null
admin wait "$(testid push-keys-table)" >/dev/null
# The rows of the keys also have the same height, with and without the revoke action
KEYS=$(admin eval "(() => { const rows = Array.from(document.querySelectorAll('[data-testid^=\"push-key-row-\"]')).map((row) => Math.round(row.getBoundingClientRect().height)); const action = document.querySelector('[data-testid=\"push-key-revoke-akamai\"]'); return JSON.stringify({ same: new Set(rows).size === 1, tallest: Math.max(...rows), label: action ? action.getAttribute('aria-label') : '', text: action ? action.textContent.trim() : 'missing' }) })()" 2>/dev/null | tr -d '\\"')
grep -q "same:true" <<<"$KEYS" || fail "the rows of the list of push keys have different heights: $KEYS"
grep -q "label:Revoke akamai" <<<"$KEYS" || fail "the revoke action has no accessible name: $KEYS"
grep -q "text:}" <<<"$KEYS" || fail "the revoke action still has visible text: $KEYS"

step "Revoking the created key"
admin open "$BASE/admin/push-keys" >/dev/null
admin wait "$(testid push-key-revoke-akamai)" >/dev/null
admin click "$(testid push-key-revoke-akamai)" >/dev/null
admin wait "$(testid confirm-accept)" >/dev/null
admin click "$(testid confirm-accept)" >/dev/null
admin wait --text "revoked" >/dev/null || fail "the key was not revoked"
push "$BASE/api/push/$GLOBAL_KEY/jobs_backup?status=up" | grep -q '"ok":false' || fail "push with the revoked key was accepted"

echo "==> OK: push monitoring end-to-end tests passed (screenshots in $PRINTS)"
