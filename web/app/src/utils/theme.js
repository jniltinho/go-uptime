// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Theme of the web interface (fork): used by the settings of the dashboard, the public status pages and the login
// screen. The same rule is applied by the server (internal/config/ui and internal/api) and by the inline script of
// index.html before the first paint; the three read the registry of src/themes/themes.json and are tested with
// theme.cases.json.
//
// - A valid theme cookie (an id of the registry) wins.
// - Otherwise the default theme of the server (ui.default-theme, or ui.dark-mode) is used, from the data-default-theme
//   attribute of <html>.
// - The preference of the operating system (prefers-color-scheme) is not used.
import { THEME_REGISTRY } from '../themes/index.js'

const THEME_COOKIE_NAME = 'theme'
const THEME_COOKIE_MAX_AGE = 31536000 // 1 year

// The themes, in the order of the theme selector, from the registry (src/themes/themes.json). className is the legacy
// class of <html> (the light theme has none); the classes are mutually exclusive, like data-theme.
export const THEMES = Object.freeze(
  Object.fromEntries(
    THEME_REGISTRY.map(({ id, label, scheme, className, themeColor }) => [id, Object.freeze({ label, scheme, className, themeColor })]),
  ),
)

export const THEME_COLORS = Object.freeze(Object.fromEntries(Object.entries(THEMES).map(([theme, { themeColor }]) => [theme, themeColor])))

export const isTheme = (value) => Object.prototype.hasOwnProperty.call(THEMES, value)

// defaultTheme returns the default theme of the server. Without the attribute, or with the template not rendered
// (development server of Vue), it is dark; an empty attribute comes from the HTML of a version that only knew dark and
// light, and means light, like any value that is not a theme.
export const defaultTheme = () => {
  const root = document.documentElement
  const value = root && root.dataset ? root.dataset.defaultTheme : undefined
  if (value === undefined || value === null || value.includes('{{')) {
    return 'dark'
  }
  return isTheme(value) ? value : 'light'
}

export const defaultThemeIsDark = () => defaultTheme() === 'dark'

// themeFromCookie returns the theme of the theme cookie, or an empty string when it is absent or invalid
export const themeFromCookie = () => {
  const match = /(?:^|;\s*)theme=([^;]*)/.exec(document.cookie || '')
  const value = match ? match[1].trim() : ''
  return isTheme(value) ? value : ''
}

// currentTheme returns the theme in use: the one of the cookie, or the default one
export const currentTheme = () => themeFromCookie() || defaultTheme()

export const wantsDarkMode = () => currentTheme() === 'dark'

// applyTheme sets data-theme and data-scheme on <html>, the legacy class of the theme (removing the one of any other
// theme) and the theme-color of the browser. It tells the components that draw with the colours of the theme (the
// response time chart) that the theme changed.
export const applyTheme = (theme) => {
  const selected = isTheme(theme) ? theme : 'light'
  const root = document.documentElement
  root.dataset.theme = selected
  root.dataset.scheme = THEMES[selected].scheme
  const classList = root.classList
  for (const { className } of Object.values(THEMES)) {
    if (className) {
      classList.toggle(className, className === THEMES[selected].className)
    }
  }
  const meta = document.querySelector('meta[name="theme-color"]')
  if (meta) {
    meta.setAttribute('content', THEMES[selected].themeColor)
  }
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function' && typeof CustomEvent === 'function') {
    window.dispatchEvent(new CustomEvent('go-uptime:theme', { detail: { theme: selected } }))
  }
  return selected
}

// setTheme saves the choice of the visitor and applies it. It returns the theme now in use.
export const setTheme = (theme) => {
  const selected = isTheme(theme) ? theme : 'light'
  document.cookie = `${THEME_COOKIE_NAME}=${selected}; path=/; max-age=${THEME_COOKIE_MAX_AGE}; samesite=strict`
  return applyTheme(selected)
}

// themeKey identifies the theme applied to an element (the root by default): its data-theme and its class. Whatever
// draws with the colours of the theme (the canvas of the response time chart) redraws when it changes: it tells two
// themes of the same scheme apart (light and bio), which data-scheme or a dark class cannot.
export const themeKey = (root = document.documentElement) => `${(root.dataset && root.dataset.theme) || ''}|${root.className || ''}`
