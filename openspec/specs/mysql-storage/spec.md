# mysql-storage Specification

## Purpose
TBD - created by archiving change add-mysql-storage. Update Purpose after archive.
## Requirements
### Requirement: Tipo de storage mysql
A configuração MUST aceitar `storage.type: mysql` para MySQL 8.4+ e MariaDB 10.11+, com a DSN do `go-sql-driver/mysql` em `storage.path`. `storage.path` MUST ser obrigatório com `mysql`, e uma DSN que o driver não consegue interpretar MUST falhar na validação da configuração, antes de qualquer conexão. O texto do erro e os logs MUST NOT conter a senha da DSN. `storage.caching`, `maximum-number-of-results` e `maximum-number-of-events` MUST valer para `mysql` como para `postgres`. Na inicialização, um servidor com versão anterior às mínimas MUST gerar aviso no log, sem impedir o início, e um servidor com `innodb_page_size` menor que 16K MUST impedir a inicialização com erro claro.

#### Scenario: DSN válida
- **WHEN** a configuração tem `storage.type: mysql` e `storage.path: "go_uptime:segredo@tcp(mariadb:3306)/go_uptime"`
- **THEN** a configuração é válida e o Go Uptime inicia com o storage MySQL

#### Scenario: Caminho ausente
- **WHEN** a configuração tem `storage.type: mysql` sem `storage.path`
- **THEN** a validação falha com erro que indica que `storage.path` é obrigatório

#### Scenario: DSN inválida sem vazar a senha
- **WHEN** `storage.path` é `go_uptime:segredo@tcp(mariadb:3306` (DSN malformada)
- **THEN** a validação falha
- **AND** nem o erro nem o log contêm `segredo`

#### Scenario: Servidor abaixo da versão mínima
- **WHEN** o Go Uptime inicia com `mysql` conectado a um MySQL 8.0
- **THEN** o log registra aviso de que a versão mínima suportada é a 8.4
- **AND** o storage inicia normalmente

### Requirement: Parâmetros fixos de conexão
O storage MySQL MUST sobrepor, a qualquer valor vindo na DSN: leitura de `DATETIME` como `time.Time` com `parseTime` ligado e localização UTC, fuso da sessão `+00:00`, `utf8mb4` com collation `utf8mb4_bin`, contagem de linhas encontradas em `RowsAffected`, `sql_mode` igual a `ANSI_QUOTES,ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION` e `innodb_lock_wait_timeout` de 10 segundos. Os demais parâmetros da DSN (TLS, timeouts, nome do banco) MUST ser preservados. O pool MUST limitar as conexões abertas a 25 e reciclar conexões com no máximo 3 minutos de vida, e uma conexão encerrada pelo servidor MUST ser descartada sem erro para as operações seguintes.

#### Scenario: DSN com fuso e parseTime do usuário
- **WHEN** `storage.path` tem `loc=America%2FSao_Paulo&parseTime=false` e um resultado é gravado com horário 12:00:00Z
- **THEN** a leitura devolve 12:00:00 UTC
- **AND** uma conexão do pool informa `@@session.time_zone` igual a `+00:00` e o `sql_mode` fixo

#### Scenario: Atualização sem mudança de valores
- **WHEN** uma escrita otimista atualiza um endpoint gerenciado com os mesmos valores e a versão esperada
- **THEN** a linha conta como afetada e a operação não é tratada como conflito de versão

#### Scenario: Conexão derrubada pelo servidor
- **WHEN** as conexões do pool são encerradas no servidor com `KILL` e o watchdog grava um novo resultado
- **THEN** a gravação conclui com sucesso usando uma conexão nova

### Requirement: Esquema automático
Na inicialização, o storage MySQL MUST criar as tabelas do upstream e as do fork (`managed_endpoints` e `managed_status_pages`) que não existirem, em InnoDB e `utf8mb4`, e MUST ser idempotente: iniciar ou recarregar a configuração sobre um esquema existente MUST NOT falhar nem apagar dados. As chaves estrangeiras MUST ser efetivas, com `ON DELETE CASCADE`, como no PostgreSQL. Colunas de texto usadas em chave única MUST usar collation binária, para `API` e `api` serem valores diferentes, e textos livres MUST aceitar ao menos o mesmo tamanho que o PostgreSQL aceita na prática (16 MB). Uma falha ao criar tabela MUST impedir a inicialização com erro.

#### Scenario: Primeira inicialização
- **WHEN** o Go Uptime inicia com `mysql` num banco vazio
- **THEN** todas as tabelas existem ao final e o monitoramento grava resultados

#### Scenario: Reinício
- **WHEN** o Go Uptime reinicia sobre um banco MySQL já inicializado e com resultados
- **THEN** a inicialização termina sem erro e os resultados anteriores continuam disponíveis

#### Scenario: Remoção em cascata
- **WHEN** um endpoint com resultados, condições, eventos, uptimes e alertas disparados é removido do store
- **THEN** nenhuma linha dessas tabelas continua referenciando o endpoint removido

### Requirement: Paridade de comportamento com o PostgreSQL
Com `mysql`, as operações do store MUST produzir os mesmos resultados observáveis que com `postgres`: inserção e leitura paginada de resultados e eventos, uptime por hora e por dia com consolidação, médias de tempo de resposta, remoção dos resultados e eventos acima dos limites, alertas disparados (inserção, atualização e remoção), suites e seus resultados, remoção de endpoints fora da configuração e `Clear`. A remoção dos excedentes MUST manter exatamente os mais recentes dentro do limite. Um horário zero gravado MUST ser lido de volta como horário zero.

#### Scenario: Limite de resultados
- **WHEN** `maximum-number-of-results` é 100 e um endpoint recebe 150 resultados
- **THEN** o store guarda exatamente os 100 mais recentes

#### Scenario: Horário zero
- **WHEN** um valor `time.Time{}` é gravado numa coluna de horário com `mysql`
- **THEN** a leitura devolve `time.Time{}`, como em SQLite e PostgreSQL

#### Scenario: Alerta disparado regravado
- **WHEN** o mesmo alerta de um endpoint é gravado duas vezes com contadores diferentes
- **THEN** existe uma única linha com os contadores da segunda gravação

#### Scenario: Suíte de conformidade
- **WHEN** a suíte de conformidade do store roda com SQLite, PostgreSQL, MySQL e MariaDB disponíveis
- **THEN** os mesmos cenários passam nos quatro bancos

### Requirement: Transações com erro e retry
Com `mysql`, qualquer comando que falhe dentro de uma transação MUST invalidar a transação: os comandos seguintes MUST falhar sem executar e o `Commit` MUST desfazer tudo e devolver o erro, como no PostgreSQL. As conexões MUST usar o isolamento `READ COMMITTED`, como o padrão do PostgreSQL. A gravação de um resultado de endpoint e a de um resultado de suite MUST repetir a operação inteira, até completar 3 tentativas, quando falharem por deadlock, espera de lock esgotada ou conflito no `COMMIT`, e MUST NOT repetir outros erros.

#### Scenario: Falha no meio da gravação
- **WHEN** um comando da gravação de um resultado falha depois de o resultado ter sido inserido na mesma transação
- **THEN** o `Commit` devolve erro
- **AND** nem o resultado nem o uptime daquela gravação ficam gravados

#### Scenario: Deadlock com retry
- **WHEN** a gravação de um resultado sofre um deadlock forçado na primeira tentativa
- **THEN** a segunda tentativa grava o resultado, os eventos e o uptime completos, uma única vez

### Requirement: Limite de tamanho das chaves com mysql
Com `storage.type: mysql`, a validação da configuração e a validação da administração MUST rejeitar endpoints, external-endpoints, suites e endpoints de suite cuja chave passe de 768 caracteres (contados em caracteres, não em bytes), com mensagem que cita o limite do MySQL, em vez de deixar a gravação falhar em execução. Com `sqlite`, `postgres` e `memory`, o limite MUST NOT ser aplicado.

#### Scenario: Chave longa na configuração
- **WHEN** `storage.type` é `mysql` e um endpoint tem grupo e nome que geram uma chave de 800 caracteres
- **THEN** a validação falha citando o limite de 768 caracteres

#### Scenario: Chave com caracteres multibyte
- **WHEN** `storage.type` é `mysql` e a chave tem 700 caracteres acentuados (mais de 768 bytes)
- **THEN** a validação aceita a chave

#### Scenario: Mesma chave com postgres
- **WHEN** a configuração com a chave de 800 caracteres usa `storage.type: postgres`
- **THEN** a validação não aplica esse limite

### Requirement: Administração e status pages com mysql
`admin.enabled: true` MUST ser aceito com `storage.type: mysql`, e o aviso de várias instâncias compartilhando o banco MUST aparecer como com `postgres`. Endpoints gerenciados e status pages gerenciadas MUST manter com `mysql` os mesmos contratos de SQLite e PostgreSQL: chave ou slug duplicado MUST responder 409, versão desatualizada MUST responder 412, e a gravação MUST acontecer antes da publicação em memória. As leituras em lote das status pages (uptime e resumos) MUST devolver os mesmos valores que `GetUptimeByKey` e `GetEndpointStatusByKey` no mesmo banco.

#### Scenario: Chave duplicada
- **WHEN** a administração cria com `mysql` um endpoint cuja chave já existe entre os gerenciados
- **THEN** a API responde 409

#### Scenario: Página pública com mysql
- **WHEN** uma status page publicada seleciona endpoints com resultados gravados em MySQL
- **THEN** `GET /api/v1/status-pages/<slug>` responde 200 com os mesmos uptimes que `GetUptimeByKey` calcula no mesmo banco

### Requirement: Tradução de placeholders
O driver do storage MySQL MUST traduzir os placeholders `$N` das consultas para `?`, repetindo e reordenando os argumentos conforme a ordem de aparição, inclusive em statements preparados, e MUST ignorar `$` dentro de literais de texto, identificadores entre aspas e comentários. Uma consulta que referencia um argumento inexistente MUST falhar com erro, sem executar.

#### Scenario: Placeholder repetido
- **WHEN** a consulta `SELECT ... WHERE a >= $1 AND b >= $1 AND c = $2` é executada com os argumentos `(10, "x")`
- **THEN** o servidor recebe `SELECT ... WHERE a >= ? AND b >= ? AND c = ?` com `(10, 10, "x")`

#### Scenario: Statement preparado
- **WHEN** a consulta com `$1` repetido é preparada e executada duas vezes com argumentos diferentes
- **THEN** as duas execuções usam os argumentos corretos, sem erro de número de argumentos

#### Scenario: Cifrão em literal
- **WHEN** a consulta contém o literal `'custa $1'` e o placeholder `$1`
- **THEN** só o placeholder fora do literal é traduzido

### Requirement: Testes em MySQL e MariaDB
A suíte de conformidade do store SQL e os testes dos helpers do fork (endpoints gerenciados, status pages gerenciadas e leituras em lote) MUST rodar em MySQL quando `GO_UPTIME_TEST_MYSQL_URL` estiver definido e em MariaDB quando `GO_UPTIME_TEST_MARIADB_URL` estiver definido, cada teste num banco próprio criado e removido pelo helper. O CI MUST executar esses testes nas versões mínimas (MySQL 8.4 e MariaDB 10.11) e nas LTS mais novas. Sem as variáveis, os testes MUST continuar passando só com SQLite (e PostgreSQL, se configurado).

#### Scenario: Execução local sem MySQL
- **WHEN** `go test ./...` roda sem `GO_UPTIME_TEST_MYSQL_URL` e sem `GO_UPTIME_TEST_MARIADB_URL`
- **THEN** os testes passam sem tentar conectar a MySQL ou MariaDB

#### Scenario: Pacotes em paralelo
- **WHEN** `go test ./...` roda com as duas variáveis e vários pacotes usam o banco ao mesmo tempo
- **THEN** cada teste usa um banco próprio e nenhum teste apaga dados de outro

#### Scenario: CI
- **WHEN** o workflow de CI roda num pull request
- **THEN** os testes executam contra MySQL e MariaDB nas versões mínimas e nas LTS mais novas e falham se algum cenário falhar

