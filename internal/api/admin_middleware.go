package api

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"

	"github.com/labstack/echo/v5"
)

const (
	// adminMaximumBodySize is the maximum size of the body of requests to the administration API
	adminMaximumBodySize = 256 * 1024

	// adminDevServerOrigin is the origin of the Vue development server, accepted when ENVIRONMENT=dev
	adminDevServerOrigin = "http://localhost:8081"
)

// adminMediaTypes are the media types accepted in the Content-Type of a request with a body to the administration.
var adminMediaTypes = []string{"application/json", "application/yaml", "application/x-yaml", "text/yaml"}

// adminRequestProtection protects the requests that change managed endpoints against CSRF and oversized bodies.
//
// The expected origin is derived only from the Host header and the scheme (TLS or X-Forwarded-Proto). Pages cannot
// set Host, Origin or Sec-Fetch-* and a cross-site request with a custom header such as X-Forwarded-Proto requires a
// CORS preflight that Gatus does not allow, so this is safe against CSRF. X-Forwarded-Host is ignored on purpose, and
// so is Echo's Scheme(), which trusts any X-Forwarded-* header.
//
// GET, HEAD and OPTIONS pass untouched. For POST, PUT, PATCH and DELETE it answers, with the body {"error": "..."}: 403
// when Sec-Fetch-Site is cross-site; 403 when the Origin header or, without it, the origin of the Referer is present and
// is neither one of admin.allowed-origins nor, when that list is empty, the origin derived from the request; 413 when
// the body exceeds adminMaximumBodySize, except on the restore routes; 415 when there is a body and the Content-Type is
// not one of adminMediaTypes. A request without Origin and without Referer is accepted.
func adminRequestProtection(adminConfig *admin.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			switch c.Request().Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				return next(c)
			}
			if strings.EqualFold(httpx.Header(c, "Sec-Fetch-Site"), "cross-site") {
				return adminError(c, http.StatusForbidden, "cross-site requests are not allowed")
			}
			if origin, present := requestOrigin(c); present && !isAllowedAdminOrigin(c, origin, adminConfig) {
				return adminError(c, http.StatusForbidden, "origin is not allowed: "+origin)
			}
			body := httpx.Body(c)
			// Fork: the restore routes accept a backup file, checked by their handler (see api/admin_backup.go)
			if len(body) > adminMaximumBodySize && !isAdminRestorePath(httpx.Path(c)) {
				return adminError(c, http.StatusRequestEntityTooLarge, "request body is too large")
			}
			if len(body) > 0 && !isAdminMediaType(httpx.Header(c, echo.HeaderContentType)) {
				return adminError(c, http.StatusUnsupportedMediaType, "content type must be application/json or application/yaml")
			}
			return next(c)
		}
	}
}

// adminError answers the given status with the JSON body {"error": message}, the error format of the administration
// and of the login routes.
func adminError(c *echo.Context, status int, message string) error {
	return httpx.JSON(c, status, map[string]any{"error": message})
}

// requestOrigin returns the Origin of the request or, without it, the origin of the Referer
func requestOrigin(c *echo.Context) (string, bool) {
	if origin := strings.TrimSpace(httpx.Header(c, echo.HeaderOrigin)); len(origin) > 0 {
		return normalizeOrigin(origin), true
	}
	if referer := strings.TrimSpace(httpx.Header(c, "Referer")); len(referer) > 0 {
		parsed, err := url.Parse(referer)
		if err != nil || len(parsed.Scheme) == 0 || len(parsed.Host) == 0 {
			return referer, true
		}
		return normalizeOrigin(parsed.Scheme + "://" + parsed.Host), true
	}
	return "", false
}

// isAllowedAdminOrigin returns whether a normalized origin can change the administration: the Vue development server
// when ENVIRONMENT=dev, then one of admin.allowed-origins when the list is not empty, otherwise only the origin derived
// from the Host header and the scheme of the request.
func isAllowedAdminOrigin(c *echo.Context, origin string, adminConfig *admin.Config) bool {
	if os.Getenv("ENVIRONMENT") == "dev" && origin == adminDevServerOrigin {
		return true
	}
	if adminConfig != nil && len(adminConfig.AllowedOrigins) > 0 {
		for _, allowedOrigin := range adminConfig.AllowedOrigins {
			if normalizeOrigin(allowedOrigin) == origin {
				return true
			}
		}
		return false
	}
	return origin == derivedOrigin(c)
}

// derivedOrigin returns the origin of the request from its Host header and scheme
func derivedOrigin(c *echo.Context) string {
	scheme := "http"
	if httpx.IsTLS(c) {
		scheme = "https"
	} else if forwardedProto, _, _ := strings.Cut(httpx.Header(c, echo.HeaderXForwardedProto), ","); slices.Contains([]string{"http", "https"}, strings.ToLower(strings.TrimSpace(forwardedProto))) {
		scheme = strings.ToLower(strings.TrimSpace(forwardedProto))
	}
	return normalizeOrigin(scheme + "://" + c.Request().Host)
}

// normalizeOrigin lowercases an origin and removes the default port of its scheme
func normalizeOrigin(origin string) string {
	origin = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
	if strings.HasPrefix(origin, "http://") {
		return strings.TrimSuffix(origin, ":80")
	}
	if strings.HasPrefix(origin, "https://") {
		return strings.TrimSuffix(origin, ":443")
	}
	return origin
}

// isAdminMediaType returns whether the Content-Type, without its parameters and in any case, is one of adminMediaTypes.
func isAdminMediaType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && slices.Contains(adminMediaTypes, strings.ToLower(mediaType))
}
