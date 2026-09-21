// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	authTestHost        = "status.example.com"
	authTestCredentials = `{"username":"admin","password":"secret"}`
	authTestWrongLogin  = `{"username":"admin","password":"wrong"}`
)

func newAuthTestBasicConfig(t *testing.T, password string) *security.BasicConfig {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}
}

// newAuthTestConfig returns a configuration with security.basic and the administration, and initializes a SQLite storage
func newAuthTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Security: &security.Config{Basic: newAuthTestBasicConfig(t, "secret")},
		Admin:    &admin.Config{Enabled: true},
		Storage:  &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 10, MaximumNumberOfEvents: 10},
	}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		// Other tests of the package use the default memory store
		_ = store.Initialize(nil)
	})
	return cfg
}

type authTestResponse struct {
	status  int
	header  http.Header
	session *http.Cookie
	body    map[string]any
}

func doAuthTestRequest(t *testing.T, app *echo.Echo, method, path, body string, prepares ...func(*http.Request)) authTestResponse {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = authTestHost
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, prepare := range prepares {
		prepare(request)
	}
	response, err := testHTTP(app, request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	result := authTestResponse{status: response.StatusCode, header: response.Header}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "go_uptime_session" {
			result.session = cookie
		}
	}
	data, _ := io.ReadAll(response.Body)
	_ = json.Unmarshal(data, &result.body)
	return result
}

func authTestHeader(key, value string) func(*http.Request) {
	return func(request *http.Request) {
		request.Header.Set(key, value)
	}
}

func authTestSession(session *http.Cookie) func(*http.Request) {
	return func(request *http.Request) {
		request.AddCookie(&http.Cookie{Name: "go_uptime_session", Value: session.Value})
	}
}

func authTestBasicAuth(password string) func(*http.Request) {
	return func(request *http.Request) {
		request.SetBasicAuth("admin", password)
	}
}

func authTestLogin(t *testing.T, app *echo.Echo) *http.Cookie {
	t.Helper()
	response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials)
	if response.status != http.StatusNoContent || response.session == nil {
		t.Fatalf("expected a login session, got %d %v", response.status, response.body)
	}
	return response.session
}

func TestAuthLogin(t *testing.T) {
	app := New(newAuthTestConfig(t)).Router()
	// Without Origin nor Referer, like curl
	response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials)
	if response.status != http.StatusNoContent || response.session == nil || response.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected 204 with a session cookie, got %d %v", response.status, response.header)
	}
	setCookie := strings.ToLower(response.header.Get("Set-Cookie"))
	for _, attribute := range []string{"httponly", "samesite=strict", "path=/", "max-age=28800"} {
		if !strings.Contains(setCookie, attribute) {
			t.Errorf("expected the session cookie to have %s, got %s", attribute, setCookie)
		}
	}
	if response.session.Secure || len(response.session.Value) != 43 {
		t.Errorf("unexpected session cookie: %+v", response.session)
	}
	session := response.session
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestSession(session), authTestHeader("Sec-Fetch-Site", "same-origin")); response.status != http.StatusOK {
		t.Errorf("expected the session to authenticate the protected routes, got %d", response.status)
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials, authTestHeader("Origin", "http://"+authTestHost), authTestHeader("Sec-Fetch-Site", "same-origin")); response.status != http.StatusNoContent {
		t.Errorf("expected 204 from the same origin, got %d %v", response.status, response.body)
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials, authTestSession(session)); response.session == nil || response.session.Value == session.Value {
		t.Error("expected a new session token when logging in with a session cookie")
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials, authTestHeader("X-Forwarded-Proto", "https")); response.session == nil || !response.session.Secure {
		t.Error("expected a Secure session cookie behind a TLS reverse proxy")
	}
	refused := map[string]struct {
		body     string
		prepare  func(*http.Request)
		expected int
	}{
		"other-origin":   {body: authTestCredentials, prepare: authTestHeader("Origin", "https://site-malicioso.exemplo"), expected: http.StatusForbidden},
		"other-referer":  {body: authTestCredentials, prepare: authTestHeader("Referer", "https://site-malicioso.exemplo/login"), expected: http.StatusForbidden},
		"cross-site":     {body: authTestCredentials, prepare: authTestHeader("Sec-Fetch-Site", "cross-site"), expected: http.StatusForbidden},
		"not-json":       {body: authTestCredentials, prepare: authTestHeader("Content-Type", "application/x-www-form-urlencoded"), expected: http.StatusUnsupportedMediaType},
		"too-large":      {body: `{"username":"admin","password":"` + strings.Repeat("a", 4096) + `"}`, prepare: func(*http.Request) {}, expected: http.StatusRequestEntityTooLarge},
		"invalid-json":   {body: `{"username":`, prepare: func(*http.Request) {}, expected: http.StatusBadRequest},
		"wrong-password": {body: authTestWrongLogin, prepare: func(*http.Request) {}, expected: http.StatusUnauthorized},
	}
	for name, scenario := range refused {
		response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", scenario.body, scenario.prepare)
		if response.status != scenario.expected || response.session != nil || response.header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: expected %d without session, got %d %v", name, scenario.expected, response.status, response.header)
		}
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestWrongLogin); response.body["error"] != "Invalid username or password" {
		t.Errorf("expected the generic error message, got %v", response.body)
	}
	if response := doAuthTestRequest(t, app, http.MethodGet, "/login", ""); response.status != http.StatusOK {
		t.Errorf("expected the login screen to be served, got %d", response.status)
	}
}

func TestAuthLogout(t *testing.T) {
	app := New(newAuthTestConfig(t)).Router()
	session := authTestLogin(t, app)
	response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/logout", "", authTestSession(session), authTestHeader("Origin", "http://"+authTestHost))
	if response.status != http.StatusNoContent || response.session == nil || response.session.Value != "" || !response.session.Expires.Before(time.Now()) || response.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected 204 with an expired session cookie, got %d %+v", response.status, response.session)
	}
	setCookie := strings.ToLower(response.header.Get("Set-Cookie"))
	for _, attribute := range []string{"httponly", "samesite=strict", "path=/"} {
		if !strings.Contains(setCookie, attribute) {
			t.Errorf("expected the expired cookie to have %s, got %s", attribute, setCookie)
		}
	}
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestSession(session), authTestHeader("Sec-Fetch-Site", "same-origin")); response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "" {
		t.Errorf("expected 401 without the Basic challenge with the token of a closed session, got %d %v", response.status, response.header)
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/logout", ""); response.status != http.StatusNoContent {
		t.Errorf("expected 204 for a logout without session, got %d", response.status)
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/logout", "", authTestHeader("Origin", "https://site-malicioso.exemplo")); response.status != http.StatusForbidden {
		t.Errorf("expected 403 for a logout from another origin, got %d", response.status)
	}
}

func TestAuthProtectedRoutes(t *testing.T) {
	app := New(newAuthTestConfig(t)).Router()
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestBasicAuth("secret")); response.status != http.StatusOK {
		t.Errorf("expected 200 with Authorization: Basic, got %d", response.status)
	}
	unknownToken := make([]byte, 32)
	_, _ = rand.Read(unknownToken)
	unknownSession := &http.Cookie{Value: base64.RawURLEncoding.EncodeToString(unknownToken)}
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestBasicAuth("secret"), authTestSession(unknownSession)); response.status != http.StatusOK {
		t.Errorf("expected 200 with Authorization: Basic and an unknown session cookie, got %d", response.status)
	}
	response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "")
	if response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "Basic" || response.header.Get("Cache-Control") != "no-store" {
		t.Errorf("expected 401 with the Basic challenge without credentials, got %d %v", response.status, response.header)
	}
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestHeader("X-Requested-With", "XMLHttpRequest")); response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "" {
		t.Errorf("expected 401 without the Basic challenge from the frontend, got %d %v", response.status, response.header)
	}
	for i := 0; i < 10; i++ {
		if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestBasicAuth("wrong")); response.status != http.StatusUnauthorized {
			t.Fatalf("expected 401 for the failure %d, got %d", i+1, response.status)
		}
	}
	response = doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestBasicAuth("wrong"))
	if response.status != http.StatusTooManyRequests || response.header.Get("Retry-After") == "" {
		t.Errorf("expected 429 with Retry-After after 10 failures, got %d %v", response.status, response.header)
	}
	if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses", "", authTestBasicAuth("secret")); response.status != http.StatusTooManyRequests {
		t.Errorf("expected 429 with the right password from a blocked client, got %d", response.status)
	}
	response = doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials)
	if response.status != http.StatusTooManyRequests || response.header.Get("Retry-After") == "" || response.session != nil {
		t.Errorf("expected 429 with Retry-After for the login of a blocked client, got %d %v", response.status, response.header)
	}
}

func TestAuthConfigState(t *testing.T) {
	app := New(newAuthTestConfig(t)).Router()
	state := func(prepares ...func(*http.Request)) (bool, bool) {
		t.Helper()
		response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/config", "", prepares...)
		adminState, _ := response.body["admin"].(map[string]any)
		if response.body["login"] != "basic" || adminState["enabled"] != true {
			t.Errorf("expected login=basic and the administration enabled, got %v", response.body)
		}
		return response.body["authenticated"] == true, adminState["authorized"] == true
	}
	if authenticated, authorized := state(); authenticated || authorized {
		t.Errorf("expected authenticated=false authorized=false without session, got %v %v", authenticated, authorized)
	}
	if authenticated, authorized := state(authTestBasicAuth("secret")); !authenticated || !authorized {
		t.Errorf("expected authenticated=true authorized=true with Authorization: Basic, got %v %v", authenticated, authorized)
	}
	if authenticated, authorized := state(authTestSession(authTestLogin(t, app))); !authenticated || !authorized {
		t.Errorf("expected authenticated=true authorized=true with a session, got %v %v", authenticated, authorized)
	}
	// Each query with a wrong password counts a single failure: after 9 of them, the login still works
	for i := 0; i < 9; i++ {
		if authenticated, _ := state(authTestBasicAuth("wrong")); authenticated {
			t.Fatal("expected a wrong password not to be authenticated")
		}
	}
	authTestLogin(t, app)
	state(authTestBasicAuth("wrong"))
	if authenticated, _ := state(authTestBasicAuth("secret")); authenticated {
		t.Error("expected the right password from a blocked client not to be authenticated")
	}
	if response := doAuthTestRequest(t, app, http.MethodPost, "/api/v1/auth/login", authTestCredentials); response.status != http.StatusTooManyRequests {
		t.Errorf("expected 429 for the login after 10 failures through the configuration, got %d", response.status)
	}
}

func TestAuthRoutesWithoutBasicLogin(t *testing.T) {
	oidcConfig := &security.OIDCConfig{IssuerURL: "https://sso.example.com/", RedirectURL: "http://localhost/authorization-code/callback", Scopes: []string{"openid"}}
	scenarios := map[string]struct {
		securityConfig *security.Config
		expectedLogin  string
	}{
		"without-security":   {},
		"oidc-and-basic":     {securityConfig: &security.Config{Basic: newAuthTestBasicConfig(t, "secret"), OIDC: oidcConfig}, expectedLogin: "oidc"},
		"oidc-without-basic": {securityConfig: &security.Config{OIDC: oidcConfig}, expectedLogin: "oidc"},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{Security: scenario.securityConfig}
			app := echo.New()
			router := app.Group("/api")
			clientIP := clientIPMiddleware(nil)
			router.GET("/v1/config", ConfigHandler{securityConfig: cfg.Security, config: cfg}.GetConfig, clientIP)
			registerAuthRoutes(router, cfg, clientIP)
			for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/logout"} {
				if response := doAuthTestRequest(t, app, http.MethodPost, path, authTestCredentials); response.status != http.StatusNotFound {
					t.Errorf("expected 404 for %s, got %d", path, response.status)
				}
			}
			if response := doAuthTestRequest(t, app, http.MethodGet, "/api/v1/config", ""); response.body["login"] != scenario.expectedLogin {
				t.Errorf("expected login=%q, got %v", scenario.expectedLogin, response.body["login"])
			}
		})
	}
	if response := doAuthTestRequest(t, New(&config.Config{}).Router(), http.MethodGet, "/login", ""); response.status == http.StatusOK {
		t.Error("expected the login screen not to be served without security.basic")
	}
}

func TestAuthSessionAcrossReload(t *testing.T) {
	cfg := newAuthTestConfig(t)
	session := authTestLogin(t, New(cfg).Router())
	// A reload closes the store, opens it again and loads a new configuration
	store.Get().Close()
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	reloaded := &config.Config{Security: &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: cfg.Security.Basic.PasswordBcryptHashBase64Encoded}}, Storage: cfg.Storage}
	if response := doAuthTestRequest(t, New(reloaded).Router(), http.MethodGet, "/api/v1/endpoints/statuses", "", authTestSession(session)); response.status != http.StatusOK {
		t.Errorf("expected the session to survive a reload, got %d", response.status)
	}
	changed := &config.Config{Security: &security.Config{Basic: newAuthTestBasicConfig(t, "another-secret")}, Storage: cfg.Storage}
	if response := doAuthTestRequest(t, New(changed).Router(), http.MethodGet, "/api/v1/endpoints/statuses", "", authTestSession(session)); response.status != http.StatusUnauthorized {
		t.Errorf("expected 401 with a session created with the previous password, got %d", response.status)
	}
}
