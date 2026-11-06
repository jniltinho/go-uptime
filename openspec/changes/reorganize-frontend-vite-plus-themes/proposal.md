## Why

The frontend is built with Vue CLI 5 on webpack 5, which is in maintenance mode, prints deprecation warnings on
Node 23+ (`fs.Stats`), takes about 7 seconds per build and keeps ESLint 8, Babel and `core-js` only for itself.
[Vite+](https://viteplus.dev/) (MIT, 1.0) is a single toolchain — `vp dev`, `vp build`, `vp check`, `vp test` — on
Vite, Rolldown, Vitest, Oxlint and Oxfmt, with one configuration file. Its migrator (`vp migrate`) expects a Vite 7+
project, so the move is done in steps: Vue CLI → Vite, then Vite → Vite+.

The same move organizes `web/app` like the frontend of [ad-manager](https://github.com/jniltinho/ad-manager/tree/main/frontend/src),
which offers many themes: Tailwind 4 with a **role contract** of colours in `src/style.css` and one file per theme in
`src/themes/<id>.css` that only redefines roles under `:root[data-theme='<id>']`. Today Go Uptime has three themes whose
colours are spread over `dark:` variants in every component and a rule kept in three places; adding a fourth means
touching every screen.

## What Changes

- **Step 1, Vue CLI → Vite:** `@vitejs/plugin-vue` replaces `@vue/cli-service`, `@vue/cli-plugin-*`, Babel and
  `core-js`; `vite.config.js` replaces `vue.config.js` and `babel.config.js`; `index.html` moves to the root of
  `web/app` with its Go template placeholders untouched; `process.env.BASE_URL` becomes `import.meta.env.BASE_URL`.
- **The output contract with the Go binary does not change:** `web/static/index.html` (the Go template),
  `js/app.js`, `css/app.css`, `fonts/<versioned name>.woff2` and the files of `public/` at the same paths, without
  content hashes (the static middleware sends no `Cache-Control`, and the names of the fonts carry their version).
  `js/chunk-vendors.js` goes away (one bundle, or named chunks under `js/`); `web/static_test.go` is updated to what is
  required, not to an incidental file list.
- **Step 2, Vite → Vite+:** `vite-plus` is a pinned devDependency (never `curl | bash` in CI); `vp migrate` is run
  once and its output reviewed. The scripts become `vp dev`, `vp build`, `vp check` and `vp test`.
- **Unit tests:** the `node:test` suites of `src/utils/*.test.mjs` run under `vp test` (Vitest), with the same file
  names; the assertion API stays `node:assert`.
- **Lint and format:** `vp check` (Oxlint, Oxfmt and type checking); ESLint with `eslint-plugin-vue` stays only for
  the Vue templates, which Oxlint does not lint yet.
  The first run only reports; formatting the whole tree is a commit of its own.
- **Makefile and CI:** `make frontend-install`, `frontend-build`, `frontend-dev` keep their names and call `vp`;
  `make frontend-check` and `make frontend-test` are added and run in `.github/workflows/ci.yml`. The dev server keeps
  port 8081 and the proxy of `/api` to `localhost:8080`.
- **Step 3, Tailwind 3.1.8 → Tailwind 4** through `@tailwindcss/vite`, with the configuration in CSS (`@theme`),
  like ad-manager. `tailwind.config.js` and `postcss.config.js` go away.
- **Step 4, themes like ad-manager:**
  - `src/style.css` holds the role contract (`@theme static`: `--color-fg`, `--color-page`, `--color-surface`,
    `--color-line`, `--color-field`, status roles, chart roles…), whose defaults are today's light theme, pixel for
    pixel; components use role utilities (`text-fg`, `bg-surface`, `border-line`) instead of `dark:` pairs and literals.
  - `src/themes/<id>.css` per theme: `light.css`, `dark.css`, `bio.css` first (the three current themes, identical
    colours), each declaring its `color-scheme`; `src/themes/index.js` is the registry (id, label, scheme, theme
    colour) read by the selector, by `utils/theme.js` and — through a generated JSON — by the server.
  - The root carries `data-theme="<id>"` (and `data-scheme="light|dark"`); the `dark:` variant is redefined as
    `[data-scheme=dark]` so components not yet migrated keep working. The `dark` and `theme-bio` classes stay for the
    whole 7.x series for `ui.custom-css` written against them.
  - New themes become one CSS file plus one registry line; this change ships only the three existing ones.
- **Layout of `src/`** follows ad-manager where it helps: `views/` → `pages/`, `style.css` at the root of `src/`,
  `themes/`; `components/` and the tested modules of `utils/` stay.
- **Large screens** use the rule of ad-manager for the whole interface: zoom 1.25 from 2400 CSS pixels and 1.6 from
  3200 (QHD and 4K without OS scaling), with a measured `--view-h` instead of `100dvh`.
- **Node:** the version is pinned (`.node-version` / `engines`), the one CI uses.
- Apart from the large-screen zoom, no change to what a user sees: the interface is byte-for-byte the same features; `test/e2e/*.sh`, the HTTP contract
  and `docs/screenshots/capture.sh` must pass unchanged.

## Capabilities

### New Capabilities
- `frontend-toolchain`: how the Vue frontend is built, checked and tested, and the output contract between the build
  and the embedded files of the Go binary.
- `frontend-themes`: the role contract, the theme files and registry, the `data-theme` attribute and how the server,
  the inline script and `utils/theme.js` agree on it.

### Modified Capabilities
- `ui-theme`: the themes come from a registry instead of a fixed list of three; `ui.default-theme` and the cookie
  accept any id of the registry; the root carries `data-theme`/`data-scheme` besides the legacy classes.

## Impact

- `web/app/package.json`, `package-lock.json`, `vite.config.js` (new), `vue.config.js`, `babel.config.js` and the
  ESLint configuration (removed), `web/app/index.html` (moved from `public/`), `src/router/index.js`, imports without
  extension resolved through `resolve.extensions`.
- `web/static_test.go`, `Makefile`, `.github/workflows/ci.yml`, `Dockerfile` (if it builds the frontend), `AGENTS.md`
  (commands), `docs/` where the build is described.
- Themes: `internal/config/ui` (`ThemeClass`, `ThemeColor`), `internal/api/spa_render.go`, the inline script of
  `index.html`, `utils/theme.js`, `utils/theme.cases.json`, `ThemeSelector.vue`, every component that uses `dark:` or
  literal greys (migrated progressively), `test/e2e/theme-colors.sh`.
- Development: `make frontend-dev` still serves on 8081 with the proxy; `ENVIRONMENT=dev` CORS unchanged.
