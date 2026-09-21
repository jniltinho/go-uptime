## MODIFIED Requirements

### Requirement: Sessões de login
Um login com sucesso MUST criar uma sessão nova identificada por um token aleatório de 32 bytes, ignorando qualquer cookie de sessão recebido, e enviar o cookie `go_uptime_session` com `HttpOnly`, `Path=/`, `SameSite=Strict` e `Max-Age` igual à validade da sessão. O cookie MUST ter `Secure` quando a conexão for TLS ou `X-Forwarded-Proto` for `https`.

O storage MUST guardar só o hash SHA-256 do token, com o usuário, o fingerprint da credencial (usuário e hash bcrypt configurados) e as datas de criação e de expiração:
- com `sqlite`, `postgres` e `mysql`, na tabela `login_sessions`, com a chave primária em `CHAR(64)` no PostgreSQL e no MySQL/MariaDB e com índice em `expires_at`;
- com `memory`, em memória.

A validade MUST ser `security.basic.session-ttl`, com padrão de 8 horas, e a configuração MUST ser recusada fora da faixa de 5 minutos a 30 dias. Cada requisição protegida MUST consultar a sessão no storage, sem cache. Uma sessão expirada, removida ou com fingerprint diferente da credencial atual MUST ser recusada a partir da requisição seguinte, em qualquer instância que use o mesmo banco. Sessões expiradas MUST ser removidas no login e quando forem encontradas numa consulta, sem tarefa em segundo plano.

#### Scenario: Sessão sobrevive a um reinício
- **WHEN** o administrador faz login com storage `sqlite` e o Go Uptime reinicia antes da expiração
- **THEN** a mesma sessão continua autenticando as requisições

#### Scenario: Troca de senha
- **WHEN** o administrador troca `password-bcrypt-base64` no YAML e a configuração é recarregada
- **THEN** a primeira requisição com uma sessão criada com a senha anterior recebe 401

#### Scenario: Logout em outra instância
- **WHEN** duas instâncias usam o mesmo PostgreSQL e o administrador faz logout pela primeira
- **THEN** a próxima requisição com o token antigo na segunda instância recebe 401

#### Scenario: Token no banco
- **WHEN** uma sessão é criada com storage `postgres`
- **THEN** a tabela `login_sessions` não contém o token em texto, só o hash SHA-256

#### Scenario: Fixação de sessão
- **WHEN** uma requisição de login com credenciais corretas traz um cookie `go_uptime_session` já existente
- **THEN** a resposta cria uma sessão nova com outro token

#### Scenario: Validade inválida
- **WHEN** o YAML tem `security.basic.session-ttl: 1m`
- **THEN** a configuração é inválida

### Requirement: API de login e logout
Com `security.basic` sem `security.oidc`, `POST /api/v1/auth/login` MUST aceitar JSON com `username` e `password` e MUST responder:
- 204 com o cookie da sessão para credenciais corretas;
- 401 com a mensagem genérica "Invalid username or password" para credenciais erradas;
- 429 com `Retry-After` quando o IP estiver bloqueado pelo limite de falhas.

`POST /api/v1/auth/logout` MUST aceitar corpo vazio, remover a sessão do cookie, se existir, expirar o cookie com os mesmos `Path`, `SameSite`, `HttpOnly` e `Secure` e responder 204. Sem `security.basic`, ou com `security.oidc`, as duas rotas MUST responder 404.

As duas rotas MUST responder com `Cache-Control: no-store` e MUST seguir a regra de origem da administração:
- recusar com 403 `Sec-Fetch-Site: cross-site`;
- recusar com 403 `Origin` (ou `Referer`) presente que não case com `admin.allowed-origins`, quando definido, ou com a origem derivada de `Host` e do esquema, ignorando `X-Forwarded-Host`;
- aceitar requisições sem `Origin` e sem `Referer`;
- aceitar `http://localhost:8081` com `ENVIRONMENT=dev`.

O login MUST recusar com 415 corpos que não sejam JSON e com 413 corpos acima de 4 KB. A senha e o token MUST NOT aparecer em logs nem em respostas.

#### Scenario: Login pela API
- **WHEN** um cliente envia `POST /api/v1/auth/login` com as credenciais corretas e `Content-Type: application/json`
- **THEN** a resposta é 204 com `Set-Cookie: go_uptime_session=...; HttpOnly; SameSite=Strict`

#### Scenario: Login de outro site
- **WHEN** chega `POST /api/v1/auth/login` com `Origin: https://site-malicioso.exemplo`
- **THEN** a resposta é 403 e nenhuma sessão é criada

#### Scenario: Login sem Origin
- **WHEN** chega `POST /api/v1/auth/login` com as credenciais corretas, sem `Origin` e sem `Referer`
- **THEN** a resposta é 204

#### Scenario: Logout
- **WHEN** o administrador com sessão envia `POST /api/v1/auth/logout` sem corpo
- **THEN** a resposta é 204 com o cookie expirado
- **AND** requisições seguintes com o token antigo recebem 401

#### Scenario: Login com OIDC configurado
- **WHEN** a configuração tem `security.oidc` e `security.basic`
- **THEN** `POST /api/v1/auth/login` responde 404 e `GET /api/v1/config` informa `"login": "oidc"`

### Requirement: Rotas protegidas com sessão ou Authorization Basic
Com `security.basic` sem `security.oidc`, as rotas protegidas da API MUST aceitar uma sessão válida ou o header `Authorization: Basic` com as credenciais corretas. Um cookie de sessão ausente, desconhecido ou expirado MUST NOT impedir a autenticação pelo header. Sem autenticação, MUST responder 401 com `Cache-Control: no-store`. A resposta 401 MUST incluir `WWW-Authenticate: Basic` somente quando a requisição não tiver `Sec-Fetch-Site`, `Sec-Fetch-Mode`, `X-Requested-With` nem `Accept` com `text/event-stream` (canal de eventos, que não aceita cabeçalhos próprios e, em página HTTP fora de localhost, não recebe `Sec-Fetch-*` do navegador). O frontend MUST enviar `X-Requested-With: XMLHttpRequest` nas chamadas à API protegida. A autoria das escritas da administração MUST ser o usuário da sessão ou do header.

#### Scenario: Script com curl
- **WHEN** `curl -u admin:senha` pede `GET /api/v1/endpoints/statuses`
- **THEN** a resposta é 200

#### Scenario: Curl com cookie antigo
- **WHEN** `curl -u admin:senha` pede `GET /api/v1/endpoints/statuses` com um cookie `go_uptime_session` expirado
- **THEN** a resposta é 200

#### Scenario: Curl sem credenciais
- **WHEN** `curl` pede `GET /api/v1/endpoints/statuses` sem credenciais nem cookie
- **THEN** a resposta é 401 com `WWW-Authenticate: Basic`

#### Scenario: Navegador sem sessão
- **WHEN** o navegador pede `GET /api/v1/endpoints/statuses` com `Sec-Fetch-Site: same-origin` e sem sessão
- **THEN** a resposta é 401 sem `WWW-Authenticate`

#### Scenario: Canal de eventos sem sessão em HTTP
- **WHEN** o navegador, numa página HTTP fora de localhost, pede `GET /api/v1/endpoints/jobs_backup/events` com `Accept: text/event-stream`, sem `Sec-Fetch-*` e sem sessão
- **THEN** a resposta é 401 sem `WWW-Authenticate`
