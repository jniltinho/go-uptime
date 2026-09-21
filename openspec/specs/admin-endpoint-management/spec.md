# admin-endpoint-management Specification

## Purpose
TBD - created by archiving change add-admin-endpoint-management. Update Purpose after archive.
## Requirements
### Requirement: Persistência de endpoints gerenciados
O sistema MUST persistir cada endpoint criado pela administração web no storage configurado (SQLite ou PostgreSQL), guardando a definição enviada sem os valores padrão, uma versão inteira incrementada a cada alteração, a data de criação, a data da última alteração e o autor da última alteração. Os valores padrão MUST ser aplicados apenas em memória.

#### Scenario: Endpoint criado sobrevive ao reinício
- **WHEN** um administrador cria o endpoint `api` no grupo `core` e o Go Uptime é reiniciado
- **THEN** o endpoint `core_api` volta a ser monitorado
- **AND** os resultados registrados antes do reinício continuam disponíveis

#### Scenario: Metadados e versão
- **WHEN** o administrador `ops@exemplo.com` altera um endpoint gerenciado que estava na versão 3
- **THEN** o registro armazenado passa à versão 4, com a data da alteração e `ops@exemplo.com` como autor

#### Scenario: Padrões do provedor não ficam congelados
- **WHEN** um endpoint gerenciado tem um alerta `slack` sem `failure-threshold` e o `default-alert` do provedor `slack` no YAML muda de 3 para 5
- **THEN** após a recarga do YAML, o alerta do endpoint gerenciado usa `failure-threshold` 5

### Requirement: Carga independente da administração
Com storage `sqlite` ou `postgres`, o sistema MUST carregar os endpoints gerenciados na inicialização e a cada recarga da configuração, monitorar os válidos e preservar o histórico de todas as chaves gerenciadas armazenadas, independentemente do valor de `admin.enabled`.

#### Scenario: Administração desligada por recarga
- **WHEN** existem endpoints gerenciados e `admin.enabled` passa a `false` numa recarga do YAML
- **THEN** os endpoints gerenciados continuam monitorados com o histórico mantido
- **AND** as rotas de administração ficam indisponíveis

### Requirement: Formato e validação estrita
O sistema MUST aceitar a definição de um endpoint gerenciado em YAML ou JSON, com as mesmas chaves de um item de `endpoints` do arquivo de configuração, e MUST aplicar no mínimo as mesmas validações e valores padrão aplicados aos endpoints do YAML. Adicionalmente, MUST rejeitar com 400 chaves desconhecidas, alertas de tipo inexistente, alertas cujo provedor não esteja configurado e overrides de provedor inválidos.

#### Scenario: Definição mínima em JSON
- **WHEN** um administrador valida `{"name": "site", "url": "https://exemplo.com", "conditions": ["[STATUS] == 200"]}`
- **THEN** a definição efetiva devolvida tem intervalo de 1 minuto e método `GET`
- **AND** a definição armazenável não contém esses valores padrão

#### Scenario: Definição sem condições
- **WHEN** um administrador envia uma definição sem `conditions`
- **THEN** a API responde 400 com a mensagem do erro de validação
- **AND** nada é persistido nem monitorado

#### Scenario: Chave desconhecida
- **WHEN** um administrador envia uma definição com a chave `intervall`
- **THEN** a API responde 400 indicando a chave desconhecida

#### Scenario: Alerta sem provedor configurado
- **WHEN** a configuração não tem o provedor `slack` e um administrador cria um endpoint com um alerta do tipo `slack`
- **THEN** a API responde 400 informando que o provedor `slack` não está configurado

### Requirement: Restrições próprias de endpoints gerenciados
O sistema MUST rejeitar definições de endpoints gerenciados que:
- gerem, pela função de chave do Go Uptime, uma chave já usada por endpoint, external-endpoint, suite ou endpoint de suite do YAML, ou por outro endpoint gerenciado (409, com a origem da chave na mensagem);
- usem `client.identity-aware-proxy`, `client.tls.certificate-file`, `client.tls.private-key-file`, `store` ou `always-run` (400);
- referenciem em `client.tunnel` um túnel inexistente em `tunneling` (400);
- declarem em `extra-labels` nomes fora da lista de labels Prometheus registrada no ciclo atual (400).

O sistema MUST NOT expandir variáveis de ambiente (`$VAR`, `${VAR}`) nas definições de endpoints gerenciados.

#### Scenario: Colisão de chave com o YAML
- **WHEN** o nome e o grupo de um endpoint enviado geram a mesma chave de um endpoint definido no YAML
- **THEN** a API responde 409 informando que a chave já é usada no arquivo de configuração

#### Scenario: Credenciais do servidor bloqueadas
- **WHEN** um administrador envia uma definição com `client.identity-aware-proxy`
- **THEN** a API responde 400 informando que o campo não é permitido em endpoints gerenciados

#### Scenario: Túnel inexistente
- **WHEN** um administrador envia `client.tunnel: bastion` e `tunneling` não define `bastion`
- **THEN** a API responde 400 informando que o túnel não existe

#### Scenario: Label Prometheus nova
- **WHEN** as labels registradas são `environment` e um administrador envia `extra-labels` com `team`
- **THEN** a API responde 400 listando as labels permitidas

#### Scenario: Variável de ambiente não é expandida
- **WHEN** um administrador cria um endpoint com o header `Authorization: Bearer ${API_TOKEN}`
- **THEN** o valor usado nas verificações é literalmente `Bearer ${API_TOKEN}`

### Requirement: API de administração de endpoints
O sistema MUST expor as operações abaixo, respondendo em JSON com erros no formato `{"error": "<mensagem>"}` (exceto o 401 do middleware de autenticação, que mantém o formato atual):
- `GET /api/v1/admin/endpoints`: lista endpoints gerenciados e do YAML, com origem (`admin` ou `config`), chave, nome, grupo, tipo, URL com credenciais mascaradas, intervalo, estado habilitado, conflito, erro de validação, versão e metadados de alteração;
- `GET /api/v1/admin/endpoints/{key}`: definição armazenada e efetiva, em YAML e JSON, com `ETag` da versão;
- `POST /api/v1/admin/endpoints`: cria;
- `PUT /api/v1/admin/endpoints/{key}`: altera;
- `POST /api/v1/admin/endpoints/{key}/enable` e `POST /api/v1/admin/endpoints/{key}/disable`: habilitam e desabilitam;
- `DELETE /api/v1/admin/endpoints/{key}`: remove;
- `POST /api/v1/admin/endpoints/parse`: decodifica a definição e a devolve como documento, sem validar, sem ler dados armazenados e sem mascarar;
- `POST /api/v1/admin/endpoints/validate`: valida sem persistir;
- `POST /api/v1/admin/endpoints/test`: valida e executa uma verificação única;
- `GET /api/v1/admin/metadata`: tipos de alerta configurados, túneis disponíveis e labels Prometheus permitidas.

Operações que alteram um endpoint existente MUST exigir `If-Match` com a versão atual, respondendo 428 sem o header e 412 com versão diferente. Corpos acima de 256 KB MUST ser rejeitados com 413, exceto nas rotas de restore da capability `admin-backup-restore`, que aceitam até 3,5 MiB. Enquanto um ciclo de partida ou recarga estiver em andamento (incluindo a partida inicial dos endpoints), as escritas MUST responder 503 sem validar nem gravar nada.

#### Scenario: Listagem indica a origem
- **WHEN** o YAML define 2 endpoints e existe 1 endpoint gerenciado
- **THEN** a listagem devolve 3 itens, 2 com origem `config` e 1 com origem `admin`

#### Scenario: Chave inexistente
- **WHEN** um administrador consulta `GET /api/v1/admin/endpoints/nao_existe`
- **THEN** a API responde 404

#### Scenario: Validação não persiste
- **WHEN** um administrador envia uma definição válida para `validate`
- **THEN** a API responde 200 com as definições armazenável e efetiva
- **AND** o endpoint não é persistido nem monitorado

#### Scenario: Edição concorrente
- **WHEN** dois administradores leem `core_api` na versão 2 e o primeiro salva uma alteração
- **THEN** o `PUT` do segundo, com `If-Match` da versão 2, recebe 412
- **AND** a alteração do primeiro é mantida

#### Scenario: Escrita durante recarga
- **WHEN** um administrador envia uma criação enquanto o hot-reload está entre parar e iniciar o Go Uptime
- **THEN** a API responde 503 e nada é persistido

### Requirement: Segredos mascarados
Toda resposta que contenha definições de endpoints MUST substituir por `********`: valores de headers cujo nome contenha `authorization`, `cookie`, `token`, `secret`, `password` ou `key` (sem diferenciar maiúsculas), a senha do userinfo da URL, os valores de parâmetros de query da URL cujo nome contenha `token`, `secret`, `password`, `key` ou `authorization`, `client.oauth2.client-secret`, `ssh.password`, `ssh.private-key` e os valores de `alerts[].provider-override`. Em `PUT` e `validate`, um valor igual à máscara MUST manter o valor armazenado.

#### Scenario: Header sensível mascarado
- **WHEN** um endpoint gerenciado tem o header `Authorization: Bearer abc123` e um administrador o consulta
- **THEN** a resposta mostra `Authorization: ********`

#### Scenario: Token na query string
- **WHEN** um endpoint gerenciado tem a URL `https://api.exemplo.com/health?api_key=abc123` e um administrador o consulta
- **THEN** a resposta mostra a URL com `api_key=********`

#### Scenario: Máscara reenviada mantém o valor
- **WHEN** um administrador altera a URL de um endpoint reenviando `Authorization: ********`
- **THEN** o endpoint continua usando `Bearer abc123`

#### Scenario: Endpoint do YAML com segredo
- **WHEN** um endpoint do YAML tem `ssh.password` e um administrador o consulta
- **THEN** a resposta mostra `password: ********`

### Requirement: Endpoints do YAML somente leitura
O sistema MUST recusar, pela API de administração, a alteração, habilitação, desabilitação ou remoção de endpoints definidos no arquivo de configuração.

#### Scenario: Tentativa de remover endpoint do YAML
- **WHEN** um administrador envia `DELETE` para a chave de um endpoint definido apenas no YAML
- **THEN** a API responde 409 informando que o endpoint é definido no arquivo de configuração
- **AND** o endpoint continua monitorado sem alteração

### Requirement: Aplicação sem reinício e sem afetar outros endpoints
Criar, alterar, habilitar, desabilitar ou remover um endpoint gerenciado MUST valer sem reiniciar o processo nem o servidor HTTP, e MUST NOT reiniciar ou pausar o monitoramento de outros endpoints.

#### Scenario: Criação inicia o monitoramento
- **WHEN** um administrador cria um endpoint com intervalo de 1 minuto
- **THEN** o primeiro resultado desse endpoint é registrado em até 1 minuto
- **AND** nenhum outro endpoint é reiniciado

#### Scenario: Alteração afeta só o endpoint alterado
- **WHEN** um administrador altera a URL do endpoint `core_api`
- **THEN** as verificações seguintes de `core_api` usam a nova URL e o histórico é mantido
- **AND** nenhum outro endpoint é reiniciado

#### Scenario: Desabilitar
- **WHEN** um administrador desabilita um endpoint gerenciado
- **THEN** o endpoint deixa de ser verificado
- **AND** a definição e o histórico continuam armazenados e visíveis

### Requirement: Parada consistente
Depois que parar, reiniciar ou remover um endpoint for concluído, o sistema MUST NOT registrar resultados, eventos, métricas ou alertas de execuções anteriores desse endpoint. A parada MUST aguardar a verificação em andamento por até o timeout do client mais 5 segundos, e resultados de execuções que terminarem após o cancelamento MUST ser descartados.

#### Scenario: Remoção durante verificação lenta
- **WHEN** um administrador remove um endpoint cuja verificação em andamento leva 8 segundos
- **THEN** nenhum resultado, evento ou alerta desse endpoint é registrado depois da remoção

#### Scenario: Alteração durante verificação lenta
- **WHEN** um administrador altera um endpoint durante uma verificação em andamento
- **THEN** o resultado dessa verificação é descartado
- **AND** não há resultados duplicados nem alertas duplicados após a alteração

### Requirement: Estado de alerta preservado na alteração
Ao alterar um endpoint gerenciado, o sistema MUST restaurar no endpoint atualizado o estado dos alertas disparados (disparo, chave de resolução e contadores) cuja configuração não mudou, e MUST limpar o estado dos alertas cuja configuração mudou.

#### Scenario: Endpoint em incidente com URL alterada
- **WHEN** um endpoint gerenciado tem um alerta disparado e um administrador altera apenas a URL
- **THEN** nenhum novo disparo é enviado para o mesmo incidente
- **AND** quando o endpoint volta a ter sucesso, o resolve é enviado com a chave de resolução original

### Requirement: Remoção de endpoint gerenciado
Remover um endpoint gerenciado MUST parar seu monitoramento e apagar, numa única transação, sua definição, status, resultados, eventos e alertas disparados; MUST invalidar os caches de status dessa chave e apagar suas séries Prometheus. A remoção MUST NOT notificar os provedores de alerta, e a resposta MUST informar quantos alertas estavam disparados.

#### Scenario: Remoção apaga dados imediatamente
- **WHEN** um administrador remove o endpoint gerenciado `core_api`
- **THEN** `core_api` não aparece em `GET /api/v1/endpoints/statuses` logo após a resposta
- **AND** não restam resultados, eventos nem séries Prometheus de `core_api`

#### Scenario: Remoção com alerta disparado
- **WHEN** um administrador remove um endpoint gerenciado com 1 alerta disparado
- **THEN** a resposta informa 1 alerta disparado
- **AND** nenhuma notificação é enviada ao provedor

### Requirement: Teste antes de salvar
A operação `test` MUST executar uma única avaliação da definição informada e devolver sucesso, resultado de cada condição, duração e erros, sem persistir resultados, disparar alertas ou publicar métricas. O teste MUST usar timeout de no máximo 10 segundos, permitir no máximo 2 testes simultâneos (429 além disso) e truncar em 512 caracteres os valores resolvidos das condições.

#### Scenario: Teste com falha
- **WHEN** um administrador testa uma definição cuja URL responde 500 e cuja condição é `[STATUS] == 200`
- **THEN** a resposta indica falha e mostra a condição como não atendida
- **AND** nenhum alerta é enviado e nenhum resultado é armazenado

#### Scenario: Muitos testes simultâneos
- **WHEN** um terceiro teste é iniciado enquanto 2 testes estão em andamento
- **THEN** a API responde 429

### Requirement: Convivência com o YAML e recarga
Quando o YAML definir uma chave usada por um endpoint gerenciado, o endpoint do YAML MUST prevalecer, e o gerenciado MUST ficar sem monitoramento, marcado como em conflito, sem ser apagado. Remover um gerenciado em conflito MUST apagar apenas a definição, sem parar o monitoramento nem apagar dados da chave. Quando o YAML deixar de definir a chave, o gerenciado MUST voltar a ser monitorado, herdando o histórico da chave. Um gerenciado que se torne inválido após uma recarga MUST ficar sem monitoramento, marcado com o erro, preservando seu histórico.

#### Scenario: Conflito com o YAML
- **WHEN** o YAML passa a definir uma chave que já existe como endpoint gerenciado
- **THEN** a versão do YAML é monitorada
- **AND** a listagem mostra o gerenciado marcado como em conflito, com a definição ainda armazenada

#### Scenario: Remoção de gerenciado em conflito
- **WHEN** um administrador remove um gerenciado marcado como em conflito
- **THEN** apenas a definição gerenciada é apagada
- **AND** o endpoint do YAML continua monitorado com todo o histórico

#### Scenario: Gerenciado inválido após recarga
- **WHEN** um endpoint gerenciado usa um alerta do tipo `slack` e a recarga do YAML remove o provedor `slack`
- **THEN** esse endpoint deixa de ser monitorado e aparece na listagem com o erro
- **AND** seu histórico é mantido e os demais endpoints continuam monitorados

### Requirement: Configuração apenas com endpoints gerenciados
Com `admin.enabled: true`, o sistema MUST validar os pré-requisitos da administração e MUST iniciar mesmo que o arquivo de configuração não defina nenhum endpoint nem suite.

#### Scenario: YAML sem endpoints
- **WHEN** o arquivo de configuração tem apenas `storage`, `security` e `admin`
- **THEN** o Go Uptime inicia e serve o dashboard sem endpoints
- **AND** um administrador consegue criar o primeiro endpoint

### Requirement: Escritas serializadas com a partida
As escritas da administração MUST ser serializadas entre si, e MUST ser recusadas com 503, sem nenhuma gravação, enquanto a partida do monitoramento ou uma recarga estiver em andamento.

#### Scenario: Remoção durante a partida
- **WHEN** um administrador tenta remover um endpoint gerenciado enquanto o monitoramento inicial ainda está iniciando os endpoints
- **THEN** a API responde 503
- **AND** o endpoint continua armazenado e é iniciado normalmente

### Requirement: Gravação confirmada só após aplicar
Uma escrita da administração MUST ser confirmada no banco apenas depois de aplicada no monitoramento. Se a aplicação falhar, a API MUST responder 500 e o banco MUST permanecer como estava antes da requisição.

#### Scenario: Falha ao aplicar
- **WHEN** a gravação de uma alteração é feita, mas aplicá-la no monitoramento falha
- **THEN** a API responde 500
- **AND** a definição armazenada continua sendo a anterior

### Requirement: Renomeação de endpoint gerenciado
A alteração de um endpoint gerenciado MUST aceitar `name` e `group` diferentes dos armazenados.

Quando a chave derivada (`grupo_nome`) mudar, o sistema MUST, numa única transação, gravar a definição sob a chave nova e mover para ela o status, os resultados, os eventos, o uptime e os alertas disparados da chave antiga. Se a transação falhar, nada MUST mudar e o monitoramento anterior MUST continuar. Quando só o texto de `name` ou `group` mudar sem mudar a chave, o nome e o grupo exibidos MUST ser atualizados, mantendo o histórico.

A chave nova MUST ser rejeitada com 409, sem alterar nada, quando for usada por endpoint, endpoint externo, suite ou endpoint de suite do arquivo de configuração, por outro endpoint gerenciado, ou quando já tiver status armazenado. Com `storage.type: mysql`, uma chave nova acima de 768 caracteres MUST ser rejeitada com 400.

Depois da resposta, o sistema MUST NOT registrar resultados, eventos, métricas ou alertas sob a chave antiga; MUST apagar as séries Prometheus e invalidar os caches de status da chave antiga; e MUST preservar no endpoint renomeado o estado dos alertas disparados (disparo, chave de resolução e contadores) cuja configuração não mudou, limpando o dos alertas cuja configuração mudou.

Um endpoint gerenciado em conflito com o arquivo de configuração MUST poder ser renomeado movendo apenas a definição: o histórico da chave antiga MUST continuar com o endpoint do arquivo, e a chave nova MUST começar sem histórico.

#### Scenario: Troca de grupo mantém o histórico
- **WHEN** um administrador envia `PUT /api/v1/admin/endpoints/web_site` com `group: clientes` para um endpoint com 100 resultados armazenados
- **THEN** a API responde 200 com a chave `clientes_site`
- **AND** `GET /api/v1/endpoints/clientes_site/statuses` mostra os 100 resultados
- **AND** `web_site` não aparece em `GET /api/v1/endpoints/statuses`

#### Scenario: Histórico preservado depois de reiniciar
- **WHEN** o Go Uptime reinicia depois da renomeação de `web_site` para `clientes_site`
- **THEN** `clientes_site` continua com o histórico anterior à renomeação
- **AND** nenhum dado é registrado sob `web_site`

#### Scenario: Troca de nome sem mudar a chave
- **WHEN** um administrador altera o nome de `My API` para `my-api`, que geram a mesma chave
- **THEN** a API responde 200 com a mesma chave
- **AND** o dashboard mostra o nome `my-api` com o histórico anterior

#### Scenario: Chave nova usada pelo arquivo de configuração
- **WHEN** um administrador renomeia `web_site` para `core_health`, definido no arquivo de configuração
- **THEN** a API responde 409
- **AND** `web_site` continua monitorado, com a definição e o histórico inalterados

#### Scenario: Chave nova com status armazenado
- **WHEN** um administrador renomeia `web_site` para `web_antigo`, que ainda tem resultados armazenados sem estar em uso
- **THEN** a API responde 409
- **AND** nenhum resultado de `web_site` ou de `web_antigo` é movido ou apagado

#### Scenario: Versão desatualizada na renomeação
- **WHEN** um administrador renomeia `web_site` enviando uma versão desatualizada em `If-Match`
- **THEN** a API responde 412
- **AND** `web_site` continua monitorado com a mesma chave e o mesmo histórico

#### Scenario: Renomeação durante verificação lenta
- **WHEN** um administrador renomeia um endpoint durante uma verificação em andamento
- **THEN** o resultado dessa verificação é descartado
- **AND** nenhum resultado, evento, métrica ou alerta é registrado sob a chave antiga depois da resposta

#### Scenario: Alerta disparado na renomeação
- **WHEN** um endpoint gerenciado tem um alerta disparado e um administrador troca apenas o grupo
- **THEN** nenhum novo disparo é enviado para o mesmo incidente
- **AND** quando o endpoint volta a ter sucesso, o resolve é enviado com a chave de resolução original

#### Scenario: Renomeação de gerenciado em conflito
- **WHEN** um administrador renomeia para `web_novo` um endpoint gerenciado marcado como em conflito com `web_site` do arquivo de configuração
- **THEN** `web_novo` passa a ser monitorado sem histórico
- **AND** `web_site` do arquivo continua monitorado com todo o histórico

### Requirement: Status pages na renomeação
Ao renomear a chave de um endpoint gerenciado, o sistema MUST trocar a chave antiga pela nova em `endpoints` e `featured` das status pages gerenciadas pela administração, na mesma transação da renomeação, incrementando a versão de cada página alterada e registrando o autor. O sistema MUST NOT alterar status pages do arquivo de configuração, e a resposta MUST listar as status pages do arquivo que selecionam a chave antiga por `endpoints` ou `featured`. Status pages que selecionam por grupo MUST seguir a regra de grupo com o grupo novo.

#### Scenario: Status page gerenciada selecionando pela chave
- **WHEN** a status page gerenciada `team` tem `web_site` em `featured` e um administrador renomeia `web_site` para `clientes_site`
- **THEN** `team` passa a ter `clientes_site` em `featured`, com a versão incrementada
- **AND** a página pública `team` continua mostrando o endpoint em destaque

#### Scenario: Status page do arquivo selecionando pela chave
- **WHEN** a status page `services` do arquivo de configuração tem `web_site` em `endpoints` e um administrador renomeia `web_site` para `clientes_site`
- **THEN** a resposta lista `services` entre as páginas do arquivo afetadas
- **AND** o arquivo de configuração não é alterado

