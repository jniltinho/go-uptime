# realtime-endpoint-updates Specification

## Purpose
TBD - created by archiving change realtime-endpoint-updates. Update Purpose after archive.
## Requirements
### Requirement: Canal SSE de resultados novos
O sistema MUST oferecer um canal Server-Sent Events por endpoint que envie o aviso pela conexão em até 1 segundo depois de cada resultado gravado pelo monitoramento para ele: verificação, push, heartbeat ou resultado da API upstream de external endpoints. Resultados de endpoints de suites ficam fora.

**Rotas:**
- `GET /api/v1/endpoints/{key}/events` MUST exigir a mesma autenticação das rotas de status e MUST responder 404 para uma chave que não seja de um endpoint do arquivo de configuração, de um external endpoint ou de um endpoint gerenciado. A verificação MUST NOT ler o storage, e um endpoint ainda sem resultados MUST poder ser observado.
- `GET /api/v1/status-pages/{slug}/endpoints/{key}/events` MUST ser pública e MUST responder o 404 idêntico das status pages, contando no limitador e sem ler o storage, quando a página não estiver publicada ou não mostrar a chave.

**Resposta:**
- MUST ter `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-store, no-transform` e `X-Accel-Buffering: no`, sem compressão;
- MUST começar com `retry: 3000` e com `id` igual à sequência atual do endpoint;
- cada resultado MUST gerar um evento `result` com `id` igual a uma sequência que só cresce e `data` igual a `{}`;
- o evento MUST NOT conter nenhum dado do resultado;
- avisos acumulados para o mesmo cliente MAY ser juntados num só.

**Conexão:**
- quando o `Last-Event-ID` recebido, pelo cabeçalho ou pelo parâmetro `lastEventId`, for menor que a sequência atual, a conexão MUST começar com um evento; `Last-Event-ID` ausente ou inválido MUST valer 0;
- a sequência MUST NOT reiniciar enquanto o processo estiver no ar, inclusive depois de recargas da configuração;
- `HEAD` MUST responder só os cabeçalhos, sem stream e sem ocupar vaga;
- a verificação de existência MUST acontecer antes dos limites, para a rota pública manter o 404 idêntico;
- MUST enviar um comentário a cada 15 segundos;
- MUST durar no máximo 5 minutos, sem ser cortada pelo prazo de escrita padrão de 15 segundos do servidor;
- somente as requisições cujo caminho, sem query, tenha exatamente a forma `/api/v1/endpoints/{key}/events` ou `/api/v1/status-pages/{slug}/endpoints/{key}/events`, com segmentos não vazios, MUST receber o prazo de escrita maior.

#### Scenario: Push aparece na hora
- **WHEN** um navegador está conectado a `/api/v1/endpoints/jobs_backup/events` e o endpoint recebe `status=pending`
- **THEN** o navegador recebe em até 1 segundo um evento `result`
- **AND** o evento não contém a mensagem, o status nem o instante do push

#### Scenario: Endpoint Push ainda sem resultados
- **WHEN** o administrador abre os detalhes de um endpoint Push recém-criado e o canal de eventos dele
- **THEN** o canal abre com 200 e avisa o primeiro push

#### Scenario: Reconexão sem perder avisos
- **WHEN** uma conexão termina pelo tempo máximo, chega um push antes da reconexão e o cliente reconecta com o `Last-Event-ID` da primeira conexão
- **THEN** a nova conexão começa com um evento `result`

#### Scenario: Conexão longa
- **WHEN** um cliente fica conectado por 3 minutos sem resultados novos
- **THEN** a conexão continua aberta, com comentários a cada 15 segundos

#### Scenario: HEAD não ocupa vaga
- **WHEN** um health check faz 20 requisições `HEAD /api/v1/status-pages/infra/endpoints/core_api/events` do mesmo IP
- **THEN** todas respondem os cabeçalhos, e o IP ainda consegue abrir 10 conexões de eventos

#### Scenario: Caminho real recebe o prazo maior
- **WHEN** o cliente abre `/api/v1/endpoints/jobs_backup/events?lastEventId=3` e fica conectado mais de 15 segundos
- **THEN** a conexão continua aberta

#### Scenario: Outra rota não ganha prazo maior
- **WHEN** uma resposta de `/api/v1/endpoints/statuses/events-export` demora mais de 15 segundos para ser escrita
- **THEN** o prazo de escrita padrão continua valendo

#### Scenario: Endpoint fora da página pública
- **WHEN** um visitante pede `/api/v1/status-pages/infra/endpoints/database_pg/events` e `database_pg` não está na página `infra`
- **THEN** a resposta é o 404 idêntico das status pages

#### Scenario: Sem autenticação
- **WHEN** um navegador sem sessão pede `/api/v1/endpoints/jobs_backup/events` com `security.basic`
- **THEN** a resposta é 401 sem `WWW-Authenticate`

### Requirement: Limites e ciclo de vida das conexões de eventos
O sistema MUST limitar as conexões de eventos abertas a 500 no total e a 10 por IP de cliente, calculado com `status-pages.trusted-proxies`, somando as duas rotas. Acima do limite, MUST responder 429 com `Retry-After: 30`, `Cache-Control: no-store` e `{"error":"too many requests"}`.

**Liberação da vaga:** uma conexão encerrada pelo cliente, por erro de escrita, pelo tempo máximo, por pânico no envio ou por falha antes de começar o envio MUST liberar a vaga. Um pânico no envio MUST NOT derrubar o processo.

**Parada:** numa recarga da configuração ou no desligamento, o sistema MUST encerrar as conexões de eventos antes de parar o servidor. A parada MUST NOT esperar mais de 10 segundos pelas conexões, e novas conexões MUST ser recusadas com 503 enquanto ela dura. Depois da recarga, o servidor novo MUST aceitar conexões de eventos assim que começar a escutar.

**Chaves esquecidas:** remover ou renomear um endpoint, ou tirá-lo da configuração numa recarga bem-sucedida, MUST descartar a sequência da chave.

#### Scenario: Limite por IP
- **WHEN** um IP já tem 10 conexões de eventos abertas e abre a 11ª
- **THEN** a resposta é 429 com `Retry-After: 30`

#### Scenario: Cliente que foi embora
- **WHEN** um IP fecha as suas 10 conexões sem avisar
- **THEN** em até 30 segundos o IP consegue abrir novas conexões

#### Scenario: Recarga com conexões abertas
- **WHEN** há 20 conexões de eventos abertas e o arquivo de configuração é recarregado
- **THEN** as conexões são encerradas e a recarga não espera 5 minutos

#### Scenario: Atrás do nginx sem trusted-proxies
- **WHEN** o Go Uptime está atrás de um proxy que não está em `status-pages.trusted-proxies` e 11 visitantes abrem os detalhes
- **THEN** o 11º recebe 429 no canal e continua com a atualização periódica
- **AND** a documentação indica configurar `trusted-proxies`

### Requirement: Detalhes do endpoint em tempo real
A página de detalhes do endpoint no dashboard e a página pública de detalhes MUST abrir o canal de eventos do endpoint e, a cada aviso, atualizar sem indicador de carregamento as barras, o painel de números, o gráfico **Response Time Trend** e a tabela de checks. No dashboard, barras, painel e gráfico MUST mostrar sempre os resultados mais recentes, mesmo com a tabela em outra página de resultados.

- **Ordem das respostas:** uma resposta mais antiga que chegue depois de uma mais nova MUST ser descartada.
- **Aba oculta:** a página MUST fechar o canal quando a aba ficar oculta e, ao voltar, MUST reabri-lo e atualizar uma vez.
- **Troca de endpoint:** a troca de endpoint na mesma tela MUST reabrir o canal para o endpoint novo.
- **Canal fechado por erro:** se o canal ficar fechado por erro (502 do proxy numa recarga, 503, 429, 404 ou 401), a página MUST continuar com a atualização periódica e MUST reabrir o canal com espera crescente de 30 segundos até 5 minutos, ou logo depois de uma atualização periódica bem-sucedida, informando o último `id` recebido. Numa queda depois de uma resposta 200, a página MUST deixar a reconexão automática do navegador agir, sem abrir uma segunda conexão.
- **Gráfico:** no período Recent, o gráfico MUST recarregar logo depois de cada atualização dos dados da página; nos períodos 3h, 6h, 24h e 1w, MUST recarregar no máximo uma vez a cada 60 segundos. Nas atualizações, o gráfico MUST NOT ser apagado nem mostrar indicador de carregamento, e um aviso que chegue durante uma carga MUST gerar uma nova busca quando ela terminar.

#### Scenario: Pending no gráfico na hora
- **WHEN** o administrador está nos detalhes de `jobs_backup` e um script envia `status=pending`
- **THEN** em até 2 segundos, a barra, a tabela e, com o gráfico em Recent, a coluna amarela do gráfico aparecem, sem recarregar a página

#### Scenario: Página pública em tempo real
- **WHEN** um visitante está em `/status/jobs/endpoints/jobs_backup` e o endpoint recebe um push
- **THEN** a página mostra o resultado novo em até 2 segundos, mesmo com a resposta de detalhes ainda em cache

#### Scenario: Recarga do Go Uptime atrás do nginx
- **WHEN** o visitante está nos detalhes, a configuração do Go Uptime é recarregada e o canal recebe 502 do nginx
- **THEN** a página continua mostrando os dados e volta a receber avisos em no máximo 5 minutos, sem ação do visitante

#### Scenario: Pending com a tabela na página 2
- **WHEN** o administrador está nos detalhes de `jobs_backup` com a tabela na página 2 e chega `status=pending`
- **THEN** as barras, o painel e, com o gráfico em Recent, a coluna amarela do gráfico mostram o Pending em até 2 segundos

#### Scenario: Canal indisponível
- **WHEN** a abertura do canal responde 429
- **THEN** a página continua mostrando os dados e se atualiza no intervalo periódico

