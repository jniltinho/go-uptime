// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// The registry of the themes of the interface (fork), in the order of the theme selector. It is the only list of themes:
// utils/theme.js reads it, the build injects it into the inline script of index.html and copies it to
// web/static/themes.json, which the server reads (internal/config/ui). A theme is this entry plus its file
// themes/<id>.css, imported by style.css.
//
// - id: the value of the theme cookie, of ui.default-theme and of data-theme on <html>
// - scheme: light or dark, the value of data-scheme on <html>, which the dark: variant follows
// - className: the class of <html> of the 7.x series (dark, theme-bio), kept for the ui.custom-css written against it
// - themeColor: the theme-color of the browser
import registry from './themes.json'

export const THEME_REGISTRY = Object.freeze(registry.map((theme) => Object.freeze({ ...theme })))
