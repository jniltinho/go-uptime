# admin-web-ui Specification

## Purpose
TBD - created by archiving change add-admin-endpoint-management. Update Purpose after archive.
## Requirements
### Requirement: Acesso às telas de administração
Com a administração habilitada e o usuário autorizado, o frontend MUST mostrar no cabeçalho um link "Admin" para `/admin`. As rotas `/admin`, `/admin/endpoints/new` e `/admin/endpoints/{key}/edit` MUST abrir diretamente pela URL, inclusive ao recarregar a página, com a chave codificada na URL.

#### Scenario: Link visível para administrador
- **WHEN** `GET /api/v1/config` indica `admin.enabled` e `admin.authorized` verdadeiros
- **THEN** o cabeçalho mostra o link "Admin"

#### Scenario: Link oculto sem permissão
- **WHEN** a configuração usa OIDC e `GET /api/v1/config` indica `admin.authorized` falso
- **THEN** o cabeçalho não mostra o link "Admin"

#### Scenario: Abertura direta da edição
- **WHEN** o navegador abre `/admin/endpoints/core_api/edit` diretamente
- **THEN** o frontend carrega a tela de edição de `core_api`

### Requirement: Lista de endpoints
A tela `/admin` MUST listar os endpoints ativos e Push, do arquivo de configuração (inclusive external endpoints) e gerenciados pela web. Cada item MUST expor nome, grupo, tipo (`PUSH` para endpoints Push), URL (com credenciais mascaradas e vazia para Push), intervalo (de heartbeat para Push), estado habilitado e origem (Web ou YAML), na tabela ou no cartão, conforme a largura da janela.

**Sem rolagem horizontal:** a lista MUST caber na largura disponível em qualquer janela a partir de 360 px, sem barra de rolagem horizontal, por mais longos que sejam o nome e a URL, e as ações de cada item MUST estar alcançáveis sem rolar para o lado. Para isso:

- a largura da tabela MUST NOT depender do conteúdo: os campos que podem crescer — nome e URL — MUST ser truncados, com o valor completo disponível ao repousar o ponteiro, e o aviso de conflito com o YAML MUST NOT somar largura à célula do nome;
- em janelas a partir de 1024 px a tabela MUST mostrar todas as colunas; entre 768 px e 1024 px o intervalo e a origem MUST sair da tabela;
- abaixo de 768 px a lista MUST deixar de ser tabela e virar um cartão por endpoint, com **todos** os campos, inclusive o intervalo, e com as mesmas ações, pelos mesmos identificadores de teste das ações da tabela.

As listas de status pages e de chaves de push MUST seguir as mesmas regras, com os campos de cada uma.

**Densidade e ações:** na tabela, todas as linhas MUST ter a mesma altura — inclusive a de um endpoint que também recebe push e a de um endpoint em conflito ou com definição inválida — e essa altura MUST ser de no máximo 32 px. Os selos MUST NOT quebrar em mais de uma linha, e o aviso de conflito ou de definição inválida MUST caber na mesma linha do nome, com a explicação disponível ao repousar o ponteiro e um nome acessível equivalente.

As ações de cada item MUST ser alvos de ícone, sem texto visível, cada um com nome acessível que inclua a ação e o nome do item, dica ao repousar o ponteiro e o mesmo identificador de teste da ação equivalente de antes. Uma ação que leva a outra página MUST continuar sendo um link, com o destino em nova aba onde já era. Uma ação indisponível MUST continuar explicando o motivo ao repousar o ponteiro. A confirmação de remoção MUST continuar a mesma, e os cartões das telas estreitas MUST usar os mesmos ícones, com alvo maior por serem telas de toque. A tela MUST permitir buscar por nome, grupo ou URL, MUST destacar endpoints em conflito ou com erro de validação e MUST permitir habilitar e desabilitar endpoints de origem Web. Endpoints de origem YAML MUST ser apenas visualizáveis.

#### Scenario: Busca
- **WHEN** o administrador digita `core` na busca
- **THEN** a lista mostra apenas endpoints cujo nome, grupo ou URL contém `core`

#### Scenario: Endpoint do YAML
- **WHEN** o administrador abre um endpoint de origem YAML
- **THEN** a tela mostra a definição em YAML com segredos mascarados, sem ações de salvar, habilitar, desabilitar ou remover

#### Scenario: External endpoint do YAML
- **WHEN** o arquivo de configuração define o external endpoint `jobs_backup`
- **THEN** a lista mostra `jobs_backup` com tipo `PUSH` e origem YAML, somente para visualização

#### Scenario: Endpoint em conflito
- **WHEN** existe um endpoint gerenciado marcado como em conflito
- **THEN** a lista mostra esse endpoint com um aviso de conflito com o YAML

#### Scenario: Janela estreita sem rolagem lateral
- **WHEN** o administrador abre `/admin` numa janela de 900 px de largura, com um endpoint de nome e URL longos
- **THEN** nem a lista nem a página têm rolagem horizontal
- **AND** as ações do endpoint gerenciado pela web ficam dentro da janela
- **AND** a URL aparece truncada, com o endereço completo ao repousar o ponteiro

#### Scenario: Lista em cartões no celular
- **WHEN** a lista é aberta com 390 px de largura
- **THEN** cada endpoint aparece como um cartão, sem tabela, com o intervalo entre os campos
- **AND** as ações continuam disponíveis pelos mesmos identificadores da tabela
- **AND** a página não tem rolagem horizontal

#### Scenario: Linhas com a mesma altura
- **WHEN** a lista, numa janela de 1000 px, mostra um endpoint que também recebe push e um endpoint em conflito com o YAML, junto de endpoints comuns
- **THEN** o selo de push e o aviso de conflito cabem na mesma linha de cada um
- **AND** todas as linhas da lista têm a mesma altura, de no máximo 32 px

#### Scenario: Ações por ícone
- **WHEN** o administrador olha a linha de um endpoint gerenciado pela web
- **THEN** as ações aparecem como ícones, sem texto visível
- **AND** cada ícone tem nome acessível com a ação e o nome do endpoint
- **AND** o ícone de remover abre a mesma confirmação de antes
- **AND** uma ação indisponível continua explicando o motivo ao repousar o ponteiro

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

### Requirement: Validar e testar antes de salvar
As telas de criação e edição MUST ter as ações Validar e Salvar e, nos tipos ativos, a ação Testar. Testar MUST exibir o resultado de cada condição e a duração. No tipo Push, a ação Testar MUST ser substituída pela URL de push e pelo exemplo de `curl`. Erros devolvidos pela API MUST ser exibidos junto ao formulário sem perder o conteúdo digitado.

#### Scenario: Erro ao salvar
- **WHEN** o administrador salva e a API responde 400
- **THEN** a mensagem de erro aparece na tela
- **AND** o conteúdo digitado continua no formulário

#### Scenario: Resultado do teste
- **WHEN** o administrador clica em Testar
- **THEN** a tela lista cada condição com indicação de atendida ou não e mostra a duração

#### Scenario: Endpoint Push sem Testar
- **WHEN** o administrador edita um endpoint Push
- **THEN** a tela não mostra a ação Testar e mostra a URL de push com o exemplo de `curl`

### Requirement: Confirmação de remoção
A remoção de um endpoint gerenciado MUST exigir confirmação explícita que informe que o histórico será apagado e, quando houver alertas disparados, que os provedores de alerta não serão notificados.

#### Scenario: Remoção cancelada
- **WHEN** o administrador clica em remover e cancela a confirmação
- **THEN** nenhuma requisição de remoção é enviada

### Requirement: Padrões do frontend
As telas de administração MUST seguir as convenções do projeto: Vue 3 com `<script setup>`, Tailwind com variantes `dark:` em todos os componentes novos, dados passados por props (sem provide/inject) e build incluído em `web/static/`.

#### Scenario: Tema escuro
- **WHEN** o usuário usa o tema escuro
- **THEN** as telas de administração são exibidas com as cores do tema escuro

#### Scenario: Tema Bio
- **WHEN** o usuário usa o tema Bio
- **THEN** as telas de administração são exibidas com as cores do tema Bio, sem nenhuma variante do tema escuro aplicada

### Requirement: Testes ponta a ponta com agent-browser
O repositório MUST ter um roteiro de testes ponta a ponta em `test/e2e/` que suba o Go Uptime local com SQLite temporário, `security.basic` e `admin.enabled`, e use o `agent-browser` para percorrer lista, criação, validação, teste, salvamento, edição, desabilitação, remoção e acesso sem credenciais (401), nos temas claro e escuro, salvando capturas de tela em `dist/prints/`. O diretório `dist/` MUST ser ignorado pelo git.

#### Scenario: Execução do roteiro
- **WHEN** alguém executa o roteiro de testes ponta a ponta
- **THEN** o roteiro termina com sucesso e grava as capturas de tela em `dist/prints/`
- **AND** `git status` não mostra arquivos novos em `dist/`

### Requirement: Tela de chaves de push
A administração MUST ter a aba "Push keys" em `/admin/push-keys`. A aba MUST listar as chaves globais com nome, dica (4 últimos caracteres), origem (Web ou YAML), autor e data de criação.

Ela MUST permitir criar uma chave global informando o nome. Depois de criada, a aba MUST mostrar a chave completa uma única vez, com um exemplo de URL `/api/push/<chave>/<chave-do-endpoint>?status=up&msg=OK&ping=` e o aviso de que ela não será mostrada de novo. Revogar uma chave de origem Web MUST exigir confirmação. As chaves de origem YAML MUST ser apenas visualizáveis.

#### Scenario: Chave criada
- **WHEN** o administrador cria a chave global `akamai`
- **THEN** a tela mostra a chave completa com a URL de exemplo e o aviso de exibição única
- **AND** ao recarregar a aba, a lista mostra somente a dica da chave

#### Scenario: Revogação cancelada
- **WHEN** o administrador clica em revogar uma chave e cancela a confirmação
- **THEN** nenhuma requisição de revogação é enviada

### Requirement: Diálogos e toasts da administração
As telas da administração MUST usar um diálogo padrão, com o visual do diálogo de confirmação, e MUST poder mostrar mensagens em toasts.

**Diálogos:**
- **Estrutura:** título, descrição opcional, botão de fechar, corpo com rolagem própria, rodapé com os botões à direita e altura máxima da janela menos uma margem;
- **Acessibilidade:** o painel MUST ter `role="dialog"`, `aria-modal`, nome pelo título e descrição associada quando existir;
- **Empilhamento:** diálogos abertos depois MUST ficar visualmente por cima dos anteriores;
- **Foco:** ao abrir, o foco MUST ir para o diálogo (no diálogo de confirmação, para Cancel) e MUST ficar preso no diálogo do topo, inclusive depois de um clique fora dele. Quando um diálogo fecha e outro continua aberto, o foco MUST ir para o diálogo que ficou no topo. Quando o último diálogo fecha, o foco MUST voltar a um elemento habilitado da página;
- **Fechar:** Esc e o botão de fechar MUST agir só no diálogo do topo e MUST NOT agir enquanto ele indicar uma operação em andamento; clicar fora MUST NOT fechar;
- **Rolagem:** a rolagem da página por trás MUST ficar bloqueada enquanto houver diálogo aberto;
- **Confirmação:** o diálogo de confirmação MUST usar esse diálogo, mantendo os botões Cancel e de confirmação e seus identificadores de teste, e fechar por Esc ou pelo botão de fechar MUST equivaler a Cancel.

**Toasts:**
- **Tipos e visual:** sucesso, informação, aviso e erro, com título opcional, ícone e cor por tipo, botão de dispensar e variantes do tema escuro;
- **Tempo:** MUST desaparecer sozinhos depois de 5 segundos (sucesso e informação), 8 segundos (aviso) ou 10 segundos (erro), salvo quando criados para ficar até serem dispensados. O tempo MUST pausar com o ponteiro ou o foco sobre os toasts e enquanto houver diálogo aberto. No máximo 4 ao mesmo tempo;
- **Posição:** centralizados, no topo em telas médias e grandes e abaixo do cabeçalho do app no celular, acima dos diálogos, sem cobrir botões de ação (inclusive os do cabeçalho) e sem bloquear cliques fora dos próprios toasts;
- **Anúncio:** erros MUST ser anunciados por uma região de alerta e os demais por uma região de status, ambas presentes antes das mensagens, e os toasts visíveis MUST aparecer em ordem cronológica;
- **Onde aparecem:** só com o app autenticado visível, nunca nas páginas públicas nem na tela de login, e as mensagens MUST ser descartadas ao trocar de tela.

#### Scenario: Esc no diálogo de confirmação
- **WHEN** o administrador abre a confirmação de remoção de um endpoint e aperta Esc
- **THEN** a confirmação fecha como Cancel e o endpoint continua existindo

#### Scenario: Foco preso
- **WHEN** um diálogo está aberto e o administrador aperta Tab repetidamente
- **THEN** o foco circula só pelos elementos do diálogo

#### Scenario: Foco depois da confirmação cancelada
- **WHEN** a prévia do restore está aberta, o administrador abre a confirmação e clica em Cancel
- **THEN** o foco fica dentro da prévia

#### Scenario: Toast não cobre a ação
- **WHEN** há um toast visível em janelas de 1280×900, 800×600 ou 390×844 e o administrador clica em Download, em Preview, num botão do rodapé de um diálogo ou, no celular, no menu do cabeçalho
- **THEN** o clique chega ao botão

#### Scenario: Toast dispensado
- **WHEN** o administrador clica no botão de dispensar de um toast de erro
- **THEN** o toast desaparece antes do tempo

#### Scenario: Troca de tela
- **WHEN** há um toast de erro na aba Backup e o administrador abre a aba Endpoints
- **THEN** o toast não aparece na aba Endpoints

