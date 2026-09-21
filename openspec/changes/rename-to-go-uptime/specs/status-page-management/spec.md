## MODIFIED Requirements

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
