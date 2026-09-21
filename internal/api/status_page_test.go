// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

var comparedStatusPageHeaders = []string{"Content-Type", "Cache-Control", "X-Robots-Tag", "X-Content-Type-Options", "Referrer-Policy", "Vary"}

func statusPageBasicSecurity(t *testing.T) *security.Config {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}}
}

func statusPagesTestConfig(enabled bool, rateLimit int) *pageconfig.Config {
	disabledPage := false
	return &pageconfig.Config{
		Enabled:   &enabled,
		RateLimit: &rateLimit,
		Pages: []*pageconfig.Page{
			{Slug: "infra", Title: "Infra", Groups: []string{"core"}},
			{Slug: "hidden", Title: "Hidden", Groups: []string{"core"}, Enabled: &disabledPage},
		},
	}
}

func newStatusPageTestApp(t *testing.T, securityConfig *security.Config, statusPages *pageconfig.Config) *echo.Echo {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	ep := &endpoint.Endpoint{Name: "api", Group: "core", URL: "https://example.org", Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := ep.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	result := &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: 10 * time.Millisecond, Hostname: "10.0.0.5", Errors: []string{"secret-error"}}
	if err := store.Get().InsertEndpointResult(ep, result); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Security: securityConfig, Endpoints: []*endpoint.Endpoint{ep}, StatusPages: statusPages}
	statuspage.Load(cfg)
	return New(cfg).Router()
}

func doStatusPageRequest(t *testing.T, app *echo.Echo, method, target string) (*http.Response, string) {
	t.Helper()
	response, err := testHTTP(app, httptest.NewRequest(method, target, nil))
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, target, err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	return response, string(body)
}

func TestStatusPage_PublicWithBasicAuth(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPagesTestConfig(true, 0))
	response, body := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra")
	if response.StatusCode != http.StatusOK || response.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("expected 200 without authentication challenge, got %d (WWW-Authenticate=%q): %s", response.StatusCode, response.Header.Get("WWW-Authenticate"), body)
	}
	expectedHeaders := map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "no-cache",
		"X-Robots-Tag":           "noindex, nofollow",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Vary":                   "Accept-Encoding",
	}
	for name, value := range expectedHeaders {
		if actual := response.Header.Get(name); actual != value {
			t.Errorf("expected header %s=%q, got %q", name, value, actual)
		}
	}
	var payload statuspage.Payload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Slug != "infra" || len(payload.Groups) != 1 || payload.Groups[0].Endpoints[0].Name != "api" || payload.Groups[0].Endpoints[0].Status != statuspage.StatusUp {
		t.Errorf("unexpected payload: %s", body)
	}
	for _, sensitive := range []string{"10.0.0.5", "secret-error", "core_api", "example.org"} {
		if strings.Contains(body, sensitive) {
			t.Errorf("expected %q not to be published, got %s", sensitive, body)
		}
	}
	if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected the protected routes to still require authentication, got %d", response.StatusCode)
	}
}

func TestStatusPage_IdenticalNotFound(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPagesTestConfig(true, 0))
	reference, referenceBody := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/missing")
	if reference.StatusCode != http.StatusNotFound || referenceBody != statusPageNotFoundBody || reference.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected reference 404: %d %s (Cache-Control=%q)", reference.StatusCode, referenceBody, reference.Header.Get("Cache-Control"))
	}
	scenarios := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/status-pages/hidden"},
		{http.MethodGet, "/api/v1/status-pages/Infra"},
		{http.MethodGet, "/api/v1/status-pages/a%2Fb"},
		{http.MethodGet, "/api/v1/status-pages/infra/extra"},
		{http.MethodGet, "/api/v1/status-pages/a/b"},
		{http.MethodGet, "/api/v1/status-pages"},
		{http.MethodGet, "/api/v1/status-pages/"},
		{http.MethodPost, "/api/v1/status-pages/infra"},
		{http.MethodDelete, "/api/v1/status-pages/missing"},
		{http.MethodHead, "/api/v1/status-pages/missing"},
		{http.MethodHead, "/api/v1/status-pages/hidden"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.method+" "+scenario.target, func(t *testing.T) {
			response, body := doStatusPageRequest(t, app, scenario.method, scenario.target)
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", response.StatusCode, body)
			}
			if scenario.method != http.MethodHead && body != referenceBody {
				t.Errorf("expected body %s, got %s", referenceBody, body)
			}
			for _, name := range append(comparedStatusPageHeaders, "WWW-Authenticate") {
				if actual, expected := response.Header.Get(name), reference.Header.Get(name); actual != expected {
					t.Errorf("header %s: expected %q, got %q", name, expected, actual)
				}
			}
		})
	}
}

func TestStatusPage_Disabled(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPagesTestConfig(false, 0))
	for _, target := range []string{"/api/v1/status-pages/infra", "/api/v1/status-pages/a/b"} {
		response, body := doStatusPageRequest(t, app, http.MethodGet, target)
		if response.StatusCode != http.StatusNotFound || body != statusPageNotFoundBody || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("%s: expected the identical 404 without authentication challenge, got %d %s", target, response.StatusCode, body)
		}
	}
}

func TestStatusPage_SinglePageApplication(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPagesTestConfig(true, 0))
	for _, scenario := range []struct{ method, target string }{
		{http.MethodGet, "/status/infra"},
		{http.MethodGet, "/status/missing"},
		{http.MethodGet, "/status/a/b"},
		{http.MethodGet, "/status/a%2Fb"},
		{http.MethodHead, "/status/hidden"},
	} {
		response, body := doStatusPageRequest(t, app, scenario.method, scenario.target)
		if response.StatusCode != http.StatusOK || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("%s %s: expected 200 without authentication challenge, got %d", scenario.method, scenario.target, response.StatusCode)
		}
		if response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Robots-Tag") != "noindex, nofollow" || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s %s: unexpected headers %v", scenario.method, scenario.target, response.Header)
		}
		if scenario.method == http.MethodGet && !strings.Contains(body, "<html") {
			t.Errorf("%s %s: expected the single page application, got %q", scenario.method, scenario.target, body)
		}
	}
}

func TestStatusPage_RateLimit(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPagesTestConfig(true, 2))
	t.Cleanup(func() { statuspage.ConfigureLimiter(0) })
	for i := 0; i < 2; i++ {
		if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/missing"); response.StatusCode != http.StatusNotFound {
			t.Fatalf("request %d: expected 404, got %d", i+1, response.StatusCode)
		}
	}
	response, body := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/other")
	retryAfter, _ := strconv.Atoi(response.Header.Get("Retry-After"))
	if response.StatusCode != http.StatusTooManyRequests || body != statusPageTooManyRequestsBody || retryAfter < 1 || retryAfter > 60 || response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("expected 429 with Retry-After, got %d %s (Retry-After=%q)", response.StatusCode, body, response.Header.Get("Retry-After"))
	}
	for name := range response.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-ratelimit") {
			t.Errorf("expected no %s header", name)
		}
	}
	if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra"); response.StatusCode != http.StatusOK {
		t.Errorf("expected a published page not to be limited, got %d", response.StatusCode)
	}
}

func TestStatusPage_OIDCWithoutSession(t *testing.T) {
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q,"id_token_signing_alg_values_supported":["RS256"]}`, issuer.URL, issuer.URL+"/authorize", issuer.URL+"/token", issuer.URL+"/jwks")
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"keys":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(issuer.Close)
	securityConfig := &security.Config{OIDC: &security.OIDCConfig{
		IssuerURL:    issuer.URL,
		RedirectURL:  "http://localhost:8080/authorization-code/callback",
		ClientID:     "go-uptime",
		ClientSecret: "secret",
		Scopes:       []string{"openid"},
	}}
	app := newStatusPageTestApp(t, securityConfig, statusPagesTestConfig(true, 0))
	for _, target := range []string{"/api/v1/status-pages/infra", "/status/infra"} {
		response, _ := doStatusPageRequest(t, app, http.MethodGet, target)
		if response.StatusCode != http.StatusOK || response.Header.Get("Location") != "" || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("%s: expected 200 without redirection or challenge, got %d (Location=%q)", target, response.StatusCode, response.Header.Get("Location"))
		}
	}
	if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/endpoints/statuses"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected the protected routes to still require a session, got %d", response.StatusCode)
	}
}
