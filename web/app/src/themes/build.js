// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Vite plugins that hand the registry of the themes (themes.json) to the two readers outside of the bundle (fork):
// the inline script of index.html, which applies the theme before the first paint, and the server, which reads
// web/static/themes.json to render the theme of the cookie and to validate ui.default-theme.
import { readFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'

// THEMES_MARKER is the placeholder of the registry in the inline script of index.html
export const THEMES_MARKER = '/* go-uptime:themes */ null'

const readRegistry = () => JSON.parse(readFileSync(fileURLToPath(new URL('./themes.json', import.meta.url)), 'utf8'))

// inlineRegistry returns the registry in the form the inline script reads: { id: [className, themeColor, scheme] }
export const inlineRegistry = (registry) =>
  JSON.stringify(Object.fromEntries(registry.map((theme) => [theme.id, [theme.className, theme.themeColor, theme.scheme]])))

// injectThemes replaces the marker of the inline script, in the build and in the development server
export function injectThemes() {
  return {
    name: 'go-uptime-inject-themes',
    transformIndexHtml: {
      order: 'pre',
      handler(html) {
        if (!html.includes(THEMES_MARKER)) {
          throw new Error(`index.html has no ${THEMES_MARKER} for the registry of the themes`)
        }
        return html.replace(THEMES_MARKER, inlineRegistry(readRegistry()))
      },
    },
  }
}

// themesFile writes themes.json next to index.html in the build
export function themesFile() {
  return {
    name: 'go-uptime-themes-file',
    apply: 'build',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'themes.json', source: JSON.stringify(readRegistry(), null, 2) + '\n' })
    },
  }
}
