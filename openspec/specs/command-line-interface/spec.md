# command-line-interface Specification

## Purpose
TBD - created by archiving change migrate-to-echo-and-cobra. Update Purpose after archive.
## Requirements
### Requirement: Comandos do binário
O binário `go-uptime` MUST oferecer os comandos `serve`, `version`, `config validate`, `password hash` e `healthcheck`, com `--help` em cada um. Executado **sem argumentos**, o binário MUST se comportar como `go-uptime serve`, para o `ENTRYPOINT ["/go-uptime"]` da imagem e quem já o executa assim continuarem funcionando.

#### Scenario: Sem argumentos
- **WHEN** o binário é executado sem argumentos com `GO_UPTIME_CONFIG_PATH` definido
- **THEN** o servidor sobe com aquela configuração, como antes desta change

#### Scenario: Ajuda
- **WHEN** o operador executa `go-uptime --help`
- **THEN** a saída lista os cinco comandos e o processo sai com 0, sem subir o servidor

#### Scenario: Comando que não depende da configuração
- **WHEN** o operador executa `go-uptime version` ou `go-uptime password hash` sem nenhum `config.yaml`
- **THEN** o comando funciona

### Requirement: Precedência de flags e variáveis de ambiente
`go-uptime serve` MUST aceitar `--config` e `--log-level`. O valor MUST vir da flag quando ela for passada; senão da variável de ambiente (`GO_UPTIME_CONFIG_PATH` e `GO_UPTIME_LOG_LEVEL`, com as variáveis antigas na ordem descrita abaixo); senão do padrão atual. `GO_UPTIME_DELAY_START_SECONDS` MUST continuar valendo. O caminho resolvido MUST ser o mesmo em toda recarga da configuração, e um `--config` explícito que não existe MUST ser erro, sem cair nos caminhos padrão.

**Variáveis antigas:** `GATUS_CONFIG_PATH`, `GATUS_LOG_LEVEL` e `GATUS_DELAY_START_SECONDS` MUST continuar aceitas como apelido da variável `GO_UPTIME_*` correspondente, com um aviso no log a cada início citando a variável nova. `GATUS_CONFIG_FILE`, que já era obsoleta, MUST continuar aceita e MUST NOT ganhar um nome novo. Uma variável de ambiente com valor vazio MUST contar como não definida; uma flag `--config` explícita, mesmo vazia, MUST continuar sendo validada como hoje. A ordem total do caminho da configuração MUST ser: `--config`, `GO_UPTIME_CONFIG_PATH`, `GATUS_CONFIG_PATH`, `GATUS_CONFIG_FILE`, o padrão; a do nível de log, `--log-level`, `GO_UPTIME_LOG_LEVEL`, `GATUS_LOG_LEVEL`, o padrão. O valor de uma variável vencida MUST ser ignorado.

#### Scenario: Flag vence o ambiente
- **WHEN** `GO_UPTIME_CONFIG_PATH=/a.yaml` está definido e o operador executa `go-uptime serve --config /b.yaml`
- **THEN** a configuração carregada é `/b.yaml`

#### Scenario: Recarga com a configuração da flag
- **WHEN** o servidor subiu com `--config /b.yaml`, `GO_UPTIME_CONFIG_PATH=/a.yaml` está definido e `/b.yaml` é alterado
- **THEN** a recarga lê `/b.yaml`

#### Scenario: Caminho explícito inexistente
- **WHEN** o operador executa `go-uptime config validate --config /nao-existe.yaml` e existe um `config/config.yaml` padrão
- **THEN** o comando sai com 1, sem validar o arquivo padrão

#### Scenario: Só o ambiente
- **WHEN** `GO_UPTIME_LOG_LEVEL=DEBUG` está definido e nenhuma flag é passada
- **THEN** o nível de log é `DEBUG`

### Requirement: Validação da configuração
`go-uptime config validate` MUST carregar e validar a configuração sem abrir o storage, sem subir o servidor e sem fazer requisições de rede. MUST sair com 0 quando válida e com 1, mostrando o erro, quando inválida.

#### Scenario: Configuração inválida
- **WHEN** o `config.yaml` tem um endpoint sem condições
- **THEN** o comando mostra o erro de validação e sai com 1
- **AND** nenhum arquivo de banco é criado

### Requirement: Hash da senha da administração
`go-uptime password hash` MUST ler a senha da entrada padrão — num terminal, duas vezes e sem eco; por pipe, uma linha, descartando a quebra de linha final — e imprimir o hash bcrypt em base64 com o alfabeto URL, aceito em `security.basic.password-bcrypt-base64` e em `auth.password-bcrypt-base64` das status pages. O comando MUST NOT aceitar a senha como argumento nem como flag, e MUST NOT registrá-la em log.

#### Scenario: Hash aceito pela configuração
- **WHEN** o operador envia `uma-senha-boa` pela entrada padrão
- **THEN** o hash impresso valida com `uma-senha-boa` na mesma comparação que o login usa

#### Scenario: Senha por pipe com quebra de linha
- **WHEN** o operador executa `printf 'uma-senha-boa\n' | go-uptime password hash`
- **THEN** o hash valida com `uma-senha-boa`, sem a quebra de linha

#### Scenario: Senha por argumento
- **WHEN** o operador executa `go-uptime password hash uma-senha-boa`
- **THEN** o comando recusa o argumento e sai com código diferente de 0

### Requirement: Verificação de saúde
`go-uptime healthcheck` MUST fazer `GET /health` no endereço, na porta e no esquema da configuração (`web.address`, `web.port`, `web.tls`), ou no de `--url`, e sair com 0 quando a resposta for 200 e com 1 em qualquer outro caso, em no máximo 5 segundos. Com `web.tls`, MUST falar HTTPS com o laço local sem verificar o certificado. Um endereço curinga MUST virar o laço local; com `--url` a configuração MUST NOT ser lida; e o comando MUST NOT aplicar o atraso de início, abrir o storage, seguir redirecionamento nem usar proxy do ambiente. As imagens Docker MUST usá-lo como `HEALTHCHECK` em forma exec, porque não têm shell.

#### Scenario: Porta configurada
- **WHEN** a configuração tem `web.port: 9090` e o servidor está no ar
- **THEN** `go-uptime healthcheck` sem `--url` sai com 0

#### Scenario: Servidor fora do ar
- **WHEN** nada escuta no endereço
- **THEN** o comando sai com 1 em até 5 segundos

### Requirement: Versão
`go-uptime version` MUST imprimir a versão, o commit e a data da construção, preenchidos na compilação pelos alvos do `Makefile`, pelos `Dockerfile` e pelo workflow de release.

#### Scenario: Binário de release
- **WHEN** o operador executa `go-uptime version` no binário da release `v7.0.0`
- **THEN** a saída contém `7.0.0`

