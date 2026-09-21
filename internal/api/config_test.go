package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/security"

	"github.com/labstack/echo/v5"
)

func TestConfigHandler_ServeHTTP(t *testing.T) {
	securityConfig := &security.Config{
		OIDC: &security.OIDCConfig{
			IssuerURL:       "https://sso.gatus.io/",
			RedirectURL:     "http://localhost:80/authorization-code/callback",
			Scopes:          []string{"openid"},
			AllowedSubjects: []string{"user1@example.com"},
		},
	}
	handler := ConfigHandler{securityConfig: securityConfig}
	// Create a fake router. We're doing this because I need the gate to be initialized.
	// The route is public and the security middleware is on a group of its own: with Echo a middleware of the router
	// covers every route, whatever the order in which they were registered
	app := echo.New()
	app.GET("/api/v1/config", handler.GetConfig)
	err := securityConfig.ApplySecurityMiddleware(app.Group("/protected"))
	if err != nil {
		t.Error("expected err to be nil, but was", err)
	}
	// Test the config handler
	request := httptest.NewRequest("GET", "/api/v1/config", http.NoBody)
	response, err := testHTTP(app, request)
	if err != nil {
		t.Error("expected err to be nil, but was", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Error("expected code to be 200, but was", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Error("expected err to be nil, but was", err)
	}
	if string(body) != `{"announcements":[],"authenticated":false,"login":"oidc","oidc":true}` {
		t.Error("expected body to be `{\"announcements\":[],\"authenticated\":false,\"login\":\"oidc\",\"oidc\":true}`, but was", string(body))
	}
}
