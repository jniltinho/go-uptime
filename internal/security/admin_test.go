package security

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/admin"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

func newAdminTestApp(t *testing.T, securityConfig *Config, adminConfig *admin.Config) *echo.Echo {
	t.Helper()
	app := echo.New()
	protected := app.Group("/api")
	if err := securityConfig.ApplySecurityMiddleware(protected); err != nil {
		t.Fatalf("failed to apply security middleware: %v", err)
	}
	protected.Use(securityConfig.AdminMiddleware(adminConfig))
	protected.GET("/admin", func(ctx *echo.Context) error {
		return ctx.String(http.StatusOK, securityConfig.RequestAuthor(ctx))
	})
	return app
}

func doAdminTestRequest(t *testing.T, app *echo.Echo, prepare func(request *http.Request)) (int, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/admin", http.NoBody)
	if prepare != nil {
		prepare(request)
	}
	response, err := testHTTP(app, request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestAdminMiddleware_OIDC(t *testing.T) {
	securityConfig := &Config{OIDC: &OIDCConfig{
		IssuerURL:   "https://sso.example.com/",
		RedirectURL: "http://localhost:8080/authorization-code/callback",
		Scopes:      []string{"openid"},
	}}
	adminConfig := &admin.Config{Enabled: true, AllowedSubjects: []string{"ops@example.com"}}
	app := newAdminTestApp(t, securityConfig, adminConfig)
	sessions.SetWithTTL("admin-test-dev", "dev@example.com", time.Hour)
	sessions.SetWithTTL("admin-test-ops", "OPS@example.com", time.Hour)
	withSession := func(token string) func(*http.Request) {
		return func(request *http.Request) {
			request.AddCookie(&http.Cookie{Name: cookieNameSession, Value: token})
		}
	}
	if status, _ := doAdminTestRequest(t, app, nil); status != http.StatusUnauthorized {
		t.Errorf("expected 401 without a session, got %d", status)
	}
	if status, _ := doAdminTestRequest(t, app, withSession("admin-test-dev")); status != http.StatusForbidden {
		t.Errorf("expected 403 for a subject outside admin.allowed-subjects, got %d", status)
	}
	status, body := doAdminTestRequest(t, app, withSession("admin-test-ops"))
	if status != http.StatusOK || body != "OPS@example.com" {
		t.Errorf("expected 200 with the subject as author for an allowed subject ignoring case, got %d %q", status, body)
	}
}

func TestAdminMiddleware_Basic(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	securityConfig := &Config{Basic: &BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}}
	withCredentials := func(username, password string) func(*http.Request) {
		return func(request *http.Request) {
			request.SetBasicAuth(username, password)
		}
	}
	app := newAdminTestApp(t, securityConfig, &admin.Config{Enabled: true})
	if status, _ := doAdminTestRequest(t, app, withCredentials("admin", "wrong")); status != http.StatusUnauthorized {
		t.Errorf("expected 401 with a wrong password, got %d", status)
	}
	if status, body := doAdminTestRequest(t, app, withCredentials("admin", "secret")); status != http.StatusOK || body != "admin" {
		t.Errorf("expected 200 with the basic user as author, got %d %q", status, body)
	}
	disabledApp := newAdminTestApp(t, securityConfig, &admin.Config{Enabled: false})
	if status, _ := doAdminTestRequest(t, disabledApp, withCredentials("admin", "secret")); status != http.StatusForbidden {
		t.Errorf("expected 403 when the administration is disabled, got %d", status)
	}
}

func TestConfig_IsAdmin_NilConfig(t *testing.T) {
	var securityConfig *Config
	if securityConfig.IsAdmin(nil, &admin.Config{Enabled: true}) {
		t.Error("expected no administrator without security configuration")
	}
}
