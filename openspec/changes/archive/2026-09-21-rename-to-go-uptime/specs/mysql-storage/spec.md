## MODIFIED Requirements

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
