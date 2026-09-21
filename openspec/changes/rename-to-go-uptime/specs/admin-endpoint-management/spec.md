## MODIFIED Requirements

### Requirement: Persistência de endpoints gerenciados
O sistema MUST persistir cada endpoint criado pela administração web no storage configurado (SQLite ou PostgreSQL), guardando a definição enviada sem os valores padrão, uma versão inteira incrementada a cada alteração, a data de criação, a data da última alteração e o autor da última alteração. Os valores padrão MUST ser aplicados apenas em memória.

#### Scenario: Endpoint criado sobrevive ao reinício
- **WHEN** um administrador cria o endpoint `api` no grupo `core` e o Go Uptime é reiniciado
- **THEN** o endpoint `core_api` volta a ser monitorado
- **AND** os resultados registrados antes do reinício continuam disponíveis

#### Scenario: Metadados e versão
- **WHEN** o administrador `ops@exemplo.com` altera um endpoint gerenciado que estava na versão 3
- **THEN** o registro armazenado passa à versão 4, com a data da alteração e `ops@exemplo.com` como autor

#### Scenario: Padrões do provedor não ficam congelados
- **WHEN** um endpoint gerenciado tem um alerta `slack` sem `failure-threshold` e o `default-alert` do provedor `slack` no YAML muda de 3 para 5
- **THEN** após a recarga do YAML, o alerta do endpoint gerenciado usa `failure-threshold` 5

### Requirement: Restrições próprias de endpoints gerenciados
O sistema MUST rejeitar definições de endpoints gerenciados que:
- gerem, pela função de chave do Go Uptime, uma chave já usada por endpoint, external-endpoint, suite ou endpoint de suite do YAML, ou por outro endpoint gerenciado (409, com a origem da chave na mensagem);
- usem `client.identity-aware-proxy`, `client.tls.certificate-file`, `client.tls.private-key-file`, `store` ou `always-run` (400);
- referenciem em `client.tunnel` um túnel inexistente em `tunneling` (400);
- declarem em `extra-labels` nomes fora da lista de labels Prometheus registrada no ciclo atual (400).

O sistema MUST NOT expandir variáveis de ambiente (`$VAR`, `${VAR}`) nas definições de endpoints gerenciados.

#### Scenario: Colisão de chave com o YAML
- **WHEN** o nome e o grupo de um endpoint enviado geram a mesma chave de um endpoint definido no YAML
- **THEN** a API responde 409 informando que a chave já é usada no arquivo de configuração

#### Scenario: Credenciais do servidor bloqueadas
- **WHEN** um administrador envia uma definição com `client.identity-aware-proxy`
- **THEN** a API responde 400 informando que o campo não é permitido em endpoints gerenciados

#### Scenario: Túnel inexistente
- **WHEN** um administrador envia `client.tunnel: bastion` e `tunneling` não define `bastion`
- **THEN** a API responde 400 informando que o túnel não existe

#### Scenario: Label Prometheus nova
- **WHEN** as labels registradas são `environment` e um administrador envia `extra-labels` com `team`
- **THEN** a API responde 400 listando as labels permitidas

#### Scenario: Variável de ambiente não é expandida
- **WHEN** um administrador cria um endpoint com o header `Authorization: Bearer ${API_TOKEN}`
- **THEN** o valor usado nas verificações é literalmente `Bearer ${API_TOKEN}`

### Requirement: API de administração de endpoints
O sistema MUST expor as operações abaixo, respondendo em JSON com erros no formato `{"error": "<mensagem>"}` (exceto o 401 do middleware de autenticação, que mantém o formato atual):
- `GET /api/v1/admin/endpoints`: lista endpoints gerenciados e do YAML, com origem (`admin` ou `config`), chave, nome, grupo, tipo, URL com credenciais mascaradas, intervalo, estado habilitado, conflito, erro de validação, versão e metadados de alteração;
- `GET /api/v1/admin/endpoints/{key}`: definição armazenada e efetiva, em YAML e JSON, com `ETag` da versão;
- `POST /api/v1/admin/endpoints`: cria;
- `PUT /api/v1/admin/endpoints/{key}`: altera;
- `POST /api/v1/admin/endpoints/{key}/enable` e `POST /api/v1/admin/endpoints/{key}/disable`: habilitam e desabilitam;
- `DELETE /api/v1/admin/endpoints/{key}`: remove;
- `POST /api/v1/admin/endpoints/parse`: decodifica a definição e a devolve como documento, sem validar, sem ler dados armazenados e sem mascarar;
- `POST /api/v1/admin/endpoints/validate`: valida sem persistir;
- `POST /api/v1/admin/endpoints/test`: valida e executa uma verificação única;
- `GET /api/v1/admin/metadata`: tipos de alerta configurados, túneis disponíveis e labels Prometheus permitidas.

Operações que alteram um endpoint existente MUST exigir `If-Match` com a versão atual, respondendo 428 sem o header e 412 com versão diferente. Corpos acima de 256 KB MUST ser rejeitados com 413, exceto nas rotas de restore da capability `admin-backup-restore`, que aceitam até 3,5 MiB. Enquanto um ciclo de partida ou recarga estiver em andamento (incluindo a partida inicial dos endpoints), as escritas MUST responder 503 sem validar nem gravar nada.

#### Scenario: Listagem indica a origem
- **WHEN** o YAML define 2 endpoints e existe 1 endpoint gerenciado
- **THEN** a listagem devolve 3 itens, 2 com origem `config` e 1 com origem `admin`

#### Scenario: Chave inexistente
- **WHEN** um administrador consulta `GET /api/v1/admin/endpoints/nao_existe`
- **THEN** a API responde 404

#### Scenario: Validação não persiste
- **WHEN** um administrador envia uma definição válida para `validate`
- **THEN** a API responde 200 com as definições armazenável e efetiva
- **AND** o endpoint não é persistido nem monitorado

#### Scenario: Edição concorrente
- **WHEN** dois administradores leem `core_api` na versão 2 e o primeiro salva uma alteração
- **THEN** o `PUT` do segundo, com `If-Match` da versão 2, recebe 412
- **AND** a alteração do primeiro é mantida

#### Scenario: Escrita durante recarga
- **WHEN** um administrador envia uma criação enquanto o hot-reload está entre parar e iniciar o Go Uptime
- **THEN** a API responde 503 e nada é persistido

### Requirement: Configuração apenas com endpoints gerenciados
Com `admin.enabled: true`, o sistema MUST validar os pré-requisitos da administração e MUST iniciar mesmo que o arquivo de configuração não defina nenhum endpoint nem suite.

#### Scenario: YAML sem endpoints
- **WHEN** o arquivo de configuração tem apenas `storage`, `security` e `admin`
- **THEN** o Go Uptime inicia e serve o dashboard sem endpoints
- **AND** um administrador consegue criar o primeiro endpoint

### Requirement: Renomeação de endpoint gerenciado
A alteração de um endpoint gerenciado MUST aceitar `name` e `group` diferentes dos armazenados.

Quando a chave derivada (`grupo_nome`) mudar, o sistema MUST, numa única transação, gravar a definição sob a chave nova e mover para ela o status, os resultados, os eventos, o uptime e os alertas disparados da chave antiga. Se a transação falhar, nada MUST mudar e o monitoramento anterior MUST continuar. Quando só o texto de `name` ou `group` mudar sem mudar a chave, o nome e o grupo exibidos MUST ser atualizados, mantendo o histórico.

A chave nova MUST ser rejeitada com 409, sem alterar nada, quando for usada por endpoint, endpoint externo, suite ou endpoint de suite do arquivo de configuração, por outro endpoint gerenciado, ou quando já tiver status armazenado. Com `storage.type: mysql`, uma chave nova acima de 768 caracteres MUST ser rejeitada com 400.

Depois da resposta, o sistema MUST NOT registrar resultados, eventos, métricas ou alertas sob a chave antiga; MUST apagar as séries Prometheus e invalidar os caches de status da chave antiga; e MUST preservar no endpoint renomeado o estado dos alertas disparados (disparo, chave de resolução e contadores) cuja configuração não mudou, limpando o dos alertas cuja configuração mudou.

Um endpoint gerenciado em conflito com o arquivo de configuração MUST poder ser renomeado movendo apenas a definição: o histórico da chave antiga MUST continuar com o endpoint do arquivo, e a chave nova MUST começar sem histórico.

#### Scenario: Troca de grupo mantém o histórico
- **WHEN** um administrador envia `PUT /api/v1/admin/endpoints/web_site` com `group: clientes` para um endpoint com 100 resultados armazenados
- **THEN** a API responde 200 com a chave `clientes_site`
- **AND** `GET /api/v1/endpoints/clientes_site/statuses` mostra os 100 resultados
- **AND** `web_site` não aparece em `GET /api/v1/endpoints/statuses`

#### Scenario: Histórico preservado depois de reiniciar
- **WHEN** o Go Uptime reinicia depois da renomeação de `web_site` para `clientes_site`
- **THEN** `clientes_site` continua com o histórico anterior à renomeação
- **AND** nenhum dado é registrado sob `web_site`

#### Scenario: Troca de nome sem mudar a chave
- **WHEN** um administrador altera o nome de `My API` para `my-api`, que geram a mesma chave
- **THEN** a API responde 200 com a mesma chave
- **AND** o dashboard mostra o nome `my-api` com o histórico anterior

#### Scenario: Chave nova usada pelo arquivo de configuração
- **WHEN** um administrador renomeia `web_site` para `core_health`, definido no arquivo de configuração
- **THEN** a API responde 409
- **AND** `web_site` continua monitorado, com a definição e o histórico inalterados

#### Scenario: Chave nova com status armazenado
- **WHEN** um administrador renomeia `web_site` para `web_antigo`, que ainda tem resultados armazenados sem estar em uso
- **THEN** a API responde 409
- **AND** nenhum resultado de `web_site` ou de `web_antigo` é movido ou apagado

#### Scenario: Versão desatualizada na renomeação
- **WHEN** um administrador renomeia `web_site` enviando uma versão desatualizada em `If-Match`
- **THEN** a API responde 412
- **AND** `web_site` continua monitorado com a mesma chave e o mesmo histórico

#### Scenario: Renomeação durante verificação lenta
- **WHEN** um administrador renomeia um endpoint durante uma verificação em andamento
- **THEN** o resultado dessa verificação é descartado
- **AND** nenhum resultado, evento, métrica ou alerta é registrado sob a chave antiga depois da resposta

#### Scenario: Alerta disparado na renomeação
- **WHEN** um endpoint gerenciado tem um alerta disparado e um administrador troca apenas o grupo
- **THEN** nenhum novo disparo é enviado para o mesmo incidente
- **AND** quando o endpoint volta a ter sucesso, o resolve é enviado com a chave de resolução original

#### Scenario: Renomeação de gerenciado em conflito
- **WHEN** um administrador renomeia para `web_novo` um endpoint gerenciado marcado como em conflito com `web_site` do arquivo de configuração
- **THEN** `web_novo` passa a ser monitorado sem histórico
- **AND** `web_site` do arquivo continua monitorado com todo o histórico
