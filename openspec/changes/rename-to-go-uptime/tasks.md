## 1. Módulo, binário e imagem

- [ ] 1.1 Módulo `github.com/jniltinho/go-uptime/v7`: `go.mod`, todos os imports, `-ldflags -X` do `Makefile` e dos `Dockerfile`s; `go build ./...`, `go vet ./...`.
- [ ] 1.2 Binário `go-uptime`: `Makefile` (`BINARY`, tarballs), `Dockerfile` e `Dockerfile.release` (`/go-uptime`, `/gatus` como ligação simbólica, `HEALTHCHECK`, `ENTRYPOINT`, **sem** `ENV GATUS_*` nem `ENV GO_UPTIME_*`, com `/LICENSE` e `/NOTICE`), `cmd/` (nome de uso do Cobra, textos de ajuda, `go-uptime version`), `.gitignore`.
- [ ] 1.3 Imagem `jniltinho/go-uptime`: `Makefile` (`DOCKER_IMAGE`), workflows, `.examples/` (painel do Grafana em `go_uptime_*`, `.examples/nixos` removido, Kubernetes), `docs/`; nas duas imagens (`Dockerfile` e `Dockerfile.release`): `/gatus` é ligação simbólica para `/go-uptime` no sistema de arquivos exportado, e um contêiner com `healthcheck` em `/gatus` fica saudável.

## 2. Compatibilidade

- [ ] 2.1 Variáveis `GO_UPTIME_CONFIG_PATH`, `GO_UPTIME_LOG_LEVEL` e `GO_UPTIME_DELAY_START_SECONDS` com as `GATUS_*` como apelido e `GATUS_CONFIG_FILE` mantida no fim da fila (função única em `cmd/root.go`, aviso uma vez por variável, vazio conta como não definida); testes da ordem total, incluindo os cruzamentos e os valores vazios; `GATUS_TEST_*` e `GATUS_MEASURE_ENDPOINT_LIMIT` renomeadas no código, no CI e nos docs.
- [ ] 2.2 Backup: `Decode`, `Unwrap` e `Decrypt` aceitando os dois formatos, `adminBackup.js` reconhecendo os quatro identificadores, escrita no novo, `appVersion` com `gatusVersion` aceito na leitura, nome `go-uptime-backup-…`; testes com arquivos **gerados pela v6.3.0** (claro e cifrado) guardados em `testdata`, de que o cabeçalho cifrado antigo continua autenticando, e pela tela (E2E `admin-backup`) com os dois arquivos da v6.
- [ ] 2.3 Cookies `go_uptime_session`, `go_uptime_state` e `go_uptime_nonce` (`internal/security`), testes do login básico e do fluxo OIDC, contrato HTTP.
- [ ] 2.4 `web/app/src/utils/storage.js` com a migração das 9 preferências; todos os componentes passando por ele; a remoção da chave legada `collapsed-groups` nos dois prefixos; eventos `go-uptime:theme` e `go-uptime:unauthorized`; testes de unidade (chave nova vence, antiga migra e é removida, storage indisponível). O `id` do `manifest.json` e o cookie `theme` não mudam.
- [ ] 2.5 `metrics-namespace` (padrão `go_uptime`, validação do nome, `gatus` reproduzindo os nomes atuais byte a byte); testes; `docs/`.
- [ ] 2.6 Alertas: os identificadores de máquina da lista do design ficam (tópico padrão do Zulip incluído, cada um com a capitalização de hoje), cada um com um comentário dizendo por quê e um teste que fixa o valor; `event-type` no Home Assistant; textos visíveis das notificações em Go Uptime; ícone do Mattermost e do Rocket.Chat no repositório novo; `User-Agent` `go-uptime/1.0` nas verificações e no Zulip; `label` do badge JSON.

## 3. Textos, documentação e ferramentas

- [ ] 3.1 Texto corrido no código, nos logs e nas mensagens de erro: `Gatus` → `Go Uptime`, por categoria e com lista de exclusão (D8, URLs de terceiros, histórico).
- [ ] 3.2 `README.md` reescrito; `NOTICE`; seção "Origem"; `docs/README.md`, `docs/status-pages.md` e demais guias; `docs/install-linux.md` em `/opt/go-uptime` com `docs/systemd/go-uptime.service` e a seção de migração da v6 com os dois caminhos; guia de migração Docker mandando manter volume, banco, nome do serviço e rótulos do Prometheus; `config.yaml`; `.examples/`; `AGENTS.fork.md` incorporado ao `AGENTS.md`.
- [ ] 3.3 Scripts `docs/*.py`: `manager-go-uptime.py` com `--url` e `GO_UPTIME_URL`/`GO_UPTIME_USERNAME`/`GO_UPTIME_PASSWORD`, aceitando `--gatus-url` e `GATUS_*` como apelidos; `docs/manager-gatus.py` fica como cópia idêntica e executável sozinha, com o teste de cópias e um teste que o executa numa pasta vazia com os argumentos e as variáveis antigos; `generate-admin-password.py` conferido contra a v7; suítes E2E, `docs/screenshots/capture.sh`, skill `create-release`.
- [ ] 3.4 Specs: blocos MODIFICADOS gerados por programa a partir do texto vigente, conferidos por diff; `openspec validate --all --strict`.
- [ ] 3.5 **Depende do aval do dono (design, D10):** aviso de uma linha em todo arquivo próprio que aceite comentário, por formato e na posição certa de cada um (depois do *shebang* e do *front matter*, antes do comentário de pacote em Go), sem depender de proveniência; `NOTICE` com o commit de partida e a lista, por caminho, dos arquivos próprios sem comentário possível; lista de exclusões explícita no script e no teste (`internal/test`, sem histórico do git); `go test`, `make lint`, `TestGoDocCoverage`, os scripts e as skills conferidos depois da inserção.
- [ ] 3.6 Teste de nomes em `internal/test`: `gatus` só pode aparecer nos arquivos e padrões de uma lista de permitidos (compatibilidade, origem, histórico).

## 4. Verificação

- [ ] 4.1 `make frontend-build`, `go test ./... -race`, `make lint`, testes de unidade do frontend, contrato HTTP regravado e revisado linha a linha.
- [ ] 4.2 Todas as suítes E2E, uma rodada de cada vez.
- [ ] 4.3 `test/e2e/upgrade.sh` reestruturado, de `jniltinho/gatus:v6.3.0` para a candidata: a mesma porta para a versão antiga e a nova, em sequência (mesma origem para cookies e `localStorage`); configuração fora do caminho padrão apontada por `GATUS_CONFIG_PATH` e `GATUS_LOG_LEVEL=DEBUG`, com o aviso no log; login pela tela com navegador antes e depois (um login a mais, sessão antiga recusada); preferência `sort-by` migrada; restore dos backups da v6 em claro e cifrado; asserts dos identificadores de formato antigo e novo.
- [ ] 4.4 Roteiro de migração do Linux executado de verdade num contêiner ou máquina descartável com systemd, nos dois caminhos do guia.

## 5. Entrega

- [ ] 5.1 PR da implementação com CI verde; revisão codex + grok do diff.
- [ ] 5.2 Com o aval do dono: `gh repo rename go-uptime`, descrição e tópicos, remotos locais (`origin` novo, `upstream` removido); o dono sai da rede de forks.
- [ ] 5.3 Release `v7.0.0`: pré-release, teste de upgrade, tag, tarballs, imagem `jniltinho/go-uptime:v7.0.0` (amd64 e arm64, sem `latest`), notas em inglês com o guia de migração no topo; script antigo e novo e a unit do systemd baixados pela URL da tag candidata e **executados** (o script numa pasta vazia).
- [ ] 5.4 Pós-release: descrição do `jniltinho/gatus` no Docker Hub apontando para o novo, `mariadb/` do dono, servidor de validação (com o aval dele), memória do agente, arquivamento da change.
