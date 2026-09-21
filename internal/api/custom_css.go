// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"github.com/jniltinho/go-uptime/v7/internal/httpx"

	"github.com/labstack/echo/v5"
)

// CustomCSSHandler serves the custom CSS of the configuration (ui.custom-css).
type CustomCSSHandler struct {
	customCSS string
}

// GetCustomCSS handles GET and HEAD /css/custom.css: it returns the custom CSS of the configuration (ui.custom-css).
//
// Authentication: none.
// Responses: 200 with the CSS as text/css; the body is empty when no custom CSS is configured.
func (handler CustomCSSHandler) GetCustomCSS(c *echo.Context) error {
	httpx.SetHeader(c, "Content-Type", "text/css")
	return httpx.SendString(c, 200, handler.customCSS)
}
