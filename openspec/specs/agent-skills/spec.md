# agent-skills Specification

## Purpose
TBD - created by archiving change add-admin-endpoint-management. Update Purpose after archive.
## Requirements
### Requirement: Skills de Go
O repositório MUST incluir em `.claude/skills/` as skills de Go do repositório `jniltinho/llama-model` aplicáveis ao Go Uptime: `golang-how-to`, `golang-code-style`, `golang-naming`, `golang-error-handling`, `golang-concurrency`, `golang-structs-interfaces`, `golang-database`, `golang-testing`, `golang-security`, `golang-lint`, `golang-modernize`, `golang-dependency-management`, `golang-documentation`, `golang-observability` e `golang-popular-libraries`, com conteúdo idêntico ao de origem, acompanhadas do texto da licença MIT de `samber/cc-skills-golang`.

#### Scenario: Skills presentes com licença
- **WHEN** alguém lista `.claude/skills/`
- **THEN** cada uma das 15 skills tem um `SKILL.md` idêntico ao do `jniltinho/llama-model`
- **AND** o texto da licença MIT de `samber/cc-skills-golang` está presente

### Requirement: Skill agent-browser
O repositório MUST incluir `.claude/skills/agent-browser/SKILL.md` idêntico ao do repositório `jniltinho/go-postfixadmin`, acompanhado do aviso de licença Apache-2.0 de `vercel-labs/agent-browser`.

#### Scenario: Skill de testes pelo navegador
- **WHEN** alguém lista `.claude/skills/agent-browser/`
- **THEN** existem o `SKILL.md` com o mesmo conteúdo do `jniltinho/go-postfixadmin` e o aviso de licença

### Requirement: Skill create-release
O repositório MUST incluir `.claude/skills/create-release/SKILL.md` com a mesma estrutura da skill do `jniltinho/llama-model` (pré-checagens, esquema de versão, revisão e categorização de commits, criação e envio da tag, acompanhamento do workflow, ajuste das notas e verificação), adaptada ao Go Uptime: branch `main`, tags SemVer simples `vX.Y.Z`, o teste de atualização `test/e2e/upgrade.sh` da última release para a candidata antes da tag, pacotes `linux/amd64` e `linux/arm64` pelo workflow, imagem `jniltinho/go-uptime` publicada com `make docker-release` a partir de uma árvore limpa no commit da tag, notas em inglês a partir da tag anterior e link de changelog para `jniltinho/go-uptime`.

#### Scenario: Próxima versão
- **WHEN** a última tag é `v7.0.0`, o repositório também contém as tags antigas `v5.36.0-fork.27` e `v6.3.0`, e um agente segue a skill para uma release de correção
- **THEN** a versão proposta é `v7.0.1`

### Requirement: Skills fora do escopo
Skills voltadas a bibliotecas que o projeto não usa (`golang-spf13-viper`, `golang-samber-lo`, `golang-samber-slog` e `golang-swagger`) MUST NOT ser incluídas. As skills de linha de comando (`golang-cli`, `golang-spf13-cobra`) deixaram de estar fora do escopo quando o binário passou a usar Cobra, na v6.0.0: incluí-las é opcional.

#### Scenario: Skill de biblioteca que o projeto não usa
- **WHEN** alguém lista `.claude/skills/`
- **THEN** não existe o diretório `golang-spf13-viper`

### Requirement: Regras para agentes
O repositório MUST ter um único `AGENTS.md`, sem um arquivo de regras "do fork" ao lado, com: as skills de Go obrigatórias; o uso da skill `create-release`; os alvos `build`, `fmt`, `vet`, `lint` e `release-cross`; as regras da administração de endpoints (endpoints gerenciados no storage, registro do watchdog, labels congeladas, validação estrita); a execução dos testes ponta a ponta com `agent-browser` e capturas em `dist/prints/` fora do git; a instrução de usar `go mod tidy` sem gerar `vendor/`; os nomes que ficam por compatibilidade com o Gatus e por quê; e a origem do projeto. O `AGENTS.md` MUST NOT descrever um procedimento de sincronização com outro repositório.

#### Scenario: Nova dependência Go
- **WHEN** um agente segue as regras do repositório para adicionar uma dependência Go
- **THEN** a instrução é executar `go mod tidy`, sem gerar `vendor/`

#### Scenario: Nome antigo num arquivo novo
- **WHEN** um agente escreve um arquivo novo e precisa citar o projeto
- **THEN** as regras mandam usar Go Uptime ou `go-uptime`, e reservam `gatus` para a lista de compatibilidade e para a origem

