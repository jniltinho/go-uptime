# status-page-management Specification

## Purpose
TBD - created by archiving change add-public-status-pages. Update Purpose after archive.
## Requirements
### Requirement: Persistência das páginas gerenciadas
Com storage `sqlite` ou `postgres`, o sistema MUST criar a tabela `managed_status_pages` com `CREATE TABLE IF NOT EXISTS`. Ela tem as colunas:
- `slug`, único;
- `definition`, com a definição enviada, sem valores padrão;
- `version`, começando em 1;
- `created_at` e `updated_at`;
- `updated_by`.

Um erro na criação da tabela MUST interromper a inicialização. As páginas gerenciadas MUST ser carregadas a cada inicialização e recarga da configuração, depois dos endpoints gerenciados e independentemente de `admin.enabled`. A carga MUST registrar no log os slugs publicados por origem.

Se a listagem das páginas gerenciadas falhar na carga, o sistema MUST publicar as páginas do YAML, MUST NOT publicar nenhuma gerenciada, MUST registrar o erro no log e MUST sinalizar a origem gerenciada como indisponível na listagem da administração.

#### Scenario: Reinício
- **WHEN** um administrador cria e habilita a página `clientes` e o Go Uptime é reiniciado
- **THEN** `GET /api/v1/status-pages/clientes` responde 200 depois da partida

#### Scenario: Administração desligada depois
- **WHEN** existe a página gerenciada publicada `clientes` e a configuração passa a ter `admin.enabled: false`
- **THEN** `GET /api/v1/status-pages/clientes` continua respondendo 200
- **AND** o log da carga lista `clientes` entre as páginas publicadas de origem gerenciada
- **AND** `GET /api/v1/admin/status-pages` com credenciais válidas responde 404

#### Scenario: Falha ao listar as gerenciadas
- **WHEN** a leitura de `managed_status_pages` falha na carga e o YAML define a página `infra`
- **THEN** `GET /api/v1/status-pages/infra` responde 200
- **AND** `GET /api/v1/admin/status-pages` informa que as páginas gerenciadas estão indisponíveis

**Listagem:** cada item da listagem MUST dizer se a página exige login, para a tela poder marcá-la, sem devolver usuário nem hash.

#### Scenario: Item da lista
- **WHEN** a administração lista as páginas e uma delas exige login
- **THEN** o item dessa página diz que ela exige login, sem a credencial

### Requirement: Autorização e proteções da API de administração
As rotas `/api/v1/admin/status-pages/*` MUST existir apenas com `admin.enabled: true` e MUST exigir autenticação e autorização de administrador, como `/api/v1/admin/endpoints`. Nas requisições `POST`, `PUT` e `DELETE`, MUST aplicar a mesma proteção CSRF, o limite de 256 KB de corpo e os mesmos tipos de mídia aceitos. A API de administração de status pages MUST continuar disponível com `status-pages.enabled: false` e MUST informar esse estado na listagem.

#### Scenario: Sem credenciais
- **WHEN** uma requisição sem credenciais chega a `GET /api/v1/admin/status-pages`
- **THEN** a API responde 401

#### Scenario: Subject OIDC sem permissão
- **WHEN** o subject da sessão não está em `admin.allowed-subjects`
- **THEN** `POST /api/v1/admin/status-pages` responde 403

#### Scenario: Requisição de outro site
- **WHEN** chega `DELETE /api/v1/admin/status-pages/clientes` com credenciais válidas e `Sec-Fetch-Site: cross-site`
- **THEN** a API responde 403
- **AND** a página não é removida

#### Scenario: Formulário HTML
- **WHEN** chega `POST /api/v1/admin/status-pages` com `Content-Type: application/x-www-form-urlencoded`
- **THEN** a API responde 415

#### Scenario: Publicação desligada no YAML
- **WHEN** a configuração tem `status-pages.enabled: false`
- **THEN** `GET /api/v1/admin/status-pages` responde 200 com `publicationEnabled: false`
- **AND** `GET /api/v1/admin/status-pages/infra/preview` responde 200

### Requirement: Criação de página
`POST /api/v1/admin/status-pages` MUST aceitar a definição em JSON ou YAML, decodificada de forma estrita, validar com as regras das páginas do YAML e gravar com versão 1. Sem o campo `enabled`, a página MUST ser criada desabilitada. A resposta MUST ser 201 com a definição e `ETag`. Uma página criada com `enabled: true` MUST ficar disponível na API pública sem reiniciar nem recarregar o Go Uptime. MUST responder:
- 400 para definição inválida, slug reservado ou campo desconhecido;
- 409 para slug usado por outra página, citando a origem;
- 501 quando o storage não suporta páginas gerenciadas.

#### Scenario: Criação publicada
- **WHEN** um administrador envia `{"slug":"clientes","title":"Clientes","groups":["core"],"enabled":true}`
- **THEN** a API responde 201 com `ETag: "1"`
- **AND** `GET /api/v1/status-pages/clientes` responde 200

#### Scenario: Criação sem enabled
- **WHEN** um administrador envia `{"slug":"parceiros","title":"Parceiros","groups":["core"]}`
- **THEN** a API responde 201 com a página desabilitada
- **AND** `GET /api/v1/status-pages/parceiros` responde 404

#### Scenario: Slug usado pelo YAML
- **WHEN** a página `infra` existe no YAML e um administrador tenta criar uma página com `slug: infra`
- **THEN** a API responde 409 com uma mensagem informando que o slug está no arquivo de configuração

#### Scenario: Slug reservado
- **WHEN** um administrador tenta criar uma página com `slug: preview`
- **THEN** a API responde 400 e nada é gravado

#### Scenario: Campo desconhecido
- **WHEN** a definição enviada tem o campo `grups`
- **THEN** a API responde 400 e nada é gravado

**Credencial da página:** a criação e a alteração MAY receber, no documento de submissão da administração, a senha em claro junto do usuário. O serviço MUST gerar o hash bcrypt e persistir **somente o hash**: a senha MUST NOT ser gravada na definição, no YAML guardado, em log nem em mensagem de erro. A senha MUST ter pelo menos 8 caracteres e no máximo 72 bytes, o limite do bcrypt; fora disso a escrita MUST ser recusada com 400.

Na alteração de uma página que já exige login, senha vazia MUST manter o hash guardado, e o hash mascarado MUST ser entendido como "manter o hash guardado", porque o administrador pode reenviar o documento que leu. Numa página que ainda não exige login, a senha MUST ser obrigatória. Remover a credencial MUST apagá-la na mesma escrita.

#### Scenario: Criação com login
- **WHEN** o administrador cria a página `clientes` com usuário `cliente` e senha `segredo-bom`
- **THEN** a definição salva tem o hash bcrypt, e nem a definição nem o YAML guardado contêm a senha

#### Scenario: Alteração sem trocar a senha
- **WHEN** o administrador salva a página `clientes` com senha vazia, ou reenvia o documento com o hash mascarado
- **THEN** o hash salvo continua o mesmo

#### Scenario: Senha curta demais
- **WHEN** a senha tem menos de 8 caracteres
- **THEN** a escrita é recusada com 400 e a definição não muda

### Requirement: Validação, opções, exposição e pré-visualização
O sistema MUST oferecer:
- `POST /api/v1/admin/status-pages/validate`: devolve a definição normalizada e avisos para grupos e chaves sem endpoint correspondente, sem gravar;
- `GET /api/v1/admin/status-pages/options`: grupos (nome e quantidade de endpoints) e endpoints publicáveis (chave, nome e grupo), sem suites nem endpoints desabilitados;
- `GET /api/v1/admin/status-pages/exposure?group=<grupo>&key=<chave>`: as páginas (slug, título, origem, `enabled` e motivo `group` ou `key`) em que um endpoint com esse grupo e essa chave apareceria; basta um dos dois parâmetros, e sem nenhum a API MUST responder 400;
- `GET /api/v1/admin/status-pages/:slug/preview`: o payload público de qualquer página, inclusive desabilitada, em conflito ou do YAML, sem usar o cache público, sem o limitador e com limite próprio de uma montagem simultânea (com o mesmo timeout de 5 s), sem ocupar as vagas de montagem da página pública.

#### Scenario: Aviso de grupo sem endpoints
- **WHEN** um administrador valida uma página com `groups: [pagamentos]` e nenhum endpoint usa esse grupo
- **THEN** a resposta é 200 com um aviso citando `pagamentos`

#### Scenario: Opções sem suites
- **WHEN** existem o endpoint `core/api`, o endpoint desabilitado `core/legacy` e uma suite do grupo `core`
- **THEN** `GET /api/v1/admin/status-pages/options` lista o grupo `core` com 1 endpoint e apenas `core_api` nos endpoints

#### Scenario: Exposição por grupo e por chave
- **WHEN** a página `infra` seleciona o grupo `core` e a página `clientes` seleciona a chave `core_api`
- **THEN** `GET /api/v1/admin/status-pages/exposure?group=core&key=core_api` lista `infra` com motivo `group` e `clientes` com motivo `key`

#### Scenario: Exposição sem parâmetros
- **WHEN** chega `GET /api/v1/admin/status-pages/exposure` sem `group` e sem `key`
- **THEN** a API responde 400

#### Scenario: Pré-visualização de página desabilitada
- **WHEN** a página gerenciada `clientes` está desabilitada
- **THEN** `GET /api/v1/admin/status-pages/clientes/preview` responde 200 com o payload da página
- **AND** `GET /api/v1/status-pages/clientes` responde 404

**Segredo mascarado:** o hash da credencial da página MUST ser mascarado em todas as leituras da administração — no detalhe (na definição e no YAML), na validação e na pré-visualização —, como já é feito com os segredos dos endpoints. O detalhe MUST dizer o usuário e que a página exige login, sem o hash. O registro em memória que serve as rotas públicas continua com o hash verdadeiro.

#### Scenario: Leitura não devolve o hash
- **WHEN** o administrador abre o detalhe, valida ou pré-visualiza uma página que exige login
- **THEN** nenhuma das respostas contém o hash da senha
- **AND** o detalhe mostra o usuário e que a página exige login

### Requirement: Alteração com concorrência otimista
`GET /api/v1/admin/status-pages/:slug` MUST devolver `ETag` com a versão. `PUT`, `DELETE`, `POST .../enable` e `POST .../disable` MUST exigir `If-Match`: 428 sem ele e 412 com uma versão diferente da atual. Um `PUT` com `slug` diferente do da URL MUST responder 400. Uma alteração bem-sucedida MUST incrementar a versão e publicar a página com uma revisão nova, de modo que a próxima requisição pública não use a resposta anterior.

#### Scenario: Sem If-Match
- **WHEN** chega `PUT /api/v1/admin/status-pages/clientes` sem `If-Match`
- **THEN** a API responde 428

#### Scenario: Versão antiga
- **WHEN** a página está na versão 3 e chega `PUT` com `If-Match: "2"`
- **THEN** a API responde 412 e nada é gravado

#### Scenario: Troca de slug
- **WHEN** chega `PUT /api/v1/admin/status-pages/clientes` com `slug: parceiros` no corpo
- **THEN** a API responde 400

#### Scenario: Desabilitar
- **WHEN** um administrador desabilita a página `clientes` com a versão atual
- **THEN** a API responde 200 com a versão incrementada
- **AND** `GET /api/v1/status-pages/clientes` responde 404 imediatamente

### Requirement: Remoção
`DELETE /api/v1/admin/status-pages/:slug` MUST remover a definição gerenciada e despublicar a página. Remover uma página gerenciada em conflito MUST apagar apenas a definição gerenciada, mantendo a página do YAML.

#### Scenario: Remoção
- **WHEN** um administrador remove a página `clientes` com a versão atual
- **THEN** a API responde 200
- **AND** `GET /api/v1/status-pages/clientes` responde 404

#### Scenario: Remoção de gerenciada em conflito
- **WHEN** a página gerenciada `infra` está em conflito com a página `infra` do YAML e um administrador a remove
- **THEN** a API responde 200
- **AND** `GET /api/v1/status-pages/infra` continua servindo a página do YAML

### Requirement: Páginas do YAML somente leitura
As páginas definidas no YAML MUST aparecer na listagem com origem `config` e MUST NOT ser alteradas, habilitadas, desabilitadas ou removidas pela API. Essas tentativas MUST responder 409.

#### Scenario: Alterar página do YAML
- **WHEN** um administrador envia `PUT /api/v1/admin/status-pages/infra` para a página `infra` do YAML
- **THEN** a API responde 409
- **AND** a página continua igual

### Requirement: Conflito de slug com o YAML
Na carga, uma página gerenciada com slug usado no YAML MUST ficar marcada como em conflito, citando a origem, e MUST NOT ser publicada. A página do YAML prevalece. Quando o YAML deixar de usar o slug, a página gerenciada MUST voltar a ser publicada na carga seguinte. Uma página gerenciada que não passar na validação durante a carga MUST ficar marcada com o erro e MUST NOT ser publicada.

#### Scenario: YAML passa a usar o slug
- **WHEN** existe a página gerenciada `clientes` e o YAML recarregado passa a definir `slug: clientes`
- **THEN** a API pública serve a página do YAML
- **AND** a listagem da administração mostra a gerenciada em conflito

#### Scenario: YAML deixa de usar o slug
- **WHEN** a página `clientes` é removida do YAML e a configuração é recarregada
- **THEN** a página gerenciada `clientes` volta a ser publicada, se estiver habilitada

### Requirement: Serialização com o ciclo de vida e publicação depois do commit
As escritas da administração de status pages MUST tentar o lock de ciclo de vida antes de validar ou gravar e MUST responder 503 se uma partida, recarga ou desligamento estiver em andamento. Um 503 MUST NOT deixar nada gravado. Com o lock e o mutex do serviço segurados, a escrita MUST gravar no banco primeiro e MUST publicar a mudança em memória só depois do commit. Se o commit falhar, nada MUST ser publicado.

#### Scenario: Escrita durante a recarga
- **WHEN** um administrador cria uma página enquanto a configuração é recarregada
- **THEN** a API responde 503
- **AND** a página não existe no banco depois da recarga

#### Scenario: Gravação falha
- **WHEN** um administrador habilita a página `clientes` e a gravação no banco falha (em teste, com um store falso injetado no serviço que devolve erro)
- **THEN** a API responde com erro
- **AND** `GET /api/v1/status-pages/clientes` responde 404 durante e depois da tentativa

### Requirement: Mapeamento de erros da administração de status pages
A API de administração de status pages MUST mapear os erros do serviço assim: storage sem suporte → 501; slug em uso ou página do YAML → 409; definição inválida, slug reservado ou troca de slug → 400; versão divergente → 412; `If-Match` ausente → 428; ciclo de vida em andamento → 503; página inexistente → 404; criação → 201. Erros inesperados MUST responder 500 sem o texto do erro.

#### Scenario: Storage sem suporte
- **WHEN** o storage não implementa as páginas gerenciadas e chega `POST /api/v1/admin/status-pages`
- **THEN** a API responde 501

### Requirement: Auditoria das escritas
Cada criação, alteração, habilitação, desabilitação e remoção de página MUST registrar no log o autor (usuário basic ou subject OIDC), o slug e a operação, sem o corpo da requisição.

#### Scenario: Log de criação
- **WHEN** o usuário basic `admin` cria a página `clientes`
- **THEN** o log contém uma linha com `admin`, `clientes` e a operação de criação

