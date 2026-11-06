## ADDED Requirements

### Requirement: Build with Vite+
The frontend SHALL be built, served in development, checked and unit-tested with the `vp` CLI of the `vite-plus`
package, pinned to an exact version in `web/app/package.json`. `make frontend-install`, `make frontend-build` and
`make frontend-dev` SHALL keep their names and behaviour, and `make frontend-check` and `make frontend-test` SHALL run
`vp check` and `vp test`.

#### Scenario: Production build
- **WHEN** `make frontend-build` runs
- **THEN** `vp build` writes the files to `web/static` and the Go binary built afterwards serves the interface

#### Scenario: Development server
- **WHEN** `make frontend-dev` runs
- **THEN** the interface is served on port 8081 and `/api` is proxied to `localhost:8080`

### Requirement: Output contract with the Go binary
The build SHALL write to `web/static`: `index.html` with every Go template placeholder of `web/app/index.html`
unchanged, `js/app.js`, `css/app.css`, the fonts under `fonts/` with their versioned names, and the files of
`public/` at the same paths, without content hashes. `/css/custom.css` SHALL stay a plain link resolved at runtime.

#### Scenario: Placeholder in a link
- **WHEN** the source has `<link rel="icon" href="{{ .UI.Favicon.Size32x32 }}">`
- **THEN** the build neither resolves nor rewrites it, and the server renders it with the configured favicon

#### Scenario: Placeholders survive the build
- **WHEN** the embedded `index.html` is compared with `web/app/index.html`
- **THEN** every `{{ ... }}` placeholder of the source is present in the embedded file

#### Scenario: Fonts keep their names
- **WHEN** the binary serves `/fonts/inter-4-1-latin.woff2`
- **THEN** it answers 200 with `Content-Type: font/woff2`

### Requirement: Checks in CI
The CI workflow SHALL run `vp check` (lint, format and type check, without fixing), ESLint with `eslint-plugin-vue`
on the Vue templates, which Oxlint does not lint, and `vp test` on every push and pull
request, and SHALL install the toolchain with `npm ci`, never with a remote install script.

#### Scenario: Formatting error
- **WHEN** a pull request changes a Vue file without the formatting of Oxfmt
- **THEN** the frontend check of CI fails

### Requirement: Same interface after the migration
The migration SHALL NOT change what a user sees or what the HTTP API answers: the HTTP contract test, the unit tests
and every end-to-end script SHALL pass without changes to their expectations.

#### Scenario: Theme colours unchanged
- **WHEN** `test/e2e/theme-colors.sh --compare` runs against a reference collected before the migration
- **THEN** no element changes colour
