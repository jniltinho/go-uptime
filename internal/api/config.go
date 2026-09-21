package api

import (
	"encoding/json"
	"fmt"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/security"

	"github.com/labstack/echo/v5"
)

// ConfigHandler serves the part of the configuration the frontend needs before anything else: the login method, whether
// the request is authenticated, the state of the administration and the announcements.
type ConfigHandler struct {
	securityConfig *security.Config
	config         *config.Config
}

// GetConfig handles GET and HEAD /api/v1/config: it tells the frontend how to log in, whether the request is
// authenticated, whether it can use the administration, and the announcements.
//
// Authentication: none (public group); the authentication of the request, if any, is only reported. With
// security.basic, a wrong Authorization: Basic header counts as a failure for the limiter of the client, but the
// response is still 200.
// Responses: 200 with the JSON object {"oidc": bool, "login": "basic"|"oidc"|"", "authenticated": bool (true without
// security), "admin": {"enabled": bool, "authorized": bool}, "announcements": array of announcement.Announcement,
// empty when there is none}; "admin" is absent when the handler has no configuration, which only happens in tests. 500
// with {"error": "..."} when the response cannot be encoded.
func (handler ConfigHandler) GetConfig(c *echo.Context) error {
	hasOIDC := false
	login := ""
	isAuthenticated := true // Default to true if no security config is set
	if handler.securityConfig != nil {
		hasOIDC = handler.securityConfig.OIDC != nil
		// Login method of the frontend (fork): "basic" shows the login screen, "oidc" the OIDC login
		login = handler.securityConfig.LoginMethod()
		// With security.basic, the request is authenticated once: IsAdmin reuses this result, so that a wrong password
		// counts as a single failure and is checked once
		isAuthenticated = handler.securityConfig.IsAuthenticated(c)
	}

	// Prepare response with announcements
	response := map[string]interface{}{
		"oidc":          hasOIDC,
		"login":         login,
		"authenticated": isAuthenticated,
	}
	// Administration of endpoints (fork): whether it is enabled and whether the current request can use it
	if handler.config != nil {
		response["admin"] = map[string]bool{
			"enabled":    handler.config.Admin.IsEnabled(),
			"authorized": handler.securityConfig.IsAdmin(c, handler.config.Admin),
		}
	}
	// Add announcements if available, otherwise use empty slice
	if handler.config != nil && handler.config.Announcements != nil && len(handler.config.Announcements) > 0 {
		response["announcements"] = handler.config.Announcements
	} else {
		response["announcements"] = []interface{}{}
	}

	// Return the config as JSON
	httpx.SetHeader(c, "Content-Type", "application/json")
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return httpx.SendString(c, 500, fmt.Sprintf(`{"error":"Failed to marshal response: %s"}`, err.Error()))
	}
	return httpx.Send(c, 200, responseBytes)
}
