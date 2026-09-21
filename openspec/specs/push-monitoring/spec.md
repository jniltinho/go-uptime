# push-monitoring Specification

## Purpose
TBD - created by archiving change add-push-monitoring. Update Purpose after archive.
## Requirements
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

### Requirement: Escopos das chaves de push
Além do token do próprio endpoint, o sistema MUST aceitar chaves globais em `/api/push/{token}/{chave-do-endpoint}`, com os mesmos parâmetros, métodos e respostas da URL do Uptime Kuma:
- uma chave global MUST autorizar qualquer endpoint habilitado que receba push;
- o token de um endpoint MUST autorizar somente esse endpoint.

Endpoint inexistente, desabilitado ou que não receba push, e chave que não autorize o endpoint, MUST responder 404 com o mesmo corpo de token desconhecido, sem revelar qual das condições falhou. Os endpoints Push MUST ser os external endpoints do arquivo de configuração e os endpoints Push gerenciados pela web, e MUST sempre receber push. Um endpoint ativo, do arquivo de configuração ou gerenciado pela web, MUST receber push somente quando a opção estiver ligada.

#### Scenario: Chave global
- **WHEN** um script usa a chave global em `/api/push/<global>/jobs_backup?status=up` e `jobs_backup` é um endpoint Push
- **THEN** a API responde 200 e `jobs_backup` registra o sucesso

#### Scenario: Token de outro endpoint
- **WHEN** um script usa o token de `jobs_backup` em `/api/push/<token-de-jobs_backup>/core_cron`
- **THEN** a API responde 404 com `{"ok":false,"msg":"Monitor not found or not active."}`
- **AND** `core_cron` não registra resultado

#### Scenario: Chave global num endpoint ativo
- **WHEN** a Akamai chama `/api/push/<global>/erp_site?status=down&msg=Latencia%20alta` e `erp_site` é um endpoint HTTP com push ligado
- **THEN** a API responde 200 e `erp_site` registra a falha com a mensagem `Latencia alta`, marcada como origem Push

#### Scenario: Endpoint ativo sem push
- **WHEN** um script usa a chave global com a chave de um endpoint HTTP com push desligado
- **THEN** a API responde 404 e nenhum resultado é armazenado

### Requirement: Tokens e chaves de push
O token de um endpoint Push, e o token opcional de um endpoint ativo com push ligado, MUST ter de 8 a 128 caracteres entre letras, dígitos, `-` e `_`. Ele MUST poder ser informado, para reaproveitar o token de um monitor do Uptime Kuma, ou gerado com 32 letras e dígitos aleatórios, com gerador criptográfico. Um token não pode identificar mais de um endpoint: a administração MUST rejeitar com 409 um token já usado por outro endpoint, do arquivo ou da web. No arquivo de configuração, `push.endpoints` MUST ligar o push de endpoints ativos pela chave, com token opcional, e uma chave que não seja de endpoint ativo do arquivo MUST ser rejeitada na carga. Quando o arquivo de configuração tiver o mesmo token em mais de um external endpoint, o sistema MUST registrar um aviso na carga, e esse token MUST responder 404 em `/api/push/{token}`.

As chaves globais criadas pela administração MUST ter um nome único de 1 a 64 caracteres e MUST ser geradas com 32 letras e dígitos aleatórios, mostradas uma única vez e armazenadas somente como hash SHA-256 com os 4 últimos caracteres como dica. Uma chave restaurada de um backup MUST manter o hash e a dica do arquivo, sem token conhecido pelo sistema, e MUST NOT ter o hash de uma chave existente nem do token de qualquer endpoint. Elas MUST poder ser revogadas, e uma chave revogada MUST parar de autorizar envios imediatamente. As chaves globais do arquivo de configuração MUST ficar em `push.keys`, com `name` e `token` com pelo menos 16 caracteres, e MUST ser somente leitura na administração.

#### Scenario: Token do Uptime Kuma reaproveitado
- **WHEN** um administrador cria um endpoint Push informando o token `keSDu7G855jvVat1xWiY2Gk4CkL1End5`
- **THEN** o endpoint é criado e `/api/push/keSDu7G855jvVat1xWiY2Gk4CkL1End5` passa a registrar os envios nesse endpoint

#### Scenario: Token repetido
- **WHEN** um administrador cria um endpoint Push com o token de um external endpoint do arquivo de configuração
- **THEN** a API responde 409 e o endpoint não é criado

#### Scenario: Chave global criada e revogada
- **WHEN** um administrador cria uma chave global
- **THEN** a resposta mostra a chave completa uma única vez e a lista mostra somente a dica
- **AND** depois que a chave é revogada, os envios com ela respondem 404

#### Scenario: Chave global do arquivo
- **WHEN** o arquivo de configuração define `push.keys` com `name: akamai` e um token de 32 caracteres
- **THEN** a chave autoriza os endpoints que recebem push e aparece na administração sem ações de revogar

#### Scenario: Chave global restaurada
- **WHEN** um backup com a chave `akamai` é restaurado em outra instalação
- **THEN** os envios com o token original de `akamai` são aceitos, e a lista mostra somente a dica

### Requirement: Proteção da rota de push
As rotas `/api/push` e `/api/push/*` MUST ser atendidas antes do middleware de segurança, inclusive para caminhos inválidos, e MUST NOT responder 401 nem enviar `WWW-Authenticate`. As respostas MUST ter `Cache-Control: no-store`. Tokens e chaves MUST NOT aparecer nos logs, e a comparação de chaves globais MUST ser feita pelo hash.

Envios válidos MUST ser sempre aceitos. Depois de 30 envios rejeitados no mesmo minuto vindos do mesmo IP de cliente, calculado com `status-pages.trusted-proxies`, os envios rejeitados seguintes desse IP MUST responder 429 com `{"ok":false,"msg":"Too many requests"}`.

#### Scenario: Caminho inválido com basic auth
- **WHEN** `security.basic` está configurado e um cliente chama `PUT /api/push/a/b/c`
- **THEN** a resposta não é 401 e não tem `WWW-Authenticate`

#### Scenario: Tentativas de adivinhar tokens
- **WHEN** um IP envia 31 tokens inválidos no mesmo minuto
- **THEN** o 31º envio responde 429
- **AND** um envio com token válido do mesmo IP continua respondendo 200

### Requirement: Heartbeat de endpoints Push
Um endpoint Push gerenciado pela web MUST ter intervalo de heartbeat, com padrão de 60 segundos e mínimo de 10 segundos. Nos external endpoints do arquivo, o heartbeat continua opcional.

Para cada intervalo completo sem envio, o sistema MUST registrar uma falha que informe o intervalo, com esse texto na mensagem e nos erros, e tratar os alertas. Em janela de manutenção, a falha MUST ser registrada sem tratar os alertas e sem consumir tentativas. Fora da manutenção, enquanto houver tentativas, o resultado MUST ser Pending em vez de falha. Intervalos consecutivos sem envio MUST gerar um resultado cada um.

O heartbeat MUST ser controlado pelo registro de monitoramento por chave, e MUST parar antes da resposta quando o endpoint for desabilitado, removido ou renomeado. Um envio aceito, em qualquer status, MUST reiniciar a contagem do intervalo.

#### Scenario: Intervalos sem envio
- **WHEN** um endpoint Push com intervalo de 1 minuto e sem tentativas fica 3 minutos sem envio
- **THEN** o endpoint registra 3 falhas informando que nenhum envio foi recebido em 1m

#### Scenario: Envio depois de falhas
- **WHEN** um endpoint Push com alerta disparado por falta de envio recebe `status=up`
- **THEN** o endpoint registra sucesso e o alerta é resolvido conforme o `success-threshold`

#### Scenario: Endpoint desabilitado
- **WHEN** um administrador desabilita um endpoint Push
- **THEN** nenhum resultado de heartbeat é registrado depois da resposta
- **AND** os envios para o token do endpoint respondem 404

### Requirement: Endpoints Push gerenciados pela web
A administração MUST aceitar definições com `type: push`, com `name`, `group`, `token`, `heartbeat.interval`, `heartbeat.retries`, `alerts`, `maintenance-windows` e `enabled`. Uma definição Push com campos de endpoint ativo (`url`, `conditions`, `headers`, `client`, `method`, `body`, `interval` e demais) MUST ser rejeitada com 400, e uma definição ativa com `token` ou `heartbeat` também. Uma definição ativa MUST aceitar `push`, com `enabled` e `token` opcional, e uma definição Push MUST rejeitar `push`. Sem `token`, a criação de um endpoint Push MUST gerar um. Os tokens MUST ser devolvidos somente pelas rotas da administração.

Os endpoints Push gerenciados MUST ter o mesmo ciclo dos endpoints gerenciados ativos: versão e `If-Match`, conflito de chave com o arquivo, renomeação com o histórico, remoção, restauração dos alertas disparados e aplicação sem reinício. A ação `test` MUST responder 400 para uma definição Push. Os endpoints Push gerenciados MUST poder ser selecionados pelas status pages por grupo, por chave e em destaque, como os external endpoints do arquivo.

#### Scenario: Criação sem token
- **WHEN** um administrador cria um endpoint Push `backup` no grupo `jobs` sem token
- **THEN** a resposta traz um token de 32 caracteres e a chave `jobs_backup`

#### Scenario: Campo de endpoint ativo
- **WHEN** um administrador envia uma definição com `type: push` e `url: https://exemplo.com`
- **THEN** a API responde 400

#### Scenario: Tentativas no formulário
- **WHEN** um administrador cria um endpoint Push com "Retries" igual a 2
- **THEN** a definição salva tem `heartbeat.retries: 2`

#### Scenario: Renomeação mantém o token
- **WHEN** um administrador troca o grupo de `jobs_backup` para `infra`
- **THEN** `/api/push/<token-do-endpoint>` continua registrando os envios em `infra_backup`, com o histórico anterior

#### Scenario: Status page com endpoint Push
- **WHEN** uma status page seleciona o grupo `jobs` e `jobs_backup` é um endpoint Push gerenciado
- **THEN** a página pública mostra `jobs_backup` com seus resultados

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

### Requirement: Mensagem dos resultados e verificações recentes
O sistema MUST armazenar a mensagem e a origem Push de cada envio com o resultado, em qualquer status, com a mensagem limitada a 1024 bytes sem cortar caracteres UTF-8. Numa falha, a mensagem MUST também entrar nos erros do resultado, usados pelos alertas. Num resultado Pending, a mensagem MUST NOT entrar nos erros.

A API protegida de status MUST devolver `message` em cada resultado. A página de detalhes do endpoint no dashboard MUST mostrar a tabela "Recent checks", do mais recente para o mais antigo, com status (Up, Down ou Pending), data e hora, mensagem e origem (Push ou verificação), na ordem das colunas do Uptime Kuma seguida da origem. A mensagem da tabela MUST ser, nesta ordem: a mensagem do envio; os erros; o status HTTP da verificação ativa.

Os payloads das status pages públicas MUST NOT conter mensagens nem erros, exceto as mensagens do payload de detalhes de uma página com `show-messages`.

#### Scenario: Envios do Uptime Kuma no dashboard
- **WHEN** um endpoint Push recebe `status=up&msg=Backup OK`, depois `status=down&msg=Falha no backup: disco cheio` e depois `status=up&msg=Backup recuperado`
- **THEN** a tabela "Recent checks" mostra, nesta ordem, Up com `Backup recuperado`, Down com `Falha no backup: disco cheio` e Up com `Backup OK`, cada um com data e hora

#### Scenario: Badge Pending na tabela
- **WHEN** um endpoint Push recebe `status=pending&msg=Aguardando`
- **THEN** a tabela mostra o badge Pending em amarelo com a mensagem `Aguardando`

#### Scenario: Mensagem fora da página pública
- **WHEN** o endpoint Push com a falha `Falha no backup: disco cheio` aparece numa status page publicada sem `show-messages`
- **THEN** a resposta pública não contém `Falha no backup`

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

### Requirement: Tentativas antes da queda de endpoints Push
Endpoints Push gerenciados e external endpoints do arquivo com heartbeat MUST aceitar `heartbeat.retries`, inteiro de 0 a 100, com padrão 0. Um valor fora da faixa MUST invalidar a definição. Um endpoint ativo com `heartbeat.retries`, ou um external endpoint do arquivo com `heartbeat.retries` maior que 0 sem `heartbeat.interval`, MUST ser rejeitado. As tentativas MUST valer para os resultados da URL de push e da API upstream `POST /api/v1/endpoints/:key/external`. Com `retries` maior que 0:
- um envio com status diferente de `up` e de `pending`, ou um intervalo completo sem envio, MUST registrar Pending quando menos de `retries` resultados assim tiverem sido convertidos em Pending desde o último sucesso;
- esgotadas as tentativas, o resultado MUST registrar falha, com alertas e evento, e os seguintes MUST continuar registrando falha até um sucesso;
- um envio `up` MUST zerar a contagem;
- um envio `pending` MUST NOT alterar a contagem.

Um resultado convertido em Pending MUST ficar sem erros, e os erros MUST virar a mensagem quando ele não tiver mensagem. A contagem MUST ser mantida em memória por chave e sobreviver a recargas da configuração. MUST ser descartada, junto com o último envio da chave, ao renomear ou remover o endpoint e quando a chave deixar de existir numa recarga do arquivo. O formulário da administração MUST mostrar o campo "Retries" junto do intervalo de heartbeat dos endpoints Push.

#### Scenario: Heartbeat atrasado com tentativas
- **WHEN** um endpoint Push com intervalo de 1 minuto e `retries: 2` fica 3 minutos sem envio
- **THEN** o endpoint registra Pending, Pending e falha, nessa ordem
- **AND** só a falha conta para os alertas e cria evento

#### Scenario: Sem tentativas
- **WHEN** um endpoint Push com `retries: 0` recebe `status=down`
- **THEN** o endpoint registra falha imediatamente

#### Scenario: Envio down com tentativas
- **WHEN** um endpoint Push com `retries: 1` recebe `status=down` e depois `status=down`
- **THEN** o primeiro é registrado como Pending e o segundo como falha

#### Scenario: Recuperação durante as tentativas
- **WHEN** um endpoint Push com `retries: 2` registra um Pending por falta de envio e depois recebe `status=up`
- **THEN** o endpoint registra sucesso e a próxima falta de envio volta a registrar Pending

#### Scenario: API upstream de external endpoints com tentativas
- **WHEN** um external endpoint do arquivo com heartbeat e `retries: 1` recebe `POST /api/v1/endpoints/<key>/external?success=false&error=timeout`
- **THEN** o endpoint registra Pending com a mensagem `timeout` e sem erros

#### Scenario: Tentativas sem heartbeat
- **WHEN** o arquivo tem um external endpoint com `heartbeat.retries: 2` e sem `heartbeat.interval`
- **THEN** a configuração é inválida

#### Scenario: Remoção descarta a contagem
- **WHEN** um endpoint Push com `retries: 2` e um Pending registrado é removido e recriado com a mesma chave
- **THEN** a próxima falta de envio registra Pending como a primeira tentativa

#### Scenario: Tentativas inválidas
- **WHEN** um administrador salva um endpoint Push com `heartbeat.retries: 101`
- **THEN** a API responde 400

