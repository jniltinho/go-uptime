## MODIFIED Requirements

### Requirement: Tema escuro por padrão
A interface web MUST oferecer três temas: escuro (`dark`), claro (`light`) e Bio (`bio`), este de base clara. Sem uma escolha de tema válida salva pelo visitante, ela MUST usar o tema padrão configurado: o de `ui.default-theme` (`dark`, `light` ou `bio`) e, só quando `ui.default-theme` não está definido, o de `ui.dark-mode`, que é escuro por padrão, sem seguir a preferência de tema do sistema operacional. Isso vale para o dashboard, as páginas de detalhes, a administração, as páginas públicas e a tela de login.

**Entrega pelo servidor:**
- o HTML entregue MUST já ter o tema inicial, para a página não trocar de tema ao carregar;
- MUST informar o tema padrão configurado;
- MUST ter a cor de tema do navegador (`theme-color`) coerente com o tema.

**Escolha do visitante:**
- a escolha feita pelo seletor de tema MUST continuar salva e MUST prevalecer sobre o tema padrão configurado nas visitas seguintes;
- um valor salvo diferente de `dark`, `light` e `bio` MUST ser ignorado;
- sem `ui.default-theme`, com `ui.dark-mode: false` e sem escolha salva, a interface MUST usar o tema claro;
- com `ui.default-theme` e `ui.dark-mode` presentes, `ui.default-theme` MUST valer e o log MUST avisar; um valor de `ui.default-theme` fora dos três MUST invalidar a configuração;
- as classes de tema no HTML MUST ser mutuamente exclusivas: trocar de tema MUST remover a do tema anterior.

#### Scenario: Primeira visita com o sistema em modo claro
- **WHEN** um visitante sem escolha de tema salva, com o sistema operacional em modo claro, abre o dashboard ou a tela de login com a configuração padrão
- **THEN** o HTML entregue já tem o tema escuro e a interface continua escura depois de carregar

#### Scenario: Tema claro escolhido
- **WHEN** o visitante escolhe o tema claro e recarrega a página
- **THEN** a interface continua no tema claro

#### Scenario: Escolha salva inválida
- **WHEN** o cookie de tema do visitante tem um valor diferente de `dark`, `light` e `bio`
- **THEN** a interface usa o tema padrão configurado, seja ele `dark`, `light` ou `bio`

#### Scenario: Claro por configuração
- **WHEN** a configuração tem `ui.dark-mode: false`, não tem `ui.default-theme`, e o visitante não tem escolha salva
- **THEN** a interface é exibida no tema claro

#### Scenario: Tema Bio escolhido
- **WHEN** o visitante escolhe o tema Bio e recarrega a página
- **THEN** o HTML entregue pelo servidor já tem a classe do tema Bio e não tem a do tema escuro
- **AND** a cor de tema do navegador é a do tema Bio

#### Scenario: Tema padrão Bio
- **WHEN** a configuração tem `ui.default-theme: bio` e o visitante não tem escolha salva
- **THEN** a interface é exibida no tema Bio, no dashboard, na administração, nas páginas públicas e na tela de login

#### Scenario: default-theme e dark-mode juntos
- **WHEN** a configuração tem `ui.default-theme: light` e `ui.dark-mode: true`
- **THEN** a interface sem escolha salva é exibida no tema claro, e o log avisa que `ui.default-theme` prevaleceu

#### Scenario: Tema padrão inválido
- **WHEN** a configuração tem `ui.default-theme: azul`
- **THEN** a configuração é inválida, no início e em `go-uptime config validate`

#### Scenario: Do escuro para o Bio e de volta
- **WHEN** o visitante troca do tema escuro para o Bio e depois para o claro
- **THEN** em cada passo o HTML tem só a classe do tema em uso, e a cor de tema do navegador acompanha

### Requirement: Tipografia entregue pelo próprio serviço
A interface MUST usar a fonte Inter entregue pelo próprio Go Uptime, em arquivos `woff2` variáveis servidos em `/fonts/`, versionados no repositório e embutidos no binário junto do restante dos estáticos. O frontend MUST NOT pedir fontes a nenhum domínio externo, nas telas autenticadas e nas páginas públicas.

Os `@font-face` MUST declarar `font-display: swap` e um `unicode-range` por subconjunto, e a pilha de fontes MUST manter as fontes do sistema como reserva, de modo que a interface continue legível se o arquivo não carregar. A licença da fonte MUST ser distribuída junto dos arquivos.

Os números da interface MUST usar algarismos tabulares, inclusive dentro de campos de formulário, para que um valor que muda sozinho não mude a largura do que está em volta e para que colunas de números fiquem alinhadas. O gráfico de tempo de resposta desenha texto em `canvas` e fica de fora dos algarismos tabulares, mas MUST usar a mesma família de fontes do restante da interface. Os badges SVG gerados no backend ficam de fora das duas regras.

#### Scenario: Fonte aplicada sem sair do servidor
- **WHEN** um visitante abre uma página pública sem credenciais
- **THEN** a fonte Inter é carregada de `/fonts/`
- **AND** nenhuma requisição vai para `fonts.googleapis.com` ou `fonts.gstatic.com`

#### Scenario: Números com a mesma largura
- **WHEN** a interface mostra dois valores de mesmo comprimento e dígitos diferentes, como `111` e `999`
- **THEN** os dois ocupam exatamente a mesma largura

#### Scenario: Fonte indisponível
- **WHEN** o arquivo da fonte não carrega
- **THEN** a interface continua legível com a fonte do sistema, sem erro na tela

#### Scenario: Fonte trocada pelo administrador
- **WHEN** o administrador define `body { font-family: … }` em `ui.custom-css`
- **THEN** a interface passa a usar a família escolhida, sem precisar de `!important`

#### Scenario: Algarismos tabulares desligados
- **WHEN** o administrador define `body { font-variant-numeric: normal !important }` em `ui.custom-css`
- **THEN** a interface volta aos algarismos proporcionais

#### Scenario: Arquivo da fonte entregue pelo binário
- **WHEN** um cliente pede o arquivo da fonte ao Go Uptime
- **THEN** a resposta é `200` com `Content-Type: font/woff2`
