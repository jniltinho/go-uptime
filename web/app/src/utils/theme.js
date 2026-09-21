// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Theme of the web interface (fork): used by the settings of the dashboard, the public status pages and the login
// screen. The same rule is applied by the server (internal/config/ui and internal/api) and by the inline script of
// public/index.html before the first paint; the three are tested with theme.cases.json.
//
// - A valid theme cookie (dark, light or bio) wins.
// - Otherwise the default theme of the server (ui.default-theme, or ui.dark-mode) is used, from the data-default-theme
//   attribute of <html>.
// - The preference of the operating system (prefers-color-scheme) is not used.
const THEME_COOKIE_NAME = 'theme'
const THEME_COOKIE_MAX_AGE = 31536000 // 1 year

// The themes, in the order of the theme selector. The classes of <html> are mutually exclusive: the light theme has none.
export const THEMES = Object.freeze({
  light: Object.freeze({ label: 'Light', className: '', themeColor: '#f7f9fb' }),
  dark: Object.freeze({ label: 'Dark', className: 'dark', themeColor: '#030712' }),
  bio: Object.freeze({ label: 'Bio', className: 'theme-bio', themeColor: '#f2f8fa' })
})

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

// applyTheme sets the class of <html>, removing the one of any other theme, and the theme-color of the browser. It
// tells the components that draw with the colours of the theme (the response time chart) that the theme changed.
export const applyTheme = (theme) => {
  const selected = isTheme(theme) ? theme : 'light'
  const classList = document.documentElement.classList
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
