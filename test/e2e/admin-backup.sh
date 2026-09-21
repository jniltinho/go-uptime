#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# End-to-end tests of the backup and restore of the administration, with agent-browser.
#
#   test/e2e/admin-backup.sh
#
# Starts a source installation (temporary SQLite), registers an endpoint, a status page and a push key through the web,
# downloads the backup with and without password, then starts a target installation, whose configuration file has a
# status page with the same slug, and restores the backup through the Backup tab. Screenshots in
# dist/prints/admin-backup/ (dist/ is in .gitignore). Requires agent-browser with Chrome installed.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18093}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/admin-backup"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (cost 10) of PASSWORD, in base64 with the URL alphabet (as Go Uptime decodes it)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'
BACKUP_PASSWORD='correct horse battery'
PUSH_TOKEN='keSDu7G855jvVat1xWiY2Gk4CkL1End5'

command -v agent-browser >/dev/null || { echo "agent-browser not found"; exit 1; }
mkdir -p "$PRINTS"

echo "==> Building"
make -s build

write_config() {
  local name=$1 extra=$2
  cat > "$WORK/$name.yaml" <<CONFIG
web:
  address: 127.0.0.1
  port: $PORT
storage:
  type: sqlite
  path: "$WORK/$name.db"
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
    interval: 30s
    conditions:
      - "[STATUS] == 200"
$extra
CONFIG
}
write_config source ""
write_config target "status-pages:
  pages:
    - slug: jobs
      title: Jobs of the file
      groups: [jobs]"

SERVER_PID=""
start_server() {
  GO_UPTIME_CONFIG_PATH="$WORK/$1.yaml" dist/go-uptime > "$WORK/$1.log" 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 1 60); do
    curl -sf "$BASE/health" >/dev/null && return 0
    sleep 1
  done
  echo "Go Uptime did not start"; cat "$WORK/$1.log"; exit 1
}
stop_server() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
}
admin() { agent-browser --session e2e-admin-backup "$@"; }
cleanup() {
  admin close >/dev/null 2>&1 || true
  stop_server
  rm -rf "$WORK"
}
trap cleanup EXIT

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
  tail -20 "$WORK"/*.log
  exit 1
}
testid() {
  echo "[data-testid=\"$1\"]"
}
js() {
  admin eval "$*" 2>/dev/null | tr -d '"'
}
authenticated() {
  curl -s -u "$USERNAME:$PASSWORD" "$@"
}
login_screen() {
  admin open "$BASE/login" >/dev/null
  admin wait "$(testid login-username)" >/dev/null || fail "the login screen did not open"
  admin fill "$(testid login-username)" "$USERNAME" >/dev/null
  admin fill "$(testid login-password)" "$PASSWORD" >/dev/null
  admin click "$(testid login-submit)" >/dev/null
  admin wait "$(testid logout-button)" >/dev/null || fail "the login did not work"
}
# layout_ok checks that the page does not scroll (with the subpixel tolerance of the lists) and that the given elements
# are inside the window, so that a content cut by the fixed height of the layout is detected
layout_ok() {
  local label=$1
  shift
  local checks="document.documentElement.scrollHeight <= innerHeight + 1 && document.documentElement.scrollWidth <= innerWidth + 1"
  for id in "$@"; do
    checks="$checks && (() => { const element = document.querySelector('[data-testid=\"$id\"]'); if (!element) return false; const rect = element.getBoundingClientRect(); return rect.top >= 0 && rect.bottom <= innerHeight + 1 })()"
  done
  [ "$(js "$checks")" = true ] || fail "the Backup tab scrolls or cuts its content: $label"
}
# not_covered checks that a click at the center of the element reaches it, for example with a toast visible
not_covered() {
  admin scrollintoview "$(testid "$1")" >/dev/null 2>&1 || true
  [ "$(js "(() => { const element = document.querySelector('[data-testid=\"$1\"]'); const rect = element.getBoundingClientRect(); const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2); return element === hit || element.contains(hit) })()")" = true ] || fail "$1 is covered at $2"
}
focus_inside() {
  js "document.querySelector('[data-testid=\"$1\"]').contains(document.activeElement)"
}
toast_text() {
  js "Array.from(document.querySelectorAll('[data-testid=\"toast\"][data-type=\"$1\"]')).map((toast) => toast.innerText).join(' | ')"
}
plan_action() {
  js "document.querySelector('[data-testid=\"restore-plan-row-$1-$2\"]')?.dataset.action || ''"
}
result_of() {
  js "document.querySelector('[data-testid=\"restore-result-row-$1-$2\"]')?.dataset.result || ''"
}

step "Source installation: an endpoint, a status page and a push key registered through the web"
start_server source
authenticated -H 'Content-Type: application/yaml' --data-binary "type: push
name: backup
group: jobs
token: $PUSH_TOKEN" "$BASE/api/v1/admin/endpoints" | grep -q '"key":"jobs_backup"' || fail "the push endpoint was not created"
authenticated -H 'Content-Type: application/yaml' --data-binary "slug: jobs
title: Jobs
groups: [jobs]
enabled: true" "$BASE/api/v1/admin/status-pages" | grep -q '"slug":"jobs"' || fail "the status page was not created"
KEY_TOKEN=$(authenticated -H 'Content-Type: application/json' -d '{"name":"akamai"}' "$BASE/api/v1/admin/push-keys" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
[ -n "$KEY_TOKEN" ] || fail "the push key was not created"

step "Backup tab: counts, download without and with password"
admin set viewport 1280 900 >/dev/null
login_screen
set_theme admin light
admin open "$BASE/admin/backup" >/dev/null
admin wait "$(testid admin-backup)" >/dev/null || fail "the Backup tab did not open"
admin wait 1000 >/dev/null
grep -q "1" <<<"$(js "document.querySelector('[data-testid=\"backup-section-download\"]').innerText")" || fail "the counts are not shown"
[ "$(js "document.querySelectorAll('[data-testid=\"backup-plaintext-warning\"]').length")" = 1 ] || fail "the warning about secrets in plain text is not shown"
layout_ok "without encryption" backup-download restore-preview
admin screenshot "$PRINTS/01-backup-tab.png" >/dev/null
admin download "$(testid backup-download)" "$WORK/plain.json" >/dev/null || fail "the plain backup was not downloaded"
admin wait "[data-testid=\"toast\"][data-type=\"success\"]" >/dev/null || fail "the download did not show a toast"
grep -q "Backup downloaded" <<<"$(toast_text success)" || fail "unexpected toast of the download"
python3 - "$WORK/plain.json" "$PUSH_TOKEN" <<'PY' || fail "the plain backup does not have the registered items"
import json, sys
backup = json.load(open(sys.argv[1]))
assert backup["format"] == "go-uptime-admin-backup"
assert [e["key"] for e in backup["endpoints"]] == ["jobs_backup"] and sys.argv[2] in backup["endpoints"][0]["definition"]
assert [p["slug"] for p in backup["statusPages"]] == ["jobs"] and [k["name"] for k in backup["pushKeys"]] == ["akamai"]
PY
admin click "$(testid backup-encrypt)" >/dev/null
admin fill "$(testid backup-password)" "$BACKUP_PASSWORD" >/dev/null
admin fill "$(testid backup-password-confirm)" "$BACKUP_PASSWORD" >/dev/null
layout_ok "with encryption" backup-download restore-preview
admin screenshot "$PRINTS/02-backup-encrypted.png" >/dev/null
for size in "1280 720" "1024 600"; do
  admin set viewport $size >/dev/null
  admin wait 300 >/dev/null
  layout_ok "with encryption at $size" backup-download restore-preview
done
admin set viewport 1280 900 >/dev/null
admin download "$(testid backup-download)" "$WORK/encrypted.json" >/dev/null || fail "the encrypted backup was not downloaded"
grep -q '"go-uptime-admin-backup-encrypted"' "$WORK/encrypted.json" || fail "the backup is not encrypted"
grep -q "$PUSH_TOKEN" "$WORK/encrypted.json" && fail "the encrypted backup has the token in plain text"
admin close >/dev/null 2>&1 || true
stop_server

step "Target installation: preview with the status page of the file skipped"
start_server target
login_screen
set_theme admin light
admin open "$BASE/admin/backup" >/dev/null
admin wait "$(testid restore-file)" >/dev/null || fail "the restore section did not open"
admin upload "$(testid restore-file)" "$WORK/plain.json" >/dev/null
admin wait 500 >/dev/null
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null || fail "the preview was not shown"
[ "$(plan_action pushKey akamai)" = create ] || fail "the push key is not planned to be created"
[ "$(plan_action endpoint jobs_backup)" = create ] || fail "the endpoint is not planned to be created"
[ "$(plan_action statusPage jobs)" = skip ] || fail "the status page of the configuration file is not skipped"
authenticated "$BASE/api/v1/admin/endpoints" | grep -q jobs_backup && fail "the preview created the endpoint"
[ "$(js "document.querySelector('[data-testid=\"restore-plan\"]').getAttribute('role')")" = dialog ] || fail "the preview is not a dialog"
layout_ok "with the preview" restore-plan-close restore-apply
admin wait 500 >/dev/null
admin screenshot "$PRINTS/03-restore-preview.png" >/dev/null
for size in "1280 720" "1024 600"; do
  admin set viewport $size >/dev/null
  admin wait 300 >/dev/null
  layout_ok "with the preview at $size" restore-plan-close restore-apply
done
admin set viewport 1280 900 >/dev/null
for _ in $(seq 1 12); do
  admin press Tab >/dev/null
done
[ "$(focus_inside restore-plan)" = true ] || fail "the focus left the preview dialog"

step "Closing the preview discards it"
admin click "$(testid restore-plan-close)" >/dev/null
admin wait 300 >/dev/null
[ "$(js "document.querySelectorAll('[data-testid=\"restore-plan-table\"]').length")" = 0 ] || fail "the preview was not closed"
admin click "$(testid restore-overwrite)" >/dev/null
admin click "$(testid restore-overwrite)" >/dev/null
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null || fail "the preview did not open again"

step "Restore with confirmation and results"
admin click "$(testid restore-apply)" >/dev/null
admin wait "$(testid confirm-cancel)" >/dev/null || fail "the confirmation was not shown"
admin wait 300 >/dev/null
[ "$(js "document.activeElement === document.querySelector('[data-testid=\"confirm-cancel\"]')")" = true ] || fail "the initial focus of the confirmation is not Cancel"
admin click "$(testid confirm-cancel)" >/dev/null
admin wait 300 >/dev/null
[ "$(focus_inside restore-plan)" = true ] || fail "the focus did not go back to the preview after cancelling the confirmation"
admin click "$(testid restore-apply)" >/dev/null
admin wait "$(testid confirm-accept)" >/dev/null || fail "the confirmation was not shown again"
admin click "$(testid confirm-accept)" >/dev/null
admin wait "$(testid restore-results-table)" >/dev/null || fail "the results were not shown"
[ "$(result_of pushKey akamai)" = created ] && [ "$(result_of endpoint jobs_backup)" = created ] && [ "$(result_of statusPage jobs)" = skipped ] || fail "unexpected results"
grep -q "Restore finished" <<<"$(toast_text success)" || fail "the restore did not show a toast"
admin wait 500 >/dev/null
admin screenshot "$PRINTS/04-restore-results.png" >/dev/null
admin click "$(testid restore-results-close)" >/dev/null
curl -s "$BASE/api/push/$KEY_TOKEN/jobs_backup?status=up&msg=restored" | grep -q '"ok":true' || fail "the restored push key does not accept the original token"
curl -s "$BASE/api/push/$PUSH_TOKEN?status=up&msg=restored" | grep -q '"ok":true' || fail "the restored push endpoint does not accept its token"

step "The same restore again leaves the items unchanged"
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null
for _ in $(seq 1 10); do
  [ "$(plan_action endpoint jobs_backup)" = unchanged ] && break
  sleep 0.3
done
[ "$(plan_action pushKey akamai)" = unchanged ] && [ "$(plan_action endpoint jobs_backup)" = unchanged ] || fail "the applied items are not unchanged"
admin press Escape >/dev/null
admin wait 300 >/dev/null
[ "$(js "document.querySelectorAll('[data-testid=\"restore-plan-table\"]').length")" = 0 ] || fail "Escape did not close the preview"

step "Encrypted backup: wrong password, then the right one, in dark mode"
set_theme admin dark
admin reload >/dev/null
admin wait "$(testid restore-file)" >/dev/null
admin upload "$(testid restore-file)" "$WORK/encrypted.json" >/dev/null
admin wait "$(testid restore-password)" >/dev/null || fail "the password of the encrypted backup is not asked"
admin fill "$(testid restore-password)" "a wrong password" >/dev/null
admin click "$(testid restore-preview)" >/dev/null
admin wait "[data-testid=\"toast\"][data-type=\"error\"]" >/dev/null || fail "the wrong password did not show an error toast"
grep -qi "invalid password" <<<"$(toast_text error)" || fail "unexpected error of the wrong password"
[ "$(js "document.querySelector('[data-testid=\"restore-password\"]').getAttribute('aria-invalid')")" = true ] || fail "the password field is not marked as invalid"
grep -qi "invalid password" <<<"$(js "document.getElementById(document.querySelector('[data-testid=\"restore-password\"]').getAttribute('aria-describedby').split(' ')[0])?.innerText || ''")" || fail "the error is not associated with the password field"
layout_ok "with an encrypted file and its error" backup-download restore-preview

step "Toasts do not cover the actions"
for size in "1280 900" "800 600" "390 844"; do
  admin set viewport $size >/dev/null
  admin wait 300 >/dev/null
  [ "$(js "document.querySelectorAll('[data-testid=\"toast\"]').length")" -ge 1 ] || fail "expected a visible toast at $size"
  not_covered restore-preview "$size"
  not_covered backup-download "$size"
done
not_covered logout-button "390 844"
admin fill "$(testid restore-password)" "$BACKUP_PASSWORD" >/dev/null
[ "$(js "document.querySelector('[data-testid=\"restore-password\"]').getAttribute('aria-invalid') || 'false'")" = false ] || fail "editing the password did not clear the invalid mark"
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null || fail "the encrypted backup was not previewed"
for size in "390 844" "800 600" "1280 900"; do
  admin set viewport $size >/dev/null
  admin wait 300 >/dev/null
  [ "$(js "document.querySelectorAll('[data-testid=\"toast\"]').length")" -ge 1 ] || fail "expected the toast to stay while the dialog is open at $size"
  not_covered restore-plan-close "$size"
  not_covered restore-apply "$size"
done
[ "$(plan_action endpoint jobs_backup)" = unchanged ] || fail "the encrypted backup is not the same as the plain one"
admin mouse move 5 5 >/dev/null 2>&1 || true
admin wait 800 >/dev/null
admin screenshot "$PRINTS/05-restore-encrypted-dark.png" >/dev/null
admin press Escape >/dev/null
set_theme admin light

step "Rejected file: persistent status and dismissed toast"
echo '{"not":"a backup"}' > "$WORK/not-backup.json"
admin upload "$(testid restore-file)" "$WORK/not-backup.json" >/dev/null
admin wait 500 >/dev/null
grep -qi "not a backup" <<<"$(toast_text error)" || fail "the rejected file did not show an error toast"
grep -qi "not a backup" <<<"$(js "document.querySelector('[data-testid=\"restore-file-status\"]').innerText")" || fail "the status of the file does not explain the rejection"
while [ "$(js "document.querySelectorAll('[data-testid=\"toast\"]').length")" -gt 0 ]; do
  admin click "$(testid toast-dismiss)" >/dev/null
  admin wait 200 >/dev/null
done
grep -qi "not a backup" <<<"$(js "document.querySelector('[data-testid=\"restore-file-status\"]').innerText")" || fail "the status of the file should stay after dismissing the toast"

step "An option changed while the preview is requested discards it"
admin upload "$(testid restore-file)" "$WORK/plain.json" >/dev/null
admin wait 500 >/dev/null
# Holds the preview requests for 1.5 seconds, so that the option changes before the answer
admin eval "(() => { const originalFetch = window.fetch; window.fetch = (input, init) => String(input).includes('/restore/preview') ? new Promise((resolve) => setTimeout(resolve, 1500)).then(() => originalFetch(input, init)) : originalFetch(input, init) })()" >/dev/null
admin click "$(testid restore-preview)" >/dev/null
admin click "$(testid restore-overwrite)" >/dev/null
admin wait 2500 >/dev/null
[ "$(js "document.querySelectorAll('[data-testid=\"restore-plan-table\"]').length")" = 0 ] || fail "the preview opened although an option changed during the request"
admin reload >/dev/null

step "Backups of v6, written while the project was called Gatus, are still recognised and previewed by the screen"
# The two files were downloaded from the published image jniltinho/gatus:v6.3.0 (internal/adminbackup/testdata)
LEGACY="$ROOT/internal/adminbackup/testdata"
grep -q '"format": "gatus-admin-backup"' "$LEGACY/backup-v6.3.0.json" || fail "the plain fixture does not have the format of v6"
admin reload >/dev/null
admin wait "$(testid restore-file)" >/dev/null
admin upload "$(testid restore-file)" "$LEGACY/backup-v6.3.0.json" >/dev/null
admin wait 500 >/dev/null
[ "$(js "document.querySelectorAll('[data-testid=\"restore-password\"]').length")" = 0 ] || fail "a plain backup of v6 was taken for an encrypted one"
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null || fail "the preview of a plain backup of v6 was not shown"
[ "$(plan_action endpoint web_site)" = create ] || fail "the endpoint of the backup of v6 is not planned to be created"
admin click "$(testid restore-plan-close)" >/dev/null
admin reload >/dev/null
admin wait "$(testid restore-file)" >/dev/null
admin upload "$(testid restore-file)" "$LEGACY/backup-v6.3.0.enc.json" >/dev/null
admin wait "$(testid restore-password)" >/dev/null || fail "the password of an encrypted backup of v6 is not asked"
admin fill "$(testid restore-password)" "fixture-backup-password-123" >/dev/null
admin click "$(testid restore-preview)" >/dev/null
admin wait "$(testid restore-plan-table)" >/dev/null || fail "the preview of an encrypted backup of v6 was not shown"
[ "$(plan_action endpoint web_site)" = create ] || fail "the endpoint of the encrypted backup of v6 is not planned to be created"
admin screenshot "$PRINTS/09-restore-backup-of-v6.png" >/dev/null
admin click "$(testid restore-plan-close)" >/dev/null

echo "OK: $STEP steps; screenshots in $PRINTS"
