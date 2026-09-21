## MODIFIED Requirements

### Requirement: Conteúdo e formato do backup
O backup MUST conter as definições completas, sem máscara, do que foi cadastrado pela web:
- os endpoints gerenciados: ativos e Push, habilitados ou não, inclusive em conflito com o arquivo de configuração;
- as status pages gerenciadas;
- as chaves globais de push criadas pela web, com nome, hash hexadecimal minúsculo, dica e, só como informação, data de criação e autor.

**O que fica de fora:** o backup MUST NOT conter histórico, alertas disparados, sessões de login nem cadastros do arquivo de configuração.

**Formato:** JSON com `format: "go-uptime-admin-backup"`, `version: 1`, `createdAt`, `createdBy`, `appVersion` opcional e as listas `endpoints`, `statusPages` e `pushKeys`, ordenadas.

**Leitura:** o sistema MUST recusar com 400 um arquivo com:
- `format` desconhecido ou versão maior que a suportada;
- chaves desconhecidas;
- listas ausentes;
- itens duplicados (chave, slug, nome ou hash).

**Limites:** o texto claro MUST ter no máximo 2 MiB, 1.000 endpoints, 200 status pages e 500 chaves. O backup MUST responder 422 quando os cadastros passam desses limites.

**Arquivos da v6:** a leitura MUST aceitar também `format: "gatus-admin-backup"` e o envelope `format: "gatus-admin-backup-encrypted"`, e o campo `gatusVersion` como sinônimo de `appVersion`, de modo que um backup feito antes da troca de nome seja restaurado. O cabeçalho de um arquivo cifrado MUST ser autenticado como foi lido, com o `format` do próprio arquivo. A escrita MUST usar sempre o formato novo.

#### Scenario: Backup com os três tipos
- **WHEN** a web tem os endpoints `jobs_backup` (Push) e `web_site` (desabilitado), a status page `jobs` e a chave `akamai`
- **THEN** o arquivo contém os dois endpoints com as definições completas (inclusive o token e `enabled: false`), a página `jobs` e a chave `akamai` só com hash e dica

#### Scenario: Cadastros do YAML ficam de fora
- **WHEN** o arquivo de configuração tem o endpoint `core_api` e a página `services`
- **THEN** o backup não contém `core_api` nem `services`

#### Scenario: Arquivo de outro formato
- **WHEN** o administrador envia um arquivo com `format: "other"`, `version: 2` ou uma chave desconhecida
- **THEN** a API responde 400 sem alterar nada

#### Scenario: Cadastros acima do limite
- **WHEN** a web tem 1.001 endpoints gerenciados
- **THEN** o backup responde 422

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
