## Context

`web/app` is Vue 3 (`<script setup>`) with Tailwind 3.1.8 and PostCSS, built by Vue CLI 5 (`vue-cli-service build
--modern`) into `../static`, which `web/static.go` embeds with `//go:embed static`. The Go server renders
`static/index.html` as a `html/template` (theme class, `window.config`, favicons, title), serves the other files under
`/`, and `ui.custom-css` is served at `/css/custom.css`, a path that does not exist at build time. The fonts are emitted
to `fonts/inter-4-1-*.woff2` and must keep that name (AGENTS.md, "Font of the interface"). Unit tests are `node:test`
files run by `node --test`.

Vite+ 1.0 bundles Vite (Rolldown), Vitest, Oxlint, Oxfmt and a task runner behind the `vp` CLI. `vp migrate` converts
a Vite 7+ project; it does not convert Vue CLI.

## Goals / Non-Goals

**Goals:** a maintained, faster toolchain; one config; identical output contract with the Go binary; same commands in
the Makefile; CI checking lint, format and unit tests of the frontend.

**Non-Goals:** TypeScript, new themes (only the structure that makes them one file each), changing the look, splitting the bundle by route, a Vite+ global
install on developer machines (it stays a devDependency).

## Decisions

1. **Two steps, two commits.** Vue CLI → Vite first (reviewable on its own, testable with the e2e scripts), then
   `vp migrate`. Alternative: hand-writing a Vite+ config from Vue CLI — rejected, the migrator is the supported path.
2. **`index.html` is a Go template and must survive the build.** Vite parses `index.html`; the placeholders are in
   attributes and inline scripts, which it copies as text. `/css/custom.css` and the favicon links built from
   placeholders must not be resolved: a plugin with a `transformIndexHtml` hook of `order: 'pre'` swaps the `<link>`
   tags whose `href` is a `{{ ... }}` placeholder or `/css/custom.css` for unique markers **before** Vite resolves the
   assets, and a hook of `order: 'post'` puts them back, and the build fails if any `{{ ... }}` of the source is missing from the output (a test in
   `web/static_test.go` compares the placeholders of `web/app/index.html` with the embedded file).
3. **Stable file names.** `build.rollupOptions.output`: `entryFileNames: 'js/app.js'`, `chunkFileNames: 'js/[name].js'`,
   `assetFileNames` → `css/app.css` for the CSS and `fonts/[name][extname]` for the fonts; `build.cssCodeSplit: false`;
   `build.outDir: '../static'`, `emptyOutDir: true`; `publicDir` keeps `public/` (icons, manifest, `fonts/OFL.txt`).
   No hashes, because the static middleware sends no `Cache-Control` and the font names are versioned by hand.
4. **Browser targets.** `--modern` of Vue CLI produced a modern bundle plus a legacy one. The legacy bundle is dropped:
   the default Vite target (baseline widely available) covers every browser the interface already requires
   (`crypto.subtle`, `EventSource`, CSS `zoom`). Documented in the release notes.
5. **Imports without extension.** `resolve.extensions` adds `.vue` (and keeps `.js`), and `@` aliases `src`, so no
   source file changes in step 1 except `process.env.BASE_URL`.
6. **Tests.** Vitest does not run `node:test` files, so they are converted mechanically: `import { test } from
   'vitest'`, with `node:assert` kept, a few lines per file. The test that extracts the inline script of `index.html`
   reads the new path.
7. **Lint.** Oxlint lints the `<script>` blocks but not the Vue templates, so ESLint with `eslint-plugin-vue`
   (`vue3-essential`) stays for the templates, run by `make frontend-check` next to `vp check` (Oxlint, Oxfmt and type
   check) until Oxlint covers templates. The first formatting is a
   separate commit so that blame stays readable; CI runs `vp check` without `--fix`.
8. **Pinning.** `vite-plus` pinned to an exact version in `devDependencies`; Node pinned in `.node-version` and
   `engines`; CI uses `npm ci` and `npx vp`. Never `curl | bash` in CI (supply chain).

9. **Role contract, as in ad-manager.** Components never use a palette value or a `dark:` pair for a role; they use
   the role utilities that `@theme static` generates. The defaults of every role are the current light theme, so the
   contract can land before any component moves. Status colours stay literal (a status has the same colour in every
   theme — the rule of `statusgray-*` today) and become roles that themes are not expected to change.
10. **One registry, three readers.** `src/themes/index.js` lists `{ id, label, scheme, themeColor }`. The build writes
   `web/static/themes.json`, embedded with the rest; `internal/config/ui` validates `ui.default-theme` against it and
   `spa_render.go` renders `data-theme`, `data-scheme`, the legacy class and `theme-color`. The inline script and
   `utils/theme.js` read the registry inlined at build time. `theme.cases.json` stays the shared test table of the three.
10b. **Inner scopes of a theme.** A theme file may redefine roles inside a scope of its own under its root selector,
   such as `:root[data-theme='bio'] .app-header` for the navy header of Bio (text, menus and focus over navy), which is
   what `index.css` does today; the contrast test of `theme-colors.sh --contrast` covers it.
10c. **Canvas.** `ResponseTimeChart.vue` observes the `class` of `<html>` today; it observes `data-theme` (and
   `data-scheme`) instead and uses the theme id as its reactive dependency, with a unit test switching between two
   themes of the same scheme.
11. **`dark:` during the migration.** `@custom-variant dark (&:where([data-scheme=dark], [data-scheme=dark] *))` keeps
   every existing `dark:` class working while components move to roles one screen at a time; `theme-colors.sh
   --compare` proves each screen unchanged.
12. **Zoom for QHD/4K on the whole interface.** `:root { zoom }` at 2400 and 3200 CSS pixels, `--z` and a measured
   `--view-h` (engines disagree on whether `dvh` follows zoom), as ad-manager. The `.screen-scale` class
   of `add-status-page-views` (on `PublicLayout.vue` and on the administration root in `App.vue`) is removed in the same
   commit, or the two zooms would multiply; `--screen-zoom` and the height `calc(100dvh / var(--screen-zoom))` of the
   administration lists become `--z` and `var(--view-h)`.

## Risks / Trade-offs

- [The template placeholders are mangled by the HTML transform] → the placeholder test of decision 2, plus
  `TestHTTPContract` and the e2e theme tests, which render the template.
- [Font paths change and `TestFontsAreServed` / `web/static_test.go` fail] → decision 3; the tests stay as the safety net.
- [Vite+ is young (1.0)] → step 1 alone is already a supported Vite setup; rolling back step 2 is reverting one commit.
- [Different minification changes timing-sensitive e2e tests] → run every `test/e2e/*.sh` and `capture.sh` before merging.
- [Tailwind 4 changes defaults (border colour, ring width, placeholder colour)] → set them in the contract to the v3
  values; `theme-colors.sh --compare` on all 24 screens and states.
- [`ui.custom-css` written against `.dark`/`.theme-bio`] → the classes stay through 7.x; documented deprecation.
- [Large diff touching every component] → steps 3 and 4 land screen by screen, each with its colour comparison.
- [Contributors with older Node] → `engines` and `.node-version`; `make frontend-install` prints the required version.

## Migration Plan

1. Step 1 on its branch: Vite config, move `index.html`, update tests and Makefile; run `make frontend-build`,
   `make test`, `npm run test:unit`, every e2e script and `test/e2e/theme-colors.sh --compare` against a reference
   collected before the change.
2. Step 2: add `vite-plus`, run `vp migrate --no-interactive`, resolve BLOCK findings, review REVIEW ones, switch the
   scripts, convert the tests, add `frontend-check`/`frontend-test` to CI.
3. Tailwind 4, with the v3 defaults pinned in CSS; compare colours.
4. Role contract and theme files (light, dark, bio); registry and `themes.json`; server and inline script read it;
   then components move to roles screen by screen; `views/` → `pages/`; root zoom.
5. Rollback: revert the commits; the Go side never depends on the toolchain, only on `web/static`.

## Open Questions

- Whether to keep `js/chunk-vendors.js` as a named chunk for cache reuse across releases (no hashes means no long
  cache anyway), or ship a single `js/app.js`. Proposed: single bundle.
