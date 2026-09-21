# http-server Specification

## Purpose
TBD - created by archiving change migrate-to-echo-and-cobra. Update Purpose after archive.
## Requirements
### Requirement: Contrato HTTP preservado na troca do servidor
A troca do servidor HTTP MUST preservar, para toda rota registrada, o método, o caminho, o código de status, o corpo e os cabeçalhos de cache, de segurança e de autenticação que a rota responde hoje. Uma suíte de contrato MUST existir e passar **antes** da troca, contra o servidor atual, e MUST passar sem alteração depois dela.

#### Scenario: Suíte de contrato nos dois servidores
- **WHEN** a suíte de contrato roda contra o roteador antes e depois da troca
- **THEN** todos os casos passam nos dois, sem mudar nenhuma expectativa

#### Scenario: Corpo do erro padrão
- **WHEN** chega `GET /api/v1/rota-que-nao-existe`
- **THEN** o status, o `Content-Type` e o corpo são os mesmos de antes da troca

### Requirement: Rotas protegidas e públicas por grupo explícito
As rotas MUST ser registradas em dois grupos explícitos sob `/api`, o público e o protegido, e a proteção MUST NOT depender da ordem de registro. Toda rota protegida MUST responder 401 sem credencial, e as rotas de captura das status pages e do push MUST continuar respondendo o 404 idêntico, sem `WWW-Authenticate`, para um caminho desconhecido.

#### Scenario: Rota protegida sem credencial
- **WHEN** chega `GET /api/v1/endpoints/statuses` sem credencial numa instalação com `security`
- **THEN** a resposta é 401

#### Scenario: Caminho desconhecido sob as status pages
- **WHEN** chega `GET /api/v1/status-pages/a/b/c` sem credencial
- **THEN** a resposta é o 404 idêntico das status pages, sem `WWW-Authenticate`

### Requirement: Caminho, cabeçalhos e corpo lidos da requisição
O servidor MUST decidir autenticação, compressão e limites pelo caminho **da requisição**, nunca pelo padrão da rota registrada. Os cabeçalhos MUST ser lidos da requisição e gravados na resposta, e o armazenamento por requisição MUST ser usado só para os valores que um middleware passa ao manipulador. Um middleware que lê o corpo MUST entregá-lo inteiro ao manipulador seguinte.

#### Scenario: HTML de uma página com login
- **WHEN** chega `GET /status/clientes/endpoints/core_api` sem credencial e a página `clientes` tem `auth`
- **THEN** a resposta é 401 com `WWW-Authenticate: Basic realm="clientes"`

#### Scenario: Origem de outro site na administração
- **WHEN** chega um `POST` da administração com `Origin: https://outro.exemplo`
- **THEN** a resposta é 403 e nada é gravado

#### Scenario: Corpo depois do middleware
- **WHEN** chega um `POST` válido de 1 KB para criar um endpoint
- **THEN** o manipulador recebe o corpo inteiro e responde 201

### Requirement: Cabeçalhos definidos antes da escrita
Nenhum manipulador ou middleware MUST gravar cabeçalho depois de começar a escrever a resposta. Os badges de uma página com login MUST chegar ao cliente com `Cache-Control: private, no-store` e `Vary: Authorization`, e o tratador de erros MUST respeitar uma resposta já iniciada.

#### Scenario: Badge de uma página com login
- **WHEN** chega `GET /api/v1/status-pages/clientes/endpoints/core_api/health/badge.svg` com a credencial da página
- **THEN** a resposta recebida tem `Cache-Control: private, no-store` e `Vary: Authorization`

### Requirement: Corpo JSON e parâmetros idênticos
As respostas JSON MUST ter os mesmos bytes de antes da troca, sem quebra de linha acrescentada, e o servidor MUST NOT emitir `ETag` automático. Os parâmetros do caminho MUST ser decodificados o mesmo número de vezes que hoje.

#### Scenario: Chave com barra codificada
- **WHEN** chega uma requisição cuja chave tem `%2F` ou `%252F`
- **THEN** a chave vista pelo manipulador é a mesma de antes da troca

### Requirement: Esquema e sessão sem confiar em cabeçalhos
A proteção contra CSRF e o atributo `Secure` do cookie de sessão MUST continuar usando o TLS da própria conexão, o `Host` da requisição e o **primeiro valor** de `X-Forwarded-Proto`, exatamente como hoje, e MUST NOT usar a resolução de esquema do framework, que também confia em `X-Forwarded-Protocol`, `X-Forwarded-Ssl` e `X-Url-Scheme`, nem `X-Forwarded-Host`. O cookie `go_uptime_session` MUST manter `Path=/`, `HttpOnly` e `SameSite=Strict`.

#### Scenario: Proxy que termina o TLS
- **WHEN** uma conexão sem TLS envia `X-Forwarded-Proto: https` no login
- **THEN** o cookie de sessão é emitido com `Secure`, como hoje

#### Scenario: Cabeçalhos que o framework aceitaria
- **WHEN** uma conexão sem TLS envia só `X-Forwarded-Ssl: on` no login
- **THEN** o cookie de sessão é emitido sem `Secure`

#### Scenario: Host forjado
- **WHEN** chega um `POST` da administração com `Origin: https://evil.example` e `X-Forwarded-Host: evil.example`
- **THEN** a resposta é 403

### Requirement: Métodos HEAD preservados
As rotas que hoje respondem `HEAD` por serem registradas com `GET` MUST continuar respondendo `HEAD`, entre elas o HTML das status pages — com o desafio 401 da página com login — e o canal de eventos. O `HEAD` do canal de eventos MUST NOT abrir stream nem ocupar vaga do limitador, e o tratamento automático de `HEAD` do roteador MUST ficar desligado.

#### Scenario: HEAD do HTML de uma página com login
- **WHEN** chega `HEAD /status/clientes` sem credencial e a página `clientes` tem `auth`
- **THEN** a resposta é 401 com `WWW-Authenticate`

### Requirement: Limite do corpo da requisição
O servidor MUST recusar com 413 qualquer corpo acima de 4 MiB, em todas as rotas, **antes de qualquer efeito do manipulador**, com ou sem `Content-Length`. Os limites menores que já existem — 256 KB na administração, 3,5 MiB no restore e 4 KB no login — MUST continuar valendo, e o excesso MUST responder 413, nunca um erro de parsing.

#### Scenario: Corpo acima do limite global
- **WHEN** chega um `POST` com 5 MiB em qualquer rota
- **THEN** a resposta é 413 e o manipulador não é chamado

#### Scenario: Corpo sem tamanho declarado num push
- **WHEN** chega um push com corpo *chunked* de 5 MiB
- **THEN** a resposta é 413 e nenhum resultado é gravado

### Requirement: IP do cliente sem confiar em cabeçalhos
O IP do cliente MUST vir do endereço da conexão e passar por `status-pages.trusted-proxies`, como hoje, considerando **todas** as linhas de `X-Forwarded-For`. O servidor MUST NOT usar resolução de IP que confie em `X-Forwarded-For` ou `X-Real-IP` de uma origem não confiável, em nenhum limitador nem log, e MUST NOT configurar um extrator de IP no roteador.

#### Scenario: Cabeçalho forjado
- **WHEN** uma conexão de `203.0.113.10`, que não é proxy confiável, envia `X-Forwarded-For: 10.0.0.1`
- **THEN** os limitadores contam a requisição para `203.0.113.10`

#### Scenario: Cabeçalho em várias linhas
- **WHEN** um proxy confiável envia duas linhas `X-Forwarded-For`, a segunda com o IP do visitante
- **THEN** o IP do visitante é o da segunda linha, como hoje

### Requirement: Caminhos e barra final
O servidor MUST tratar `/status/infra/` como `/status/infra`, sem redirecionar. Os caminhos MUST distinguir maiúsculas de minúsculas.

#### Scenario: Barra final
- **WHEN** chega `GET /status/infra/`
- **THEN** a resposta é a mesma de `GET /status/infra`

#### Scenario: Maiúsculas
- **WHEN** chega `GET /API/v1/config`
- **THEN** a resposta é 404

### Requirement: Canal de eventos sem goroutine de escrita
O canal de eventos MUST escrever e descarregar a resposta no próprio manipulador, MUST encerrar quando o contexto da requisição terminar, MUST ter prazo de escrita próprio, maior que a duração máxima do canal e definido antes do primeiro byte, e MUST NOT ser comprimido, qualquer que seja o `Accept-Encoding`. No desligamento e na recarga, os canais MUST ser fechados **antes** de o servidor parar, o encerramento normal do servidor MUST NOT ser tratado como erro fatal, e o processo MUST sobreviver a uma recarga com canal aberto. Os limites de 500 streams no total e 10 por IP MUST continuar valendo, com a vaga liberada em qualquer saída do manipulador, inclusive pânico.

#### Scenario: Cliente que desconecta
- **WHEN** o cliente fecha a conexão de um canal de eventos aberto
- **THEN** o manipulador termina e a vaga do IP é liberada

#### Scenario: Recarga com canal aberto
- **WHEN** a configuração é recarregada com um canal de eventos aberto
- **THEN** o canal é fechado, o servidor novo sobe e o processo continua no ar

#### Scenario: Canal além do prazo de escrita do servidor
- **WHEN** um canal de eventos fica aberto por 20 segundos
- **THEN** ele continua entregando pings e eventos

#### Scenario: Evento entregue sem espera
- **WHEN** um resultado novo é gravado para o endpoint observado
- **THEN** o evento chega ao cliente sem esperar o fechamento da resposta nem um buffer de compressão

### Requirement: Caminho desconhecido sob a API
Com `security`, um caminho que não é rota sob `/api/` MUST continuar respondendo 401 sem credencial e 404 com credencial, exceto sob as capturas públicas das status pages e do push.

#### Scenario: Caminho desconhecido sem credencial
- **WHEN** chega `GET /api/v1/nao-existe` sem credencial numa instalação com `security`
- **THEN** a resposta é 401

### Requirement: Arquivos estáticos sem listagem
Os arquivos estáticos embutidos MUST ser servidos sem listagem de diretório, sem desfazer a codificação do caminho e sem `Cache-Control` nas fontes. `/index.html` MUST responder 301 para `/`.

#### Scenario: Diretório
- **WHEN** chega `GET /js/`
- **THEN** a resposta não lista os arquivos do diretório

### Requirement: Opção web.read-buffer-size
`web.read-buffer-size` MUST continuar aceita no `config.yaml` e MUST limitar, de forma aproximada, o tamanho dos cabeçalhos da requisição; a documentação MUST dizer que o limite é aproximado.

#### Scenario: Cabeçalho acima do limite
- **WHEN** `web.read-buffer-size` é `8192` e chega uma requisição com 16 KB de cabeçalhos
- **THEN** o servidor recusa a requisição com 431

### Requirement: Sem adaptadores entre servidores HTTP
As métricas do Prometheus, o OIDC e o portão de autenticação MUST ser ligados ao servidor como manipuladores e middlewares `net/http`, sem conversão da requisição, e o módulo MUST NOT depender de `github.com/gofiber/fiber/v2` nem de `github.com/valyala/fasthttp`.

#### Scenario: Dependências
- **WHEN** `go mod tidy` roda depois da troca
- **THEN** `go.mod` não lista `gofiber/fiber` nem `valyala/fasthttp`

