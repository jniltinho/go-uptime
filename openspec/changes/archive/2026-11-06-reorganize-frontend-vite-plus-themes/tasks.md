## 1. Vue CLI → Vite

- [x] 1.1 Collect the references: `test/e2e/theme-colors.sh` output and screenshots of `docs/screenshots/capture.sh`
- [x] 1.2 Add `vite` and `@vitejs/plugin-vue`; write `vite.config.js` (alias `@`, `.vue` extension, outDir `../static`, stable names, `cssCodeSplit: false`, dev port 8081 and proxy)
- [x] 1.3 Move `public/index.html` to `web/app/index.html` with the entry `<script type="module" src="/src/main.js">`; `pre`/`post` plugin that protects the placeholder links and `/css/custom.css`
- [x] 1.4 Replace `process.env.BASE_URL` by `import.meta.env.BASE_URL`
- [x] 1.5 Remove Vue CLI, Babel, `core-js`, `vue.config.js`, `babel.config.js`
- [x] 1.6 Placeholder test in `web/static_test.go`; update the required files (no `chunk-vendors.js`)
- [x] 1.7 `make frontend-build`, `make test`, unit tests, every `test/e2e/*.sh`, `theme-colors.sh --compare`

## 2. Vite → Vite+

- [x] 2.1 Add `vite-plus` pinned; run `vp migrate --no-interactive`; resolve BLOCK and review REVIEW findings
- [x] 2.2 Scripts `dev`, `build`, `check`, `test` through `vp`; Makefile targets, including `frontend-check` and `frontend-test`
- [x] 2.3 Convert `src/utils/*.test.mjs` to Vitest, keeping `node:assert`
- [x] 2.4 `vp check` for scripts, format and types; ESLint + `eslint-plugin-vue` kept for templates; format the tree in a commit of its own
- [x] 2.5 Pin Node (`.node-version`, `engines`); CI runs `npm ci`, `vp check` and `vp test`

## 3. Tailwind 4

- [x] 3.1 `@tailwindcss/vite` and `@import 'tailwindcss'`; move `tailwind.config.js` to `@theme` in `src/style.css`
- [x] 3.2 Pin the v3 defaults that changed (border colour, ring, placeholder); `theme-colors.sh --compare`

## 4. Themes like ad-manager

- [x] 4.1 Role contract in `src/style.css` (`@theme static`) with the light theme as defaults
- [x] 4.2 `src/themes/light.css`, `dark.css`, `bio.css` and the registry `src/themes/index.js`; `web/static/themes.json`
- [x] 4.3 `data-theme` and `data-scheme` on the root from the server, the inline script and `utils/theme.js`; `theme.cases.json` extended; legacy classes kept
- [x] 4.4 `dark:` variant on `data-scheme`
- [ ] 4.4b Migrate the components from `dark:` pairs to role utilities, screen by screen, comparing colours (follow-up)
- [x] 4.5 `views/` → `pages/`; router and imports
- [x] 4.5b `ResponseTimeChart.vue` observes `data-theme`; unit test switching two light themes
- [x] 4.5c Bio header roles in an inner scope of `themes/bio.css`; `theme-colors.sh --contrast`
- [x] 4.5d `internal/config/ui` validates `ui.default-theme` against `themes.json`; delta of `ui-theme` applied
- [x] 4.6 Root zoom for QHD/4K with `--z` and measured `--view-h`; remove `.screen-scale` from `PublicLayout.vue` and `App.vue` in the same commit (no double zoom), and replace `--screen-zoom` and `calc(100dvh / var(--screen-zoom))` by `var(--view-h)`
- [x] 4.7 Document how to add a theme (one CSS file and one registry line)

## 5. Documentation

- [x] 5.1 Update `AGENTS.md` (commands), `docs/` and the README where the build is described
- [x] 5.2 Release notes: the legacy bundle of `--modern` is gone; required Node version
