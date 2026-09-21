// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/jniltinho/go-uptime/v7/internal/config/ui"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	static "github.com/jniltinho/go-uptime/v7/web"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// renderSPA returns a handler that always renders the single page application with status 200, from a template parsed
// once when the router is created (once per start or reload), and sets headers on every response
//
// It is the handler of GET and HEAD /status/:slug and /status/*, the HTML of the public status pages, registered behind
// statusPageHTMLAuth.
//
// Authentication: none, or HTTP Basic with the login of the page when the slug is a published page that requires one.
// Request: the path parameter slug is only read by the frontend and by statusPageHTMLAuth; the optional cookie theme
// (dark or light) picks the theme.
// Responses: 200 with the rendered index.html as text/html, Cache-Control: no-cache (private, no-store and Vary:
// Authorization for a page with a login), X-Robots-Tag: noindex, nofollow, X-Content-Type-Options: nosniff and
// Referrer-Policy: strict-origin-when-cross-origin, whether or not the page exists; 401 with WWW-Authenticate: Basic
// and 429 with Retry-After from statusPageHTMLAuth; 500 as text/plain when the template cannot be parsed or executed.
func renderSPA(uiConfig *ui.Config, headers func(c *echo.Context)) echo.HandlerFunc {
	indexTemplate, parseErr := template.ParseFS(static.FileSystem, static.IndexPath)
	return func(c *echo.Context) error {
		if parseErr != nil {
			// This should never happen, because ui.ValidateAndSetDefaults validates that the template works.
			logr.Errorf("[api.renderSPA] Failed to parse template: %s", parseErr.Error())
			return httpx.SendString(c, http.StatusInternalServerError, "Failed to parse template. This should never happen, because the template is validated on start.")
		}
		var body bytes.Buffer
		if err := indexTemplate.Execute(&body, ui.NewViewData(uiConfig, themeFromRequest(c, uiConfig))); err != nil {
			logr.Errorf("[api.renderSPA] Failed to execute template: %s", err.Error())
			return httpx.SendString(c, http.StatusInternalServerError, "Failed to execute template. This should never happen, because the template is validated on start.")
		}
		// The template depends on the theme cookie. The headers of the route come after, so that a page that requires a
		// login can keep its HTML out of any shared cache (fork).
		setThemedHTMLHeaders(c)
		headers(c)
		return httpx.Send(c, http.StatusOK, body.Bytes())
	}
}

// setThemedHTMLHeaders sets the headers of every HTML page of the interface. The HTML carries the class of the theme,
// which comes from the theme cookie: it varies with the cookie, and a cache has to revalidate it. A page that requires
// a login replaces no-cache with private, no-store afterwards.
func setThemedHTMLHeaders(c *echo.Context) {
	httpx.SetHeader(c, echo.HeaderCacheControl, "no-cache")
	httpx.Vary(c, echo.HeaderCookie)
	httpx.SetHeader(c, echo.HeaderContentType, "text/html")
}

// themeFromRequest returns the identifier of the theme of a valid theme cookie (dark, light or bio) or, without one,
// the default theme of the configuration. Fork: an invalid cookie is ignored, like in the browser (see
// web/app/src/utils/theme.js).
func themeFromRequest(c *echo.Context, uiConfig *ui.Config) string {
	if cookie, err := c.Cookie("theme"); err == nil && ui.IsTheme(cookie.Value) {
		return cookie.Value
	}
	return uiConfig.Theme()
}
