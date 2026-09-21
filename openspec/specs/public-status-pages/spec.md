# public-status-pages Specification

## Purpose
TBD - created by archiving change add-public-status-pages. Update Purpose after archive.
## Requirements
### Requirement: Seção de configuração status-pages
O arquivo de configuração MUST aceitar a seção opcional `status-pages` com:
- `enabled` (booleano, padrão `true`);
- `trusted-proxies` (lista de IPs ou CIDRs, padrão vazia);
- `rate-limit` (inteiro não negativo, padrão `120`, `0` desliga o limite);
- `maximum-endpoints-per-page` (inteiro de `1` a `1000`, padrão `400`): quantos endpoints uma página mostra;
- `pages` (lista de páginas).

Cada página MUST aceitar `slug`, `title`, `description`, `groups`, `endpoints`, `show-certificate-expiration` (booleano, padrão `false`), `show-messages` (booleano, padrão `false`) e `enabled` (padrão `true` no YAML). Com `enabled: false` na seção, nenhuma página MUST ser publicada, e as rotas públicas MUST continuar respondendo como para uma página inexistente, sem `WWW-Authenticate`, com ou sem `security`.

#### Scenario: Página definida no YAML
- **WHEN** a configuração tem `status-pages.pages` com a página `infra`, título `Infraestrutura` e `groups: [core]`
- **THEN** a configuração é válida
- **AND** `GET /api/v1/status-pages/infra` responde 200

#### Scenario: Seção desligada
- **WHEN** a configuração tem `security.basic`, `status-pages.enabled: false` e a página `infra`
- **THEN** `GET /api/v1/status-pages/infra` responde o 404 idêntico sem `WWW-Authenticate`
- **AND** `GET /status/infra` responde 200 com o HTML da SPA

#### Scenario: Sem a seção
- **WHEN** a configuração não tem a seção `status-pages`
- **THEN** a configuração é válida
- **AND** `GET /api/v1/status-pages/qualquer` responde 404

#### Scenario: Página com a expiração do certificado
- **WHEN** a página `infra` do YAML tem `show-certificate-expiration: true`
- **THEN** a configuração é válida
- **AND** os endpoints da página com certificado têm `certificateExpiresInDays` no payload

#### Scenario: Página com mensagens
- **WHEN** a página `jobs` do YAML tem `show-messages: true`
- **THEN** a configuração é válida
- **AND** o payload de detalhes dos endpoints de `jobs` tem `page.showMessages: true`

#### Scenario: Limite de endpoints fora do intervalo
- **WHEN** a configuração tem `status-pages.maximum-endpoints-per-page` igual a `0`, a `-1`, a `1001` ou a um valor que não é inteiro
- **THEN** a configuração é inválida, no início e em `go-uptime config validate`, com a mensagem citando o intervalo aceito

### Requirement: Validação das páginas
A validação do arquivo de configuração MUST ser estrutural e MUST recusar:
- `slug` fora de `^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`;
- `slug` reservado (`options`, `validate`, `new`, `preview`, `exposure`);
- `slug` repetido;
- `title` vazio ou com mais de 100 runas depois de remover espaços das pontas;
- `description` com mais de 1000 runas;
- página sem nenhum item em `groups`, em `endpoints` e em `featured` (uma página só com destaques é válida, como define `status-page-highlights`);
- mais de 50 grupos, grupo com mais de 200 runas ou mais de 1000 chaves, qualquer que seja `maximum-endpoints-per-page`;
- itens repetidos;
- entradas de `trusted-proxies` que não sejam IP nem CIDR;
- `rate-limit` negativo.

Um grupo ou uma chave sem endpoint correspondente MUST NOT invalidar a página. O aviso correspondente MUST ser registrado na carga das páginas, depois da carga dos endpoints gerenciados, e MUST NOT ser registrado na validação do arquivo.

#### Scenario: Slug inválido
- **WHEN** uma página do YAML tem `slug: Infra_1`
- **THEN** a configuração é inválida com uma mensagem citando o slug

#### Scenario: Slug reservado
- **WHEN** uma página do YAML tem `slug: options`
- **THEN** a configuração é inválida

#### Scenario: Slug repetido
- **WHEN** duas páginas do YAML têm `slug: infra`
- **THEN** a configuração é inválida

#### Scenario: Página sem seleção
- **WHEN** uma página do YAML não tem `groups`, nem `endpoints`, nem `featured`
- **THEN** a configuração é inválida

#### Scenario: Título com acentos
- **WHEN** o título tem 100 caracteres acentuados (mais de 100 bytes)
- **THEN** a configuração é válida

#### Scenario: Proxy confiável inválido
- **WHEN** `status-pages.trusted-proxies` contém `nginx.local`
- **THEN** a configuração é inválida

#### Scenario: Grupo ainda inexistente
- **WHEN** uma página tem `groups: [pagamentos]` e nenhum endpoint do YAML nem gerenciado usa esse grupo
- **THEN** a configuração é válida
- **AND** a carga das páginas registra no log um aviso citando a página e o grupo

#### Scenario: Grupo existente só em gerenciados
- **WHEN** uma página do YAML tem `groups: [clientes]` e só endpoints gerenciados usam esse grupo
- **THEN** nenhum aviso de grupo sem correspondência é registrado

**Credencial da página:** com `auth` na definição, o usuário MUST ser não vazio e a senha MUST ser um hash bcrypt válido, codificado em base64 com o alfabeto URL, como em `security.basic`. Uma definição com `auth` incompleto ou com hash inválido MUST ser recusada, com a mesma severidade das demais validações de página. Sem `auth`, a página continua pública. Uma página com `auth` numa instalação **sem** `security` MUST registrar um aviso na carga, porque as rotas por chave do dashboard continuam abertas e publicam mais do que a página protegida.

#### Scenario: Credencial incompleta
- **WHEN** uma página traz `auth` com o usuário vazio, ou com um hash que não é bcrypt
- **THEN** a definição é recusada com erro de validação

#### Scenario: Página com login sem security na instalação
- **WHEN** o arquivo de configuração define uma página com `auth` e não define `security`
- **THEN** a carga registra um aviso de que as rotas por chave continuam públicas

#### Scenario: Mais chaves do que o limite de exibição
- **WHEN** uma definição lista 500 chaves de endpoint e `maximum-endpoints-per-page` é `400`
- **THEN** a definição é válida, no arquivo de configuração, na administração e num restore

#### Scenario: Acima do teto de chaves
- **WHEN** uma definição lista 1001 chaves de endpoint
- **THEN** a definição é recusada, mesmo com `maximum-endpoints-per-page: 1000`

### Requirement: Seleção dos endpoints da página
Uma página MUST incluir os endpoints publicáveis cujo `group`, depois de remover espaços das pontas, seja igual (diferenciando maiúsculas) a um item de `groups`, e os endpoints publicáveis cuja chave esteja em `endpoints`. São publicáveis:
- os endpoints habilitados do YAML;
- os external-endpoints habilitados;
- os endpoints gerenciados válidos, habilitados e sem conflito.

Suites, endpoints de suites, endpoints de instâncias `remote` e endpoints desabilitados MUST NOT ser incluídos. A seleção MUST ser recalculada a cada montagem, sem serializar objetos em monitoramento. Um endpoint incluído só pela chave MUST aparecer com o nome real do seu grupo.

#### Scenario: Endpoint novo no grupo
- **WHEN** a página `infra` seleciona o grupo `core` e um administrador cria o endpoint gerenciado `core/cache`
- **THEN** a próxima montagem da página inclui `cache` no grupo `core`

#### Scenario: Endpoint por chave
- **WHEN** a página seleciona `endpoints: [database_postgres]`
- **THEN** o endpoint `postgres` aparece na seção `database`, mesmo sem o grupo `database` em `groups`

#### Scenario: Endpoint desabilitado
- **WHEN** o endpoint `core/legacy` tem `enabled: false`
- **THEN** ele não aparece na página que seleciona o grupo `core`

#### Scenario: Suite no mesmo grupo
- **WHEN** existe uma suite com `group: core`
- **THEN** a suite e os endpoints dela não aparecem na página que seleciona o grupo `core`

#### Scenario: External endpoint
- **WHEN** um external-endpoint habilitado tem `group: core`
- **THEN** ele aparece na página que seleciona o grupo `core`

#### Scenario: Gerenciado em conflito
- **WHEN** um endpoint gerenciado está em conflito de chave com o YAML
- **THEN** só o endpoint do YAML aparece na página

### Requirement: Ordem de exibição
As seções MUST seguir a ordem de `groups`, seguidas, em ordem alfabética, dos grupos presentes só por `endpoints` e, por último, de uma seção com `name` vazio para os endpoints sem grupo. Dentro de cada seção, os endpoints MUST ser ordenados por nome sem diferenciar maiúsculas.

#### Scenario: Ordem das seções
- **WHEN** a página tem `groups: [web, core]` e `endpoints: [database_postgres, _ping]`, sendo `ping` um endpoint sem grupo
- **THEN** as seções aparecem na ordem `web`, `core`, `database` e a seção com `name` vazio

### Requirement: Acesso público sem autenticação
`GET /status/:slug`, qualquer caminho sob `/status/`, `GET /api/v1/status-pages/:slug` e qualquer outro caminho ou método sob `/api/v1/status-pages` MUST ser atendidos sem credenciais nem sessão, com qualquer configuração de `security` e de `status-pages.enabled`, **exceto as rotas de uma página que exija login** (requisito "Páginas com login próprio"). Fora essa exceção, nenhuma dessas respostas MUST ser 401 nem incluir `WWW-Authenticate`, e elas MUST ser iguais para requisições anônimas e autenticadas. As rotas de status já protegidas MUST continuar exigindo autenticação.

O `security` da instalação MUST NOT valer para as rotas públicas: a credencial ou a sessão da administração MUST NOT abrir uma página que exige login, e a credencial de uma página MUST NOT abrir nenhuma rota protegida nem outra página.

A rota HTML `/status/*` MUST responder sempre 200 com o HTML da SPA, para `GET` e `HEAD`, sem revelar se a página existe — **exceto** o caminho de uma página que exige login e os caminhos sob ele, que respondem 401 em `GET` e em `HEAD`.

#### Scenario: Basic auth configurado
- **WHEN** a configuração usa `security.basic` e uma requisição sem credenciais pede `GET /api/v1/status-pages/infra`
- **THEN** a API responde 200 sem `WWW-Authenticate`
- **AND** `GET /api/v1/endpoints/statuses` sem credenciais continua respondendo 401

#### Scenario: OIDC sem sessão
- **WHEN** a configuração usa `security.oidc` e uma requisição sem cookie de sessão pede `GET /status/infra` e `GET /api/v1/status-pages/infra`
- **THEN** as duas respondem 200 sem redirecionar para o provedor OIDC

#### Scenario: Caminhos fora do padrão da API
- **WHEN** a configuração usa `security.basic` e chegam, sem credenciais, `GET /api/v1/status-pages/`, `GET /api/v1/status-pages/a/b`, `GET /api/v1/status-pages/infra/extra` e `POST /api/v1/status-pages/infra`
- **THEN** todas respondem o 404 idêntico, sem `WWW-Authenticate`

#### Scenario: Rota HTML de página inexistente
- **WHEN** chega `HEAD /status/nao-existe` e `GET /status/a%2Fb`
- **THEN** as duas respondem 200 com os mesmos cabeçalhos de `GET /status/infra`

### Requirement: Payload público sanitizado
A resposta de `GET /api/v1/status-pages/:slug` MUST conter apenas:
- `slug`, `title`, `description`, `status`, `updatedAt`, `truncated`, `groupsCollapsed`, `summary`, `featured` e `groups`;
- em `summary`, a contagem dos endpoints da página por estado: `total`, `up`, `down`, `pending` e `unknown`;
- em cada grupo, `name`, `status`, `summary` e `endpoints`, com `summary` nos mesmos cinco campos do `summary` da página;
- em cada destaque, os campos de endpoint e `group`;
- em cada endpoint, `name`, `status`, `uptime` (`24h`, `7d`, `30d`), `responseTime` (`24h`, `7d`, `30d`) e `results`, e `certificateExpiresInDays` e `certificateExpiresAt` somente quando a página tem `show-certificate-expiration: true` e o endpoint tem resultado publicado com certificado;
- em cada resultado, `timestamp`, `success` e `durationMs`, e `pending: true` somente quando o resultado é Pending.

MUST NOT conter nenhum outro campo, nem os valores de chave, URL, hostname, IP, porta, código HTTP, código DNS, erros, mensagens, condições, eventos, outras datas de expiração, alertas, `extra-labels` ou origem do endpoint. `certificateExpiresInDays` MUST ser um número inteiro de dias, e `certificateExpiresAt` MUST ser o instante do vencimento do mesmo resultado, como define a capacidade `certificate-expiration`: é a única data de expiração que o payload publica. `groupsCollapsed` MUST ser o valor de `groups-collapsed` da definição da página, `false` quando ausente. `updatedAt` MUST ser o instante da montagem, no relógio do servidor.

A contagem de `summary` MUST considerar todos os endpoints do payload, inclusive os destaques, e `total` MUST ser a soma dos quatro estados. Numa página truncada ela MUST contar os endpoints publicados, que são os mesmos que a página mostra junto do aviso de página truncada: carregar o resumo dos demais anularia o corte. O `summary` de um grupo MUST contar, com as mesmas regras, só os endpoints listados naquele grupo: um endpoint em destaque não é listado em grupo nenhum e MUST NOT entrar na contagem de nenhum. O payload de detalhes de um endpoint MUST NOT conter `summary`.

Os resultados MUST ser os últimos `min(50, storage.maximum-number-of-results)`, do mais antigo para o mais recente. O uptime MUST ser `null` num período sem execuções, com qualquer tipo de storage. Uma página que seleciona mais endpoints do que `maximum-endpoints-per-page` MUST devolver os primeiros, até esse limite, na ordem de exibição — os destaques antes das seções —, com `truncated: true`. O limite MUST ser o mesmo para o payload, para as rotas por endpoint da página e para as contagens da administração, inclusive durante uma recarga da configuração. As telas públicas MUST mostrar os resultados Pending em amarelo, com o rótulo "Pending".

#### Scenario: Resultado com dados sensíveis
- **WHEN** o último resultado do endpoint `core/api` tem hostname `10.0.0.5`, código HTTP 500, erro `dial tcp 10.0.0.5:443` e condições resolvidas
- **THEN** o JSON da página é decodificado sem erro por structs que só conhecem os campos permitidos, rejeitando campos desconhecidos
- **AND** não contém os textos `10.0.0.5` nem `dial tcp`

#### Scenario: Contagem dos endpoints
- **WHEN** a página `infra` publica 12 endpoints no ar, 2 fora, 1 pendente e 1 sem dados
- **THEN** `summary` é `{"total":16,"up":12,"down":2,"pending":1,"unknown":1}`

#### Scenario: Contagem numa página truncada
- **WHEN** `maximum-endpoints-per-page` é `200`, a página `infra` seleciona 250 endpoints e os 200 publicados estão no ar
- **THEN** o payload tem `truncated: true` com 200 endpoints
- **AND** `summary.total` é 200 e `summary.up` é 200

#### Scenario: Contagem de um grupo
- **WHEN** o grupo `apis` lista quatro endpoints: dois no ar, um fora e um sem dados
- **THEN** o grupo tem `summary` igual a `{"total":4,"up":2,"down":1,"pending":0,"unknown":1}`

#### Scenario: Destaque fora da contagem do grupo
- **WHEN** `sites_website` está em destaque e fora do ar, e o grupo `sites` lista outros dois endpoints, no ar
- **THEN** o `summary` de `sites` é `{"total":2,"up":2,"down":0,"pending":0,"unknown":0}`
- **AND** o `summary` da página conta os três

#### Scenario: Estado inicial dos grupos no payload
- **WHEN** uma página tem `groups-collapsed: true` e outra não define o campo
- **THEN** o payload da primeira tem `groupsCollapsed: true` e o da segunda `groupsCollapsed: false`

#### Scenario: Sem a opção
- **WHEN** a seção `status-pages` não define `maximum-endpoints-per-page`, ou a configuração não tem a seção, e uma página seleciona 450 endpoints
- **THEN** o payload tem 400 endpoints e `truncated: true`

#### Scenario: Limite maior
- **WHEN** `maximum-endpoints-per-page` é `800` e a página `infra` seleciona 650 endpoints
- **THEN** o payload traz os 650, com `truncated: false`

#### Scenario: Limite menor depois de a página existir
- **WHEN** uma página gerenciada seleciona 300 endpoints, e o limite passa de `500` a `200` numa recarga da configuração
- **THEN** a página continua publicada, com 200 endpoints e `truncated: true`
- **AND** o payload guardado antes da recarga não é mais servido

#### Scenario: Limite 1 com vários destaques
- **WHEN** `maximum-endpoints-per-page` é `1` e a página tem três destaques e um grupo
- **THEN** o payload traz só o primeiro destaque, com `truncated: true` e `summary.total` igual a 1

### Requirement: Estados agregados
O estado de um endpoint MUST ser:
- `up` quando o último resultado teve sucesso;
- `pending` quando o último resultado é Pending;
- `down` quando o último resultado falhou sem ser Pending;
- `unknown` sem resultados.

O estado de um grupo e o da página MUST ser calculados sobre os endpoints conhecidos (ignorando `unknown`):
- `operational` se todos estiverem `up`;
- `down` se todos estiverem `down`;
- `degraded` em qualquer outra combinação, inclusive quando todos estiverem `pending`;
- `unknown` se nenhum for conhecido.

#### Scenario: Todos no ar
- **WHEN** todos os endpoints da página estão `up` e um está `unknown`
- **THEN** a página tem `status: operational`

#### Scenario: Falha parcial
- **WHEN** um endpoint do grupo `core` está `down` e os outros estão `up`
- **THEN** o grupo `core` e a página têm `status: degraded`

#### Scenario: Tudo fora
- **WHEN** todos os endpoints conhecidos da página estão `down`
- **THEN** a página tem `status: down`

#### Scenario: Endpoint Pending
- **WHEN** o grupo `jobs` tem um endpoint `pending` e os outros `up`
- **THEN** o endpoint tem `status: pending` e o grupo tem `status: degraded`

#### Scenario: Só Pending
- **WHEN** o único endpoint conhecido da página está `pending`
- **THEN** a página tem `status: degraded`

### Requirement: Páginas não publicadas indistinguíveis
Para slug inexistente, slug inválido, slug vazio, caminho com mais segmentos, página desabilitada, página gerenciada em conflito ou inválida, e para qualquer página com `status-pages.enabled: false`, a API MUST responder 404 com o corpo `{"error":"status page not found"}`, sem consultar o storage. Status, corpo e os cabeçalhos `Content-Type`, `Cache-Control`, `X-Robots-Tag`, `X-Content-Type-Options`, `Referrer-Policy` e `Vary` MUST ser iguais em todos esses casos, com `GET` e `HEAD`; `Date` não entra na comparação. Nenhuma resposta pública MUST incluir cabeçalhos `X-RateLimit-*`.

#### Scenario: Inexistente e desabilitada
- **WHEN** a página `interna` está desabilitada e a página `nao-existe` não existe
- **THEN** `GET /api/v1/status-pages/interna` e `GET /api/v1/status-pages/nao-existe` têm o mesmo status, corpo e cabeçalhos comparados

#### Scenario: Slug malformado
- **WHEN** chega `GET /api/v1/status-pages/..%2Fadmin`
- **THEN** a API responde 404 com o mesmo corpo e cabeçalhos de uma página inexistente
- **AND** o storage não é consultado

### Requirement: Cabeçalhos das rotas públicas
As respostas das rotas públicas MUST incluir `X-Robots-Tag: noindex, nofollow`, `X-Content-Type-Options: nosniff` e `Referrer-Policy: strict-origin-when-cross-origin`. As respostas da API pública MUST incluir `Vary: Accept-Encoding`, com ou sem compressão. A API MUST responder `Cache-Control: no-cache` com 200 e `Cache-Control: no-store` com 401, 404, 429 e 503, para que nenhum cache HTTP mantenha no ar uma página desabilitada. As respostas 200 das rotas de uma página que exige login MUST usar `Cache-Control: private, no-store` no lugar de `no-cache`, e o canal de eventos dessa página MUST usar `private, no-cache, no-store, no-transform`, para nenhum cache compartilhado guardar conteúdo restrito. Os canais de eventos MUST responder 200 com `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-store, no-transform` e `X-Accel-Buffering: no`, sem compressão, e 429 e 503 com `Cache-Control: no-store` e os corpos JSON das demais respostas da API pública. A rota HTML MUST responder `Cache-Control: no-cache`. A API pública MUST NOT enviar cabeçalhos CORS.

#### Scenario: Página publicada
- **WHEN** chega `GET /api/v1/status-pages/infra` para uma página publicada
- **THEN** a resposta inclui `X-Robots-Tag: noindex, nofollow`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `Vary: Accept-Encoding` e `Cache-Control: no-cache`

#### Scenario: Rota HTML
- **WHEN** chega `GET /status/infra`
- **THEN** a resposta inclui `X-Robots-Tag: noindex, nofollow` e `Cache-Control: no-cache`

#### Scenario: Canal de eventos
- **WHEN** chega `GET /api/v1/status-pages/infra/endpoints/core_api/events` com `Accept-Encoding: br`
- **THEN** a resposta é 200 sem compressão, com `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-store, no-transform`, `X-Accel-Buffering: no` e os cabeçalhos `X-Robots-Tag`, `X-Content-Type-Options` e `Referrer-Policy`

#### Scenario: Página com login
- **WHEN** chega `GET /api/v1/status-pages/clientes` com a credencial certa de uma página que exige login
- **THEN** a resposta é 200 com `Cache-Control: private, no-store`
- **AND** sem credencial a resposta é 401 com `Cache-Control: no-store`

### Requirement: Cache e montagem única
Cada requisição MUST capturar uma única vez o slug, a revisão em memória, a geração do ciclo e a definição da página, e a montagem MUST usar só a definição capturada. A resposta de cada revisão de página MUST ser mantida em cache próprio por até 30 s, com chave formada por slug, revisão e geração; a revisão MUST mudar a cada publicação e a cada carga, e MUST NOT ser a versão do banco. Requisições simultâneas à mesma revisão MUST disparar no máximo uma montagem, síncrona na goroutine da requisição, e no máximo 4 montagens públicas MUST rodar ao mesmo tempo. A vaga MUST ser pedida só pela montagem que efetivamente monta, depois da deduplicação, de modo que requisições da mesma revisão ocupem uma única vaga. Uma montagem que esperar mais de 5 s por vaga MUST responder 503 a todas as requisições que aguardavam por ela, sem guardar a resposta em cache. O leitor do storage MUST ser obtido a cada montagem, e nenhum leitor MUST ser reaproveitado entre recargas.

Com storage SQL, os resultados e o uptime dos endpoints da página MUST ser lidos numa única transação de leitura, com uma consulta para os resultados e uma para o uptime.

#### Scenario: Muitos visitantes ao mesmo tempo
- **WHEN** 100 requisições simultâneas pedem a página `infra` com o cache vazio
- **THEN** a página é montada uma única vez, ocupando uma única vaga de montagem, e todas recebem a mesma resposta

#### Scenario: Alteração pela administração
- **WHEN** um administrador remove o grupo `web` da página `infra`
- **THEN** a próxima requisição à página não inclui o grupo `web`, mesmo antes de 30 s

#### Scenario: Escrita durante uma montagem
- **WHEN** uma montagem da página `infra` está bloqueada na leitura do storage e um administrador altera a página
- **THEN** a requisição seguinte dispara outra montagem com a definição nova, sem se juntar à montagem bloqueada

#### Scenario: Recarga do YAML
- **WHEN** a página `infra` é removida do YAML e a configuração é recarregada
- **THEN** `GET /api/v1/status-pages/infra` responde 404

#### Scenario: Semáforo esgotado
- **WHEN** 4 montagens de páginas diferentes estão bloqueadas por mais de 5 s e chega uma requisição de outra página com o cache vazio
- **THEN** a API responde 503 genérico
- **AND** a requisição seguinte a essa página tenta montar de novo

### Requirement: Limite de requisições por IP
A API pública MUST limitar cada IP de cliente a `status-pages.rate-limit` respostas 404 por minuto, numa janela deslizante, contando os 404 da rota específica e dos caminhos fora do padrão. Requisições a uma página publicada (servida do cache, montada ou respondida com 503) MUST NOT contar nem ser bloqueadas, mesmo com o limite do IP esgotado. A exceção são os canais de eventos dos endpoints de uma página publicada (`/api/v1/status-pages/{slug}/endpoints/{key}/events`), que MUST respeitar o limite de conexões abertas por IP e no total dos canais de eventos, com 429, sem contar no limite de 404. Ao exceder, a API MUST responder 429 com `Retry-After`, `Cache-Control: no-store` e `{"error":"too many requests"}`.

O IP do cliente MUST ser o IP da conexão. Quando esse IP estiver em `trusted-proxies`, MUST ser o primeiro IP fora de `trusted-proxies` ao percorrer da direita para a esquerda todas as linhas de `X-Forwarded-For`, na ordem de chegada; com header ausente, entrada inválida, mais de 20 entradas ou linha acima de 1 KB, MUST ser o IP da conexão. Entradas com porta (`IP:porta`, `[v6]:porta`) MUST ser aceitas. O IP da conexão e as entradas MUST ser normalizados (IPv4 mapeado em IPv6 vira IPv4) antes da comparação com `trusted-proxies`. Endereços IPv6 MUST ser agregados por /64 na chave do limite. O comportamento de `c.IP()` no restante da aplicação MUST NOT mudar.

O limitador MUST manter no máximo 50 000 chaves, descartando as mais antigas, MUST NOT criar goroutines e MUST ser reaproveitado entre ciclos de recarga. Na primeira requisição de cada ciclo vinda de IP fora de `trusted-proxies` que seja privado (RFC 1918, `100.64.0.0/10`, `fc00::/7`), loopback ou link-local e traga `X-Forwarded-For`, o sistema MUST registrar um único aviso de limite compartilhado citando o IP; depois de uma recarga, o aviso MUST poder aparecer de novo.

#### Scenario: Limite excedido
- **WHEN** `rate-limit` é 120 e o mesmo IP recebe a 121ª resposta 404 no mesmo minuto
- **THEN** a API responde 429 com `Retry-After`
- **AND** a resposta não inclui cabeçalhos `X-RateLimit-*`

#### Scenario: Página publicada nunca é limitada
- **WHEN** `rate-limit` é 10, o mesmo IP já recebeu 10 respostas 404 no minuto e em seguida pede 500 vezes uma página publicada, inclusive depois de o cache da página expirar
- **THEN** todas as respostas da página publicada são 200

#### Scenario: Limite desligado
- **WHEN** `rate-limit` é 0
- **THEN** 500 requisições 404 do mesmo IP no mesmo minuto respondem 404

#### Scenario: Header forjado por cliente direto
- **WHEN** `trusted-proxies` está vazio e um cliente envia `X-Forwarded-For` diferente a cada requisição
- **THEN** todas as requisições contam para o IP da conexão

#### Scenario: Atrás do nginx no Docker
- **WHEN** `trusted-proxies` contém `172.30.0.1/32`, a conexão vem de `172.30.0.1` e o nginx repassa `X-Forwarded-For: 203.0.113.9, 198.51.100.7`
- **THEN** a requisição conta para o IP `198.51.100.7`

#### Scenario: Proxy com endereço IPv4 mapeado
- **WHEN** `trusted-proxies` contém `172.30.0.1/32`, a conexão aparece como `::ffff:172.30.0.1` e traz `X-Forwarded-For: 198.51.100.7`
- **THEN** a requisição conta para o IP `198.51.100.7`

#### Scenario: Header em duas linhas
- **WHEN** a conexão vem de um proxy confiável e chegam as linhas `X-Forwarded-For: 203.0.113.9` e `X-Forwarded-For: 198.51.100.7`
- **THEN** a requisição conta para o IP `198.51.100.7`

#### Scenario: Header gigante
- **WHEN** a conexão vem de um proxy confiável e `X-Forwarded-For` tem 10 000 entradas
- **THEN** a requisição conta para o IP da conexão

#### Scenario: Proxy não configurado
- **WHEN** `trusted-proxies` está vazio e chegam várias requisições de `172.30.0.1` com `X-Forwarded-For`
- **THEN** o log registra uma única vez, no ciclo, o aviso de limite compartilhado citando `172.30.0.1`
- **AND** depois de uma recarga da configuração, a próxima requisição nas mesmas condições registra o aviso de novo

#### Scenario: Recargas sucessivas
- **WHEN** a configuração é recarregada 20 vezes
- **THEN** o número de goroutines do processo não cresce por causa do limitador

#### Scenario: Canal de eventos acima do limite de conexões
- **WHEN** o IP de um visitante já tem 10 canais de eventos abertos e abre mais um para um endpoint de página publicada
- **THEN** a resposta é 429 com `Retry-After: 30`, `Cache-Control: no-store` e `{"error":"too many requests"}`
- **AND** o limite de respostas 404 do IP não muda

### Requirement: Erros internos sem detalhes
Um endpoint selecionado sem registro no storage MUST ser tratado como `unknown`. Qualquer outra falha ao ler o storage durante a montagem MUST resultar em 503 com `{"error":"status page temporarily unavailable"}` e `Cache-Control: no-store`, sem o texto do erro na resposta. O erro MUST ser registrado no log com o slug. A falha MUST ficar em cache negativo por 5 s para a mesma revisão da página, e nenhum payload de revisão anterior MUST ser servido no lugar.

#### Scenario: Banco indisponível
- **WHEN** a leitura em lote falha com `connection refused` durante a montagem da página `infra`
- **THEN** a API responde 503 sem o texto `connection refused`
- **AND** o log registra o erro com o slug

#### Scenario: Sem stampede
- **WHEN** a leitura em lote falha e chegam 50 requisições sequenciais à página `infra` em 1 s
- **THEN** o storage é consultado no máximo uma vez

### Requirement: Mensagens opcionais nas status pages
Cada status page, do arquivo ou gerenciada, MUST aceitar `show-messages`, booleana e desligada por padrão. O formulário da administração MUST ter a opção "Show messages" junto de "Show certificate expiration". Somente com a opção ligada, o payload de detalhes do endpoint MUST incluir em cada resultado:
- `message`: a mensagem do resultado (envio ou heartbeat); senão o texto de heartbeat de resultados antigos, reconhecido pelo prefixo `heartbeat: no update received within ` nos erros; senão `HTTP <código>` de uma verificação ativa, somente com código igual ou maior que 100; senão, num resultado sem sucesso e **não pendente**, o motivo da falha; senão ausente;
- `origin`: `push` nos resultados de envio, inclusive nos recebidos pela API externa de resultados.

O motivo da falha MUST ser um destes textos, e nenhum outro: `Certificate error`, `DNS error`, `Timeout`, `Connection failed` e `Check failed`. A categoria MUST ser a primeira dessa ordem que casar com **qualquer um** dos erros do resultado, por marcadores que não ocorram numa URL comum; sem erros, o motivo MUST ser `Connection failed` quando o resultado não chegou a conectar e `Check failed` quando conectou. O texto do erro MUST NOT ser copiado, recortado nem reescrito no payload, e os cinco textos MUST ser estáveis e não localizados.

Os erros das verificações ativas MUST NOT ser publicados, com ou sem `auth` na página. O payload da página MUST NOT conter mensagens nem origem, com ou sem a opção. Mudar a opção MUST publicar uma nova revisão da página.

#### Scenario: Página com mensagens
- **WHEN** a página `jobs` tem `show-messages: true` e o endpoint Push `jobs/backup` recebeu `status=down&msg=Disco cheio`
- **THEN** o resultado no payload de detalhes tem `message` `Disco cheio` e `origin` `push`

#### Scenario: Conexão recusada
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/api` falhou com o erro `Get "https://api.exemplo.com/certificate-status": dial tcp 10.0.0.5:443: connect: connection refused`
- **THEN** o resultado no payload de detalhes tem `message` `Connection failed`
- **AND** o payload de detalhes não contém `10.0.0.5`, `dial tcp` nem `certificate-status`

#### Scenario: Certificado que não bate com o host
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/site` falhou com o erro `Get "https://tag.exemplo.com": tls: failed to verify certificate: x509: certificate is valid for *.exemplo.com, not tag.exemplo.com`
- **THEN** o resultado no payload de detalhes tem `message` `Certificate error`
- **AND** o payload de detalhes não contém `x509` nem `*.exemplo.com`

#### Scenario: Nome que não resolve
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/sso` falhou com o erro `Get "https://sso.exemplo.com": dial tcp: lookup sso.exemplo.com on 127.0.0.11:53: no such host`
- **THEN** o resultado no payload de detalhes tem `message` `DNS error`
- **AND** o payload de detalhes não contém `127.0.0.11` nem `lookup`

#### Scenario: Tempo esgotado
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/lento` falhou com o erro `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- **THEN** o resultado no payload de detalhes tem `message` `Timeout`

#### Scenario: Falha sem erro registrado
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação do endpoint TCP `core/banco` falhou sem conectar e sem erro registrado
- **THEN** o resultado no payload de detalhes tem `message` `Connection failed`

#### Scenario: Condição que falhou com o serviço respondendo
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/dns` conectou, não registrou erro e falhou apenas a condição
- **THEN** o resultado no payload de detalhes tem `message` `Check failed`

#### Scenario: Verificação SSH sem status HTTP
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/bastion` falhou com status `1` e o erro `dial tcp 10.0.0.9:22: connect: connection refused`
- **THEN** o resultado no payload de detalhes tem `message` `Connection failed`
- **AND** o payload de detalhes não contém `HTTP 1` nem `10.0.0.9`

#### Scenario: Verificação ativa com status HTTP
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação de `core/site` recebeu HTTP 200
- **THEN** o resultado no payload de detalhes tem `message` `HTTP 200`

#### Scenario: Resultado bem-sucedido sem status HTTP
- **WHEN** a página `infra` tem `show-messages: true` e a última verificação do endpoint ICMP `core/gateway` teve sucesso sem status HTTP
- **THEN** o resultado no payload de detalhes não tem `message`

#### Scenario: Resultado pendente
- **WHEN** a página `jobs` tem `show-messages: true` e o último resultado de `jobs/backup` é pendente sem mensagem
- **THEN** o resultado no payload de detalhes não tem `message`

#### Scenario: Resultado da API externa de resultados
- **WHEN** a página `jobs` tem `show-messages: true` e `jobs/fila` recebeu `POST /api/v1/endpoints/jobs_fila/external?success=false&error=timeout na fila de pagamento`
- **THEN** o resultado no payload de detalhes tem `origin` `push` e não tem `message`
- **AND** o payload de detalhes não contém `fila de pagamento`

#### Scenario: Falha de heartbeat pública
- **WHEN** a página `jobs` tem `show-messages: true` e o endpoint Push `jobs/backup` ficou um intervalo de 1 minuto sem envio
- **THEN** o resultado no payload de detalhes tem `message` `heartbeat: no update received within 1m0s`

#### Scenario: Página com login próprio
- **WHEN** a página `clientes` tem `auth` e `show-messages: true` e a última verificação de `core/api` falhou com `connect: connection refused`
- **THEN** o payload de detalhes autenticado tem `message` `Connection failed` e não contém o texto do erro

#### Scenario: Página sem a opção
- **WHEN** a página `jobs` não tem `show-messages`
- **THEN** o payload de detalhes não tem `message` nem `origin`

#### Scenario: Payload da página
- **WHEN** a página `infra` tem `show-messages: true` e um dos resultados falhou com erro
- **THEN** o payload da página não tem `message` nem `origin` em nenhum resultado

### Requirement: Páginas com login próprio
Uma página MAY exigir usuário e senha para ser vista. Com `auth` na definição, as rotas daquela página MUST exigir autenticação HTTP Basic com a credencial **daquela página**:

- a rota HTML `/status/<slug>` e os caminhos sob ela, em `GET` e em `HEAD`;
- `GET /api/v1/status-pages/<slug>` e as rotas de detalhes, de eventos e do gráfico de tempo de resposta dos endpoints dela;
- as rotas de badge da página (`/api/v1/status-pages/<slug>/endpoints/<chave>/health/badge.svg` e `.../response-times/<período>/badge.svg`), que a página de detalhes MUST usar no lugar das rotas globais por chave.

As rotas globais por chave (`/api/v1/endpoints/<chave>/...`) MUST continuar públicas, como no Gatus original: proteger uma página esconde o conjunto que ela publica, não cada número de um endpoint cuja chave já seja conhecida, e a documentação MUST dizer isso.

Sem credencial, ou com credencial errada, a resposta MUST ser 401 com `WWW-Authenticate: Basic realm="<slug>", charset="UTF-8"`, com o slug vindo da definição publicada, e o cabeçalho MUST ser enviado em qualquer requisição, inclusive as que parecem de navegador. A verificação MUST comparar o usuário em tempo constante e a senha com bcrypt, sempre as duas. Sessão, OIDC e a credencial do `security` da instalação MUST ser ignorados por essa verificação.

A requisição MUST resolver a página uma única vez: a definição capturada na autorização MUST ser a mesma usada para montar a resposta. As regras **de página** — inexistente, não publicada, em conflito, `status-pages.enabled: false` e caminho fora do padrão da API — MUST continuar respondendo o mesmo 404 de hoje, sem `WWW-Authenticate`, **antes** do desafio; já o 404 de uma chave que não pertence à página MUST vir **depois** do 401, para que a resposta não diga quais endpoints a página tem.

Tentativas com credencial errada MUST entrar num limite por página e por IP do cliente, resolvido com `status-pages.trusted-proxies`: uma página bloqueada MUST NOT bloquear outra. Estourado o limite, a resposta MUST ser 429 com `Retry-After`, sem comparar a senha, inclusive para quem apresentar a credencial certa dentro da janela. Uma verificação bem-sucedida MUST poder ser memorizada por no máximo cinco minutos para não repetir o bcrypt, com a memória perdendo efeito quando o usuário ou o hash da página mudar, e sem que credenciais diferentes possam colidir na mesma entrada.

Uma página sem `auth` MUST continuar respondendo exatamente como hoje, sem 401 e sem `WWW-Authenticate`.

#### Scenario: Página protegida sem credencial
- **WHEN** chegam, sem credencial, `GET /status/clientes`, `HEAD /status/clientes`, `GET /status/clientes/endpoints/core_api`, `GET /api/v1/status-pages/clientes` e o badge da página
- **THEN** todas respondem 401 com `WWW-Authenticate: Basic` e `Cache-Control: no-store`

#### Scenario: Requisição que parece de navegador
- **WHEN** a requisição sem credencial traz `Sec-Fetch-Site: same-origin` e `X-Requested-With`
- **THEN** a resposta continua 401 **com** `WWW-Authenticate`

#### Scenario: Página protegida com a credencial certa
- **WHEN** a requisição traz a credencial da página
- **THEN** a resposta é 200 com o mesmo conteúdo de uma página pública equivalente e `Cache-Control: private, no-store`

#### Scenario: Credencial de outra origem não serve
- **WHEN** a requisição traz a credencial ou a sessão do `security` da instalação, ou a credencial de outra status page
- **THEN** a resposta é 401

#### Scenario: Chave que não está na página
- **WHEN** chega, sem credencial, o detalhe de uma chave que não pertence à página protegida
- **THEN** a resposta é 401, e não 404

#### Scenario: Página protegida e desabilitada
- **WHEN** a página com `auth` está desabilitada e chega `GET /api/v1/status-pages/clientes` sem credencial
- **THEN** a resposta é o 404 de sempre, sem `WWW-Authenticate`

#### Scenario: Excesso de tentativas numa página
- **WHEN** um mesmo cliente erra a senha da página `clientes` mais vezes que o limite dentro da janela
- **THEN** as requisições seguintes a `clientes` respondem 429 com `Retry-After`, sem comparar a senha, mesmo com a credencial certa
- **AND** a página `parceiros` continua aceitando a credencial dela no mesmo IP

#### Scenario: Página pública não muda
- **WHEN** chega `GET /api/v1/status-pages/infra` sem credencial, para uma página sem `auth`
- **THEN** a resposta é 200, sem `WWW-Authenticate`

### Requirement: Campo groups-collapsed da página
A definição de uma página MUST aceitar o campo opcional `groups-collapsed`, booleano com padrão `false`, no arquivo de configuração e nas páginas gerenciadas, e MUST preservá-lo na normalização, na pré-visualização e no backup da administração. Um valor que não seja booleano MUST ser recusado como os demais campos de tipo errado.

#### Scenario: Página gerenciada
- **WHEN** a administração grava uma página com `groups-collapsed: true` e a lê de volta
- **THEN** a definição devolvida tem `groups-collapsed: true`

#### Scenario: Tipo errado
- **WHEN** a definição enviada tem `groups-collapsed: "sim"`
- **THEN** a validação recusa a página

#### Scenario: Backup e restore
- **WHEN** uma página com `groups-collapsed: true` é salva num backup e restaurada noutra instalação da mesma versão
- **THEN** a página restaurada tem `groups-collapsed: true`

### Requirement: Grupos recolhíveis na página pública
A página pública MUST permitir recolher e expandir cada grupo por um elemento `button` no cabeçalho do grupo, com `aria-expanded` e `aria-controls`, acionável por mouse, Enter e Espaço, e com foco visível nos temas claro e escuro. Um grupo recolhido MUST NOT renderizar as linhas dos seus endpoints. Recolhido ou expandido, o cabeçalho MUST mostrar o nome do grupo, o estado agregado e a contagem do `summary` do grupo, omitindo os estados com zero. A seção de destaques MUST NOT ser recolhível. O aviso de página truncada MUST continuar visível com grupos recolhidos.

#### Scenario: Recolher um grupo
- **WHEN** o visitante aciona o cabeçalho do grupo `sites`, expandido e operacional
- **THEN** as linhas de `sites` deixam de existir no documento, o botão passa a `aria-expanded="false"` e o cabeçalho mostra o estado e a contagem do grupo

#### Scenario: Teclado
- **WHEN** o visitante foca o cabeçalho de um grupo e pressiona Enter ou Espaço
- **THEN** o grupo alterna entre recolhido e expandido

#### Scenario: Destaques
- **WHEN** a página tem endpoints em destaque
- **THEN** a seção `Featured` não tem botão de recolher

### Requirement: Precedência do estado de um grupo
O estado agregado de um grupo é `operational`, `degraded`, `down` ou `unknown`. A cada payload recebido, inclusive os do recarregamento periódico e os da volta da aba, o estado de cada grupo MUST ser decidido nesta ordem: um grupo cujo estado agregado não é `operational` MUST estar expandido; senão, vale a escolha do visitante para aquele grupo, a desta visita ou a lembrada de uma visita anterior; senão, o grupo está recolhido quando `groupsCollapsed` é `true` e expandido quando é `false`. O visitante MAY recolher um grupo não operacional, e essa ação MUST valer só até o próximo payload e MUST NOT ser lembrada. Expandir à força MUST NOT apagar nem alterar a escolha do visitante. A escolha desta visita MUST ser mantida em memória e MUST valer até a página ser fechada, mesmo quando o navegador não consegue lembrá-la entre visitas.

#### Scenario: Página com grupos recolhidos e um problema
- **WHEN** a página tem `groupsCollapsed: true`, `sites` está `operational` e `apis` está `degraded`, sem escolhas lembradas
- **THEN** `sites` aparece recolhido e `apis` expandido

#### Scenario: Grupo recolhido pelo visitante passa a falhar
- **WHEN** o visitante recolheu `sites`, e um payload seguinte traz `sites` como `degraded`
- **THEN** `sites` aparece expandido
- **AND** quando um payload posterior traz `sites` como `operational`, ele volta a aparecer recolhido

#### Scenario: Recolher durante um incidente
- **WHEN** `apis` está `degraded` e o visitante o recolhe
- **THEN** `apis` fica recolhido até o próximo payload, que o expande de novo se o estado continuar não operacional
- **AND** nada é gravado no navegador

#### Scenario: Grupo sem dados
- **WHEN** a página tem `groupsCollapsed: true` e todos os endpoints do grupo `novos` estão sem resultados, com o grupo em `unknown`
- **THEN** `novos` aparece expandido
- **AND** quando um payload posterior traz `novos` como `operational`, ele passa a aparecer recolhido

#### Scenario: Escolha da visita sem armazenamento
- **WHEN** o navegador bloqueia o armazenamento, o visitante recolhe `sites`, `sites` passa a `degraded` e depois volta a `operational`, tudo sem recarregar
- **THEN** `sites` aparece expandido durante o incidente e recolhido de novo depois dele

#### Scenario: Padrão da página sem escolha lembrada
- **WHEN** a página tem `groupsCollapsed: false` e o visitante nunca mexeu em `sites`
- **THEN** `sites` aparece expandido

### Requirement: Escolha do visitante lembrada sem nomes de grupos
A escolha de recolher ou expandir um grupo operacional MUST ser lembrada no navegador, por página e por grupo, e MUST NOT gravar o nome de nenhum grupo nem o slug em texto legível: a chave de cada escolha MUST ser derivada por SHA-256 do slug e do nome bruto do grupo do payload, em que o grupo sem nome é a string vazia. Quando o navegador não oferece `crypto.subtle` ou armazenamento, a página MUST funcionar sem lembrar. Dados guardados inválidos MUST ser ignorados, e ao passar de 500 escolhas guardadas as mais antigas MUST ser descartadas primeiro. As chaves e as escolhas MUST estar resolvidas antes de um payload ser exibido, para que nenhum grupo apareça num estado e mude em seguida, e o resultado de uma derivação que termina depois de o visitante mudar de página MUST ser descartado. A pré-visualização da administração MUST NOT ler nem gravar essas escolhas.

#### Scenario: Lembrar entre visitas
- **WHEN** o visitante recolhe `sites` na página `services` e recarrega
- **THEN** `sites` aparece recolhido

#### Scenario: Páginas diferentes
- **WHEN** o visitante recolhe `sites` na página `services`
- **THEN** o grupo `sites` da página `internal` não é afetado

#### Scenario: Grupo sem nome e grupo chamado "Other services"
- **WHEN** a página tem um grupo sem nome, exibido como "Other services", e um grupo cujo nome é `Other services`
- **THEN** recolher um não recolhe o outro

#### Scenario: Página com login
- **WHEN** o visitante recolhe um grupo numa página com login próprio
- **THEN** o que fica no navegador não contém o nome do grupo nem o slug

#### Scenario: Sem piscar na carga
- **WHEN** o visitante tem `sites` lembrado como recolhido e abre a página
- **THEN** `sites` já aparece recolhido na primeira exibição, sem aparecer expandido antes

#### Scenario: Troca de página durante a derivação
- **WHEN** o visitante navega de `services` para `internal` antes de a derivação das chaves de `services` terminar
- **THEN** nenhuma escolha de `services` é aplicada aos grupos de `internal`

#### Scenario: Dados guardados inválidos
- **WHEN** o item guardado no navegador não é um objeto de chaves hexadecimais com os valores esperados
- **THEN** ele é ignorado e a página usa o padrão

#### Scenario: Pré-visualização com escolha oposta
- **WHEN** o administrador recolheu `sites` na página pública e abre a pré-visualização de uma página com `groups-collapsed: false`
- **THEN** a pré-visualização mostra `sites` expandido

#### Scenario: Armazenamento indisponível
- **WHEN** o navegador bloqueia o armazenamento ou a página é servida sem contexto seguro
- **THEN** recolher e expandir funcionam, e a escolha não é lembrada

### Requirement: O limite de exibição também limita o acesso
Um endpoint que a página seleciona mas que fica além de `maximum-endpoints-per-page` MUST NOT ser acessível pelas rotas por endpoint dessa página: a API de detalhes, o gráfico de tempo de resposta, o stream de eventos e os badges MUST responder 404, como respondem para um endpoint que a página não seleciona, depois das respostas que vêm antes dessa verificação — 401 numa página com login próprio sem a credencial, e 429 do limitador. A rota HTML de detalhes MUST continuar respondendo 200 com a SPA, sem depender da chave. Um payload de detalhes ou de gráfico guardado em cache MUST NOT ser servido para um endpoint que saiu do corte. A página, a credencial e o limite usados para autorizar um pedido MUST ser os mesmos usados para montar sua resposta, sem nova resolução da página no meio. Subir o limite MUST tornar acessíveis os endpoints que passam a ser exibidos, e baixá-lo MUST torná-los inacessíveis, a partir da recarga da configuração.

#### Scenario: Endpoint fora do corte
- **WHEN** a página `infra` seleciona 250 endpoints com o limite em `200`, e o visitante pede a API de detalhes, o gráfico, o stream de eventos e o badge do 201º na ordem de exibição
- **THEN** as quatro rotas respondem 404, como para um endpoint que a página não seleciona
- **AND** `GET /status/infra/endpoints/<chave>` responde 200 com o HTML da SPA

#### Scenario: Página com login
- **WHEN** a mesma página exige login e o pedido do 201º endpoint vem sem a credencial
- **THEN** a resposta é 401, e só com a credencial passa a 404

#### Scenario: Detalhes guardados antes de baixar o limite
- **WHEN** os detalhes do 201º endpoint foram servidos com o limite em `500`, o limite passa a `200` numa recarga, e nenhum resultado novo chegou para esse endpoint
- **THEN** o pedido seguinte responde 404, e não o payload guardado

#### Scenario: Limite elevado
- **WHEN** o limite passa a `500` numa recarga da configuração
- **THEN** as mesmas quatro rotas passam a responder para o 201º endpoint

