// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package security

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

func initializeLoginSessionTestStore(t *testing.T) {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 10, MaximumNumberOfEvents: 10}); err != nil {
		t.Fatalf("failed to initialize the store: %v", err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		_ = store.Initialize(nil)
	})
}

func newBasicAuthTestConfig(t *testing.T, password string) *Config {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &Config{Basic: &BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}}
}

// countPasswordChecks replaces the bcrypt comparison by one counting its calls
func countPasswordChecks(t *testing.T) *atomic.Int32 {
	t.Helper()
	calls := &atomic.Int32{}
	original := compareHashAndPassword
	compareHashAndPassword = func(hash, password []byte) error {
		calls.Add(1)
		return original(hash, password)
	}
	t.Cleanup(func() { compareHashAndPassword = original })
	return calls
}

// newBasicAuthTestApp returns an app with login and logout handlers, the authentication state used by /api/v1/config
// and a protected administration route answering the author of the request
func newBasicAuthTestApp(t *testing.T, c *Config) *echo.Echo {
	t.Helper()
	adminConfig := &admin.Config{Enabled: true}
	app := echo.New()
	app.POST("/login", func(ctx *echo.Context) error {
		err := c.Login(ctx, httpx.Header(ctx, "X-Username"), httpx.Header(ctx, "X-Password"))
		var tooManyFailures *TooManyFailuresError
		switch {
		case err == nil:
			return ctx.NoContent(http.StatusNoContent)
		case errors.As(err, &tooManyFailures):
			return ctx.NoContent(http.StatusTooManyRequests)
		case errors.Is(err, ErrInvalidCredentials):
			return ctx.NoContent(http.StatusUnauthorized)
		default:
			return ctx.String(http.StatusInternalServerError, err.Error())
		}
	})
	app.POST("/logout", func(ctx *echo.Context) error {
		if err := c.Logout(ctx); err != nil {
			return err
		}
		return ctx.NoContent(http.StatusNoContent)
	})
	app.GET("/state", func(ctx *echo.Context) error {
		return ctx.JSON(http.StatusOK, map[string]any{"authenticated": c.IsAuthenticated(ctx), "admin": c.IsAdmin(ctx, adminConfig)})
	})
	protected := app.Group("/api")
	if err := c.ApplySecurityMiddleware(protected); err != nil {
		t.Fatalf("failed to apply security middleware: %v", err)
	}
	protected.Use(c.AdminMiddleware(adminConfig))
	protected.GET("/admin", func(ctx *echo.Context) error {
		return ctx.String(http.StatusOK, c.RequestAuthor(ctx))
	})
	return app
}

type basicAuthTestResponse struct {
	status  int
	header  http.Header
	body    string
	session *http.Cookie
}

func doBasicAuthTestRequest(t *testing.T, app *echo.Echo, method, path string, prepares ...func(*http.Request)) basicAuthTestResponse {
	t.Helper()
	request := httptest.NewRequest(method, path, http.NoBody)
	for _, prepare := range prepares {
		prepare(request)
	}
	response, err := testHTTP(app, request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	result := basicAuthTestResponse{status: response.StatusCode, header: response.Header, body: string(bytes.TrimSpace(body))}
	for _, cookie := range response.Cookies() {
		if cookie.Name == cookieNameSession {
			result.session = cookie
		}
	}
	return result
}

func basicAuthTestLogin(username, password string) func(*http.Request) {
	return func(request *http.Request) {
		request.Header.Set("X-Username", username)
		request.Header.Set("X-Password", password)
	}
}

func basicAuthTestCredentials(username, password string) func(*http.Request) {
	return func(request *http.Request) {
		request.SetBasicAuth(username, password)
	}
}

func basicAuthTestSession(session *http.Cookie) func(*http.Request) {
	return func(request *http.Request) {
		request.AddCookie(&http.Cookie{Name: cookieNameSession, Value: session.Value})
	}
}

func basicAuthTestHeader(key, value string) func(*http.Request) {
	return func(request *http.Request) {
		request.Header.Set(key, value)
	}
}

func basicAuthTestLoginSession(t *testing.T, app *echo.Echo) *http.Cookie {
	t.Helper()
	response := doBasicAuthTestRequest(t, app, http.MethodPost, "/login", basicAuthTestLogin("admin", "secret"))
	if response.status != http.StatusNoContent || response.session == nil {
		t.Fatalf("expected a login session, got %d %q", response.status, response.body)
	}
	return response.session
}

func TestBasicAuthentication_LoginSession(t *testing.T) {
	initializeLoginSessionTestStore(t)
	c := newBasicAuthTestConfig(t, "secret")
	app := newBasicAuthTestApp(t, c)
	if response := doBasicAuthTestRequest(t, app, http.MethodPost, "/login", basicAuthTestLogin("admin", "wrong")); response.status != http.StatusUnauthorized || response.session != nil {
		t.Errorf("expected 401 without session for a wrong password, got %d", response.status)
	}
	session := basicAuthTestLoginSession(t, app)
	if !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/" || session.MaxAge != int(DefaultBasicSessionTTL.Seconds()) || session.Secure {
		t.Errorf("unexpected session cookie attributes: %+v", session)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusOK || response.body != "admin" {
		t.Errorf("expected 200 with the session user as author, got %d %q", response.status, response.body)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/state", basicAuthTestSession(session)); response.body != `{"admin":true,"authenticated":true}` {
		t.Errorf("expected the session to be authenticated and administrator, got %s", response.body)
	}
	// Only the hash of the token is stored
	loginSessionStore, _ := store.GetLoginSessionStore()
	if _, err := loginSessionStore.GetLoginSession(session.Value); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected the token not to be stored in clear, got %v", err)
	}
	if stored, err := loginSessionStore.GetLoginSession(hashLoginSessionToken(session.Value)); err != nil || stored.Username != "admin" || !stored.ExpiresAt.After(time.Now().Add(DefaultBasicSessionTTL-time.Minute)) {
		t.Errorf("expected the session to be stored by the hash of its token, got %+v %v", stored, err)
	}
	// A login with a session cookie creates a new session
	if response := doBasicAuthTestRequest(t, app, http.MethodPost, "/login", basicAuthTestLogin("admin", "secret"), basicAuthTestSession(session)); response.session == nil || response.session.Value == session.Value {
		t.Error("expected a new session token when logging in with a session cookie")
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodPost, "/login", basicAuthTestLogin("admin", "secret"), basicAuthTestHeader("X-Forwarded-Proto", "https")); response.session == nil || !response.session.Secure {
		t.Error("expected a Secure session cookie behind a TLS reverse proxy")
	}
	response := doBasicAuthTestRequest(t, app, http.MethodPost, "/logout", basicAuthTestSession(session))
	if response.status != http.StatusNoContent || response.session == nil || response.session.Value != "" || !response.session.Expires.Before(time.Now()) || !response.session.HttpOnly || response.session.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected the logout to expire the session cookie, got %d %+v", response.status, response.session)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusUnauthorized {
		t.Errorf("expected 401 with the token of a closed session, got %d", response.status)
	}
	// An invalid session cookie does not prevent the authentication with Authorization: Basic
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestSession(session), basicAuthTestCredentials("admin", "secret")); response.status != http.StatusOK || response.body != "admin" {
		t.Errorf("expected 200 with Authorization: Basic and an old session cookie, got %d", response.status)
	}
}

func TestBasicAuthentication_InvalidSessions(t *testing.T) {
	initializeLoginSessionTestStore(t)
	c := newBasicAuthTestConfig(t, "secret")
	app := newBasicAuthTestApp(t, c)
	loginSessionStore, _ := store.GetLoginSessionStore()
	// Expired session
	expired := &http.Cookie{Value: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, loginSessionTokenSize))}
	now := time.Now()
	if err := loginSessionStore.CreateLoginSession(&common.LoginSession{TokenHash: hashLoginSessionToken(expired.Value), Username: "admin", CredentialFingerprint: c.Basic.credentialFingerprint(), CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestSession(expired)); response.status != http.StatusUnauthorized {
		t.Errorf("expected 401 with an expired session, got %d", response.status)
	}
	if _, err := loginSessionStore.GetLoginSession(hashLoginSessionToken(expired.Value)); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected the expired session to be deleted, got %v", err)
	}
	// Another instance using the same database, with the same credential
	session := basicAuthTestLoginSession(t, app)
	otherInstance := newBasicAuthTestApp(t, &Config{Basic: &BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: c.Basic.PasswordBcryptHashBase64Encoded}})
	if response := doBasicAuthTestRequest(t, otherInstance, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusOK {
		t.Errorf("expected the session to be valid in another instance, got %d", response.status)
	}
	doBasicAuthTestRequest(t, app, http.MethodPost, "/logout", basicAuthTestSession(session))
	if response := doBasicAuthTestRequest(t, otherInstance, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusUnauthorized {
		t.Errorf("expected 401 in another instance after the logout, got %d", response.status)
	}
	// Changed credential
	session = basicAuthTestLoginSession(t, app)
	changedPassword := newBasicAuthTestApp(t, newBasicAuthTestConfig(t, "another-secret"))
	if response := doBasicAuthTestRequest(t, changedPassword, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusUnauthorized {
		t.Errorf("expected 401 with a session created with another password, got %d", response.status)
	}
	if _, err := loginSessionStore.GetLoginSession(hashLoginSessionToken(session.Value)); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected the session created with another password to be deleted, got %v", err)
	}
}

func TestBasicAuthentication_MemoryStore(t *testing.T) {
	if err := store.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	app := newBasicAuthTestApp(t, newBasicAuthTestConfig(t, "secret"))
	session := basicAuthTestLoginSession(t, app)
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestSession(session)); response.status != http.StatusOK {
		t.Errorf("expected the memory storage to keep the session, got %d", response.status)
	}
}

func TestBasicAuthentication_Unauthorized(t *testing.T) {
	initializeLoginSessionTestStore(t)
	app := newBasicAuthTestApp(t, newBasicAuthTestConfig(t, "secret"))
	response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin")
	if response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "Basic" || response.header.Get("Cache-Control") != "no-store" || response.body != `{"error":"authentication required"}` {
		t.Errorf("expected 401 with the Basic challenge without a browser, got %d %v %q", response.status, response.header, response.body)
	}
	// Fork: an EventSource on an HTTP page outside of localhost only sends Accept: text/event-stream
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestHeader("Accept", "text/event-stream")); response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "" {
		t.Errorf("expected 401 without the Basic challenge for an event stream, got %d %v", response.status, response.header)
	}
	for _, header := range []string{"Sec-Fetch-Site", "Sec-Fetch-Mode", "X-Requested-With"} {
		response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestHeader(header, "value"))
		if response.status != http.StatusUnauthorized || response.header.Get("WWW-Authenticate") != "" || response.header.Get("Cache-Control") != "no-store" {
			t.Errorf("expected 401 without the Basic challenge with %s, got %d %v", header, response.status, response.header)
		}
	}
}

func TestBasicAuthentication_FailureLimiter(t *testing.T) {
	initializeLoginSessionTestStore(t)
	c := newBasicAuthTestConfig(t, "secret")
	app := newBasicAuthTestApp(t, c)
	checks := countPasswordChecks(t)
	for i := 0; i < failureLimiterMaximumFailures; i++ {
		if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestCredentials("admin", "wrong")); response.status != http.StatusUnauthorized {
			t.Fatalf("expected 401 for the failure %d, got %d", i+1, response.status)
		}
	}
	response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestCredentials("admin", "wrong"))
	if response.status != http.StatusTooManyRequests || response.header.Get("Retry-After") == "" || response.header.Get("Cache-Control") != "no-store" {
		t.Errorf("expected 429 with Retry-After after 10 failures, got %d %v", response.status, response.header)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/api/admin", basicAuthTestCredentials("admin", "secret")); response.status != http.StatusTooManyRequests {
		t.Errorf("expected 429 with the right password from a blocked client, got %d", response.status)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/state", basicAuthTestCredentials("admin", "secret")); response.body != `{"admin":false,"authenticated":false}` {
		t.Errorf("expected a blocked client not to be authenticated by the state, got %s", response.body)
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodPost, "/login", basicAuthTestLogin("admin", "secret")); response.status != http.StatusTooManyRequests {
		t.Errorf("expected 429 for the login of a blocked client, got %d", response.status)
	}
	if calls := checks.Load(); calls != failureLimiterMaximumFailures {
		t.Errorf("expected no password check for a blocked client, got %d checks", calls)
	}
}

func TestBasicAuthentication_StateChecksPasswordOnce(t *testing.T) {
	initializeLoginSessionTestStore(t)
	c := newBasicAuthTestConfig(t, "secret")
	app := newBasicAuthTestApp(t, c)
	checks := countPasswordChecks(t)
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/state", basicAuthTestCredentials("admin", "wrong")); response.body != `{"admin":false,"authenticated":false}` {
		t.Errorf("expected a wrong password not to be authenticated, got %s", response.body)
	}
	if calls := checks.Load(); calls != 1 {
		t.Errorf("expected a single password check, got %d", calls)
	}
	for _, element := range c.failureLimiter().entries {
		if failures := element.Value.(*failureLimiterEntry).failures; failures != 1 {
			t.Errorf("expected a single failure, got %d", failures)
		}
	}
	if response := doBasicAuthTestRequest(t, app, http.MethodGet, "/state", basicAuthTestCredentials("admin", "secret")); response.body != `{"admin":true,"authenticated":true}` {
		t.Errorf("expected the right password to be authenticated and administrator, got %s", response.body)
	}
	if calls := checks.Load(); calls != 2 {
		t.Errorf("expected a single password check per request, got %d", calls)
	}
}

func TestBasicAuthentication_CorrectLoginsDoNotCount(t *testing.T) {
	initializeLoginSessionTestStore(t)
	c := newBasicAuthTestConfig(t, "secret")
	app := newBasicAuthTestApp(t, c)
	for i := 0; i < 20; i++ {
		basicAuthTestLoginSession(t, app)
	}
	if len(c.failureLimiter().entries) != 0 {
		t.Error("expected the right credentials not to count as failures")
	}
}

func TestBasicConfig_CheckCredentials(t *testing.T) {
	c := newBasicAuthTestConfig(t, "secret")
	checks := countPasswordChecks(t)
	if c.Basic.checkCredentials("root", "secret") {
		t.Error("expected a wrong username to be refused")
	}
	if calls := checks.Load(); calls != 1 {
		t.Errorf("expected the password to be checked even with a wrong username, got %d checks", calls)
	}
	if c.Basic.checkCredentials("admin", "wrong") || !c.Basic.checkCredentials("admin", "secret") {
		t.Error("expected only the right username and password to be accepted")
	}
	if calls := checks.Load(); calls != 3 {
		t.Errorf("expected a password check per verification, got %d", calls)
	}
}

func TestConfig_LoginMethod(t *testing.T) {
	var nilConfig *Config
	scenarios := []struct {
		config         *Config
		expectedMethod string
		expectedBasic  bool
	}{
		{config: nilConfig},
		{config: &Config{}},
		{config: &Config{Basic: &BasicConfig{}}, expectedMethod: "basic", expectedBasic: true},
		{config: &Config{OIDC: &OIDCConfig{}}, expectedMethod: "oidc"},
		{config: &Config{Basic: &BasicConfig{}, OIDC: &OIDCConfig{}}, expectedMethod: "oidc"},
	}
	for _, scenario := range scenarios {
		if method, basic := scenario.config.LoginMethod(), scenario.config.UsesBasicLogin(); method != scenario.expectedMethod || basic != scenario.expectedBasic {
			t.Errorf("expected method=%q basic=%v, got method=%q basic=%v", scenario.expectedMethod, scenario.expectedBasic, method, basic)
		}
	}
}
