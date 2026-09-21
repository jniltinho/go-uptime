package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/admin"

	"github.com/labstack/echo/v5"
)

func TestAdminRequestProtection(t *testing.T) {
	scenarios := []struct {
		name           string
		method         string
		target         string
		body           string
		headers        map[string]string
		adminConfig    *admin.Config
		devEnvironment bool
		expectedStatus int
	}{
		{name: "get-is-not-checked", method: http.MethodGet, headers: map[string]string{"Origin": "https://site-malicioso.exemplo"}, expectedStatus: http.StatusNoContent},
		{name: "same-origin", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "http://status.example.com", "Content-Type": "application/json"}, expectedStatus: http.StatusNoContent},
		{name: "different-origin", method: http.MethodDelete, headers: map[string]string{"Origin": "https://site-malicioso.exemplo"}, expectedStatus: http.StatusForbidden},
		{name: "cross-site-with-basic-credentials", method: http.MethodPost, body: "{}", headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json", "Authorization": "Basic YWRtaW46c2VjcmV0"}, expectedStatus: http.StatusForbidden},
		{name: "referer-of-other-site", method: http.MethodPost, body: "{}", headers: map[string]string{"Referer": "https://site-malicioso.exemplo/page", "Content-Type": "application/json"}, expectedStatus: http.StatusForbidden},
		{name: "html-form", method: http.MethodPost, body: "name=x", headers: map[string]string{"Origin": "http://status.example.com", "Content-Type": "application/x-www-form-urlencoded"}, expectedStatus: http.StatusUnsupportedMediaType},
		{name: "json-with-charset", method: http.MethodPut, body: "{}", headers: map[string]string{"Origin": "http://status.example.com", "Content-Type": "application/json; charset=utf-8"}, expectedStatus: http.StatusNoContent},
		{name: "yaml", method: http.MethodPost, body: "name: x", headers: map[string]string{"Content-Type": "application/yaml"}, expectedStatus: http.StatusNoContent},
		{name: "command-line-client-without-origin", method: http.MethodPost, body: "{}", headers: map[string]string{"Content-Type": "application/json"}, expectedStatus: http.StatusNoContent},
		{name: "delete-without-body", method: http.MethodDelete, headers: map[string]string{"Origin": "http://status.example.com"}, expectedStatus: http.StatusNoContent},
		{name: "behind-https-reverse-proxy", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "https://status.example.com", "X-Forwarded-Proto": "https", "Content-Type": "application/json"}, expectedStatus: http.StatusNoContent},
		{name: "x-forwarded-host-is-ignored", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "https://site-malicioso.exemplo", "X-Forwarded-Host": "site-malicioso.exemplo", "X-Forwarded-Proto": "https", "Content-Type": "application/json"}, expectedStatus: http.StatusForbidden},
		{name: "configured-origin-with-port", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "https://status.example.com:8443", "Content-Type": "application/json"}, adminConfig: &admin.Config{Enabled: true, AllowedOrigins: []string{"https://status.example.com:8443"}}, expectedStatus: http.StatusNoContent},
		{name: "derived-origin-is-not-used-with-configured-origins", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "http://status.example.com", "Content-Type": "application/json"}, adminConfig: &admin.Config{Enabled: true, AllowedOrigins: []string{"https://status.example.com:8443"}}, expectedStatus: http.StatusForbidden},
		{name: "dev-server", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "http://localhost:8081", "Content-Type": "application/json"}, devEnvironment: true, expectedStatus: http.StatusNoContent},
		{name: "dev-server-outside-dev-environment", method: http.MethodPost, body: "{}", headers: map[string]string{"Origin": "http://localhost:8081", "Content-Type": "application/json"}, expectedStatus: http.StatusForbidden},
		{name: "body-too-large", method: http.MethodPost, body: "{\"a\":\"" + strings.Repeat("x", adminMaximumBodySize) + "\"}", headers: map[string]string{"Content-Type": "application/json"}, expectedStatus: http.StatusRequestEntityTooLarge},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.devEnvironment {
				t.Setenv("ENVIRONMENT", "dev")
			}
			adminConfig := scenario.adminConfig
			if adminConfig == nil {
				adminConfig = &admin.Config{Enabled: true}
			}
			app := echo.New()
			app.Use(adminRequestProtection(adminConfig))
			app.Any("/api/v1/admin/endpoints", func(c *echo.Context) error {
				return c.NoContent(http.StatusNoContent)
			})
			request := httptest.NewRequest(scenario.method, "/api/v1/admin/endpoints", strings.NewReader(scenario.body))
			request.Host = "status.example.com"
			for name, value := range scenario.headers {
				request.Header.Set(name, value)
			}
			response, err := testHTTP(app, request)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != scenario.expectedStatus {
				t.Errorf("expected %d, got %d", scenario.expectedStatus, response.StatusCode)
			}
		})
	}
}
