## MODIFIED Requirements

### Requirement: URL de push compatível com o Uptime Kuma
O sistema MUST receber envios em `/api/push/{token}` por qualquer método HTTP, sem autenticação do Go Uptime, lendo somente os parâmetros de query `status`, `msg` e `ping`, com as regras do Uptime Kuma e uma extensão do Go Uptime:
- `status` igual a `up` MUST registrar sucesso, e o padrão é `up`;
- `status` igual a `pending` MUST registrar um resultado Pending, extensão do Go Uptime, porque no Uptime Kuma esse valor registra falha;
- qualquer outro valor de `status` MUST registrar falha, sujeito às tentativas do endpoint;
- `msg` MUST ser a mensagem do resultado, com padrão `OK`;
- `ping` MUST ser lido como número em milissegundos e usado como duração do resultado; vazio ou não numérico MUST ser ignorado; menor que 0 ou maior que 100000000000 MUST ser rejeitado.

Um envio aceito, inclusive Pending, MUST responder 200 com `{"ok":true}` e MUST contar como envio recebido para o heartbeat. Um envio rejeitado MUST responder 404 com `{"ok":false,"msg":"<motivo>"}` e MUST NOT armazenar resultado. Token desconhecido e endpoint desabilitado MUST usar o motivo `Monitor not found or not active.`. Nenhum envio MUST criar endpoint.

#### Scenario: URL copiada do Uptime Kuma
- **WHEN** um script chama `GET /api/push/keSDu7G855jvVat1xWiY2Gk4CkL1End5?status=up&msg=OK&ping=` e existe um endpoint Push com esse token
- **THEN** a API responde 200 com `{"ok":true}`
- **AND** o endpoint registra um resultado de sucesso com a mensagem `OK` e sem duração

#### Scenario: Falha com mensagem e ping
- **WHEN** um script chama `POST /api/push/<token>?status=down&msg=Falha%20no%20backup&ping=120` para um endpoint sem tentativas
- **THEN** o endpoint registra uma falha com a mensagem `Falha no backup` e duração de 120 ms

#### Scenario: Status diferente de up e de pending
- **WHEN** um script envia `status=warning` para um endpoint sem tentativas
- **THEN** o endpoint registra uma falha

#### Scenario: Status pending
- **WHEN** um script envia `status=pending&msg=Aguardando`
- **THEN** a API responde 200 com `{"ok":true}`
- **AND** o endpoint registra um resultado Pending com a mensagem `Aguardando`, sem erros
- **AND** num endpoint Push, o intervalo do heartbeat recomeça

#### Scenario: Envio sem parâmetros
- **WHEN** um script chama `GET /api/push/<token>` sem query
- **THEN** o endpoint registra um sucesso com a mensagem `OK`

#### Scenario: Token desconhecido
- **WHEN** um script chama `/api/push/tokeninexistente?status=up`
- **THEN** a API responde 404 com `{"ok":false,"msg":"Monitor not found or not active."}`
- **AND** nenhum resultado é armazenado e nenhum endpoint é criado

#### Scenario: Ping fora do intervalo
- **WHEN** um script envia `ping=-5`
- **THEN** a API responde 404 com `ok` falso e nenhum resultado é armazenado

### Requirement: Push em endpoints ativos
Receber push MUST ser uma opção de cada endpoint ativo, desligada por padrão. Um push aceito para um endpoint ativo com a opção ligada MUST registrar um resultado no mesmo histórico das verificações do Go Uptime, com status, mensagem, duração e origem Push. Esse resultado MUST contar para o uptime, os eventos, as métricas e os alertas como as verificações, exceto um push Pending, que segue as regras do status Pending. Os endpoints ativos MUST NOT ter tentativas. O processamento das verificações ativas e dos pushes de um mesmo endpoint MUST ser serializado, sem perder os contadores dos alertas. O heartbeat MUST NOT ser aplicado a endpoints ativos. Um push para um endpoint ativo com a opção desligada, parado, desabilitado ou removido MUST responder 404.

#### Scenario: Notificação da Akamai num serviço ativo
- **WHEN** `erp_site` tem push ligado, verifica a URL a cada minuto com sucesso e a Akamai envia `status=down&msg=Latencia alta` às 10:00:20
- **THEN** o histórico de `erp_site` mostra o push down às 10:00:20, com origem Push, entre as verificações up de 10:00:00 e 10:01:00
- **AND** um alerta com `failure-threshold: 1` dispara com a mensagem `Latencia alta`

#### Scenario: Push Pending num serviço ativo
- **WHEN** `erp_site` tem push ligado e recebe `status=pending&msg=Janela de deploy`
- **THEN** o histórico mostra o resultado Pending com origem Push
- **AND** os contadores de alerta e os eventos de `erp_site` não mudam

#### Scenario: Push desligado
- **WHEN** um administrador desliga o push de `erp_site`
- **THEN** os envios para `erp_site` com a chave global ou com o token do endpoint respondem 404
- **AND** as verificações do Go Uptime continuam registrando resultados

#### Scenario: Push durante a verificação ativa
- **WHEN** um push chega enquanto a verificação ativa do mesmo endpoint grava seu resultado
- **THEN** os dois resultados são armazenados, e os contadores de falhas e sucessos seguidos refletem a ordem em que foram processados

### Requirement: Status Pending dos resultados
O sistema MUST suportar um terceiro status de resultado, Pending, além de sucesso e falha. Um resultado Pending MUST ter `success: false` e `pending: true` na API protegida de status, e MUST ser persistido com storage `memory`, `sqlite`, `postgres` e `mysql`, sem mudar as tabelas do upstream. A coluna MUST ser criada nas instalações novas e acrescentada às existentes de forma idempotente, inclusive com duas instâncias subindo juntas. Um resultado Pending:
- MUST NOT alterar os contadores de falhas e sucessos seguidos nem disparar ou resolver alertas;
- MUST NOT criar evento;
- MUST contar como execução sem sucesso no uptime e nas métricas.

A criação de eventos MUST comparar cada resultado que não é Pending com o tipo do último evento HEALTHY ou UNHEALTHY do endpoint: um sucesso MUST criar HEALTHY e uma falha MUST criar UNHEALTHY quando esse último evento for do outro tipo ou não existir. A ausência de evento HEALTHY ou UNHEALTHY MUST ser tratada como "não existe", sem erro, inclusive quando o único evento é START. Sem eventos, o evento START MUST continuar sendo gravado. Sem resultados Pending, os eventos MUST ser os mesmos de antes.

As telas do dashboard MUST mostrar Pending em amarelo, com o rótulo "Pending":
- nas barras de resultados;
- no tooltip;
- no estado atual;
- no badge da tabela de verificações.

O resumo do dashboard MUST mostrar a contagem de endpoints cujo último resultado é Pending, sem contá-los também como down. Os contadores de falha dos grupos e os filtros MUST tratar Pending como falha.

#### Scenario: Job em andamento
- **WHEN** o endpoint Push `jobs_backup`, com último resultado de sucesso, recebe `status=pending&msg=Backup em andamento`
- **THEN** o último resultado aparece em amarelo com o rótulo Pending e a mensagem `Backup em andamento`
- **AND** nenhum evento é criado e nenhum alerta é disparado

#### Scenario: Queda depois de Pending
- **WHEN** `jobs_backup` recebe `status=up`, depois `status=pending` e depois `status=down`
- **THEN** é criado um único evento UNHEALTHY, no resultado down
- **AND** um alerta com `failure-threshold: 1` dispara no resultado down

#### Scenario: Volta depois de Pending
- **WHEN** `jobs_backup` recebe `status=up`, depois `status=pending` e depois `status=up`
- **THEN** nenhum evento novo é criado

#### Scenario: Primeiro resultado Pending
- **WHEN** um endpoint novo recebe primeiro `status=pending` e depois `status=up`
- **THEN** os eventos são START e HEALTHY

#### Scenario: Muitos Pending seguidos
- **WHEN** `storage.maximum-number-of-results` é 10, o endpoint está com evento UNHEALTHY e recebe 15 envios `status=pending` e depois `status=down`
- **THEN** nenhum evento novo é criado

#### Scenario: Pending persistido
- **WHEN** um resultado Pending é gravado com storage `postgres` e o Go Uptime reinicia
- **THEN** a API protegida devolve o resultado com `success: false` e `pending: true`

#### Scenario: Banco de versão anterior
- **WHEN** o Go Uptime inicia com uma tabela `endpoint_result_messages` sem a coluna `pending`, com storage `sqlite`, `postgres` ou `mysql`
- **THEN** a coluna é acrescentada, as mensagens existentes continuam legíveis e os resultados antigos não são Pending

#### Scenario: Uptime com Pending
- **WHEN** um endpoint tem, na última hora, 3 resultados de sucesso e 1 Pending
- **THEN** o uptime da última hora é 75%
