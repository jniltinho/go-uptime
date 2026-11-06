## MODIFIED Requirements

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
