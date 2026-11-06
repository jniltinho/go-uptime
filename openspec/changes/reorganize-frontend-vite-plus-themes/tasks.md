## 1. Vue CLI → Vite

- [ ] 1.1 Collect the references: `test/e2e/theme-colors.sh` output and screenshots of `docs/screenshots/capture.sh`
- [ ] 1.2 Add `vite` and `@vitejs/plugin-vue`; write `vite.config.js` (alias `@`, `.vue` extension, outDir `../static`, stable names, `cssCodeSplit: false`, dev port 8081 and proxy)
- [ ] 1.3 Move `public/index.html` to `web/app/index.html` with the entry `<script type="module" src="/src/main.js">`; `pre`/`post` plugin that protects the placeholder links and `/css/custom.css`
- [ ] 1.4 Replace `process.env.BASE_URL` by `import.meta.env.BASE_URL`
- [ ] 1.5 Remove Vue CLI, Babel, `core-js`, `vue.config.js`, `babel.config.js`
- [ ] 1.6 Placeholder test in `web/static_test.go`; update the required files (no `chunk-vendors.js`)
- [ ] 1.7 `make frontend-build`, `make test`, unit tests, every `test/e2e/*.sh`, `theme-colors.sh --compare`

## 2. Vite → Vite+

- [ ] 2.1 Add `vite-plus` pinned; run `vp migrate --no-interactive`; resolve BLOCK and review REVIEW findings
- [ ] 2.2 Scripts `dev`, `build`, `check`, `test` through `vp`; Makefile targets, including `frontend-check` and `frontend-test`
- [ ] 2.3 Convert `src/utils/*.test.mjs` to Vitest, keeping `node:assert`
- [ ] 2.4 `vp check` for scripts, format and types; ESLint + `eslint-plugin-vue` kept for templates; format the tree in a commit of its own
- [ ] 2.5 Pin Node (`.node-version`, `engines`); CI runs `npm ci`, `vp check` and `vp test`

## 3. Tailwind 4

- [ ] 3.1 `@tailwindcss/vite` and `@import 'tailwindcss'`; move `tailwind.config.js` to `@theme` in `src/style.css`
- [ ] 3.2 Pin the v3 defaults that changed (border colour, ring, placeholder); `theme-colors.sh --compare`

## 4. Themes like ad-manager

- [ ] 4.1 Role contract in `src/style.css` (`@theme static`) with the light theme as defaults
- [ ] 4.2 `src/themes/light.css`, `dark.css`, `bio.css` and the registry `src/themes/index.js`; `web/static/themes.json`
- [ ] 4.3 `data-theme` and `data-scheme` on the root from the server, the inline script and `utils/theme.js`; `theme.cases.json` extended; legacy classes kept
- [ ] 4.4 `dark:` variant on `data-scheme`; migrate components to role utilities screen by screen, comparing colours
- [ ] 4.5 `views/` → `pages/`; router and imports
- [ ] 4.5b `ResponseTimeChart.vue` observes `data-theme`; unit test switching two light themes
- [ ] 4.5c Bio header roles in an inner scope of `themes/bio.css`; `theme-colors.sh --contrast`
- [ ] 4.5d `internal/config/ui` validates `ui.default-theme` against `themes.json`; delta of `ui-theme` applied
- [ ] 4.6 Root zoom for QHD/4K with `--z` and measured `--view-h`; remove `.public-scale`
- [ ] 4.7 Document how to add a theme (one CSS file and one registry line)

## 5. Documentation

- [ ] 5.1 Update `AGENTS.md` (commands), `docs/` and the README where the build is described
- [ ] 5.2 Release notes: the legacy bundle of `--modern` is gone; required Node version
