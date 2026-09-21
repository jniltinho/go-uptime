## MODIFIED Requirements

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
