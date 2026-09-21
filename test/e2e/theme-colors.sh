#!/usr/bin/env bash
# Collects the computed colours of every element of every screen, per theme and per state, so that two versions of the
# frontend can be compared element by element:
#
#   make build && test/e2e/theme-colors.sh /tmp/before        # on the reference version
#   make build && test/e2e/theme-colors.sh /tmp/after         # on the changed version
#   test/e2e/theme-colors.sh --compare /tmp/before /tmp/after
#
# It also measures the contrast of every text of the bio theme, on the same screens and states:
#
#   test/e2e/theme-colors.sh --contrast /tmp/contrast && test/e2e/theme-colors.sh --contrast-report /tmp/contrast
#
# A text passes with 4.5:1 (3:1 when large), or when the same element has no better contrast in the light theme: the
# status colours are the same in every theme, and the bio theme must not make them worse.
#
# It is what proves that a change of the colour plumbing (the Tailwind gray scale going through CSS variables, for
# example) leaves the existing themes untouched. Comparing screenshots is not deterministic; this is, under these
# conditions: fixed data (Push endpoints with seeded results, one check of the own /health, no external host), fixed
# window and media, transitions and animations disabled, stable content before collecting, and an explicit list of the
# states that are activated. A state that this script does not activate is not covered. Out of its reach: the pixels of
# images and of the canvas of the response time chart.
set -euo pipefail

if [ "${1:-}" = "--compare" ]; then
  python3 - "$2" "$3" <<'PY'
import json, os, sys
before, after = sys.argv[1:3]
names = sorted(set(os.listdir(before)) | set(os.listdir(after)))
differences = 0
for name in names:
    paths = [os.path.join(directory, name) for directory in (before, after)]
    if not all(os.path.exists(path) for path in paths):
        print(f"{name}: only in one of the two"); differences += 1; continue
    old, new = (json.load(open(path)) for path in paths)
    for key in sorted(set(old) | set(new)):
        if old.get(key) != new.get(key):
            differences += 1
            if differences <= 40:
                print(f"{name}: {key}\n    - {old.get(key)}\n    + {new.get(key)}")
print(f"{len(names)} collections compared, {differences} differences")
sys.exit(1 if differences else 0)
PY
  exit $?
fi

if [ "${1:-}" = "--contrast-report" ]; then
  python3 - "$2" <<'REPORT'
import json, os, sys
directory = sys.argv[1]
failures = debts = checked = 0
for name in sorted(os.listdir(directory)):
    if not name.startswith("bio-"):
        continue
    bio = json.load(open(os.path.join(directory, name)))
    light_path = os.path.join(directory, "light-" + name[len("bio-"):])
    light = json.load(open(light_path)) if os.path.exists(light_path) else {}
    for key, item in bio.items():
        checked += 1
        minimum = 3.0 if item["large"] else 4.5
        if item["ratio"] >= minimum:
            continue
        reference = light.get(key)
        # A text that misses the minimum is accepted only when the light theme is not better for the same element:
        # the status colours are the same in every theme, and the bio theme must not make them worse
        if reference and reference["ratio"] <= item["ratio"] + 0.01:
            debts += 1
            continue
        failures += 1
        was = f'{reference["ratio"]} in the light theme' if reference else "absent in the light theme"
        print(f'{name}: {key} "{item["text"]}": {item["ratio"]} ({item["foreground"]} on {item["background"]}), {was}')
print(f"{checked} texts checked in the bio theme, {failures} below the minimum and worse than in the light theme, {debts} below the minimum but not worse than in the light theme")
sys.exit(1 if failures else 0)
REPORT
  exit $?
fi

COLLECTOR_FILE=test/e2e/lib/computed-colors.js
if [ "${1:-}" = "--contrast" ]; then
  # Same screens and states, in the light and in the bio themes, with the contrast of every text instead of the colours
  shift
  COLLECTOR_FILE=test/e2e/lib/contrast.js
  THEMES="light bio"
fi
OUT=${1:?usage: theme-colors.sh <output directory> | --compare <before> <after> | --contrast <output directory> | --contrast-report <directory>}
THEMES=${THEMES:-"light dark"}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
WORK=$(mktemp -d)
PORT=${PORT:-18096}
BASE="http://127.0.0.1:$PORT"
USERNAME=admin
PASSWORD='e2e-colors-password'
SERVER_PID=""
mkdir -p "$OUT"

[ -x dist/go-uptime ] || { echo "dist/go-uptime not found: run make build"; exit 1; }
HASH=$(printf '%s\n' "$PASSWORD" | dist/go-uptime password hash)
token_of() { printf 'e2e-color-token-%s-0123456789' "$1"; }
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
endpoints:
  - name: health
    group: core
    url: "$BASE/health"
    interval: 24h
    conditions:
      - "[STATUS] == 200"
suites:
  - name: flow
    group: core
    interval: 24h
    endpoints:
      - name: first
        url: "$BASE/health"
        conditions:
          - "[STATUS] == 200"
      - name: second
        url: "$BASE/health"
        conditions:
          - "[STATUS] == 418"
      - name: third
        url: "$BASE/health"
        conditions:
          - "[STATUS] == 200"
external-endpoints:
CONFIG
  for name in up down silent; do
    printf '  - name: %s\n    group: jobs\n    token: "%s"\n' "$name" "$(token_of "$name")"
  done
  cat <<CONFIG
status-pages:
  rate-limit: 0
  pages:
    - slug: services
      title: "Services"
      description: "Fixed data for the comparison of colours"
      groups: [core, jobs]
      featured: [core_health]
      show-certificate-expiration: true
      show-messages: true
    - slug: compact
      title: "Compact"
      groups: [core, jobs]
      groups-collapsed: true
CONFIG
} > "$WORK/config.yaml"

browser() { agent-browser --session e2e-theme-colors "$@"; }
cleanup() {
  browser close >/dev/null 2>&1 || true
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

dist/go-uptime --config "$WORK/config.yaml" > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do curl -sf "$BASE/health" >/dev/null && break; sleep 1; done
curl -sf "$BASE/health" >/dev/null || { echo "Go Uptime did not start"; cat "$WORK/go-uptime.log"; exit 1; }
# Seeded results: three pushes up, two up and one down, and none for "silent" (no data)
for _ in $(seq 1 20); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/push/$(token_of up)?status=up&ping=100")" = 200 ] && break
  sleep 0.5
done
for status in up up; do curl -s -o /dev/null "$BASE/api/push/$(token_of up)?status=$status&ping=100"; done
for status in up up down; do curl -s -o /dev/null "$BASE/api/push/$(token_of down)?status=$status&msg=failure&ping=100"; done
curl -s -o /dev/null -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/json' \
  -d '{"slug":"managed","title":"Managed","enabled":true,"groups":["jobs"],"featured":["jobs_up"]}' "$BASE/api/v1/admin/status-pages"
curl -s -o /dev/null -u "$USERNAME:$PASSWORD" -H 'Content-Type: application/json' -d '{"name":"scripts"}' "$BASE/api/v1/admin/push-keys"
# The check of /health and the suite run once at the start: wait for them, then nothing else produces results
for _ in $(seq 1 40); do
  curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/suites/statuses" | grep -q '"results":\[{' && break
  sleep 0.5
done

STILL="* , *::before, *::after { transition: none !important; animation: none !important; caret-color: transparent !important; }"
COLLECTOR=$(cat "$COLLECTOR_FILE")
set_theme() {
  browser cookies set theme "$1" --url "$BASE" >/dev/null
}
settle() {
  browser eval "(() => { if (!document.getElementById('e2e-still')) { const style = document.createElement('style'); style.id = 'e2e-still'; style.textContent = \"$STILL\"; document.head.appendChild(style) } })()" >/dev/null
  browser eval "document.fonts.ready.then(() => true)" >/dev/null
}
collect() { # name
  settle
  local previous="" current="" attempt
  for attempt in 1 2 3 4 5 6 7 8; do
    browser wait 400 >/dev/null
    current=$(browser eval "$COLLECTOR")
    [ -n "$previous" ] && [ "$current" = "$previous" ] && break
    previous=$current
  done
  [ "$attempt" -lt 8 ] || echo "    warning: $1 did not settle" >&2
  python3 -c 'import json,sys; data=json.loads(sys.stdin.read()); data=json.loads(data) if isinstance(data,str) else data; json.dump(data, open(sys.argv[1],"w"), indent=0, sort_keys=True)' "$OUT/$1.json" <<<"$current"
  echo "    $1: $(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))))' "$OUT/$1.json") entries"
}
screen() { # name path [selector to wait for]
  browser open "$BASE$2" >/dev/null
  [ -n "${3:-}" ] && browser wait "$3" >/dev/null
  collect "$1"
}

browser open "$BASE/login" >/dev/null
browser set viewport 1280 900 >/dev/null
browser set media light reduced-motion >/dev/null 2>&1 || true
for theme in $THEMES; do
  echo "==> $theme"
  set_theme "$theme"
  screen "$theme-login" /login '[data-testid="login-username"]'
  browser focus '[data-testid="login-username"]' >/dev/null
  collect "$theme-login-focus"
  browser fill '[data-testid="login-username"]' "$USERNAME" >/dev/null
  browser fill '[data-testid="login-password"]' "wrong-password" >/dev/null
  browser click '[data-testid="login-submit"]' >/dev/null
  browser wait 1200 >/dev/null
  collect "$theme-login-error"
  browser fill '[data-testid="login-password"]' "$PASSWORD" >/dev/null
  browser click '[data-testid="login-submit"]' >/dev/null
  browser wait '[data-testid="logout-button"]' >/dev/null

  screen "$theme-dashboard" /
  browser hover 'main button' >/dev/null 2>&1 || true
  collect "$theme-dashboard-hover"
  screen "$theme-endpoint" /endpoints/core_health
  screen "$theme-endpoint-push-down" /endpoints/jobs_down
  screen "$theme-suite" /suites/core_flow
  screen "$theme-admin-endpoints" /admin
  screen "$theme-admin-endpoint-new" /admin/endpoints/new
  screen "$theme-admin-status-pages" /admin/status-pages
  screen "$theme-admin-status-page-new" /admin/status-pages/new '[data-testid="status-page-field-slug"]'
  browser click '[data-testid="status-page-save"]' >/dev/null 2>&1 || true
  browser wait 800 >/dev/null
  collect "$theme-admin-status-page-new-errors"
  screen "$theme-admin-status-page-edit" /admin/status-pages/managed/edit '[data-testid="status-page-preview-button"]'
  browser click '[data-testid="status-page-preview-button"]' >/dev/null
  browser wait '[data-testid="status-page-preview-close"]' >/dev/null
  collect "$theme-admin-status-page-preview"
  screen "$theme-admin-status-page-readonly" /admin/status-pages/services/edit
  screen "$theme-admin-push-keys" /admin/push-keys
  screen "$theme-admin-backup" /admin/backup
  screen "$theme-status-page" /status/services '[data-testid="status-group-toggle-jobs"]'
  browser focus '[data-testid="status-group-toggle-jobs"]' >/dev/null
  collect "$theme-status-page-focus"
  browser click '[data-testid="status-group-toggle-jobs"]' >/dev/null
  collect "$theme-status-page-collapsed"
  browser click '[data-testid="status-group-toggle-jobs"]' >/dev/null
  screen "$theme-status-page-compact" /status/compact '[data-testid="status-group-toggle-jobs"]'
  screen "$theme-status-page-endpoint" /status/services/endpoints/jobs_down
  screen "$theme-status-page-missing" /status/missing
  browser click '[data-testid="logout-button"]' >/dev/null 2>&1 || { browser open "$BASE/" >/dev/null; browser click '[data-testid="logout-button"]' >/dev/null 2>&1 || true; }
  browser wait 800 >/dev/null
done
echo "OK: collections in $OUT"
