#!/usr/bin/env bash
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
# Testes ponta a ponta das telas de administração de endpoints, com agent-browser.
#
#   test/e2e/admin.sh
#
# Sobe o Go Uptime compilado localmente (SQLite temporário, basic auth, admin habilitado), percorre as telas e salva
# capturas de tela em dist/prints/e2e/ (dist/ está no .gitignore). Exige agent-browser com Chrome instalado.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
PORT=${E2E_PORT:-18090}
BASE="http://127.0.0.1:$PORT"
PRINTS="$ROOT/dist/prints/e2e"
WORK=$(mktemp -d)
USERNAME=admin
PASSWORD='e2e-senha'
# bcrypt (custo 10) de PASSWORD, em base64 com alfabeto de URL (como o Go Uptime decodifica)
PASSWORD_HASH='JDJhJDEwJHo1LnE5empYYkN5Vm1Vd1RmNXZPMS5SeWRCdlc3UlMxMXBHdmpwcDBUUTZiMXlIQ1R3RVRT'

command -v agent-browser >/dev/null || { echo "agent-browser não encontrado"; exit 1; }
mkdir -p "$PRINTS"

# Storage: SQLite temporário por padrão. E2E_STORAGE_TYPE e E2E_STORAGE_PATH rodam o roteiro com outro banco, que
# precisa estar vazio (ex.: E2E_STORAGE_TYPE=mysql E2E_STORAGE_PATH='root:senha@tcp(127.0.0.1:53307)/go_uptime_e2e')
STORAGE_TYPE=${E2E_STORAGE_TYPE:-sqlite}
STORAGE_PATH=${E2E_STORAGE_PATH:-$WORK/go-uptime.db}

echo "==> Compilando"
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
endpoints:
  - name: health
    group: core
    url: $BASE/health
    interval: 30s
    conditions:
      - "[STATUS] == 200"
CONFIG

GO_UPTIME_CONFIG_PATH="$WORK/config.yaml" dist/go-uptime > "$WORK/go-uptime.log" 2>&1 &
SERVER_PID=$!
cleanup() {
  agent-browser close >/dev/null 2>&1 || true
  kill "$SERVER_PID" >/dev/null 2>&1 || true
  wait "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

for _ in $(seq 1 60); do
  curl -sf "$BASE/health" >/dev/null && break
  sleep 1
done
curl -sf "$BASE/health" >/dev/null || { echo "Go Uptime não subiu"; cat "$WORK/go-uptime.log"; exit 1; }

STEP=0
step() {
  STEP=$((STEP + 1))
  echo "==> $STEP. $*"
}
fail() {
  echo "FALHOU: $*"
  agent-browser screenshot --full "$PRINTS/erro.png" >/dev/null 2>&1 || true
  tail -20 "$WORK/go-uptime.log"
  exit 1
}
shot() {
  agent-browser screenshot --full "$PRINTS/$1.png" >/dev/null
}
wait_text() {
  agent-browser wait --text "$1" >/dev/null || fail "texto '$1' não apareceu"
}
wait_for() {
  agent-browser wait "$1" >/dev/null || fail "elemento '$1' não apareceu"
}
testid() {
  echo "[data-testid=\"$1\"]"
}
api_status() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}

agent-browser set viewport 1440 900 >/dev/null

step "API sem credenciais responde 401"
[ "$(api_status "$BASE/api/v1/admin/endpoints")" = 401 ] || fail "esperado 401 sem credenciais"

# Sem sessão, a administração leva à tela de login do security.basic, que volta para a página pedida
step "Lista com o endpoint do arquivo de configuração, depois da tela de login"
agent-browser open "$BASE/admin" >/dev/null
wait_for "$(testid login-username)"
agent-browser fill "$(testid login-username)" "$USERNAME" >/dev/null
agent-browser fill "$(testid login-password)" "$PASSWORD" >/dev/null
agent-browser click "$(testid login-submit)" >/dev/null
wait_for "$(testid admin-row-core_health)"
shot 02-lista

step "Link Admin no dashboard"
agent-browser open "$BASE/" >/dev/null
wait_for "$(testid admin-link)"
shot 03-dashboard

step "Criação pelo formulário: validar, testar e alternar para YAML"
agent-browser open "$BASE/admin/endpoints/new" >/dev/null
wait_for "$(testid admin-field-name)"
agent-browser fill "$(testid admin-field-name)" "site" >/dev/null
agent-browser click "$(testid admin-field-group-select) button" >/dev/null
wait_for "$(testid admin-group-option-core)"
agent-browser click "$(testid admin-group-new)" >/dev/null
wait_for "$(testid admin-field-group)"
agent-browser fill "$(testid admin-field-group)" "web" >/dev/null
agent-browser fill "$(testid admin-field-url)" "$BASE/health" >/dev/null
agent-browser fill "$(testid admin-field-interval)" "1m" >/dev/null
agent-browser click "$(testid admin-field-insecure)" >/dev/null
agent-browser click "$(testid admin-add-header)" >/dev/null
agent-browser fill "$(testid admin-field-header-name-0)" "Authorization" >/dev/null
agent-browser fill "$(testid admin-field-header-value-0)" "Bearer e2e-secret" >/dev/null
agent-browser click "$(testid admin-validate)" >/dev/null
wait_text "Valid definition"
agent-browser click "$(testid admin-test)" >/dev/null
wait_for "$(testid admin-test-result)"
shot 04-formulario-testado
# Depois do teste a página rola e as abas saem da tela: o click do agent-browser não rola até elas
agent-browser scrollintoview "$(testid admin-mode-yaml)" >/dev/null
agent-browser click "$(testid admin-mode-yaml)" >/dev/null
wait_for "$(testid admin-yaml)"
agent-browser get value "$(testid admin-yaml)" | grep -q "Bearer e2e-secret" || fail "o YAML gerado não contém o header digitado"
agent-browser get value "$(testid admin-yaml)" | grep -q "insecure: true" || fail "o YAML gerado não contém client.insecure"
agent-browser get value "$(testid admin-yaml)" | grep -q "ignore-redirect" && fail "o YAML gerado não deveria conter client.ignore-redirect"
shot 05-modo-yaml
agent-browser click "$(testid admin-mode-form)" >/dev/null
wait_for "$(testid admin-field-header-value-0)"
[ "$(agent-browser get value "$(testid admin-field-header-value-0)")" = "Bearer e2e-secret" ] || fail "o segredo digitado mudou ao voltar para o formulário"
agent-browser click "$(testid admin-save)" >/dev/null
wait_for "$(testid admin-row-web_site)"
shot 06-lista-com-endpoint

step "Edição: segredo mascarado, troca de grupo mantendo o histórico e novo intervalo"
RESULTS_BEFORE=$(curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/web_site/statuses" | grep -o '"timestamp"' | wc -l)
[ "$RESULTS_BEFORE" -gt 0 ] || fail "esperado histórico em web_site antes da troca de grupo"
agent-browser click "$(testid admin-open-web_site)" >/dev/null
wait_for "$(testid admin-field-interval)"
[ "$(agent-browser get value "$(testid admin-field-header-value-0)")" = "********" ] || fail "o segredo deveria aparecer mascarado"
[ "$(agent-browser eval "document.querySelector('[data-testid=admin-field-insecure]').checked")" = "true" ] || fail "client.insecure salvo deveria aparecer marcado"
[ "$(agent-browser eval "document.querySelector('[data-testid=admin-field-follow-redirects]').checked")" = "true" ] || fail "seguir redirecionamentos deveria continuar marcado"
[ "$(agent-browser eval "document.querySelector('[data-testid=admin-field-name]').disabled")" = "false" ] || fail "o nome deveria ser editável"
agent-browser click "$(testid admin-field-group-select) button" >/dev/null
wait_for "$(testid admin-group-option-web)"
agent-browser click "$(testid admin-group-new)" >/dev/null
wait_for "$(testid admin-field-group)"
agent-browser fill "$(testid admin-field-group)" "clientes" >/dev/null
wait_for "$(testid admin-key-change)"
agent-browser get text "$(testid admin-key-change)" | grep -q "clientes_site" || fail "o aviso de troca de chave não mostra a chave nova"
agent-browser fill "$(testid admin-field-interval)" "2m" >/dev/null
shot 07-edicao-troca-de-grupo
agent-browser scrollintoview "$(testid admin-save)" >/dev/null
agent-browser click "$(testid admin-save)" >/dev/null
wait_for "$(testid admin-row-clientes_site)"
wait_text "2m0s"
DEFINITION=$(curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/endpoints/clientes_site")
echo "$DEFINITION" | grep -q '"version":2' || fail "esperada a versão 2 depois da edição"
[ "$(api_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/endpoints/web_site")" = 404 ] || fail "a chave antiga ainda existe"
RESULTS_AFTER=$(curl -s -u "$USERNAME:$PASSWORD" "$BASE/api/v1/endpoints/clientes_site/statuses" | grep -o '"timestamp"' | wc -l)
[ "$RESULTS_AFTER" -ge "$RESULTS_BEFORE" ] || fail "o histórico não foi mantido na chave nova ($RESULTS_BEFORE antes, $RESULTS_AFTER depois)"

step "Desabilitar e habilitar"
agent-browser click "$(testid admin-toggle-clientes_site)" >/dev/null
wait_text "clientes_site disabled"
shot 08-desabilitado
agent-browser click "$(testid admin-toggle-clientes_site)" >/dev/null
wait_text "clientes_site enabled"

step "Endpoint do arquivo de configuração somente leitura"
agent-browser click "$(testid admin-open-core_health)" >/dev/null
wait_text "can only be viewed"
shot 09-yaml-somente-leitura

step "Tema escuro"
agent-browser open "$BASE/admin" >/dev/null
wait_for "$(testid admin-row-clientes_site)"
agent-browser eval "document.documentElement.classList.add('dark')" >/dev/null
# Espera a transição de cores dos botões terminar antes da captura
agent-browser wait 700 >/dev/null
shot 10-lista-escuro
agent-browser open "$BASE/admin/endpoints/new" >/dev/null
wait_for "$(testid admin-field-name)"
agent-browser eval "document.documentElement.classList.add('dark')" >/dev/null
# Espera a transição de cores dos botões terminar antes da captura
agent-browser wait 700 >/dev/null
shot 11-formulario-escuro

step "Remoção: cancelar e confirmar"
agent-browser open "$BASE/admin" >/dev/null
wait_for "$(testid admin-remove-clientes_site)"
agent-browser click "$(testid admin-remove-clientes_site)" >/dev/null
wait_for "$(testid confirm-dialog)"
shot 12-confirmar-remocao
agent-browser wait 300 >/dev/null
[ "$(agent-browser eval "document.activeElement === document.querySelector('[data-testid=\"confirm-cancel\"]')" 2>/dev/null | tr -d '"')" = true ] || fail "o foco inicial da confirmação não está em Cancel"
agent-browser press Escape >/dev/null
agent-browser wait 300 >/dev/null
[ "$(agent-browser eval "document.querySelectorAll('[data-testid=\"confirm-dialog\"]').length" 2>/dev/null | tr -d '"')" = 0 ] || fail "Esc não fechou a confirmação"
[ "$(api_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/endpoints/clientes_site")" = 200 ] || fail "Esc na confirmação removeu o endpoint"
agent-browser click "$(testid admin-remove-clientes_site)" >/dev/null
wait_for "$(testid confirm-cancel)"
agent-browser click "$(testid confirm-cancel)" >/dev/null
wait_for "$(testid admin-row-clientes_site)"
agent-browser click "$(testid admin-remove-clientes_site)" >/dev/null
wait_for "$(testid confirm-accept)"
agent-browser click "$(testid confirm-accept)" >/dev/null
wait_text "clientes_site removed"
shot 13-removido
[ "$(api_status -u "$USERNAME:$PASSWORD" "$BASE/api/v1/admin/endpoints/clientes_site")" = 404 ] || fail "o endpoint removido ainda existe"

echo "OK: $STEP etapas; capturas em $PRINTS"
