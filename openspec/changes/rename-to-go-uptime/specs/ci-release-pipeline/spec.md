## RENAMED Requirements

- FROM: `### Requirement: Versionamento do fork`
- TO: `### Requirement: Versionamento`


## MODIFIED Requirements

### Requirement: Workflow de CI
O repositório MUST ter `.github/workflows/ci.yml`, executado em push para `main` e em pull requests para `main`, que faça checkout com histórico completo antes de configurar o Go pela versão de `go.mod`, execute `make lint` e `make build`, e execute os testes Go com detector de race, com os privilégios exigidos pelo teste de ICMP e preservando os caches do Go. Quando existirem testes do store em PostgreSQL, o workflow MUST disponibilizar um PostgreSQL para eles. O workflow MUST falhar se qualquer etapa falhar.

#### Scenario: Código do fork sem formatação
- **WHEN** um pull request adiciona um arquivo Go que não passa no `gofmt`
- **THEN** o CI falha na etapa de lint

#### Scenario: Pull request correto
- **WHEN** um pull request passa em lint, build e testes
- **THEN** o CI conclui com sucesso

### Requirement: Alvos do Makefile
O `Makefile` MUST oferecer:
- `build`: binário estático em `dist/go-uptime`, com `CGO_ENABLED=0` só nas receitas que geram binários para distribuir (`build` e `release-cross`), nunca exportado para os testes;
- `fmt`: `gofmt` em todos os arquivos Go do repositório, versionados ou novos, fora de `third_party/`;
- `vet`: `go vet ./...`;
- `lint`: falha se algum desses arquivos precisar de `gofmt` ou se `go vet` falhar;
- `release-cross VERSION=<versão>`: gera `dist/go-uptime_<versão>_linux_amd64.tar.gz` e `dist/go-uptime_<versão>_linux_arm64.tar.gz`, cada um com `go-uptime`, `config.yaml`, `LICENSE`, `NOTICE` e `README.md`, e os binários em `dist/pkg/linux_<arch>/go-uptime`;
- `docker-release VERSION=<versão>`: executa `release-cross` e publica a imagem `jniltinho/go-uptime:v<versão>` para `linux/amd64` e `linux/arm64`.

Os alvos existentes (`install`, `run`, `test`, `frontend-install`, `frontend-build`, `frontend-dev`) MUST continuar funcionando.

#### Scenario: Lint cobre o repositório inteiro
- **WHEN** qualquer arquivo Go do repositório, fora de `third_party/`, não passa no `gofmt`
- **THEN** `make lint` falha citando o arquivo

#### Scenario: Pacotes de release
- **WHEN** alguém executa `make release-cross VERSION=7.0.0`
- **THEN** são gerados os dois pacotes com `go-uptime`, `config.yaml`, `LICENSE`, `NOTICE` e `README.md`

### Requirement: Imagem de release
O repositório MUST ter `Dockerfile.release`, que monta a imagem a partir dos binários de `dist/pkg/linux_<arch>/`, com certificados de CA e dados de fuso horário obtidos num estágio executado na plataforma de build, sem executar comandos na plataforma de destino. O binário MUST ficar em `/go-uptime`, que é o `ENTRYPOINT` e o comando do `HEALTHCHECK` (`/go-uptime healthcheck`). O `Dockerfile` de desenvolvimento MUST produzir uma imagem com o mesmo binário, o mesmo `ENTRYPOINT` e o mesmo `HEALTHCHECK`. `make docker-release` MUST publicar apenas a tag da versão em `jniltinho/go-uptime`, MUST NOT publicar `latest`, MUST NOT publicar em `jniltinho/gatus` e MUST recusar execução sem `VERSION` explícita.

#### Scenario: Build multi-arquitetura sem emulação
- **WHEN** a imagem de release é construída para `linux/amd64` e `linux/arm64` numa máquina sem QEMU
- **THEN** o build conclui com sucesso para as duas arquiteturas

#### Scenario: Publicação local da imagem
- **WHEN** alguém com `docker login` no Docker Hub executa `make docker-release VERSION=7.0.0`
- **THEN** a imagem `jniltinho/go-uptime:v7.0.0` fica disponível para `linux/amd64` e `linux/arm64`
- **AND** a tag `latest` não é criada nem alterada
- **AND** nada é publicado em `jniltinho/gatus`

#### Scenario: Sem versão explícita
- **WHEN** alguém executa `make docker-release` sem `VERSION`
- **THEN** o alvo falha sem publicar nada

### Requirement: Versionamento
As releases MUST usar tags SemVer simples, `vX.Y.Z`, sem sufixo. As tags `v5.36.0-fork.N`, do tempo em que o projeto era um fork, e as `v6.x`, publicadas com o nome Gatus, ficam no histórico e MUST NOT ser recriadas nem movidas. A primeira release com o nome go-uptime MUST ser a `v7.0.0`, porque a troca do binário, da imagem, do cookie de sessão, do prefixo das métricas e do agente HTTP é incompatível.

#### Scenario: Próxima release
- **WHEN** a última release é `v7.0.0` e a mudança seguinte é compatível e acrescenta uma opção
- **THEN** a próxima release é `v7.1.0`

#### Scenario: Tag com sufixo
- **WHEN** alguém envia a tag `v7.0.0-fork.1`
- **THEN** o workflow de release não é executado

### Requirement: Workflow de release
O repositório MUST ter `.github/workflows/release.yml`, executado no push de tags `vX.Y.Z` (`v[0-9]+.[0-9]+.[0-9]+`), que, nesta ordem, execute os testes Go com detector de race, execute `make release-cross` com a versão da tag sem o prefixo `v` e crie a GitHub Release com notas geradas a partir da tag `v*` imediatamente anterior, sem marcar como pré-release, marcada como a mais recente, com os dois pacotes `go-uptime_<versão>_linux_<arch>.tar.gz` anexados, conferindo ao final que a release tem os dois. Reexecutado sobre uma release que já existe, o workflow MUST reenviar os pacotes sem recriá-la. O workflow MUST NOT publicar imagens de container.

#### Scenario: Release publicada
- **WHEN** a tag `v7.0.0` é enviada ao GitHub
- **THEN** existe a GitHub Release `v7.0.0` com os pacotes `linux_amd64` e `linux_arm64`

#### Scenario: Testes falham
- **WHEN** os testes falham no workflow de release
- **THEN** nenhuma GitHub Release é criada

#### Scenario: Notas da segunda release
- **WHEN** a tag `v7.0.1` é publicada
- **THEN** as notas da release cobrem apenas os commits desde `v7.0.0`

### Requirement: Limpeza dos workflows herdados
O fork MUST NOT manter workflows que dependam de segredos ou processos do projeto original: `benchmark.yml`, `labeler.yml`, `publish-custom.yml`, `publish-experimental.yml`, `publish-latest.yml`, `publish-release.yml`, `regenerate-static-assets.yml`, `test.yml` e `test-ui.yml` MUST ser removidos, com os testes Go cobertos por `ci.yml`. O Dependabot MUST atualizar apenas `github-actions`.

#### Scenario: Push para main
- **WHEN** um commit é enviado para `main`
- **THEN** apenas o workflow de CI é executado
- **AND** nenhum login em registry de container é tentado
