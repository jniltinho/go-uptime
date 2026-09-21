package security

import (
	"net/http"

	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/labstack/echo/v5"
)

// RequestAuthor returns the identity of the authenticated user of the request: the OIDC subject of its session or,
// with basic authentication, the username. It returns an empty string when the request is not authenticated.
func (c *Config) RequestAuthor(ctx *echo.Context) string {
	if c == nil {
		return ""
	}
	if c.OIDC != nil {
		subject, _ := c.sessionSubject(ctx)
		return subject
	}
	if username, ok := ctx.Get(localsUsername).(string); ok {
		return username
	}
	return ""
}

// IsAdmin returns whether the request is authorized to use the administration of endpoints.
//
// With OIDC, the subject of the session must be in admin.allowed-subjects. With basic authentication only, the single
// basic user is an administrator, so this returns whether the request is authenticated by a login session or by
// Authorization: Basic, reusing the authentication of the request when it was already done.
func (c *Config) IsAdmin(ctx *echo.Context, adminConfig *admin.Config) bool {
	if c == nil || !adminConfig.IsEnabled() {
		return false
	}
	if c.OIDC != nil {
		subject, ok := c.sessionSubject(ctx)
		return ok && adminConfig.IsSubjectAllowed(subject)
	}
	return c.Basic != nil && c.authenticateBasic(ctx).authenticated
}

// AdminMiddleware returns a middleware responding with 403 to requests that are not authorized to use the
// administration. It must be registered after ApplySecurityMiddleware, which responds with 401 to unauthenticated
// requests.
func (c *Config) AdminMiddleware(adminConfig *admin.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx *echo.Context) error {
			if !c.IsAdmin(ctx, adminConfig) {
				return httpx.JSON(ctx, http.StatusForbidden, map[string]string{"error": "administrator permission required"})
			}
			return next(ctx)
		}
	}
}

// sessionSubject returns the OIDC subject of the session of the request, if there is a valid session
func (c *Config) sessionSubject(ctx *echo.Context) (string, bool) {
	if c.gate == nil {
		return "", false
	}
	value, exists := sessions.Get(c.gate.ExtractTokenFromRequest(ctx.Request()))
	if !exists {
		return "", false
	}
	subject, ok := value.(string)
	return subject, ok
}
