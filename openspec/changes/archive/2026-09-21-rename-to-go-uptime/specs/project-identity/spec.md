## ADDED Requirements

### Requirement: Nome do projeto
O projeto MUST chamar-se go-uptime, nas formas: `go-uptime` para o repositório (`jniltinho/go-uptime`), o binário, a imagem (`jniltinho/go-uptime`), os pacotes de release e os caminhos de instalação; `Go Uptime` em texto corrido; `GO_UPTIME_` como prefixo das variáveis de ambiente; `go_uptime` como prefixo padrão das métricas e no cookie de sessão; `go-uptime:` como prefixo das chaves que a interface guarda no navegador. O módulo Go MUST ser `github.com/jniltinho/go-uptime/v7`; a instalação por `go install <módulo>@<versão>` MUST NOT ser prometida na documentação enquanto o `go.mod` tiver diretivas `replace` para `third_party/`, que o `go install` recusa. O nome antigo MUST NOT aparecer no repositório fora de uma lista de permitidos, verificada por um teste: os nomes mantidos por compatibilidade, o crédito à origem, as changes arquivadas e as notas históricas.

#### Scenario: Nome antigo num arquivo novo
- **WHEN** um arquivo novo cita `gatus` fora da lista de permitidos
- **THEN** o teste de nomes falha citando o arquivo e a linha

#### Scenario: Construção a partir do código
- **WHEN** alguém clona o repositório na tag `v7.0.0` e executa `make build VERSION=7.0.0`
- **THEN** o binário gerado é `dist/go-uptime` e `dist/go-uptime version` mostra `7.0.0`

### Requirement: Atualização a partir da v6 sem perda
Uma instalação da v6.3.0 em Docker MUST poder ser atualizada trocando o nome da imagem, sem migração de banco e mantendo volumes, banco, nome do serviço no compose e rótulos do Prometheus. Para que um compose com `entrypoint` ou `healthcheck` próprios que chamam `/gatus` continue funcionando, as imagens da série 7.x MUST ter `/gatus` como ligação simbólica para `/go-uptime`. O guia de migração MUST enumerar as adaptações: **obrigatória**, o nome da imagem; **condicionais**, `metrics-namespace: gatus` para quem tem painéis ou alertas sobre as métricas antigas, a variável no compose para quem expande `${GATUS_*}` no YAML, e a liberação do agente `go-uptime/1.0` para quem filtra o agente HTTP; **nenhuma outra** edição do `config.yaml`. Numa instalação Linux pelo guia, a atualização MUST ter dois caminhos documentados: manter `/opt/gatus`, o usuário e a unit existentes trocando só o binário e a linha `ExecStart`; ou mudar para `/opt/go-uptime`, caso em que o guia MUST listar o que precisa ser editado (`storage.path` do `config.yaml`, `ReadWritePaths`, usuário e grupo, *drop-ins* e filtros do journal por `SyslogIdentifier`). As variáveis `GATUS_CONFIG_PATH`, `GATUS_LOG_LEVEL` e `GATUS_DELAY_START_SECONDS` MUST continuar valendo como apelido das novas, e a já obsoleta `GATUS_CONFIG_FILE` MUST continuar aceita, com aviso no log; uma variável definida com valor vazio MUST contar como não definida, e as imagens MUST NOT definir nenhuma das duas famílias com `ENV`, para que o padrão da imagem não esconda a variável antiga de um compose existente. Um YAML que expande `${GATUS_CONFIG_PATH}` ou `${GATUS_LOG_LEVEL}` contando com o valor que a imagem definia passa a receber vazio, e precisa definir a variável no compose. Um backup da administração feito pela v6, em claro ou cifrado, MUST ser restaurado, pela API e pela tela (o detector de formato do frontend MUST reconhecer os quatro identificadores). As nove preferências que a interface guarda no `localStorage` com o prefixo `gatus:` (`sort-by`, `filter-by`, `show-average-response-time`, `uncollapsed-groups`, `refresh-interval`, `response-time-chart-period`, `show-events`, `show-recent-checks-table` e `status-page-groups`) MUST ser migradas para o prefixo novo na primeira leitura, e a chave antiga MUST ser removida depois de migrada. O cookie `theme` e o `id` do `manifest.json` (`gatus`, que identifica a PWA já instalada) MUST NOT mudar. Essas compatibilidades MUST valer por toda a série 7.x.

O que muda de forma incompatível MUST estar nas notas da release e no guia de migração: o nome do binário e da imagem, os cookies de sessão e do fluxo OIDC (um login a mais), o prefixo padrão das métricas, o agente HTTP das verificações e dos alertas do Zulip, o rótulo do badge JSON (`go-uptime`), os títulos visíveis das notificações e o fato de a v6 não ler um backup feito pela v7.

#### Scenario: Compose da v6 com a imagem nova
- **WHEN** um `compose.yaml` da v6 tem a imagem trocada para `jniltinho/go-uptime:v7.0.0` e mantém `GATUS_CONFIG_PATH=/etc/monitor/config.yaml` (fora do caminho padrão) e `GATUS_LOG_LEVEL=DEBUG`
- **THEN** o Go Uptime inicia com essa configuração e em DEBUG, sobre o mesmo banco e o mesmo volume
- **AND** o log traz um aviso por variável antiga, citando a nova

#### Scenario: Compose que chama o executável antigo
- **WHEN** um `compose.yaml` da v6 define `healthcheck: test: ["CMD", "/gatus", "healthcheck"]` e só troca a imagem para a v7
- **THEN** a verificação de saúde continua passando, pela ligação `/gatus`

#### Scenario: A ligação é uma ligação
- **WHEN** o sistema de arquivos da imagem do `Dockerfile` e o da imagem do `Dockerfile.release` são exportados
- **THEN** nos dois `/gatus` é uma ligação simbólica cujo destino é `/go-uptime`, não uma segunda cópia do binário

#### Scenario: As duas variáveis definidas
- **WHEN** `GO_UPTIME_CONFIG_PATH=/a.yaml` e `GATUS_CONFIG_PATH=/b.yaml` estão definidas
- **THEN** a configuração lida é `/a.yaml`

#### Scenario: Variável nova vazia
- **WHEN** `GO_UPTIME_CONFIG_PATH` está definida com valor vazio e `GATUS_CONFIG_FILE=/c.yaml`
- **THEN** a configuração lida é `/c.yaml`

#### Scenario: Variável da imagem dentro do YAML
- **WHEN** um `config.yaml` da v6 usa `${GATUS_LOG_LEVEL}` num valor e o compose não define essa variável
- **THEN** o valor expandido passa a ser vazio, porque a imagem deixou de defini-la, e o guia de migração avisa disso

#### Scenario: Backup cifrado da v6
- **WHEN** um administrador restaura na v7 um arquivo com `format: "gatus-admin-backup-encrypted"` e a senha certa
- **THEN** a prévia e o restore funcionam como com um arquivo novo

#### Scenario: Backup da v6 pela tela
- **WHEN** um administrador escolhe na aba Backup um arquivo `gatus-admin-backup` feito pela v6
- **THEN** a tela o reconhece como backup em claro e a prévia abre

#### Scenario: Preferência do visitante
- **WHEN** um navegador com `gatus:sort-by` igual a `health` e sem `go-uptime:sort-by` abre o dashboard da v7
- **THEN** a ordenação guardada é aplicada, `go-uptime:sort-by` passa a existir e `gatus:sort-by` é removida

#### Scenario: Tema do visitante
- **WHEN** um navegador com o cookie `theme=bio` abre a v7
- **THEN** o tema Bio é aplicado, porque o cookie não mudou de nome

### Requirement: Prefixo das métricas
As métricas Prometheus MUST usar o prefixo definido por `metrics-namespace`, opção de primeiro nível cujo padrão é `go_uptime`. O valor MUST ser um nome válido de métrica Prometheus (`[a-zA-Z_][a-zA-Z0-9_]*`); outro valor MUST invalidar a configuração, também em `go-uptime config validate`. Com `metrics-namespace: gatus`, os nomes das métricas MUST ser idênticos aos da v6.

#### Scenario: Painel antigo
- **WHEN** a configuração tem `metrics: true` e `metrics-namespace: gatus`
- **THEN** `/metrics` expõe `gatus_results_total` e nenhuma métrica `go_uptime_*`

#### Scenario: Padrão
- **WHEN** a configuração tem `metrics: true` e não define `metrics-namespace`
- **THEN** `/metrics` expõe `go_uptime_results_total`

#### Scenario: Nome inválido
- **WHEN** a configuração tem `metrics-namespace: "go-uptime"`
- **THEN** a configuração é inválida, porque hífen não é válido em nome de métrica

### Requirement: Identificadores que vivem em outros sistemas
Os valores que as integrações de alerta usam para **identificar, deduplicar, fechar ou filtrar** coisas em sistemas de terceiros MUST continuar os mesmos da v6, porque mudá-los deixaria alertas abertos sem fechamento e filtros sem resultado:
- Opsgenie: as opções `source` (padrão `gatus`), `entity-prefix` (padrão `gatus-`) e `alias-prefix` (padrão `gatus-healthcheck-`), que compõem os campos `source`, `entity` e `alias` enviados;
- SIGNL4: o campo `X-S4-ExternalID`, `gatus-<chave>`; Squadcast: o campo `event_id`, `gatus-<chave>`, e a tag `source` com `gatus`;
- GitHub e Gitea: o título `alert(gatus): <endpoint>`, que é comparado por igualdade para fechar a issue;
- GitLab: a opção `monitoring-tool` (padrão `gatus`), enviada em `monitoring_tool` e usada para compor o título `alert(<monitoring-tool>): …`, que continua derivado dela;
- Datadog: a tag `source:gatus` e `source_type_name` com `gatus`;
- New Relic: `eventType` com `GatusAlert`, `service` com `Gatus` e `source` com `gatus`;
- PagerDuty: `source` com `Gatus`;
- Splunk: as opções `source` (padrão `gatus`) e `sourcetype` (padrão `gatus:alert`);
- Home Assistant: o evento `gatus_alert`, na URL e no campo `event_type`;
- iLert: a URL `…/events/gatus/`, que é o caminho da integração no serviço deles;
- Zulip: o tópico padrão `Gatus`, que decide onde as mensagens ficam agrupadas; um `topic` configurado continua valendo.

Cada valor MUST ser mantido com a capitalização acima, que é a de hoje, e MUST ter um teste que o fixe.

Os que já são configuráveis MUST continuar sendo, e o provedor do Home Assistant MUST aceitar `event-type`. O texto **visível** das notificações (títulos do Slack, Teams, Teams Workflows, Discord, Telegram, ntfy, Gotify, Pushover, incident.io, n8n, Mattermost e Rocket.Chat, e o nome do remetente nestes dois) MUST passar a dizer Go Uptime, e o ícone do Mattermost e do Rocket.Chat MUST apontar para um arquivo do próprio repositório, não do projeto original. O agente HTTP padrão das verificações e o do provedor Zulip MUST ser `go-uptime/1.0`, sem a versão do projeto; o das verificações MUST continuar substituível pelo cabeçalho `User-Agent` de cada endpoint.

#### Scenario: Alerta aberto antes da atualização
- **WHEN** um alerta do Opsgenie, uma issue do GitHub e um incidente do Squadcast foram abertos pela v6 e o endpoint se recupera depois da atualização para a v7
- **THEN** os três são fechados, porque alias, título e `event_id` são os mesmos

#### Scenario: Automação do Home Assistant
- **WHEN** a configuração do provedor não define `event-type`
- **THEN** o evento disparado é `gatus_alert`

#### Scenario: Notificação no Slack
- **WHEN** um alerta é enviado ao Slack pela v7
- **THEN** o título da mensagem diz Go Uptime

### Requirement: Scripts baixados por URL
`docs/manager-go-uptime.py` MUST aceitar `--url`, `GO_UPTIME_URL`, `GO_UPTIME_USERNAME` e `GO_UPTIME_PASSWORD`, e MUST aceitar `--gatus-url`, `GATUS_URL`, `GATUS_USERNAME` e `GATUS_PASSWORD` como apelidos, com as novas vencendo. O caminho antigo, `docs/manager-gatus.py`, MUST continuar em `main` (a branch padrão, que se chamava `master` até a v6) durante a série 7.x como um arquivo **completo e executável sozinho**, idêntico byte a byte ao novo, porque quem o baixa por URL não baixa o outro; um teste MUST falhar quando os dois arquivos diferirem.

#### Scenario: Script antigo baixado sozinho
- **WHEN** alguém baixa só `docs/manager-gatus.py` para uma pasta vazia e executa `GATUS_URL=… GATUS_PASSWORD=… python3 manager-gatus.py endpoints` contra a v7
- **THEN** o comando funciona como na v6

#### Scenario: Cópias divergentes
- **WHEN** um commit altera `docs/manager-go-uptime.py` e não `docs/manager-gatus.py`
- **THEN** o teste de cópias falha

### Requirement: Origem e licença
O repositório MUST manter o arquivo `LICENSE` (Apache-2.0) e MUST ter um arquivo `NOTICE` dizendo que o projeto deriva do Gatus, de TwiN, com o endereço do projeto original e o commit de que partiu, e declarando que os arquivos foram modificados desde então. Para a seção 4(b) da licença, **todo arquivo próprio do repositório** que aceite comentário — código, `go.mod`, `Dockerfile`, `Makefile`, workflows, HTML, CSS, SVG, YAML, scripts, units do systemd, documentação — MUST levar um aviso de uma linha, na sintaxe de comentário do formato, dizendo que o arquivo faz parte do go-uptime, derivado do Gatus, que os arquivos que existiam no Gatus foram modificados, e remetendo ao `NOTICE`. A regra MUST NOT depender de descobrir quais arquivos vieram do original: a detecção de renomeação do git perde arquivos movidos e muito alterados (`api/config.go` → `internal/api/config.go` aparece como apagado e criado), então o aviso vai em todos.

**Posição do aviso:** na primeira linha, exceto onde isso quebraria o arquivo — depois do *shebang* em scripts, depois do *front matter* em Markdown que o tenha, depois do `DOCTYPE` ou da declaração XML em HTML e SVG, e, em Go, **antes** do comentário de pacote existente, separado dele por uma linha em branco, de modo que o comentário continue colado à declaração `package` e `TestGoDocCoverage` continue passando.

**Arquivos próprios que não aceitam comentário** (JSON como `web/app/package.json`, `package-lock.json` e `manifest.json`, `go.sum`, `.nvmrc`) MUST ser tratados um a um: o `NOTICE` MUST listá-los **por caminho**, dizendo de cada um que deriva do Gatus e foi modificado onde existia lá, porque o formato não comporta o aviso. Um teste MUST falhar quando um arquivo próprio não tiver o aviso nem constar dessa lista, sem precisar do histórico do git.

**Fora da regra**, porque não são arquivos do projeto ou não podem mudar de bytes: `third_party/`, `node_modules`, os arquivos gerados de `web/static`, `openspec/`, as skills importadas de `.claude/skills/` (que MUST continuar idênticas à origem) com a licença delas, as fontes Inter e a licença OFL, o próprio `LICENSE`, os arquivos de `testdata/` e demais *fixtures* (contrato HTTP, backups da v6, certificados, arquivos deliberadamente inválidos), imagens binárias (PNG, JPG, ICO), fontes e `.gitkeep`. Os SVG do projeto, como `.github/assets/logo.svg`, são texto e levam o aviso. O `NOTICE` MUST NOT declarar esses arquivos como modificados do Gatus. O `README.md` MUST ter uma seção sobre a origem. Todas as formas de distribuição MUST levar `LICENSE` e `NOTICE`: os pacotes de release e as imagens de container (em `/LICENSE` e `/NOTICE`). Avisos de copyright existentes nos arquivos MUST NOT ser removidos.

#### Scenario: Pacote de release
- **WHEN** alguém extrai `go-uptime_7.0.0_linux_amd64.tar.gz`
- **THEN** encontra `go-uptime`, `config.yaml`, `LICENSE`, `NOTICE` e `README.md`

#### Scenario: Arquivo sem o aviso
- **WHEN** o `Makefile` ou um `internal/novo/novo.go` recém-criado não tem a linha de aviso
- **THEN** o teste de avisos falha citando o arquivo

#### Scenario: Formato sem comentário
- **WHEN** alguém procura o aviso em `web/app/package.json`
- **THEN** não há, e o `NOTICE` lista `web/app/package.json` pelo caminho entre os arquivos modificados que não comportam o aviso

#### Scenario: Script com shebang
- **WHEN** o aviso é posto em `test/e2e/login.sh`
- **THEN** a primeira linha continua sendo o *shebang* e o aviso é a segunda

#### Scenario: Skill importada
- **WHEN** o script de avisos percorre `.claude/skills/golang-testing/`
- **THEN** nenhum arquivo é alterado

#### Scenario: Imagem de container
- **WHEN** alguém copia `/LICENSE` e `/NOTICE` de `jniltinho/go-uptime:v7.0.0`
- **THEN** os dois arquivos existem
