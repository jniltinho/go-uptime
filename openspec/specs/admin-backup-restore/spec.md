# admin-backup-restore Specification

## Purpose
TBD - created by archiving change add-admin-backup-restore. Update Purpose after archive.
## Requirements
### Requirement: Conteúdo e formato do backup
The backup MUST contain the complete, unmasked definitions of what was registered from the web:
- the managed endpoints: active and Push, enabled or not, including those in conflict with the configuration file;
- the managed status pages;
- the global push keys created from the web, with name, lowercase hexadecimal hash, hint and, as information only, creation time and author;
- the managed alerting providers, including those shadowed by the file, with the complete, unmasked definition.

**What is left out:** the backup MUST NOT contain history, triggered alerts, login sessions nor anything registered in the configuration file.

**Format:** JSON with `format: "go-uptime-admin-backup"`, `version: 1`, `createdAt`, `createdBy`, an optional `appVersion` and the lists `endpoints`, `statusPages`, `pushKeys` and `alertProviders`, sorted. The `alertProviders` list MUST be optional when reading, so that a backup made before it stays valid, and MUST always be written.

**Reading:** the system MUST refuse with 400 a file with:
- an unknown `format` or a version greater than the supported one;
- unknown keys;
- missing lists (except `alertProviders`);
- duplicated items (key, slug, name, hash or provider type).

**Limits:** the plaintext MUST have at most 2 MiB, 1,000 endpoints, 200 status pages, 500 keys and one provider per alert type. The backup MUST answer 422 when the registered items exceed these limits.

**Files of v6:** reading MUST also accept `format: "gatus-admin-backup"` and the envelope `format: "gatus-admin-backup-encrypted"`, and the `gatusVersion` field as a synonym of `appVersion`, so that a backup made before the rename is restored. The header of an encrypted file MUST be authenticated as it was read, with the file's own `format`. Writing MUST always use the new format.

#### Scenario: Backup com os três tipos
- **WHEN** the web has the endpoints `jobs_backup` (Push) and `web_site` (disabled), the status page `jobs` and the key `akamai`
- **THEN** the file contains both endpoints with their complete definitions (including the token and `enabled: false`), the page `jobs` and the key `akamai` with only its hash and hint

#### Scenario: Cadastros do YAML ficam de fora
- **WHEN** the configuration file has the endpoint `core_api` and the page `services`
- **THEN** the backup contains neither `core_api` nor `services`

#### Scenario: Arquivo de outro formato
- **WHEN** the administrator submits a file with `format: "other"`, `version: 2` or an unknown key
- **THEN** the API answers 400 without changing anything

#### Scenario: Cadastros acima do limite
- **WHEN** the web has 1,001 managed endpoints
- **THEN** the backup answers 422

#### Scenario: Backup with an alerting provider
- **WHEN** the web has the `telegram` provider
- **THEN** the file contains `telegram` with its token in plaintext, and the encrypted file protects it like the tokens of the endpoints

#### Scenario: Backup made before the providers
- **WHEN** the administrator restores a backup without the `alertProviders` list
- **THEN** the file is accepted and no provider changes

### Requirement: Cifragem opcional do backup
Ao gerar o backup, o administrador MUST poder escolher cifrá-lo com uma senha de 12 a 1.024 bytes UTF-8.

**Envelope:**
- o arquivo cifrado MUST ser um envelope JSON `format: "go-uptime-admin-backup-encrypted"`, `version: 1`;
- MUST usar Argon2id com `time: 2`, `memoryKiB: 19456` e `threads: 1`, salt aleatório de 16 bytes, AES-256-GCM com nonce aleatório de 12 bytes e o cabeçalho canônico como dado autenticado;
- o leitor MUST recusar outros parâmetros, tamanhos de salt ou nonce, nomes de algoritmo e campos desconhecidos.

**Senha:**
- senha errada e arquivo alterado MUST responder o mesmo 400, "invalid password or corrupted file";
- arquivo cifrado sem senha e senha com arquivo sem cifra MUST responder 400;
- a senha MUST NOT ser guardada nem registrada no log.

**Proteção contra abuso:**
- no máximo 2 derivações de chave MUST rodar ao mesmo tempo, com 429 e `Retry-After` quando esperar mais de 5 segundos;
- as senhas erradas MUST contar num limitador próprio por IP do cliente (calculado com `status-pages.trusted-proxies`), separado do limitador do login, que responde 429 depois de 10 falhas em 15 minutos, sem mudar a janela do login;
- backup e restore MUST compartilhar o limite de derivações, e nenhuma derivação MUST acontecer enquanto bloqueia uma recarga da configuração.

#### Scenario: Backup cifrado
- **WHEN** o administrador baixa o backup com a senha `correct horse battery`
- **THEN** o arquivo é o envelope cifrado e não contém em texto claro nenhuma definição nem token

#### Scenario: Senha errada
- **WHEN** o administrador pede a prévia de um arquivo cifrado com a senha errada
- **THEN** a API responde 400 com "invalid password or corrupted file"

#### Scenario: Parâmetros forjados
- **WHEN** o envelope declara `memoryKiB: 262144`
- **THEN** a API responde 400 sem derivar a chave

#### Scenario: Tentativas em massa
- **WHEN** o mesmo IP envia 11 senhas erradas em 15 minutos
- **THEN** a 11ª tentativa responde 429 e o login do administrador continua funcionando

### Requirement: Rota de backup
`POST /api/v1/admin/backup` MUST:
- exigir administrador e as proteções contra CSRF, com `Content-Type: application/json`;
- aceitar `{}` ou `{"password": "..."}`;
- responder 503 durante uma recarga ou com um dos registros indisponível, inclusive quando a lista dos endpoints gerenciados não pôde ser carregada;
- responder o arquivo com `Content-Disposition: attachment`, nome `go-uptime-backup-<AAAAMMDD-HHMMSS>.json` (ou `.enc.json` cifrado) e `Cache-Control: no-store`.

A operação MUST ser registrada no log com o autor, as quantidades e se houve cifragem, sem o conteúdo.

#### Scenario: Sem autenticação
- **WHEN** uma requisição sem credenciais chega a `POST /api/v1/admin/backup`
- **THEN** a API responde 401

#### Scenario: Outro site
- **WHEN** a requisição chega com `Sec-Fetch-Site: cross-site`
- **THEN** a API responde 403 e nenhum arquivo é gerado

### Requirement: Prévia do restore
`POST /api/v1/admin/restore/preview` MUST aceitar só `application/json` com corpo de até 3,5 MiB, no formato `{"file", "password", "overwrite", "disableEndpoints"}`. O limite de 256 KB das outras rotas da administração MUST NOT valer para as rotas de restore.

**Resposta e efeitos:**
- MUST devolver o plano (resumo, avisos, fingerprint e itens com ação, motivo e avisos);
- MUST NOT alterar o store, o monitoramento nem as publicações;
- MUST responder 503 quando um dos registros estiver indisponível.

**Simulação:** o plano MUST prever a aplicação simulando o estado acumulado, na ordem chaves de push, endpoints e status pages.

**Regras dos endpoints e das status pages:**
- `create` para item inexistente na web;
- item existente com definição normalizada igual é `unchanged`; diferente é `update` com `overwrite` e `skip` ("already exists") sem ele;
- `skip` com motivo para:
  - chave ou slug usado pelo arquivo de configuração;
  - definição recusada pelas validações da web, considerando os tokens e as chaves planejados;
  - `key` diferente da chave calculada;
  - troca de tipo de endpoint;
  - segredo mascarado em qualquer lugar mascarado pela API (headers, senha ou query da URL, segredos de client ou SSH, tokens e folhas de `provider-override`), **exceto** o hash da credencial de uma status page que já existe no destino: nesse caso o hash do destino MUST ser mantido e o item MUST seguir como `update` ou `unchanged` pelo restante da definição. A mesclagem MUST acontecer **antes** da validação da definição, senão o valor mascarado é recusado como hash inválido. Uma status page nova com o hash mascarado MUST ser `skip`, porque não há credencial para manter;
  - endpoint Push sem token;
- nome ou grupo diferentes com a mesma chave MUST ser `update` com o motivo "name or group changes";
- com `disableEndpoints`, os endpoints criados ou atualizados MUST ficar desabilitados.

**Regras das chaves de push:**
- mesmo nome e mesmo hash de uma chave existente é `unchanged`;
- MUST ser `skip`:
  - formato inválido;
  - nome em uso por uma chave do YAML ou por outra chave;
  - hash em uso por outra chave;
  - hash igual ao token de qualquer endpoint: do arquivo de configuração, de qualquer endpoint gerenciado (inclusive em conflito ou inválido) ou de qualquer endpoint do backup.

**Avisos:**
- a prévia MUST informar quantos endpoints habilitados começarão a ser monitorados e quantos têm alertas;
- as status pages MUST trazer os avisos de seleção calculados com os endpoints existentes e os planejados.

#### Scenario: Prévia sem efeitos
- **WHEN** o administrador pede a prévia de um arquivo com um endpoint novo
- **THEN** a resposta mostra o endpoint como `create` e ele não é criado nem monitorado

#### Scenario: Existente sem sobrescrever
- **WHEN** o arquivo tem `web_site` com outro intervalo e `overwrite` é falso
- **THEN** o item aparece como `skip` com "already exists"

#### Scenario: Página em conflito com o YAML
- **WHEN** a página gerenciada `services` existe e o arquivo de configuração passou a definir `services`
- **THEN** o item do arquivo aparece como `skip` com o motivo do conflito

#### Scenario: Token repetido dentro do arquivo
- **WHEN** o arquivo tem dois endpoints Push com o mesmo token
- **THEN** o primeiro aparece como `create` e o segundo como `skip` com o motivo do token repetido

#### Scenario: Chave de push igual a token de endpoint
- **WHEN** o hash de uma chave do arquivo é o SHA-256 do token do endpoint `jobs_backup`
- **THEN** a chave aparece como `skip` com "token hash in use"

#### Scenario: Segredo mascarado
- **WHEN** um endpoint do arquivo tem `headers.Authorization: "********"`, a URL `https://user:********@api.example.org` ou `provider-override.webhook-url: "********"`
- **THEN** o item aparece como `skip` com "masked secret"

#### Scenario: Chave com token de endpoint em conflito
- **WHEN** o hash de uma chave do arquivo é o SHA-256 do token de um endpoint gerenciado em conflito com o arquivo de configuração
- **THEN** a chave aparece como `skip` com "token hash in use"

#### Scenario: Restaurar desabilitado
- **WHEN** a prévia e a aplicação usam `disableEndpoints` com um endpoint novo
- **THEN** a prévia mostra a definição com `enabled: false` e o endpoint é criado sem começar a ser monitorado

#### Scenario: Troca de tipo
- **WHEN** o arquivo tem `jobs_backup` como endpoint ativo e ele existe como Push, com `overwrite`
- **THEN** o item aparece como `skip` com "type cannot change"

#### Scenario: Chave de push repetida
- **WHEN** o arquivo tem a chave `akamai` com o mesmo hash da chave `akamai` existente
- **THEN** o item aparece como `unchanged`

#### Scenario: Credencial mascarada de uma página existente
- **WHEN** o arquivo restaurado traz a página `clientes`, que já existe no destino exigindo login, com o hash mascarado
- **THEN** a prévia mostra `update` ou `unchanged` pelo restante da definição
- **AND** a credencial do destino é mantida

#### Scenario: Credencial mascarada de uma página nova
- **WHEN** o mesmo arquivo traz a página `parceiros`, que não existe no destino, com o hash mascarado
- **THEN** a prévia mostra `skip` com o motivo do segredo mascarado

### Requirement: Aplicação do restore
`POST /api/v1/admin/restore` MUST receive the body of the preview plus the `fingerprint`.

**Validation:**
- it MUST compute the plan again and answer 409 without applying anything when the fingerprint differs;
- the fingerprint MUST depend on the content of the file, the options, the configuration generation and the current versions and definitions of the items, **and of every managed alerting provider**, even those absent from the backup, because the endpoints of the plan are validated against them;
- the plan MUST validate the endpoints against a simulated alerting configuration: the file's providers, the current managed ones, and the providers the plan will create or update, respecting the overwrite option and the precedence of the file. The preview MUST list, for each provider it updates, the managed endpoints outside the backup that will be restarted.

**Applying:**
- alerting providers MUST be applied after the push keys and before the endpoints, so that a restored endpoint finds the provider it uses; a provider whose type is configured in the destination's file MUST be `skipped` with the reason;
- it MUST apply the `create` and `update` items in the order of the plan, using the same normalized definition shown in the preview (including with `disableEndpoints`), with the rules and effects of creating and updating from the web (monitoring, publishing after the commit, audit) and the author of the restore, without generating tokens;
- it MUST NOT delete items nor change keys or slugs;
- the hash check of a restored key MUST be serialized with the writes of managed endpoints.

**Result:**
- the response MUST carry the result of each item (`created`, `updated`, `unchanged`, `skipped` or `failed`, with the message) and the selection warnings of the applied pages;
- the failure of one item MUST NOT interrupt the others;
- an item changed between the check and the application MUST be `failed`;
- when a configuration reload starts during the application, the remaining items MUST be `skipped` with "configuration reload in progress".

**After applying:**
- the status cache of the endpoints MUST be cleared;
- the log MUST record the summary with the author;
- a restored push key MUST accept pushes with the original token.

#### Scenario: Restore em outra instalação
- **WHEN** the backup of an installation with SQLite is restored on a new installation with MariaDB
- **THEN** the endpoints are monitored, the page is published and the scripts keep sending pushes with the original global key

#### Scenario: Mudança depois da prévia
- **WHEN** another administrator changes `web_site` between the preview and the application
- **THEN** the application answers 409 and nothing is applied

#### Scenario: Outro arquivo com os mesmos itens
- **WHEN** the application sends a file with the same ids as the preview and different definitions
- **THEN** the application answers 409

#### Scenario: Restore repetido
- **WHEN** the same file is restored twice
- **THEN** the second time every item applied before shows as `unchanged`

#### Scenario: Recarga no meio
- **WHEN** a configuration reload starts after the third item
- **THEN** the following items are `skipped` with "configuration reload in progress" and a new restore after the reload applies what was left

#### Scenario: Endpoint and provider in the same restore
- **WHEN** the backup has the `ntfy` provider and the endpoint `core_api` with an `ntfy` alert, and the destination has no `ntfy`
- **THEN** the provider is created first and `core_api` is created with its alert

### Requirement: Aba Backup da administração
A administração MUST ter a aba **Backup** em `/admin/backup`, depois de "Push keys", servida também ao abrir a URL diretamente.

**Layout:**
- em telas médias e grandes, a aba MUST ocupar a altura da janela como as listas da administração, sem rolagem da página e sem cortar conteúdo;
- Download e Restore MUST ficar em dois cartões lado a lado, cada um com cabeçalho, corpo com rolagem própria e rodapé com os botões, e os rodapés MUST continuar visíveis em janelas baixas;
- no celular, a página MUST poder rolar.

**Download:**
- MUST mostrar as quantidades;
- MUST oferecer a cifragem com senha e confirmação, desligada por padrão, validando o tamanho em bytes;
- sem senha, MUST avisar no cartão que o arquivo contém segredos em texto claro.

**Restore:**
- **Arquivo:** MUST aceitar qualquer arquivo, detectando o formato pelo conteúdo, e pedir a senha quando ele estiver cifrado. Um arquivo recusado MUST deixar a explicação visível junto do campo enquanto ele estiver selecionado;
- **Opções:** MUST ter "Overwrite existing items" e "Restore endpoints as disabled";
- **Prévia:** MUST abrir num diálogo com os avisos, o resumo, o filtro por ação e a tabela, e com Cancel e Restore no rodapé. Fechar o diálogo por Cancel, pelo botão de fechar ou por Esc MUST descartar a prévia;
- **Prévia em andamento:** trocar ou reler o arquivo, a senha ou as opções enquanto a prévia é calculada MUST descartar a resposta, sem abrir o diálogo;
- **Confirmação:** Restore MUST pedir confirmação com as quantidades de criações e atualizações do plano, e durante a aplicação a prévia MUST NOT poder ser fechada;
- **Resultado:** depois de aplicar, MUST abrir um diálogo com o resultado por item;
- **Senha errada:** além da mensagem, o campo de senha MUST ficar marcado como inválido, com o erro associado a ele, até ser editado.

**Mensagens:**
- o download concluído, o resumo do restore e os erros de download, das quantidades, da leitura do arquivo, da prévia e do restore MUST aparecer em toasts legíveis;
- os erros com instrução (409, 413, 422 e 429) MUST ficar até serem dispensados;
- os avisos de texto claro, de senha e de monitoramento MUST continuar no cartão ou no diálogo.

#### Scenario: Download cifrado pela tela
- **WHEN** o administrador marca "Encrypt with a password", digita a senha duas vezes e clica em Download
- **THEN** o navegador baixa `go-uptime-backup-<data>.enc.json`
- **AND** um toast de sucesso informa o nome do arquivo

#### Scenario: Restore pela tela
- **WHEN** o administrador escolhe um backup, clica em Preview, confere a tabela no diálogo e confirma
- **THEN** o diálogo de resultados mostra o resultado de cada item, um toast resume o restore e as listas mostram os itens restaurados

#### Scenario: Sem rolagem nem corte
- **WHEN** o administrador abre a aba Backup em janelas de 1280×900, 1280×720 ou 1024×600, com a cifragem ligada, com um arquivo cifrado selecionado ou com a prévia aberta
- **THEN** a página não rola, não há rolagem horizontal e os botões Download, Preview e os do rodapé do diálogo ficam dentro da janela

#### Scenario: Prévia fechada
- **WHEN** o administrador fecha a prévia com Esc
- **THEN** a prévia é descartada e um restore exige uma nova prévia

#### Scenario: Opção alterada durante a prévia
- **WHEN** o administrador clica em Preview e marca "Overwrite existing items" antes da resposta
- **THEN** o diálogo da prévia não abre

#### Scenario: Senha errada na tela
- **WHEN** a prévia de um backup cifrado é pedida com a senha errada
- **THEN** um toast de erro mostra "Invalid password or corrupted file.", nenhum diálogo abre e o campo de senha fica marcado como inválido com essa mensagem associada

#### Scenario: Arquivo que não é backup
- **WHEN** o administrador escolhe um arquivo JSON que não é backup
- **THEN** um toast de erro aparece e o status do arquivo continua explicando que ele não é um backup

#### Scenario: Backup cifrado no limite
- **WHEN** o administrador escolhe o envelope cifrado de um backup de 2 MiB
- **THEN** a tela aceita o arquivo e pede a senha

#### Scenario: URL aberta diretamente
- **WHEN** o administrador recarrega `/admin/backup`
- **THEN** a aba Backup é mostrada

