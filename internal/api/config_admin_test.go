// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/security"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

func TestConfigHandler_Admin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	basicConfig := func() *security.Config {
		return &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}}
	}
	scenarios := []struct {
		name                  string
		securityConfig        *security.Config
		adminConfig           *admin.Config
		withCredentials       bool
		expectedEnabled       bool
		expectedAuthorized    bool
		expectedAuthenticated bool
		expectedLogin         string
	}{
		{name: "admin-disabled", securityConfig: basicConfig(), withCredentials: true, expectedAuthenticated: true, expectedLogin: "basic"},
		{name: "basic-only-without-credentials", securityConfig: basicConfig(), adminConfig: &admin.Config{Enabled: true}, expectedEnabled: true, expectedLogin: "basic"},
		{name: "basic-only-with-credentials", securityConfig: basicConfig(), adminConfig: &admin.Config{Enabled: true}, withCredentials: true, expectedEnabled: true, expectedAuthorized: true, expectedAuthenticated: true, expectedLogin: "basic"},
		{name: "oidc-without-session", securityConfig: &security.Config{OIDC: &security.OIDCConfig{IssuerURL: "https://sso.example.com/", RedirectURL: "http://localhost/authorization-code/callback", Scopes: []string{"openid"}}}, adminConfig: &admin.Config{Enabled: true, AllowedSubjects: []string{"ops@example.com"}}, expectedEnabled: true, expectedLogin: "oidc"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &config.Config{Security: scenario.securityConfig, Admin: scenario.adminConfig}
			app := echo.New()
			app.GET("/api/v1/config", ConfigHandler{securityConfig: cfg.Security, config: cfg}.GetConfig)
			if err := cfg.Security.ApplySecurityMiddleware(app.Group("/protected")); err != nil {
				t.Fatalf("failed to apply security middleware: %v", err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/config", http.NoBody)
			if scenario.withCredentials {
				request.SetBasicAuth("admin", "secret")
			}
			response, err := testHTTP(app, request)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer response.Body.Close()
			var body struct {
				Login         string `json:"login"`
				Authenticated bool   `json:"authenticated"`
				Admin         struct {
					Enabled    bool `json:"enabled"`
					Authorized bool `json:"authorized"`
				} `json:"admin"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("invalid response: %v", err)
			}
			if body.Admin.Enabled != scenario.expectedEnabled || body.Admin.Authorized != scenario.expectedAuthorized {
				t.Errorf("expected enabled=%v authorized=%v, got enabled=%v authorized=%v", scenario.expectedEnabled, scenario.expectedAuthorized, body.Admin.Enabled, body.Admin.Authorized)
			}
			if body.Login != scenario.expectedLogin || body.Authenticated != scenario.expectedAuthenticated {
				t.Errorf("expected login=%q authenticated=%v, got login=%q authenticated=%v", scenario.expectedLogin, scenario.expectedAuthenticated, body.Login, body.Authenticated)
			}
		})
	}
}
