// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/jniltinho/go-uptime/v7/internal/config/ui"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	static "github.com/jniltinho/go-uptime/v7/web"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// SinglePageApplication serves the single page application of the dashboard. The template is parsed once, when the
// router is created, and not on every request as it used to be.
//
// It returns the handler of GET and HEAD /, /endpoints/:key and /suites/:key, of /login when security.basic is used
// without OIDC, and, when the administration is enabled, of /admin, /admin/endpoints/new,
// /admin/endpoints/:endpointKey/edit, /admin/status-pages, /admin/status-pages/new, /admin/status-pages/:slug/edit,
// /admin/push-keys and /admin/backup. The path parameters are only read by the frontend.
//
// Authentication: none: the HTML is public, and the frontend asks the API.
// Request: the optional cookie theme (dark or light) picks the theme rendered in the page; without a valid one, the
// theme is the one of ui.dark-mode.
// Responses: 200 with the rendered index.html as text/html; 500 as text/plain when the template cannot be parsed or
// executed, which the validation of the configuration prevents.
func SinglePageApplication(uiConfig *ui.Config) echo.HandlerFunc {
	t, parseErr := template.ParseFS(static.FileSystem, static.IndexPath)
	return func(c *echo.Context) error {
		if parseErr != nil {
			// This should never happen, because ui.ValidateAndSetDefaults validates that the template works.
			logr.Errorf("[api.SinglePageApplication] Failed to parse template. This should never happen, because the template is validated on start. Error: %s", parseErr.Error())
			return httpx.SendString(c, 500, "Failed to parse template. This should never happen, because the template is validated on start.")
		}
		// Fork: the same theme rules as the public status pages, see themeFromRequest
		vd := ui.NewViewData(uiConfig, themeFromRequest(c, uiConfig))
		// Rendered into a buffer: once the first byte is written the answer cannot become an error anymore
		var body bytes.Buffer
		if err := t.Execute(&body, vd); err != nil {
			// This should never happen, because ui.ValidateAndSetDefaults validates that the template works.
			logr.Errorf("[api.SinglePageApplication] Failed to execute template. This should never happen, because the template is validated on start. Error: %s", err.Error())
			return httpx.SendString(c, 500, "Failed to parse template. This should never happen, because the template is validated on start.")
		}
		setThemedHTMLHeaders(c)
		return httpx.Send(c, 200, body.Bytes())
	}
}
