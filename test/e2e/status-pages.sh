#!/usr/bin/env bash
# End-to-end tests of the public status pages and of their administration screens, with agent-browser.
#
#   test/e2e/status-pages.sh
#
# Starts the locally built Go Uptime (temporary SQLite, basic auth, administration enabled, local endpoints), opens the
# public page in a session without credentials and the administration screens in a session with credentials, and saves
# screenshots in dist/prints/status-pages/ (dist/ is in .gitignore). Requires agent-browser with Chrome installed.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18091}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/status-pages"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Go Uptime decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'

command -v agent-browser >/dev/null || { echo "agent-browser not found"; exit 1; }
mkdir -p "$PRINTS"

# Storage: temporary SQLite by default. E2E_STORAGE_TYPE and E2E_STORAGE_PATH run the script with another database,
# which must be empty (e.g. E2E_STORAGE_TYPE=mysql E2E_STORAGE_PATH='root:password@tcp(127.0.0.1:53307)/go_uptime_e2e')
STORAGE_TYPE=${E2E_STORAGE_TYPE:-sqlite}
STORAGE_PATH=${E2E_STORAGE_PATH:-$WORK/go-uptime.db}

echo "==> Building"
make -s build

# Fork: endpoints that only exist to make the list of the form overflow its column, so that the layout can be measured
FILLER_ENDPOINTS=$(for i in $(seq 1 25); do
  printf '  - name: filler-%02d\n    group: filler\n    url: %s/health\n    interval: 1h\n    conditions:\n      - "[STATUS] == 200"\n' "$i" "$BASE"
done)

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
endpoints:
  - name: health
    group: core
    url: $BASE/health
    interval: 5s
    conditions:
      - "[STATUS] == 200"
  - name: offline
    group: core
    url: http://127.0.0.1:1/
    interval: 5s
    conditions:
      - "[STATUS] == 200"
  - name: panel
    url: $BASE/health
    interval: 5s
    conditions:
      - "[STATUS] == 200"
  # Fork: long name and long URL, so that the admin list is measured with the case it has to survive
  - name: a-very-long-endpoint-name-for-checking-the-truncation-of-the-list
    group: filler
    url: $BASE/health?a-very-long-query-string-that-keeps-going-and-going-to-check-the-truncation-of-the-url-column=1
    interval: 1h
    conditions:
      - "[STATUS] == 200"
$FILLER_ENDPOINTS
# Fork: push endpoint of the real-time test of the public details page
external-endpoints:
  - name: backup
    group: jobs
    token: realtime-job-token-0000000000000000
status-pages:
  rate-limit: 0
  pages:
    - slug: jobs
      title: "Jobs"
      groups: [jobs]
      show-messages: true
    - slug: services
      title: "Services"
      description: "End-to-end test page"
      groups: [core]
      endpoints: [_panel]
      featured: [core_health]
    - slug: draft
      title: "Draft"
      groups: [core]
      enabled: false
    - slug: messages
      title: "Messages"
      groups: [core]
      show-messages: true
CONFIG

GO_UPTIME_CONFIG_PATH="$WORK/config.yaml" dist/go-uptime > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
public() { agent-browser --session e2e-status-public "$@"; }
admin() { agent-browser --session e2e-status-admin "$@"; }
cleanup() {
  public close >/dev/null 2>&1 || true
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
  public screenshot --full "$PRINTS/error-public.png" >/dev/null 2>&1 || true
  admin screenshot --full "$PRINTS/error-admin.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/go-uptime.log"
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
api_status() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}
body_text() {
  "$1" eval "document.body.innerText" 2>/dev/null
}
js() {
  local session=$1
  shift
  "$session" eval "$*" 2>/dev/null | tr -d '"'
}
# Fork: layout_ok checks that the page does not scroll and that the given elements are inside the window, so that
# content cut by the fixed height of the layout is detected (document.scrollHeight alone is useless with overflow-hidden)
layout_ok() {
  local label=$1
  shift
  local checks="document.documentElement.scrollHeight <= innerHeight + 1 && document.documentElement.scrollWidth <= innerWidth + 1"
  for id in "$@"; do
    checks="$checks && (() => { const element = document.querySelector('[data-testid=\"$id\"]'); if (!element) return false; const rect = element.getBoundingClientRect(); return rect.top >= 0 && rect.bottom <= innerHeight + 1 })()"
  done
  [ "$(js admin "$checks")" = true ] || fail "the status page form scrolls or cuts its content: $label"
}
# not_covered checks that a click at the center of the element reaches it, for example with a toast visible
not_covered() {
  admin scrollintoview "$(testid "$1")" >/dev/null 2>&1 || true
  [ "$(js admin "(() => { const element = document.querySelector('[data-testid=\"$1\"]'); const rect = element.getBoundingClientRect(); const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2); return element === hit || element.contains(hit) })()")" = true ] || fail "$1 is covered at $2"
}
toast_text() {
  js admin "Array.from(document.querySelectorAll('[data-testid=\"toast\"][data-type=\"$1\"]')).map((toast) => toast.innerText).join(' | ')"
}
legend_series() {
  local session=$1
  js "$session" "Array.from(document.querySelectorAll('[data-testid=\"response-time-chart-legend\"] [data-series]')).map((item) => item.dataset.series).join(',')"
}

step "Public API without credentials and protected routes"
[ "$(api_status "$BASE/api/v1/status-pages/services")" = 200 ] || fail "expected 200 from the public API"
[ "$(api_status "$BASE/api/v1/status-pages/draft")" = 404 ] || fail "expected 404 for the disabled page"
[ "$(api_status "$BASE/api/v1/status-pages/a/b")" = 404 ] || fail "expected 404 for an invalid path, without 401"
[ "$(api_status "$BASE/status/missing")" = 200 ] || fail "expected 200 from the HTML route"
[ "$(api_status "$BASE/api/v1/endpoints/statuses")" = 401 ] || fail "expected 401 from the protected routes"
if curl -s "$BASE/api/v1/status-pages/services" | grep -qE '127\.0\.0\.1|connection refused|core_health'; then
  fail "the public API exposed a URL, an error or a key"
fi

step "Public page in light mode, without credentials"
public set viewport 1280 900 >/dev/null
set_theme public light
public open "$BASE/status/services" >/dev/null
public wait --text "Partial outage" >/dev/null || fail "the page did not show the partial outage"
# Fork: the banner counts the endpoints of the page, from the payload
summary=$(js public "document.querySelector('[data-testid=\"status-summary\"]').innerText.replace(/\n/g, ' ')")
grep -q "up" <<<"$summary" && grep -q "down" <<<"$summary" || fail "the banner does not count the endpoints: $summary"
curl -s "$BASE/api/v1/status-pages/services" | grep -q '"summary":{"total":3,"up":2,"down":1,"pending":0,"unknown":0}' || fail "the payload of the page does not count the endpoints"
public wait 700 >/dev/null
public screenshot --full "$PRINTS/01-services-light.png" >/dev/null
[ "$(js public 'document.title')" = "Services" ] || fail "document.title is not the title of the page"
[ "$(js public 'document.documentElement.lang')" = "en" ] || fail "the public layout must not change the language of the document"
grep -q "Other services" <<<"$(body_text public)" || fail "the endpoint without group is not in the Other services section"
requests=$(public network requests 2>/dev/null)
grep -q "/api/v1/config" <<<"$requests" && fail "the public page called /api/v1/config"
grep -qE '\b401\b' <<<"$requests" && fail "a request of the public page received 401"
# Fork: the Inter comes from Go Uptime itself. The FontFace has to be loaded: document.fonts.check() answers true even
# without any @font-face, because the family then resolves to a font of the system
font_loaded=$(public eval "(async () => { await document.fonts.ready; return Array.from(document.fonts).some((face) => face.family === 'Inter' && face.status === 'loaded') })()" 2>/dev/null | tr -d '"')
[ "$font_loaded" = "true" ] || fail "the Inter of the interface was not loaded: $font_loaded"
font_request=$(public eval "(() => { const entries = performance.getEntriesByType('resource'); const inter = entries.find((entry) => entry.name.includes('/fonts/inter-4-1-latin.woff2')); const external = entries.some((entry) => entry.name.includes('fonts.googleapis.com') || entry.name.includes('fonts.gstatic.com')); return JSON.stringify({ downloaded: Boolean(inter && inter.decodedBodySize > 0), external }) })()" 2>/dev/null | tr -d '\\"')
grep -q 'downloaded:true' <<<"$font_request" || fail "the font was not downloaded from Go Uptime: $font_request"
grep -q 'external:false' <<<"$font_request" || fail "the page asked a font service for a font: $font_request"
# Tabular figures: the pair of control shows that the feature applies, and not only that the font has fixed digits
tabular=$(public eval "(async () => { await document.fonts.ready; const make = (variant, digits) => { const span = document.createElement('span'); span.style.cssText = 'position:absolute;visibility:hidden;white-space:nowrap;font-variant-numeric:' + variant; span.textContent = digits; document.body.appendChild(span); return span }; const normalOne = make('normal', '1111111111'); const normalNine = make('normal', '9999999999'); const tabularOne = make('tabular-nums', '1111111111'); const tabularNine = make('tabular-nums', '9999999999'); const result = { tabularEqual: tabularOne.offsetWidth === tabularNine.offsetWidth, proportionalDiffers: normalOne.offsetWidth !== normalNine.offsetWidth }; [normalOne, normalNine, tabularOne, tabularNine].forEach((span) => span.remove()); return JSON.stringify(result) })()" 2>/dev/null | tr -d '\\"')
grep -q 'tabularEqual:true' <<<"$tabular" || fail "the figures are not tabular: $tabular"
grep -q 'proportionalDiffers:true' <<<"$tabular" || fail "the font does not distinguish proportional from tabular figures: $tabular"

step "Slim layout: line height, aligned uptime columns and floating tooltip"
# Fork: measured on /status/messages, which selects the group core without featured, so two endpoints share one list
public open "$BASE/status/messages" >/dev/null
public wait "$(testid status-endpoint-offline)" >/dev/null || fail "the messages page did not open"
slim=$(public eval "(() => { const row = document.querySelector('[data-testid=\"status-endpoint-offline\"]'); const columns = (name) => Array.from(document.querySelectorAll('[data-testid=\"status-endpoint-' + name + '\"] dl > div')).map((cell) => ({ left: Math.round(cell.getBoundingClientRect().left), width: Math.round(cell.getBoundingClientRect().width), text: cell.textContent.trim() })); const offline = columns('offline'); const health = columns('health'); const labels = Array.from(document.querySelectorAll('[data-testid=\"status-group-core\"] > div:first-of-type dt')); const rowLabels = Array.from(document.querySelectorAll('[data-testid=\"status-group-core\"] li dt')); return JSON.stringify({ height: Math.round(row.getBoundingClientRect().height), columns: offline.length === 3 && health.length === 3, filled: [...offline, ...health].every((cell) => cell.width > 0 && /[0-9]|—/.test(cell.text)), aligned: offline.every((cell, index) => Math.abs(cell.left - health[index].left) <= 1), groupLabels: labels.length === 3 && labels.every((label) => getComputedStyle(label).display !== 'none'), rowLabelsHidden: rowLabels.length > 0 && rowLabels.every((label) => getComputedStyle(label).display === 'none'), liveRegions: document.querySelectorAll('[data-testid=\"status-endpoint-offline\"] [aria-live]').length, liveEmpty: document.querySelector('[data-testid=\"status-endpoint-offline\"] [aria-live]').textContent.trim() === '', tooltip: document.querySelectorAll('[data-testid=\"status-endpoint-detail\"]').length }) })()" 2>/dev/null | tr -d '\\"')
case "$(sed -n 's/.*height:\([0-9]*\).*/\1/p' <<<"$slim")" in
  5[6-9] | 6[0-9] | 7[0-2]) ;;
  *) fail "the line of the endpoint is outside of 56 to 72 px: $slim" ;;
esac
grep -q "columns:true" <<<"$slim" || fail "the uptime columns are missing: $slim"
grep -q "filled:true" <<<"$slim" || fail "the uptime columns have no visible value: $slim"
grep -q "aligned:true" <<<"$slim" || fail "the uptime columns of the two endpoints are not aligned: $slim"
grep -q "groupLabels:true" <<<"$slim" || fail "the group header does not show the labels of the periods: $slim"
grep -q "rowLabelsHidden:true" <<<"$slim" || fail "the labels of the periods are repeated in the rows: $slim"
grep -q "liveRegions:1" <<<"$slim" || fail "the live region of the detail is missing: $slim"
grep -q "liveEmpty:true" <<<"$slim" || fail "the live region should be empty without an active check: $slim"
grep -q "tooltip:0" <<<"$slim" || fail "the tooltip should not exist before any check is active: $slim"
# The last bar is the one that would leak out of the row
public hover "$(testid status-endpoint-offline) [role=group] > span:last-child" >/dev/null
tooltip=$(public eval "(() => { const row = document.querySelector('[data-testid=\"status-endpoint-offline\"]'); const next = document.querySelector('[data-testid=\"status-endpoint-health\"]'); const tip = document.querySelector('[data-testid=\"status-endpoint-detail\"]'); if (!tip) { return JSON.stringify({ shown: false }) } const rowRect = row.getBoundingClientRect(); const tipRect = tip.getBoundingClientRect(); return JSON.stringify({ shown: / ms$/.test(tip.textContent.trim()), height: Math.round(rowRect.height), nextTop: Math.round(next.getBoundingClientRect().top), inside: tipRect.left >= rowRect.left - 1 && tipRect.right <= rowRect.right + 1, pointerEvents: getComputedStyle(tip).pointerEvents, noScroll: document.documentElement.scrollWidth <= innerWidth + 1 }) })()" 2>/dev/null | tr -d '\\"')
grep -q "shown:true" <<<"$tooltip" || fail "the tooltip of the check did not show up: $tooltip"
grep -q "inside:true" <<<"$tooltip" || fail "the tooltip leaked out of the line: $tooltip"
grep -q "pointerEvents:none" <<<"$tooltip" || fail "the tooltip captures the pointer: $tooltip"
grep -q "noScroll:true" <<<"$tooltip" || fail "the tooltip added horizontal scroll: $tooltip"
[ "$(sed -n 's/.*height:\([0-9]*\).*/\1/p' <<<"$tooltip")" = "$(sed -n 's/.*height:\([0-9]*\).*/\1/p' <<<"$slim")" ] || fail "the tooltip changed the height of the line"
# Takes the pointer off the bars, so that the screenshots that follow do not carry the tooltip
public hover "$(testid status-page-title)" >/dev/null
public screenshot --full "$PRINTS/01b-messages-slim.png" >/dev/null
# Narrow screens keep the label next to the value and no horizontal scroll
public set viewport 360 800 >/dev/null
public reload >/dev/null
public wait "$(testid status-endpoint-offline)" >/dev/null
narrow=$(public eval "(() => { const labels = Array.from(document.querySelectorAll('[data-testid=\"status-group-core\"] li dt')); return JSON.stringify({ noScroll: document.documentElement.scrollWidth <= innerWidth + 1, labelsVisible: labels.length > 0 && labels.every((label) => getComputedStyle(label).display !== 'none'), bars: document.querySelectorAll('[data-testid=\"status-endpoint-offline\"] [role=group] > span').length }) })()" 2>/dev/null | tr -d '\\"')
grep -q "noScroll:true" <<<"$narrow" || fail "the page scrolls horizontally at 360 px: $narrow"
grep -q "labelsVisible:true" <<<"$narrow" || fail "the labels of the periods disappeared on a narrow screen: $narrow"
grep -q "bars:25" <<<"$narrow" || fail "a narrow screen should show 25 bars: $narrow"
public set viewport 1280 900 >/dev/null
public open "$BASE/status/services" >/dev/null
public wait --text "Partial outage" >/dev/null

step "Detail of a check with the keyboard"
public eval "document.querySelector('[data-testid=\"status-endpoint-health\"] [role=group]').focus()" >/dev/null
public press ArrowLeft >/dev/null
public wait 300 >/dev/null
grep -q " ms" <<<"$(js public "document.querySelector('[data-testid=\"status-endpoint-health\"] [data-testid=status-endpoint-detail]').textContent")" || fail "the keyboard did not show the detail of the check"
public screenshot "$PRINTS/02-services-keyboard-detail.png" >/dev/null

step "Featured endpoint and endpoint details page"
public wait "$(testid status-featured)" >/dev/null || fail "the featured section did not show up"
grep -q "Avg response" <<<"$(js public "document.querySelector('[data-testid=status-featured]').innerText")" || fail "the featured card did not show the average response times"
[ "$(js public "document.querySelector('[data-testid=\"status-group-core\"] [data-testid=\"status-endpoint-health\"]') ? 'yes' : 'no'")" = "no" ] || fail "the featured endpoint is repeated in its group"
[ "$(js public "document.querySelector('[data-testid=status-endpoint-details-health]') ? 'yes' : 'no'")" = "yes" ] || fail "the featured card has no link to the details page"
[ "$(js public "document.querySelector('canvas') ? 'yes' : 'no'")" = "no" ] || fail "the status page still shows an inline chart"
public network requests --clear >/dev/null 2>&1 || true
public click "$(testid status-endpoint-link-panel)" >/dev/null
public wait "$(testid status-endpoint-details)" >/dev/null || fail "the details page of panel did not open"
[ "$(js public 'location.pathname')" = "/status/services/endpoints/_panel" ] || fail "unexpected address of the details page"
grep -q "^panel" <<<"$(js public 'document.title')" || fail "document.title is not the name of the endpoint"
public wait "[data-testid=\"status-endpoint-chart\"] canvas" >/dev/null || fail "the response time chart did not show up"
public wait "$(testid status-endpoint-events)" >/dev/null || fail "the events did not show up"
# Same order as the monitor page of the Uptime Kuma: bars, numbers, chart and table of checks, without messages
top_public() {
  js public "Math.round(document.querySelector('[data-testid=\"$1\"]').getBoundingClientRect().top + window.scrollY)"
}
[ "$(top_public status-endpoint-recent-checks)" -lt "$(top_public status-endpoint-chart)" ] && [ "$(top_public status-endpoint-chart)" -lt "$(top_public status-endpoint-checks-table)" ] || fail "expected the bars, the chart and the table of checks in this order"
public click "$(testid recent-checks-toggle)" >/dev/null
public wait "$(testid recent-check-0)" >/dev/null || fail "the public table of checks did not expand"
[ "$(js public "document.querySelectorAll('[data-testid=\"recent-check-message\"]').length")" = 0 ] || fail "the public table of checks shows messages"
public screenshot --full "$PRINTS/endpoint-details-kuma-order.png" >/dev/null
# The events are collapsed by default, like the Checks table, and the browser remembers when they are expanded
[ "$(js public "document.querySelectorAll('[data-testid=\"events-list\"]').length")" = 0 ] || fail "the events should be collapsed by default"
[ "$(js public "document.querySelector('[data-testid=\"events-toggle\"]').getAttribute('aria-expanded')")" = false ] || fail "the toggle of the events should not be expanded"
grep -qE 'Events(\\n| )?\([0-9]+\)' <<<"$(js public "document.querySelector('[data-testid=\"events-toggle\"]').innerText")" || fail "the toggle of the events does not show their number"
public scrollintoview "$(testid events-toggle)" >/dev/null
public click "$(testid events-toggle)" >/dev/null
public wait "$(testid events-list)" >/dev/null || fail "the events did not expand"
grep -q "Monitoring started" <<<"$(js public "document.querySelector('[data-testid=\"events-list\"]').innerText")" || fail "the events do not have the texts of the dashboard"
public reload >/dev/null
public wait "$(testid events-list)" >/dev/null || fail "the expanded events were not remembered after a reload"
public screenshot --full "$PRINTS/endpoint-details-events.png" >/dev/null
public scrollintoview "$(testid events-toggle)" >/dev/null
public click "$(testid events-toggle)" >/dev/null
# Chart in the format of the Uptime Kuma: Recent by default, then the aggregates of 24 hours, remembered after a reload
[ "$(js public "document.querySelector('[data-testid=\"response-time-chart\"]').dataset.period")" = "recent" ] || fail "the chart did not open in Recent"
# The local checks of _panel may take less than 1 ms, which the chart does not draw (like the Uptime Kuma): only the load
# of the chart is checked
public wait "[data-testid=\"response-time-chart\"][data-loading=\"false\"] canvas" >/dev/null || fail "the Recent chart was not drawn"
public eval "(() => { const select = document.querySelector('[data-testid=\"status-endpoint-chart-duration\"]'); select.value = '24h'; select.dispatchEvent(new Event('change')) })()" >/dev/null
public wait "[data-testid=\"response-time-chart\"][data-period=\"24h\"]" >/dev/null || fail "the chart did not load the 24h period"
public wait 1500 >/dev/null
# Fork: legend of the series, in the order they are drawn
[ "$(legend_series public)" = "average,minimum,maximum" ] || fail "the legend of the aggregates is not average, minimum and maximum: $(legend_series public)"
[ "$(js public "document.querySelector('[data-testid=\"response-time-chart\"]').contains(document.querySelector('[data-testid=\"response-time-chart-legend\"]'))")" = false ] || fail "the legend should be outside of the element with the height of the chart"
public screenshot --full "$PRINTS/02b-endpoint-details.png" >/dev/null
requests=$(public network requests 2>/dev/null)
grep -q "/api/v1/status-pages/services/endpoints/_panel/response-time-chart?period=24h" <<<"$requests" || fail "changing the period did not load the public chart of 24h"
[ "$(api_status "$BASE/api/v1/status-pages/services/endpoints/core_missing/response-time-chart?period=24h")" = 404 ] || fail "expected 404 from the chart of an endpoint that is not on the page"
[ "$(api_status "$BASE/api/v1/status-pages/services/endpoints/_panel/response-time-chart?period=30d")" = 400 ] || fail "expected 400 from an invalid period of the chart"
public reload >/dev/null
public wait "[data-testid=\"response-time-chart\"][data-period=\"24h\"]" >/dev/null || fail "the period of the chart was not remembered after a reload"
public eval "(() => { const select = document.querySelector('[data-testid=\"status-endpoint-chart-duration\"]'); select.value = 'recent'; select.dispatchEvent(new Event('change')) })()" >/dev/null
public wait "[data-testid=\"response-time-chart\"][data-period=\"recent\"]" >/dev/null
[ "$(legend_series public)" = "response-time" ] || fail "the legend of Recent is not the response time: $(legend_series public)"
grep -q "/api/v1/config" <<<"$requests" && fail "the details page called /api/v1/config"
grep -qE '\b401\b' <<<"$requests" && fail "a request of the details page received 401"
public click "$(testid status-endpoint-back)" >/dev/null
public wait "$(testid status-page-title)" >/dev/null || fail "the back link did not return to the status page"
[ "$(api_status "$BASE/api/v1/status-pages/services/endpoints/_panel")" = 200 ] || fail "expected 200 from the public endpoint details"
[ "$(api_status "$BASE/api/v1/status-pages/services/endpoints/core_missing")" = 404 ] || fail "expected 404 for an endpoint that is not on the page"
[ "$(api_status "$BASE/api/v1/status-pages/draft/endpoints/core_health")" = 404 ] || fail "expected 404 for an endpoint of a disabled page"
[ "$(api_status "$BASE/api/v1/status-pages/services/response-times/24h")" = 404 ] || fail "the former response times route still responds"
if curl -s "$BASE/api/v1/status-pages/services/endpoints/_panel" | grep -qE '127\.0\.0\.1|_panel'; then
  fail "the endpoint details exposed a URL or a key"
fi

step "Page with show-messages: same table of checks as the dashboard, without the errors of the checks"
details=$(curl -s "$BASE/api/v1/status-pages/messages/endpoints/core_health")
grep -q '"showMessages":true' <<<"$details" || fail "the details payload does not say that the page shows messages"
grep -q '"message":"HTTP 200"' <<<"$details" || fail "the details payload does not have the HTTP status as message"
offline=$(curl -s "$BASE/api/v1/status-pages/messages/endpoints/core_offline")
grep -qE '127\.0\.0\.1|connection refused|dial tcp' <<<"$offline" && fail "the errors of the checks were published"
# Fork: the check that failed without answering publishes the reason of the failure, from a closed set
grep -q '"message":"Connection failed"' <<<"$offline" || fail "the details payload does not have the reason of the failure"
grep -q '"showMessages":false' <<<"$(curl -s "$BASE/api/v1/status-pages/services/endpoints/_panel")" || fail "the page without the option should not show messages"
public open "$BASE/status/messages/endpoints/core_health" >/dev/null
public wait "$(testid details-summary)" >/dev/null || fail "the panel of numbers is not shown on the public details page"
public wait "$(testid recent-checks-toggle)" >/dev/null
[ "$(js public "document.querySelectorAll('[data-testid=\"recent-checks-table\"]').length")" = 0 ] && public click "$(testid recent-checks-toggle)" >/dev/null
public wait "$(testid recent-check-message)" >/dev/null || fail "the public table of a page with show-messages has no message column"
grep -q "HTTP 200" <<<"$(js public "document.querySelector('[data-testid=\"recent-checks-table\"]').innerText")" || fail "the public table does not show the HTTP status as message"
public wait 1500 >/dev/null
public screenshot --full "$PRINTS/endpoint-details-messages.png" >/dev/null
# The table of the endpoint that is offline shows the reason, and nothing of the error
public open "$BASE/status/messages/endpoints/core_offline" >/dev/null
public wait "$(testid recent-checks-toggle)" >/dev/null
[ "$(js public "document.querySelectorAll('[data-testid=\"recent-checks-table\"]').length")" = 0 ] && public click "$(testid recent-checks-toggle)" >/dev/null
public wait "$(testid recent-check-message)" >/dev/null || fail "the table of the endpoint that is offline has no message column"
offline_table=$(js public "document.querySelector('[data-testid=\"recent-checks-table\"]').innerText")
grep -q "Connection failed" <<<"$offline_table" || fail "the public table does not show the reason of the failure: $offline_table"
grep -qE '127\.0\.0\.1|connection refused|dial tcp' <<<"$offline_table" && fail "the public table showed the error of the check"

step "Real time: a pending push shows up on the public details page without reloading it"
curl -s "$BASE/api/push/realtime-job-token-0000000000000000?status=up&msg=First%20run&ping=20" | grep -q '"ok":true' || fail "the first push was not accepted"
[ "$(api_status -H 'Accept: text/event-stream' --max-time 2 "$BASE/api/v1/status-pages/jobs/endpoints/jobs_backup/events")" = 200 ] || fail "expected 200 from the public events route"
[ "$(api_status -H 'Accept: text/event-stream' "$BASE/api/v1/status-pages/services/endpoints/jobs_backup/events")" = 404 ] || fail "expected 404 from the events route of an endpoint that is not on the page"
[ "$(api_status -H 'Accept: text/event-stream' "$BASE/api/v1/endpoints/jobs_backup/events")" = 401 ] || fail "expected 401 from the protected events route"
public open "$BASE/status/jobs/endpoints/jobs_backup" >/dev/null
public wait "$(testid details-summary)" >/dev/null || fail "the public details page of the push endpoint is not shown"
public wait "$(testid recent-checks-toggle)" >/dev/null
[ "$(js public "document.querySelectorAll('[data-testid=\"recent-checks-table\"]').length")" = 0 ] && public click "$(testid recent-checks-toggle)" >/dev/null
public wait "$(testid recent-check-message)" >/dev/null
# The marker only survives if the page is not reloaded; the details are still in the 30 s cache of the public API
public eval "window.__e2eNoReload = true" >/dev/null
public wait 1500 >/dev/null
curl -s "$BASE/api/push/realtime-job-token-0000000000000000?status=pending&msg=Realtime%20pending" | grep -q '"ok":true' || fail "the pending push was not accepted"
shown=false
for _ in $(seq 1 10); do
  if js public "document.querySelector('[data-testid=\"recent-checks-table\"]').innerText" | grep -q "Realtime pending"; then
    shown=true
    break
  fi
  sleep 0.5
done
[ "$shown" = true ] || fail "the pending push did not show up on the public details page within 5 seconds"
for _ in $(seq 1 10); do
  [ "$(js public "document.querySelector('[data-testid=\"response-time-chart\"]').dataset.pendingColumns")" -gt 0 ] && break
  sleep 0.5
done
[ "$(js public "document.querySelector('[data-testid=\"response-time-chart\"]').dataset.pendingColumns")" -gt 0 ] || fail "the pending push is not a yellow column of the public chart"
[ "$(js public "window.__e2eNoReload === true")" = true ] || fail "the public details page was reloaded"
public wait 2000 >/dev/null
public screenshot --full "$PRINTS/endpoint-details-realtime-pending.png" >/dev/null

step "Dark mode and 390 px screen"
set_theme public dark
public open "$BASE/status/services" >/dev/null
public wait "$(testid status-summary)" >/dev/null || fail "the status banner did not show up"
public wait 700 >/dev/null
public screenshot --full "$PRINTS/03-services-dark.png" >/dev/null
public set viewport 390 844 >/dev/null
public open "$BASE/status/services" >/dev/null
public wait "$(testid status-summary)" >/dev/null || fail "the status banner did not show up at 390 px"
public wait 700 >/dev/null
public screenshot --full "$PRINTS/04-services-390px-dark.png" >/dev/null
[ "$(js public "document.querySelectorAll('[data-testid=\"status-endpoint-health\"] [role=group] > span').length")" = 25 ] || fail "expected 25 bars on a narrow screen"
public set viewport 1280 900 >/dev/null
set_theme public light

step "Missing page, disabled page and malformed slug"
public network requests --clear >/dev/null 2>&1 || true
for path in missing draft "a%2Fb" "a/b" "services/endpoints/core_missing"; do
  public open "$BASE/status/$path" >/dev/null
  public wait --text "Page not found" >/dev/null || fail "/status/$path did not show Page not found"
done
public screenshot "$PRINTS/05-page-not-found.png" >/dev/null
requests=$(public network requests 2>/dev/null)
grep -q "/api/v1/status-pages/a" <<<"$requests" && fail "the malformed slug called the API"
grep -qE '\b401\b' <<<"$requests" && fail "a page not found received 401"

step "Simulated OIDC: no login screen on the public page; positive control on the dashboard"
public network route "**/api/v1/config" --body '{"oidc":true,"authenticated":false}' >/dev/null
public open "$BASE/status/services" >/dev/null
public wait "$(testid status-page-title)" >/dev/null || fail "the public page did not load with simulated OIDC"
grep -q "Login with OIDC" <<<"$(body_text public)" && fail "the public page showed the login screen"
public open "$BASE/" >/dev/null
public wait --text "Login with OIDC" >/dev/null || fail "the positive control did not show the login screen"
public screenshot "$PRINTS/06-oidc-dashboard-control.png" >/dev/null
public network unroute >/dev/null 2>&1 || true

step "Administration: list with the pages of the configuration file"
admin set viewport 1280 900 >/dev/null
set_theme admin light
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
admin open "$BASE/admin/status-pages" >/dev/null
admin wait "$(testid status-page-row-config-services)" >/dev/null || fail "the list did not show the services page"
admin wait "$(testid status-page-row-config-draft)" >/dev/null || fail "the list did not show the draft page"
admin screenshot --full "$PRINTS/07-admin-list.png" >/dev/null

# Fork: page of the configuration file, read-only: the YAML scrolls inside its own area and the warning stays visible
admin click "$(testid status-page-edit-services)" >/dev/null
admin wait "$(testid status-page-yaml)" >/dev/null || fail "the YAML of the page of the configuration file did not open"
layout_ok "page of the configuration file" status-page-preview-button admin-back
admin screenshot "$PRINTS/07b-admin-yaml.png" >/dev/null
admin click "$(testid admin-back)" >/dev/null
admin wait "$(testid admin-new-status-page)" >/dev/null || fail "Back did not return to the list of status pages"

step "Administration: create with the form and validate"
admin click "$(testid admin-new-status-page)" >/dev/null
admin wait "$(testid status-page-field-slug)" >/dev/null || fail "the form did not open"
admin fill "$(testid status-page-field-slug)" "team" >/dev/null
admin fill "$(testid status-page-field-title)" "Team" >/dev/null
admin fill "$(testid status-page-field-description)" "Services used by the team" >/dev/null
admin scrollintoview "$(testid status-page-group-core)" >/dev/null
admin click "$(testid status-page-group-core)" >/dev/null
admin fill "$(testid status-page-endpoint-search)" "panel" >/dev/null
admin scrollintoview "$(testid status-page-endpoint-_panel)" >/dev/null
admin click "$(testid status-page-endpoint-_panel)" >/dev/null
admin click "$(testid status-page-featured-_panel)" >/dev/null
[ "$(js admin "Math.abs(document.querySelector('[data-testid=\"status-page-copy-link\"]').getBoundingClientRect().height - document.querySelector('[data-testid=\"status-page-field-slug\"]').getBoundingClientRect().height)")" = 0 ] || fail "the Copy link button does not have the height of the slug field"
admin fill "$(testid status-page-endpoint-search)" "" >/dev/null
admin click "$(testid status-page-only-selected)" >/dev/null
[ "$(js admin "document.querySelectorAll('[data-testid^=\"status-page-endpoint-_\"], [data-testid^=\"status-page-endpoint-core_\"]').length")" = 1 ] || fail "Only selected should list only the selected endpoint"
admin click "$(testid status-page-only-selected)" >/dev/null
admin click "$(testid status-page-validate)" >/dev/null
admin wait "[data-testid=\"toast\"][data-type=\"info\"]" >/dev/null || fail "the validation did not show a toast"
grep -q "The page will show 3 endpoints" <<<"$(toast_text info)" || fail "the validation did not count the 3 endpoints"
[ "$(js admin "document.querySelectorAll('[data-testid=\"status-page-validation\"]').length")" = 0 ] || fail "a validation without warnings should not show the band"
admin screenshot "$PRINTS/08-admin-validation.png" >/dev/null

step "Administration: save (created disabled) and preview"
admin click "$(testid status-page-save)" >/dev/null
admin wait --text "Status page created" >/dev/null || fail "the creation was not confirmed"
[ "$(js admin 'location.pathname')" = "/admin/status-pages/team/edit" ] || fail "the creation did not lead to the edition"
[ "$(api_status "$BASE/api/v1/status-pages/team")" = 404 ] || fail "the page that was just created should not be public"
# Fork: the form fills the window, scrolls inside its columns and keeps the actions visible
layout_ok "edition of the created page" status-page-save status-page-validate status-page-preview-button admin-back
[ "$(js admin "(() => { const list = document.querySelector('[data-testid=\"status-page-endpoint-list\"]'); return list.scrollHeight > list.clientHeight && list.clientHeight > 320 })()")" = true ] || fail "the list of endpoints does not fill the column and scroll inside it"
[ "$(js admin "(() => { const column = document.querySelector('[data-testid=\"status-page-column-general\"]'); return column.scrollHeight > column.clientHeight })()")" = true ] || fail "the left column does not scroll inside itself"
not_covered admin-back "the toast of the creation"
not_covered status-page-save "the toast of the creation"
admin click "$(testid status-page-preview-button)" >/dev/null
admin wait "$(testid status-page-preview)" >/dev/null || fail "the preview did not show up"
admin wait "$(testid status-page-preview-featured)" >/dev/null || fail "the preview did not show the featured endpoint"
admin screenshot "$PRINTS/09-admin-preview.png" >/dev/null
admin press Escape >/dev/null
[ "$(js admin "document.querySelectorAll('[data-testid=\"status-page-preview\"]').length")" = 0 ] || fail "Escape did not close the preview"
[ "$(js admin "document.querySelector('[data-testid=\"status-page-field-title\"]').value")" = "Team" ] || fail "closing the preview lost what was typed"

step "Administration: publish and open without credentials"
admin scrollintoview "$(testid status-page-field-enabled)" >/dev/null
admin click "$(testid status-page-field-enabled)" >/dev/null
admin click "$(testid status-page-save)" >/dev/null
admin wait --text "saved and published" >/dev/null || fail "the publication was not confirmed"
public open "$BASE/status/team" >/dev/null
public wait --text "Services used by the team" >/dev/null || fail "the published page did not open without credentials"
public wait "[data-testid=\"status-featured\"] [data-testid=\"status-endpoint-panel\"]" >/dev/null || fail "the team page did not show panel as featured"
public screenshot --full "$PRINTS/10-team-public.png" >/dev/null

step "Administration: lists without horizontal scrolling"
# Fork: measured with a long name and a long URL, in widths away from the breakpoints (lg = 1024, md = 768)
list_fits() {
  local label=$1 row=$2
  [ "$(js admin "(() => { const panel = document.querySelector('[data-testid=\"admin-list-scroll\"]'); const row = document.querySelector('[data-testid=\"$row\"]'); const action = row ? row.querySelector('button, a') : null; return panel.scrollWidth <= panel.clientWidth && document.documentElement.scrollWidth <= innerWidth + 1 && Boolean(action) && action.getBoundingClientRect().right <= innerWidth + 1 })()")" = true ] || fail "$label has horizontal scrolling or actions out of the window"
}
for width in 1100 900 820; do
  admin set viewport "$width" 800 >/dev/null
  admin open "$BASE/admin" >/dev/null
  admin wait "$(testid admin-table)" >/dev/null || fail "the list of endpoints did not open at $width px"
  list_fits "the list of endpoints at $width px" "admin-row-filler_a-very-long-endpoint-name-for-checking-the-truncation-of-the-list"
  admin open "$BASE/admin/status-pages" >/dev/null
  admin wait "$(testid status-pages-table)" >/dev/null || fail "the list of status pages did not open at $width px"
  list_fits "the list of status pages at $width px" "status-page-row-admin-team"
done
# Below md the tables give way to cards, with the same actions
for width in 700 390 360; do
  admin set viewport "$width" 800 >/dev/null
  admin open "$BASE/admin" >/dev/null
  admin wait "$(testid admin-card-core_health)" >/dev/null || fail "the list of endpoints is not in cards at $width px"
  [ "$(js admin "(() => { const table = document.querySelector('[data-testid=\"admin-table\"]'); return (!table || getComputedStyle(table).display === 'none') && document.documentElement.scrollWidth <= innerWidth + 1 && Boolean(document.querySelector('[data-testid=\"admin-open-core_health\"]')) })()")" = true ] || fail "the cards of the endpoints at $width px still have a table, horizontal scrolling or no actions"
  admin open "$BASE/admin/status-pages" >/dev/null
  admin wait "$(testid status-page-card-admin-team)" >/dev/null || fail "the list of status pages is not in cards at $width px"
  [ "$(js admin "(() => { const table = document.querySelector('[data-testid=\"status-pages-table\"]'); return (!table || getComputedStyle(table).display === 'none') && document.documentElement.scrollWidth <= innerWidth + 1 && Boolean(document.querySelector('[data-testid=\"status-page-edit-team\"]')) })()")" = true ] || fail "the cards of the status pages at $width px still have a table, horizontal scrolling or no actions"
done
admin set viewport 1280 900 >/dev/null
admin open "$BASE/admin/status-pages" >/dev/null
admin wait "$(testid status-pages-table)" >/dev/null
# Fork: rows of the same height, with the actions as icons with an accessible name
PAGES=$(js admin "(() => { const rows = Array.from(document.querySelectorAll('[data-testid^=\"status-page-row-\"]')).map((row) => Math.round(row.getBoundingClientRect().height)); const edit = document.querySelector('[data-testid=\"status-page-edit-team\"]'); const open = document.querySelector('[data-testid=\"status-page-open-team\"]'); return JSON.stringify({ same: new Set(rows).size === 1, tallest: Math.max(...rows), label: edit ? edit.getAttribute('aria-label') : '', text: edit ? edit.textContent.trim() : 'missing', link: open ? open.tagName : 'missing' }) })()" | tr -d '\\')
grep -q "same:true" <<<"$PAGES" || fail "the rows of the list of status pages have different heights: $PAGES"
[ "$(sed -n 's/.*tallest:\([0-9]*\).*/\1/p' <<<"$PAGES")" -le 32 ] || fail "the rows of the list of status pages are taller than 32 px: $PAGES"
grep -q "label:Edit team" <<<"$PAGES" || fail "the edit action has no accessible name: $PAGES"
grep -q "text:," <<<"$PAGES" || fail "the edit action still has visible text: $PAGES"
grep -q "link:A" <<<"$PAGES" || fail "the action that opens the public page must stay a link: $PAGES"
admin open "$BASE/admin" >/dev/null
admin wait "$(testid admin-table)" >/dev/null
# The long URL is truncated, with the whole address in the title
[ "$(js admin "(() => { const cell = document.querySelector('[data-testid=\"admin-row-filler_a-very-long-endpoint-name-for-checking-the-truncation-of-the-list\"] td:nth-child(4)'); const span = cell.querySelector('span'); return span.scrollWidth > span.clientWidth && cell.title.includes('a-very-long-query-string') })()")" = true ] || fail "the long URL is not truncated with the whole address in the title"
admin screenshot "$PRINTS/12-admin-list-wide.png" >/dev/null

step "Details of the endpoint: the two screens with the same header and the same history"
admin open "$BASE/endpoints/core_health" >/dev/null
admin wait "$(testid endpoint-name)" >/dev/null || fail "the details page of the dashboard did not open"
admin wait 1500 >/dev/null
dashboard_details=$(js admin "(() => { const bar = document.querySelector('[data-testid=\"recent-checks-card\"] .flex-1.rounded-sm'); const card = document.querySelector('[data-testid=\"recent-checks-card\"]'); const blocks = ['recent-checks-card', 'details-summary', 'response-time-trend', 'checks-table-card', 'details-badges', 'details-health'].map((id) => { const el = document.querySelector('[data-testid=\"' + id + '\"]'); return el ? Math.round(el.getBoundingClientRect().top + scrollY) : -1 }); return JSON.stringify({ fontSize: getComputedStyle(document.querySelector('[data-testid=\"endpoint-name\"]')).fontSize, bar: bar ? Math.round(bar.getBoundingClientRect().height) : 0, name: card.innerText.includes('health'), order: blocks.join(','), sorted: blocks.every((top, index) => index === 0 || (top > 0 && top >= blocks[index - 1])) })})()" | tr -d '\\')
grep -q "bar:20" <<<"$dashboard_details" || fail "the bars of the dashboard are not 20 px: $dashboard_details"
grep -q "name:false" <<<"$dashboard_details" || fail "the card of the history of the dashboard repeats the name: $dashboard_details"
grep -q "sorted:true" <<<"$dashboard_details" || fail "the blocks of the dashboard are out of order: $dashboard_details"
dashboard_font=$(sed -n 's/.*fontSize:\([0-9]*\)px.*/\1/p' <<<"$dashboard_details")
public open "$BASE/status/services/endpoints/core_health" >/dev/null
public wait "$(testid status-endpoint-name)" >/dev/null || fail "the public details page did not open"
public wait 1500 >/dev/null
public_details=$(js public "(() => { const bar = document.querySelector('[data-testid=\"status-endpoint-recent-checks\"] [role=group] > span'); const card = document.querySelector('[data-testid=\"status-endpoint-recent-checks\"]').cloneNode(true); card.querySelectorAll('.sr-only, [aria-hidden=true]').forEach((node) => node.remove()); const blocks = ['status-endpoint-recent-checks', 'details-summary', 'status-endpoint-chart', 'status-endpoint-checks-table', 'details-badges', 'details-health'].map((id) => { const el = document.querySelector('[data-testid=\"' + id + '\"]'); return el ? Math.round(el.getBoundingClientRect().top + scrollY) : -1 }); return JSON.stringify({ fontSize: getComputedStyle(document.querySelector('[data-testid=\"status-endpoint-name\"]')).fontSize, bar: bar ? Math.round(bar.getBoundingClientRect().height) : 0, name: card.innerText.includes('health'), group: document.body.innerText.includes('Group:'), sorted: blocks.every((top, index) => index === 0 || (top > 0 && top >= blocks[index - 1])) })})()" | tr -d '\\')
grep -q "bar:20" <<<"$public_details" || fail "the bars of the public page are not 20 px: $public_details"
grep -q "name:false" <<<"$public_details" || fail "the card of the history of the public page shows the name: $public_details"
grep -q "group:true" <<<"$public_details" || fail "the public page does not show the group: $public_details"
grep -q "sorted:true" <<<"$public_details" || fail "the blocks of the public page are out of order: $public_details"
public_font=$(sed -n 's/.*fontSize:\([0-9]*\)px.*/\1/p' <<<"$public_details")
[ "$dashboard_font" = "$public_font" ] || fail "the title has different sizes: $dashboard_font px and $public_font px"
# The public payload keeps the host to itself and publishes the expiration with the date
grep -q "127.0.0.1" <<<"$(js public "document.querySelector('[data-testid=\"status-endpoint-details\"]').innerText")" && fail "the public details page shows the host"
public screenshot "$PRINTS/12b-public-endpoint-details.png" >/dev/null

step "Administration: exposure warning in the endpoint form"
admin open "$BASE/admin/endpoints/new" >/dev/null
admin wait "$(testid admin-field-group-select)" >/dev/null || fail "the endpoint form did not open"
admin click "$(testid admin-field-group-select) button" >/dev/null
admin wait "$(testid admin-group-option-core)" >/dev/null || fail "the group field did not list the core group"
admin click "$(testid admin-group-option-core)" >/dev/null
admin fill "$(testid admin-field-name)" "new" >/dev/null
admin wait "$(testid admin-endpoint-exposure)" >/dev/null || fail "the exposure warning did not show up"
exposure=$(js admin "document.querySelector('[data-testid=admin-endpoint-exposure]').innerText")
grep -q "Team" <<<"$exposure" && grep -q "by group" <<<"$exposure" || fail "the exposure warning did not mention the Team page by group"
admin screenshot "$PRINTS/11-admin-exposure.png" >/dev/null

step "Administration: rename a managed endpoint featured on a managed page"
curl -sf -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/yaml' \
  --data-binary $'name: site\ngroup: web\ninterval: 5s\nurl: '"$BASE"$'/health\nconditions: ["[STATUS] == 200"]\n' \
  "$BASE/api/v1/admin/endpoints" >/dev/null || fail "the managed endpoint was not created"
curl -sf -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/json' \
  --data '{"slug":"clients","title":"Clients","featured":["web_site"],"enabled":true}' \
  "$BASE/api/v1/admin/status-pages" >/dev/null || fail "the clients page was not created"
admin open "$BASE/admin/endpoints/web_site/edit" >/dev/null
admin wait "$(testid admin-field-group-select)" >/dev/null || fail "the edition of web_site did not open"
admin click "$(testid admin-field-group-select) button" >/dev/null
admin wait "$(testid admin-group-new)" >/dev/null || fail "the group selector did not open"
admin click "$(testid admin-group-new)" >/dev/null
admin fill "$(testid admin-field-group)" "clientes" >/dev/null
admin wait "$(testid admin-key-change)" >/dev/null || fail "the key change warning did not show up"
admin screenshot "$PRINTS/12-admin-rename-warning.png" >/dev/null
admin scrollintoview "$(testid admin-save)" >/dev/null
admin click "$(testid admin-save)" >/dev/null
admin wait "$(testid admin-row-clientes_site)" >/dev/null || fail "the renamed endpoint is not in the list"
curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/status-pages/clients" | grep -q 'clientes_site' || fail "the clients page does not select the new key"
public open "$BASE/status/clients" >/dev/null
public wait "[data-testid=\"status-featured\"] [data-testid=\"status-endpoint-site\"]" >/dev/null || fail "the clients page did not show the renamed endpoint as featured"
if curl -s "$BASE/api/v1/status-pages/clients" | grep -qE 'web_site|clientes_site'; then
  fail "the public API exposed a key"
fi

step "Administration: page with a login of its own"
# Fork: created through the form, checked with curl only, so that the browser never opens the native credential dialog
admin open "$BASE/admin/status-pages" >/dev/null
admin wait "$(testid admin-new-status-page)" >/dev/null || fail "the list of status pages did not open"
admin click "$(testid admin-new-status-page)" >/dev/null
admin wait "$(testid status-page-field-slug)" >/dev/null || fail "the form did not open"
admin fill "$(testid status-page-field-slug)" "private" >/dev/null
admin fill "$(testid status-page-field-title)" "Private" >/dev/null
admin scrollintoview "$(testid status-page-group-core)" >/dev/null
admin click "$(testid status-page-group-core)" >/dev/null
admin scrollintoview "$(testid status-page-field-requires-login)" >/dev/null
admin click "$(testid status-page-field-requires-login)" >/dev/null
admin wait "$(testid status-page-field-auth-username)" >/dev/null || fail "the credential fields did not show up"
admin fill "$(testid status-page-field-auth-username)" "client" >/dev/null
admin fill "$(testid status-page-field-auth-password)" "page-secret-e2e" >/dev/null
admin click "$(testid status-page-field-enabled)" >/dev/null
admin screenshot "$PRINTS/13-admin-page-login.png" >/dev/null
admin click "$(testid status-page-save)" >/dev/null
admin wait --text "created and published" >/dev/null || fail "the page with a login was not created"
# Every route of the page answers the challenge without the credential, and 200 with it
page_headers() {
  local route=$1
  shift
  curl -s -o /dev/null -D - --max-time 10 "$@" "$BASE$route"
}
PROTECTED_ROUTES=(
  "/status/private"
  "/status/private/endpoints/core_health"
  "/api/v1/status-pages/private"
  "/api/v1/status-pages/private/endpoints/core_health"
  "/api/v1/status-pages/private/endpoints/core_health/response-time-chart?period=recent"
  "/api/v1/status-pages/private/endpoints/core_health/health/badge.svg"
  "/api/v1/status-pages/private/endpoints/core_health/response-times/24h/badge.svg"
  "/api/v1/status-pages/private/endpoints/core_health/events"
)
# With the credential first, because every failure counts towards the limit of the page
for route in "${PROTECTED_ROUTES[@]}"; do
  method=()
  # The event stream only ends after minutes: HEAD answers the same headers without opening it
  [ "${route%/events}" = "$route" ] || method=(-I)
  allowed=$(page_headers "$route" "${method[@]}" -u "client:page-secret-e2e")
  grep -qi '^HTTP/1.1 200' <<<"$allowed" || fail "$route did not answer 200 with the credential: $(head -1 <<<"$allowed")"
  grep -qi '^Cache-Control: private' <<<"$allowed" || fail "$route answered without a private cache-control"
done
curl -s -u "client:page-secret-e2e" "$BASE/api/v1/status-pages/private" | grep -q '"title":"Private"' || fail "the payload of the page with a login is not the usual one"
# And the challenge on every route without it
for route in "${PROTECTED_ROUTES[@]}"; do
  method=()
  [ "${route%/events}" = "$route" ] || method=(-I)
  challenge=$(page_headers "$route" "${method[@]}")
  grep -qi '^HTTP/1.1 401' <<<"$challenge" || fail "$route answered without the challenge: $(head -1 <<<"$challenge")"
  grep -qi '^WWW-Authenticate: Basic realm="private"' <<<"$challenge" || fail "$route did not send WWW-Authenticate"
  grep -qi '^Cache-Control: no-store' <<<"$challenge" || fail "$route did not answer the challenge with no-store"
done
# The credential of the installation and a wrong password do not open the page
[ "$(api_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/status-pages/private")" = 401 ] || fail "the credential of the installation opened the page"
[ "$(api_status -u "client:wrong" "$BASE/api/v1/status-pages/private")" = 401 ] || fail "a wrong password opened the page"
# Ten failures later the page waits, even for the right credential, and only that page
blocked=$(page_headers "/api/v1/status-pages/private" -u "client:page-secret-e2e")
grep -qi '^HTTP/1.1 429' <<<"$blocked" || fail "the failures of the page were not limited: $(head -1 <<<"$blocked")"
grep -qi '^Retry-After:' <<<"$blocked" || fail "the 429 of the page came without Retry-After"
# The public pages keep answering without any credential
[ "$(api_status "$BASE/api/v1/status-pages/services")" = 200 ] || fail "the public page stopped answering without credentials"
[ "$(api_status "$BASE/status/services")" = 200 ] || fail "the HTML of the public page stopped answering without credentials"
# The administration never shows the hash, and the list shows the lock
detail=$(curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/status-pages/private")
grep -q '\*\*\*\*\*\*\*\*' <<<"$detail" || fail "the administration did not mask the hash of the credential"
grep -qE '\$2[aby]\$|page-secret-e2e' <<<"$detail" && fail "the administration answered with the hash or the password"
admin open "$BASE/admin/status-pages" >/dev/null
admin wait "$(testid status-page-requires-login-private)" >/dev/null || fail "the list did not show the lock of the page with a login"
grep -q "with login" <<<"$(js admin "document.querySelector('[data-testid=\"admin-list-footer\"]') ? document.querySelector('[data-testid=\"admin-list-footer\"]').innerText : document.body.innerText")" || fail "the footer of the list does not count the pages with a login"

step "Administration: dark mode and removal"
set_theme admin dark
admin open "$BASE/admin/status-pages" >/dev/null
admin wait "$(testid status-page-row-admin-team)" >/dev/null || fail "the list did not show the team page"
grep -q "Published" <<<"$(js admin "document.querySelector('[data-testid=\"status-page-row-admin-team\"]').innerText")" || fail "the team page is not shown as published"
admin wait 700 >/dev/null
admin screenshot --full "$PRINTS/12-admin-list-dark.png" >/dev/null
admin click "$(testid status-page-remove-team)" >/dev/null
admin click "$(testid confirm-accept)" >/dev/null
admin wait --text "removed" >/dev/null || fail "the removal was not confirmed"
[ "$(api_status "$BASE/api/v1/status-pages/team")" = 404 ] || fail "the removed page is still public"

echo "OK: $STEP steps; screenshots in $PRINTS"
