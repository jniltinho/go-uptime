# ui-theme Specification

## Purpose
TBD - created by archiving change refine-admin-backup-layout. Update Purpose after archive.
## Requirements
### Requirement: Tema escuro por padrão
The web interface MUST offer the themes of the theme registry (`web/app/src/themes/index.js`), which ships with dark
(`dark`), light (`light`) and Bio (`bio`, a light-based theme). Without a valid theme choice saved by the visitor, it
MUST use the configured default theme: the one of `ui.default-theme` (any id of the registry) and, only when
`ui.default-theme` is not set, the one of `ui.dark-mode`, which is dark by default, without following the theme
preference of the operating system. This applies to the dashboard, the details pages, the administration, the public
pages and the login screen.

**Delivery by the server:**
- the delivered HTML MUST already carry the initial theme (`data-theme`, `data-scheme` and, for the 7.x series, the
  legacy class `dark` or `theme-bio`), so that the page does not change theme while loading;
- MUST state the configured default theme;
- MUST carry the browser theme colour (`theme-color`) of the theme, from the registry.

**Visitor choice:**
- the choice made with the theme selector MUST stay saved and MUST win over the configured default on later visits;
- a saved value that is not an id of the registry MUST be ignored;
- without `ui.default-theme`, with `ui.dark-mode: false` and without a saved choice, the interface MUST use the light
  theme;
- with both `ui.default-theme` and `ui.dark-mode` present, `ui.default-theme` MUST win and the log MUST warn; a value of
  `ui.default-theme` that is not an id of the registry MUST invalidate the configuration;
- the theme attributes and legacy classes MUST be mutually exclusive: changing theme MUST replace those of the previous
  theme.

#### Scenario: Primeira visita com o sistema em modo claro
- **WHEN** a visitor without a saved theme, with the operating system in light mode, opens the dashboard or the login screen with the default configuration
- **THEN** the delivered HTML already has the dark theme and the interface stays dark after loading

#### Scenario: Tema claro escolhido
- **WHEN** the visitor chooses the light theme and reloads the page
- **THEN** the interface stays in the light theme

#### Scenario: Escolha salva inválida
- **WHEN** the theme cookie of the visitor has a value that is not an id of the registry
- **THEN** the interface uses the configured default theme

#### Scenario: Claro por configuração
- **WHEN** the configuration has `ui.dark-mode: false`, has no `ui.default-theme`, and the visitor has no saved choice
- **THEN** the interface is shown in the light theme

#### Scenario: Tema Bio escolhido
- **WHEN** the visitor chooses the Bio theme and reloads the page
- **THEN** the HTML delivered by the server already has `data-theme="bio"` and the class `theme-bio`, and not the dark class
- **AND** the browser theme colour is the one of the Bio theme

### Requirement: Cores da barra de rolagem pelo tema
As cores da barra de rolagem MUST vir das variáveis do tema, com um tom de repouso e, onde o navegador permite estilizar o realce da barra, um tom mais forte ao passar o mouse. As cores MUST mudar junto com o tema, entre quaisquer dois dos três temas, inclusive quando o visitante troca pelo seletor de tema.

O tom de repouso MUST ter pelo menos 3:1 de contraste com a superfície de menor contraste de cada tema (os fundos claros das áreas com rolagem nos temas claro e Bio e o fundo das tabelas no escuro). Com `prefers-contrast: more`, o repouso e o realce MUST ficar mais fortes, com o realce acima do repouso. Com `forced-colors: active`, a barra MUST voltar às cores do sistema.

#### Scenario: Troca de tema
- **WHEN** o visitante troca do tema escuro para o claro numa página com rolagem
- **THEN** a variável de cor da barra passa a ter o valor do tema claro

#### Scenario: Alto contraste do sistema
- **WHEN** o sistema operacional está no modo de alto contraste
- **THEN** a barra usa as cores do sistema, e não as do tema

#### Scenario: Contraste do polegar
- **WHEN** as cores da barra são medidas sobre o fundo da página nos temas claro e Bio e sobre o fundo das tabelas no tema escuro
- **THEN** o contraste do tom de repouso é de pelo menos 3:1 nos três casos

#### Scenario: Mais contraste no tema Bio
- **WHEN** o sistema pede mais contraste (`prefers-contrast: more`) e o tema em uso é o Bio
- **THEN** o repouso e o realce da barra ficam mais fortes que os do tema Bio sem essa preferência, com o realce acima do repouso

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

### Requirement: Seletor de tema
The theme MUST be changed through a selector that shows the theme in use and offers one option per theme of the
registry (`web/app/src/themes/index.js`, in its order, with its labels), instead of a toggle button: in the settings
of the dashboard, in the header of the public pages and on the login screen. The selector MUST be a button that opens a
menu with one option per theme, operable by keyboard (Enter or Space open it, the arrows move through the options, Home
and End go to the ends, Esc closes it and gives the focus back to the button), with the option in use marked for screen
readers. Choosing a theme MUST apply it without reloading the page and MUST save the choice. In the header of the public
pages, the closed selector and the open menu MUST fit without horizontal scrolling from 360 px of width, with a logo and
a long title; with many themes the menu MUST scroll inside itself instead of leaving the window.

#### Scenario: Escolher pelo teclado
- **WHEN** the visitor focuses the selector, opens the menu with Enter, moves to "Bio" with the arrows and confirms
- **THEN** the interface switches to the Bio theme without reloading, the menu closes and the focus returns to the selector

#### Scenario: Tema em uso
- **WHEN** the visitor opens the selector in the dark theme
- **THEN** the option of the dark theme is marked as the chosen one

#### Scenario: Página pública a 360 px
- **WHEN** a public page with a logo and a long title is opened at 360 px of width and the visitor opens the selector
- **THEN** the menu appears whole inside the window and the page gets no horizontal scrolling

#### Scenario: Um tema novo no registro
- **WHEN** a theme is added to the registry
- **THEN** the selector offers it in the order of the registry, without any change to the component

### Requirement: Cores do tema Bio
O tema Bio MUST definir o texto, as superfícies, as bordas, os links, o anel de foco e os botões primários, e o texto MUST ter pelo menos 4,5:1 de contraste com a superfície do tema em que aparece, e as bordas de controles e o anel de foco pelo menos 3:1. O verde-água do tema MUST NOT ser cor de texto nem a única indicação de foco sobre superfícies claras. As cores de estado (no ar, fora, pendente, sem dados, e o passo pulado de uma suíte), inclusive as que hoje são tons de cinza, e as demais cores que não são cinza nem variável do tema MUST ser, no tema Bio, as mesmas do tema claro, e MUST continuar, nos temas claro e escuro, as que cada um tem hoje: para cada par de cor de estado e superfície, o contraste no tema Bio MUST ser maior ou igual ao do mesmo par no tema claro.

#### Scenario: Medição
- **WHEN** o contraste é medido por script sobre as telas no tema Bio
- **THEN** todo texto nas cores do tema tem pelo menos 4,5:1, toda borda de controle e anel de foco pelo menos 3:1
- **AND** nenhum par de cor de estado e superfície tem contraste menor que o do mesmo par no tema claro

#### Scenario: Cinza de estado
- **WHEN** um endpoint sem dados e um passo pulado de uma suíte são exibidos no tema Bio
- **THEN** a cor do indicador de cada um é idêntica à do tema claro, e não a da escala de cinza do tema Bio

### Requirement: Temas existentes inalterados pela escala de cinza por variáveis
A passagem da escala de cinza para variáveis CSS MUST NOT alterar os temas claro e escuro. Isso MUST ser verificado antes de qualquer cor do tema Bio e antes do seletor de tema: em todas as telas, nos dois temas, as cores computadas de todos os elementos (texto, fundo, bordas, contorno, preenchimento e traço de SVG, sombra), sua opacidade e sua imagem de fundo MUST ser iguais antes e depois da mudança, com transições e animações desativadas, nos estados que o roteiro de verificação enumera e ativa (entre eles `hover`, `focus`, diálogos e menus abertos e grupos recolhidos e expandidos). Os pixels de imagens e do `canvas` do gráfico ficam fora dessa verificação.

#### Scenario: Cores computadas iguais
- **WHEN** as cores computadas de todos os elementos de cada tela são coletadas nos temas claro e escuro antes e depois da mudança da escala de cinza, com os mesmos dados
- **THEN** não há diferença em nenhum elemento

### Requirement: Componentes sem cores próprias por tema
O gráfico de tempo de resposta MUST ler as cores de grade, texto e marcas das variáveis do tema, no lugar de cores fixas escolhidas por ser ou não o tema escuro, e MUST ser redesenhado quando o tema muda, entre quaisquer dois temas. As regras de estilo que dependem do seletor do tema escuro, como o traço do cabeçalho dos cartões de suíte, MUST ter o valor do tema Bio.

#### Scenario: Gráfico do claro para o Bio
- **WHEN** o visitante troca do tema claro para o Bio numa página com o gráfico de tempo de resposta
- **THEN** a grade, o texto e as marcas do gráfico passam às cores do tema Bio, sem recarregar

### Requirement: HTML da interface varia com o cookie de tema
As respostas HTML da interface, no dashboard e nas páginas públicas, em GET e em HEAD, MUST incluir `Vary: Cookie`, porque o tema entregue depende do cookie, e `Cache-Control: no-cache` — exceto as páginas protegidas por login próprio, que MUST manter `Cache-Control: private, no-store` no lugar de `no-cache`, com o `Vary` que já têm mais `Cookie`.

#### Scenario: Dois visitantes com temas diferentes
- **WHEN** um visitante com `theme=bio` e outro com `theme=dark` pedem a mesma página através de um cache intermediário
- **THEN** cada um recebe o HTML com a classe do seu tema

