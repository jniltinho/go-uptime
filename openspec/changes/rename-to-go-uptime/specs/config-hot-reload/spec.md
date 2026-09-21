## MODIFIED Requirements

### Requirement: Recarga preserva endpoints gerenciados
A recarga da configuração MUST repetir a carga dos endpoints gerenciados antes de reiniciar o monitoramento, e MUST impedir escritas da administração entre parar e iniciar o Go Uptime.

#### Scenario: Recarga válida com endpoints gerenciados
- **WHEN** o YAML é alterado de forma válida e existem endpoints gerenciados
- **THEN** após a recarga os endpoints gerenciados continuam monitorados com o histórico mantido
