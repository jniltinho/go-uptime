# basic-login-page Specification

## Purpose
TBD - created by archiving change add-basic-login-page. Update Purpose after archive.
## Requirements
### Requirement: Tela de login do security.basic
Com `security.basic` configurado e sem `security.oidc`, o frontend MUST oferecer a rota `/login`, atendida pela SPA sem o cabeçalho do dashboard. A tela MUST mostrar:
- um cartão com o logo de `ui.logo` (o logo padrão embutido quando a opção não está definida, nenhum com `ui.logo: none`), o título de `ui.header`, os campos de usuário e de senha e o botão de entrar;
- o cartão centralizado na horizontal, com a borda superior a 10% da altura da janela;
- o tema (claro, escuro ou Bio) lido do mesmo cookie de tema das outras telas, com o mesmo seletor de tema das outras telas;
- o visual quadrado do fork e variantes `dark:`.

Com credenciais erradas, a tela MUST mostrar uma mensagem genérica que não indique se o usuário existe. Com o limite de falhas estourado, a tela MUST pedir para tentar mais tarde.

Depois de um login com sucesso, a tela MUST recarregar `GET /api/v1/config` antes de sair de `/login`, para o estado de autenticação da SPA refletir a nova sessão. Em seguida, MUST decodificar o `redirect` repetidamente, até ele parar de mudar (no máximo 3 vezes), e MUST levar a esse caminho somente quando:
- nenhuma decodificação tiver falhado (`%` malformado);
- o valor decodificado não tiver mais `%`;
- começar com um único `/`;
- não contiver `//`, `\`, esquema nem caracteres de controle;
- não apontar para `/login`.

Nos outros casos, MUST levar ao dashboard `/`. Sem `security.basic`, ou com `security.oidc`, a rota `/login` MUST levar a `/`.

#### Scenario: Visitante sem sessão abre a administração
- **WHEN** um navegador sem sessão abre `/admin`
- **THEN** a SPA mostra `/login?redirect=/admin` sem abrir a janela nativa de usuário e senha do navegador e sem o cabeçalho do dashboard
- **AND** o cartão de login fica a 10% do topo da janela, no tema do cookie de tema

#### Scenario: Senha errada
- **WHEN** o visitante envia a senha errada
- **THEN** a tela mostra "Invalid username or password" e continua em `/login`

#### Scenario: Login e redirecionamento
- **WHEN** o visitante envia as credenciais corretas em `/login?redirect=/admin/status-pages`
- **THEN** a SPA recarrega `GET /api/v1/config` e abre `/admin/status-pages`
- **AND** não volta para `/login`

#### Scenario: Redirecionamentos recusados
- **WHEN** o visitante faz login com `redirect` igual a `//site-malicioso.exemplo`, `/%2F%2Fsite-malicioso.exemplo`, `/%252F%252Fsite-malicioso.exemplo`, `/\site-malicioso.exemplo`, `https://site-malicioso.exemplo`, `/%ZZ` ou `/login`
- **THEN** a SPA abre o dashboard `/` em todos os casos

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

### Requirement: Limite de falhas de autenticação
O sistema MUST contar as falhas de autenticação por IP de cliente, calculado com `status-pages.trusted-proxies` e com IPv6 agrupado por /64. Contam como falha:
- as tentativas de login com credenciais erradas;
- as requisições às rotas protegidas com `Authorization: Basic` errado;
- as consultas a `GET /api/v1/config` com `Authorization: Basic` errado.

O limite MUST ser consultado antes da verificação da senha, e logins e headers corretos MUST NOT contar como falha. Depois de 10 falhas no mesmo minuto, o login e as requisições às rotas protegidas com `Authorization: Basic` desse IP MUST responder 429 com `Retry-After` até a janela reiniciar, inclusive com as credenciais corretas. Em `GET /api/v1/config`, o header de um IP bloqueado MUST ser tratado como não autenticado, sem verificar a senha. Com o mesmo critério das status pages, o sistema MUST registrar um aviso no log, uma vez por geração da configuração, quando uma requisição trouxer `X-Forwarded-For` de uma conexão com IP privado, loopback, link-local ou CGNAT fora de `status-pages.trusted-proxies`. O usuário e a senha MUST ser conferidos juntos, com comparação em tempo constante do usuário e bcrypt sempre contra o hash configurado. A contagem MUST ser por processo, sem compartilhamento entre instâncias.

#### Scenario: Força bruta no login
- **WHEN** um IP envia 11 senhas erradas para `POST /api/v1/auth/login` no mesmo minuto
- **THEN** a 11ª tentativa responde 429 com `Retry-After`
- **AND** uma tentativa com a senha certa do mesmo IP no mesmo minuto também responde 429

#### Scenario: Força bruta pelo header
- **WHEN** um IP envia 11 requisições com `Authorization: Basic` errado para `GET /api/v1/endpoints/statuses` no mesmo minuto
- **THEN** a 11ª requisição responde 429 sem executar a verificação da senha

#### Scenario: Força bruta pela configuração
- **WHEN** um IP envia 11 consultas a `GET /api/v1/config` com `Authorization: Basic` errado no mesmo minuto
- **THEN** a próxima consulta desse IP com o header correto responde `"authenticated": false` sem verificar a senha
- **AND** o login desse IP no mesmo minuto responde 429

#### Scenario: Login correto não conta
- **WHEN** um IP faz 20 logins corretos no mesmo minuto
- **THEN** todos respondem 204

### Requirement: Estado de login exposto ao frontend
`GET /api/v1/config` MUST incluir `login`:
- `"basic"` com `security.basic` sem OIDC;
- `"oidc"` com `security.oidc`;
- vazio sem `security`.

MUST manter `oidc`. `authenticated` MUST ser verdadeiro com uma sessão basic válida ou `Authorization: Basic` correto de um IP não bloqueado, e um header errado MUST contar como falha no limite. `GET /api/v1/config` MUST autenticar uma única vez por requisição e derivar `authenticated` e `admin.authorized` desse resultado, de modo que uma senha errada conte uma falha e a senha seja verificada uma vez. Com `security.basic`, `admin.authorized` MUST ser verdadeiro somente quando `authenticated` for verdadeiro. Com `login: "basic"` e sem autenticação, o frontend MUST levar as telas protegidas (dashboard, detalhes de endpoints e de suites e administração) para `/login` com o `redirect` do caminho atual. Um 401 recebido pelas chamadas do dashboard, dos detalhes de endpoints e de suites ou da administração MUST levar à mesma tela. Com `login: "basic"` e autenticação, o cabeçalho MUST mostrar o botão "Logout", que chama `POST /api/v1/auth/logout` e leva a `/login`. As status pages públicas MUST continuar abrindo sem login.

#### Scenario: Estado sem sessão
- **WHEN** a configuração usa `security.basic` e um navegador sem sessão consulta `GET /api/v1/config`
- **THEN** a resposta contém `"login": "basic"` e `"authenticated": false`

#### Scenario: Administração sem sessão
- **WHEN** a configuração usa `security.basic` com `admin.enabled: true` e um navegador sem sessão consulta `GET /api/v1/config`
- **THEN** a resposta contém `"admin": {"enabled": true, "authorized": false}`

#### Scenario: Uma verificação por consulta
- **WHEN** a configuração usa `security.basic` com `admin.enabled: true` e uma consulta a `GET /api/v1/config` traz `Authorization: Basic` errado
- **THEN** o contador de falhas do IP aumenta em exatamente 1 e a senha é verificada uma única vez

#### Scenario: Estado com o header
- **WHEN** a configuração usa `security.basic` e `curl -u admin:senha` consulta `GET /api/v1/config`
- **THEN** a resposta contém `"authenticated": true`

#### Scenario: Logout pelo cabeçalho
- **WHEN** o administrador logado clica em "Logout" no dashboard
- **THEN** a sessão é encerrada e a SPA mostra `/login`

#### Scenario: Status page pública
- **WHEN** um visitante sem sessão abre `/status/services`
- **THEN** a página pública abre sem passar pela tela de login

