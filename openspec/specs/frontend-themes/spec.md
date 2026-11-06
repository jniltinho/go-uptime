# frontend-themes Specification

## Purpose
TBD - created by archiving change reorganize-frontend-vite-plus-themes. Update Purpose after archive.
## Requirements
### Requirement: Role contract of colours
`web/app/src/style.css` SHALL define the colour roles of the interface (the variables `--background`, `--card`,
`--border`, `--muted`, the gray scale `--gray-N`, the chart colours `--chart-*`, ...), their defaults being the light
theme, and expose them as utilities through `@theme`. New and changed components SHALL use the role utilities for text,
page, surfaces, lines and fields; the existing `dark:` pairs keep working through the `data-scheme` variant and are
migrated screen by screen. Status colours SHALL keep the same value in every theme (`statusgray` and the literal
status families).

#### Scenario: A new component
- **WHEN** a new component needs the colour of a card
- **THEN** it uses `bg-card`, and every theme renders it with its own card colour without a `dark:` class

### Requirement: One file per theme
Each theme SHALL be a file `web/app/src/themes/<id>.css` that only redefines roles and parameters under
`:root[data-theme='<id>']`, or under scopes inside it (such as the header of the Bio theme), plus one entry `{ id, label, scheme, themeColor }` in
`web/app/src/themes/index.js`. The change SHALL ship the themes `light`, `dark` and `bio` with their current colours.

#### Scenario: Adding a theme
- **WHEN** a contributor adds `themes/grafana.css` and its registry entry
- **THEN** the theme appears in the theme selector, `ui.default-theme: grafana` is valid and the server renders it
  before the first paint

### Requirement: Theme on the root element
The root element SHALL carry `data-theme="<id>"` and `data-scheme="light|dark"`, set by the server in the rendered
HTML, by the inline script before the first paint and by `utils/theme.js` on a change, following the same table of
cases (`theme.cases.json`). The classes `dark` and `theme-bio` SHALL still be set for the 7.x series.

#### Scenario: Cookie of a valid theme
- **WHEN** a request carries the cookie `theme=bio`
- **THEN** the HTML comes with `data-theme="bio"`, `data-scheme="light"`, the class `theme-bio` and the theme colour of bio

#### Scenario: Colours unchanged by the reorganization
- **WHEN** `test/e2e/theme-colors.sh --compare` runs against a reference collected before step 4
- **THEN** no element of the 24 screens and states changes colour in any of the three themes

### Requirement: Canvas follows the theme
Elements drawn on a canvas SHALL read their colours from the roles and redraw when `data-theme` changes, including a
change between two themes of the same scheme.

#### Scenario: Two light themes
- **WHEN** the visitor switches from `light` to `bio` with a response time chart on the screen
- **THEN** the chart is redrawn with the colours of `bio`

### Requirement: Large screens
The whole interface SHALL be zoomed by 1.25 from 2400 CSS pixels of viewport and by 1.6 from 3200, and heights that
fill the window SHALL use a measured `--view-h` instead of `100dvh`.

#### Scenario: 4K without OS scaling
- **WHEN** the dashboard is opened on a viewport 3840 pixels wide
- **THEN** the root has a zoom of 1.6 and no screen has a vertical scrollbar caused by the zoom

