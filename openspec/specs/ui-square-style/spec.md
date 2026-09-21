# ui-square-style Specification

## Purpose
TBD - created by archiving change add-admin-endpoint-management. Update Purpose after archive.
## Requirements
### Requirement: Interface sem cantos arredondados
Todos os elementos retangulares da interface web MUST ser exibidos com raio de borda zero, em todos os temas (claro, escuro e Bio), no dashboard, nas páginas de detalhes e nas telas de administração. Isso inclui cards, botões, campos de texto, selects, badges, contadores, banners de anúncio, modais, tooltips (inclusive o tooltip do gráfico de tempo de resposta), a barra e os botões de configurações, popovers, barras de progresso, tabelas e contêineres de gráficos.

#### Scenario: Card de endpoint
- **WHEN** o dashboard exibe um card de endpoint
- **THEN** o card, seus botões e seus badges têm `border-radius` computado igual a `0px`

#### Scenario: Barra de configurações e contador
- **WHEN** o dashboard exibe a barra de configurações, seus botões e o contador de endpoints com falha
- **THEN** todos têm `border-radius` computado igual a `0px`

#### Scenario: Barra de progresso de suite
- **WHEN** a página de uma suite exibe a barra de progresso do fluxo
- **THEN** a barra e seu preenchimento têm extremidades retas

#### Scenario: Tooltip do gráfico
- **WHEN** o usuário passa o mouse sobre o gráfico de tempo de resposta
- **THEN** o tooltip é desenhado com cantos retos

### Requirement: Indicadores circulares preservados
Indicadores circulares pequenos (pontos de status, marcadores de etapa do fluxo de suites, marcadores da linha do tempo de anúncios e o indicador de carregamento) MUST continuar circulares.

#### Scenario: Ponto de status
- **WHEN** o dashboard exibe o ponto de status de um endpoint
- **THEN** o ponto continua circular

### Requirement: Raio de borda centralizado no tema
O raio de borda MUST ser definido no tema do Tailwind de forma que `rounded`, `rounded-sm`, `rounded-md`, `rounded-lg` e as variantes maiores resultem em zero, mantendo valor não zero apenas em `rounded-full`, reservado a indicadores circulares.

#### Scenario: Componente novo com classe de arredondamento
- **WHEN** um componente novo usa a classe `rounded-lg`
- **THEN** o componente é exibido com cantos retos sem nenhuma alteração adicional

### Requirement: Badges SVG sem cantos arredondados
Os badges SVG servidos pela API (saúde, uptime e tempo de resposta) MUST ser gerados sem cantos arredondados. O endpoint `badge.shields`, que devolve dados para o shields.io renderizar, MUST NOT ter a forma da resposta alterada por esta regra; o rótulo que ele devolve é o nome do projeto (`go-uptime`), definido em `project-identity`.

#### Scenario: Badge de saúde
- **WHEN** um cliente requisita `GET /api/v1/endpoints/core_api/health/badge.svg`
- **THEN** nenhum elemento `rect` do SVG tem atributo `rx` maior que zero

### Requirement: Barra de rolagem fina e quadrada
A barra de rolagem da interface web MUST ser fina e sem cantos arredondados, com trilho transparente e sem os botões de seta, valendo no dashboard, nas páginas de detalhes, na administração, nas páginas públicas e na tela de login, tanto na rolagem da página quanto nas áreas com rolagem própria, na vertical e na horizontal.

**Espessura:** nos navegadores baseados em Chromium e no WebKit, a barra MUST ocupar exatamente 10 px. No Firefox, que não permite definir a espessura, ela MUST usar a barra fina do navegador.

**Estilo por navegador:** o estilo MUST usar as regras `::-webkit-scrollbar` para Chromium e WebKit e as propriedades padrão apenas onde essas regras não existem, porque definir as duas formas ao mesmo tempo faz o Chromium ignorar as regras específicas.

**Telas de toque:** em dispositivos de ponteiro grosso, como celulares e tablets sem trackpad, a barra do sistema MUST ser mantida, inclusive com o alto contraste do sistema ligado.

#### Scenario: Espessura da barra vertical
- **WHEN** a lista de endpoints da administração tem mais itens que a altura do painel, no Chrome, num dispositivo de ponteiro fino
- **THEN** a diferença entre a largura visível e a largura interna do painel é de 9 a 10 px

#### Scenario: Espessura da barra horizontal
- **WHEN** a tabela de checks tem mais colunas que a largura disponível, no Chrome
- **THEN** a diferença entre a altura visível e a altura interna da área é de 9 a 10 px

#### Scenario: Barra quadrada
- **WHEN** a interface é exibida num navegador que aceita as regras `::-webkit-scrollbar`
- **THEN** o polegar da barra é desenhado sem cantos arredondados e sem botões de seta

