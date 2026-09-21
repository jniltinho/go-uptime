# ui-theme Specification

## Purpose
TBD - created by archiving change refine-admin-backup-layout. Update Purpose after archive.
## Requirements
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
A troca de tema MUST ser feita por um seletor que mostra o tema em uso e oferece os três temas, no lugar de um botão que alterna: nas configurações do dashboard, no cabeçalho das páginas públicas e na tela de login. O seletor MUST ser um botão que abre um menu com uma opção por tema, operável por teclado (Enter ou Espaço abrem, as setas percorrem as opções, Esc fecha e devolve o foco ao botão), com a opção em uso marcada para leitores de tela. Escolher um tema MUST aplicá-lo sem recarregar a página e MUST salvar a escolha. No cabeçalho das páginas públicas, o seletor fechado e o menu aberto MUST caber sem rolagem horizontal a partir de 360 px de largura, com logo e um título longo.

#### Scenario: Escolher pelo teclado
- **WHEN** o visitante foca o seletor, abre o menu com Enter, vai até "Bio" com as setas e confirma
- **THEN** a interface passa ao tema Bio sem recarregar, o menu fecha e o foco volta ao seletor

#### Scenario: Tema em uso
- **WHEN** o visitante abre o seletor no tema escuro
- **THEN** a opção do tema escuro aparece marcada como a escolhida

#### Scenario: Página pública a 360 px
- **WHEN** uma página pública com logo e título longo é aberta a 360 px de largura e o visitante abre o seletor
- **THEN** o menu aparece inteiro dentro da janela e a página não ganha rolagem horizontal

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

