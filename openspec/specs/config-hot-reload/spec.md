# config-hot-reload Specification

## Purpose
TBD - created by archiving change add-admin-endpoint-management. Update Purpose after archive.
## Requirements
### Requirement: Validar a configuração antes de parar
Ao detectar alteração no arquivo de configuração, o sistema MUST carregar e validar a nova configuração antes de parar o servidor HTTP e o monitoramento. Com `skip-invalid-config-update: true` e configuração inválida, o sistema MUST continuar servindo HTTP e monitorando com a configuração anterior, registrar o erro no log e não tentar recarregar o mesmo arquivo de novo até que ele seja alterado. Com `skip-invalid-config-update: false` e configuração inválida, o sistema MUST manter o comportamento atual de encerrar o processo com erro.

#### Scenario: YAML inválido com skip-invalid-config-update
- **WHEN** `skip-invalid-config-update` é `true` e o arquivo de configuração é salvo com YAML inválido
- **THEN** `GET /health` continua respondendo `{"status":"UP"}`
- **AND** os endpoints continuam registrando resultados com a configuração anterior
- **AND** o log registra o erro de validação

#### Scenario: Correção posterior
- **WHEN** depois de um YAML inválido ignorado o arquivo é corrigido
- **THEN** a nova configuração é aplicada no ciclo de recarga seguinte

#### Scenario: YAML inválido sem skip-invalid-config-update
- **WHEN** `skip-invalid-config-update` é `false` e o arquivo de configuração é salvo com YAML inválido
- **THEN** o processo encerra com o erro de validação, como na versão atual

### Requirement: Recarga preserva endpoints gerenciados
A recarga da configuração MUST repetir a carga dos endpoints gerenciados antes de reiniciar o monitoramento, e MUST impedir escritas da administração entre parar e iniciar o Go Uptime.

#### Scenario: Recarga válida com endpoints gerenciados
- **WHEN** o YAML é alterado de forma válida e existem endpoints gerenciados
- **THEN** após a recarga os endpoints gerenciados continuam monitorados com o histórico mantido

