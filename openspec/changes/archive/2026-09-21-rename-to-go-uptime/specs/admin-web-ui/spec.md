## MODIFIED Requirements

### Requirement: Formulário e editor YAML
As telas de criação e edição MUST oferecer um modo formulário e um modo YAML, preservando o conteúdo ao alternar entre eles.

O formulário MUST começar pelo tipo de monitor:
- **ativos** (HTTP(s), TCP, Ping, DNS e os demais inferidos pela URL): nome, grupo, URL, método, intervalo, condições, headers, alertas entre os tipos configurados, estado habilitado e a opção "Accept push" (receber push), desligada por padrão, que ao ser ligada mostra o token opcional, a URL de push copiável e o exemplo com a chave global;
- **Push (passivo):** nome, grupo, token, intervalo de heartbeat, tentativas ("Retries", `heartbeat.retries`, de 0 a 100, padrão 0) ao lado do intervalo, alertas e estado habilitado.

No tipo Push, a tela MUST mostrar:
- a URL de push copiável no formato do Uptime Kuma (`<endereço do Go Uptime>/api/push/<token>?status=up&msg=OK&ping=`);
- a explicação de que o envio deve ocorrer a cada intervalo de heartbeat e aceita `status`, `msg` e `ping`;
- um exemplo de `curl`;
- a ação de gerar um token novo;
- um campo para informar um token existente.

O grupo MUST ser escolhido entre os grupos dos endpoints existentes, "sem grupo" ou um grupo novo digitado. Na edição de um endpoint gerenciado, nome e grupo MUST ser editáveis; o tipo MUST ser somente leitura. Quando a chave derivada mudar, a tela MUST avisar antes de salvar:
- que a chave muda;
- que as URLs de badges e da página de detalhes mudam;
- nos endpoints que recebem push, que as URLs com chave global mudam, e que a URL com o token do endpoint não muda;
- quais status pages do arquivo de configuração deixam de mostrar o endpoint.

Depois de salvar, a tela MUST usar a chave nova. Segredos mascarados MUST ser exibidos como `********` e mantidos quando não forem alterados.

#### Scenario: Formulário para YAML
- **WHEN** o administrador preenche o formulário e alterna para o modo YAML
- **THEN** o editor mostra a definição equivalente em YAML

#### Scenario: YAML inválido
- **WHEN** o administrador digita um YAML inválido e tenta alternar para o modo formulário
- **THEN** a tela mostra o erro e permanece no modo YAML com o texto digitado

#### Scenario: Grupo existente ou novo
- **WHEN** o administrador abre o seletor de grupo num formulário de endpoint e existem endpoints no grupo `core`
- **THEN** o seletor oferece `core`, "sem grupo" e a opção de digitar um grupo novo

#### Scenario: Tipo Push
- **WHEN** o administrador escolhe o tipo Push num endpoint novo
- **THEN** o formulário esconde URL, método, condições e headers e mostra a URL de push copiável com um token gerado, o intervalo de heartbeat de 60 segundos e um exemplo de `curl`

#### Scenario: Tentativas no endpoint Push
- **WHEN** o administrador informa 2 em "Retries" num endpoint Push e salva
- **THEN** a definição salva tem `heartbeat.retries: 2`
- **AND** ao editar, o campo mostra 2

#### Scenario: Push num endpoint ativo
- **WHEN** o administrador liga "Accept push" num endpoint HTTP
- **THEN** o formulário mostra um token gerado, a URL de push copiável e o exemplo com a chave global, sem esconder URL, condições e headers

#### Scenario: Push desligado no endpoint ativo
- **WHEN** o administrador desliga "Accept push" num endpoint HTTP e salva
- **THEN** a definição fica sem `push` e o endpoint deixa de aceitar envios

#### Scenario: Token do Uptime Kuma
- **WHEN** o administrador cola o token de um monitor Push do Uptime Kuma no campo de token
- **THEN** a URL de push mostrada passa a usar esse token

#### Scenario: Troca de grupo na edição
- **WHEN** o administrador troca o grupo de `web_site` para `clientes` na edição
- **THEN** antes de salvar, a tela avisa que a chave muda de `web_site` para `clientes_site` e que as URLs de badges e da página de detalhes mudam
- **AND** depois de salvar, a lista mostra `clientes_site` com o histórico anterior

#### Scenario: Status page do arquivo afetada
- **WHEN** o administrador troca o nome de um endpoint selecionado pela chave na status page `services` do arquivo de configuração
- **THEN** antes de salvar, a tela avisa que `services` deixa de mostrar o endpoint até o arquivo ser corrigido

#### Scenario: Edição concorrente
- **WHEN** o administrador salva e a API responde 412
- **THEN** a tela informa que o endpoint foi alterado por outra pessoa e oferece recarregar a versão atual, sem descartar o conteúdo digitado

### Requirement: Testes ponta a ponta com agent-browser
O repositório MUST ter um roteiro de testes ponta a ponta em `test/e2e/` que suba o Go Uptime local com SQLite temporário, `security.basic` e `admin.enabled`, e use o `agent-browser` para percorrer lista, criação, validação, teste, salvamento, edição, desabilitação, remoção e acesso sem credenciais (401), nos temas claro e escuro, salvando capturas de tela em `dist/prints/`. O diretório `dist/` MUST ser ignorado pelo git.

#### Scenario: Execução do roteiro
- **WHEN** alguém executa o roteiro de testes ponta a ponta
- **THEN** o roteiro termina com sucesso e grava as capturas de tela em `dist/prints/`
- **AND** `git status` não mostra arquivos novos em `dist/`
