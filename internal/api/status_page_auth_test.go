// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/security"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

// Fork: login of a status page

func protectedStatusPages(t *testing.T, password string) *pageconfig.Config {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	rateLimit := 0
	return &pageconfig.Config{
		Enabled:   &enabled,
		RateLimit: &rateLimit,
		Pages: []*pageconfig.Page{
			{Slug: "infra", Title: "Infra", Groups: []string{"core"}},
			{
				Slug:   "clients",
				Title:  "Clients",
				Groups: []string{"core"},
				Auth:   &pageconfig.PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)},
			},
		},
	}
}

func doStatusPageRequestWithHeaders(t *testing.T, app *echo.Echo, method, target string, headers map[string]string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := testHTTP(app, request)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, target, err)
	}
	_ = response.Body.Close()
	return response
}

func basicAuthorization(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func protectedRoutes() []string {
	return []string{
		"/status/clients",
		"/status/clients/endpoints/core_api",
		"/api/v1/status-pages/clients",
		"/api/v1/status-pages/clients/endpoints/core_api",
		"/api/v1/status-pages/clients/endpoints/core_api/events",
		"/api/v1/status-pages/clients/endpoints/core_api/response-time-chart?period=recent",
		"/api/v1/status-pages/clients/endpoints/core_api/health/badge.svg",
		"/api/v1/status-pages/clients/endpoints/core_api/response-times/24h/badge.svg",
	}
}

func TestStatusPageAuth_ChallengesEveryRouteOfThePage(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	for _, target := range protectedRoutes() {
		t.Run(target, func(t *testing.T) {
			response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, target, nil)
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 without credentials, got %d", response.StatusCode)
			}
			if challenge := response.Header.Get("WWW-Authenticate"); !strings.HasPrefix(challenge, `Basic realm="clients"`) {
				t.Errorf("expected the challenge of the page, got %q", challenge)
			}
			if cacheControl := response.Header.Get("Cache-Control"); cacheControl != "no-store" {
				t.Errorf("expected Cache-Control: no-store, got %q", cacheControl)
			}
			// The event stream only ends after minutes, so with the right credential it is asked with HEAD, which
			// answers the same headers without opening the stream
			method := http.MethodGet
			if strings.HasSuffix(target, "/events") {
				method = http.MethodHead
			}
			response = doStatusPageRequestWithHeaders(t, app, method, target, map[string]string{"Authorization": basicAuthorization("client", "page-secret")})
			if response.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 with the credentials of the page, got %d", response.StatusCode)
			}
			if cacheControl := response.Header.Get("Cache-Control"); !strings.Contains(cacheControl, "private") || !strings.Contains(cacheControl, "no-store") {
				t.Errorf("expected a private answer that no shared cache keeps, got %q", cacheControl)
			}
		})
	}
	// HEAD of the HTML is challenged too, otherwise the browser would render the page without the credentials
	if response := doStatusPageRequestWithHeaders(t, app, http.MethodHead, "/status/clients", nil); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected HEAD of the HTML to be challenged, got %d", response.StatusCode)
	}
}

func TestStatusPageAuth_ChallengeEvenForRequestsThatLookLikeABrowser(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	headers := map[string]string{"Sec-Fetch-Site": "same-origin", "X-Requested-With": "XMLHttpRequest"}
	response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients", headers)
	if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("expected 401 with WWW-Authenticate, got %d and %q", response.StatusCode, response.Header.Get("WWW-Authenticate"))
	}
}

func TestStatusPageAuth_OtherCredentialsDoNotOpenThePage(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	scenarios := map[string]string{
		"the credentials of the installation":  basicAuthorization("admin", "secret"),
		"the right user with a wrong password": basicAuthorization("client", "wrong"),
		"a wrong user with the right password": basicAuthorization("other", "page-secret"),
	}
	for name, authorization := range scenarios {
		t.Run(name, func(t *testing.T) {
			response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients", map[string]string{"Authorization": authorization})
			if response.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401 with %s, got %d", name, response.StatusCode)
			}
		})
	}
}

func TestStatusPageAuth_PublicPagesAndUnknownPathsDoNotChange(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra")
	if response.StatusCode != http.StatusOK || response.Header.Get("WWW-Authenticate") != "" {
		t.Errorf("expected the public page to answer 200 without a challenge, got %d and %q", response.StatusCode, response.Header.Get("WWW-Authenticate"))
	}
	if cacheControl := response.Header.Get("Cache-Control"); cacheControl != "no-cache" {
		t.Errorf("expected the public page to keep Cache-Control: no-cache, got %q", cacheControl)
	}
	// A page that does not exist, one that is not published and a path that is not a page keep the identical 404
	for _, target := range []string{"/api/v1/status-pages/missing", "/api/v1/status-pages/clients/extra", "/api/v1/status-pages/"} {
		response, _ := doStatusPageRequest(t, app, http.MethodGet, target)
		if response.StatusCode != http.StatusNotFound || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("expected the identical 404 without a challenge for %s, got %d and %q", target, response.StatusCode, response.Header.Get("WWW-Authenticate"))
		}
	}
	// Any other path under /status/ keeps answering the HTML, revealing nothing
	for _, target := range []string{"/status/missing", "/status/infra"} {
		response, _ := doStatusPageRequest(t, app, http.MethodGet, target)
		if response.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", target, response.StatusCode)
		}
	}
}

func TestStatusPageAuth_KeyThatIsNotOnThePageIsChallengedFirst(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	// Without credentials the answer must not tell whether the key belongs to the page
	response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients/endpoints/core_missing", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 before any 404 by key, got %d", response.StatusCode)
	}
	// With the credentials of the page, the key that is not on it answers the identical 404
	response = doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients/endpoints/core_missing", map[string]string{"Authorization": basicAuthorization("client", "page-secret")})
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for a key that is not on the page, got %d", response.StatusCode)
	}
}

func TestStatusPageAuth_FailuresAreLimitedPerPage(t *testing.T) {
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), protectedStatusPages(t, "page-secret"))
	wrong := map[string]string{"Authorization": basicAuthorization("client", "wrong")}
	var blocked bool
	for i := 0; i < 12; i++ {
		response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients", wrong)
		if response.StatusCode == http.StatusTooManyRequests {
			if response.Header.Get("Retry-After") == "" {
				t.Error("expected Retry-After with the 429")
			}
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("expected the failures to be limited")
	}
	// Even the right credentials wait for the window
	response := doStatusPageRequestWithHeaders(t, app, http.MethodGet, "/api/v1/status-pages/clients", map[string]string{"Authorization": basicAuthorization("client", "page-secret")})
	if response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 during the window, got %d", response.StatusCode)
	}
	// The public page is not affected
	if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra"); response.StatusCode != http.StatusOK {
		t.Errorf("expected the other page to keep answering, got %d", response.StatusCode)
	}
}

func TestStatusPageAuth_InvalidCredentialInTheDefinition(t *testing.T) {
	scenarios := map[string]*pageconfig.PageAuth{
		"without a username": {Username: "", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString([]byte("$2a$04$abcdefghijklmnopqrstuv"))},
		"without a hash":     {Username: "client"},
		"with a hash that is not bcrypt": {
			Username:                        "client",
			PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString([]byte("not-a-bcrypt-hash")),
		},
		"with a hash that is not base64": {Username: "client", PasswordBcryptHashBase64Encoded: "not base64!"},
	}
	for name, auth := range scenarios {
		t.Run(name, func(t *testing.T) {
			page := &pageconfig.Page{Slug: "clients", Title: "Clients", Groups: []string{"core"}, Auth: auth}
			if err := page.ValidateAndSetDefaults(); err == nil {
				t.Error("expected the definition to be refused")
			}
		})
	}
}

func TestStatusPageAuth_CredentialCheckIsConstantTimeOnTheUsername(t *testing.T) {
	// The check always compares both, so that the answer does not tell whether the username exists
	hash, err := bcrypt.GenerateFromPassword([]byte("page-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.URLEncoding.EncodeToString(hash)
	if !security.CheckCredentials("client", encoded, "client", "page-secret") {
		t.Error("expected the right credentials to be accepted")
	}
	if security.CheckCredentials("client", encoded, "other", "page-secret") {
		t.Error("expected a wrong username to be refused")
	}
	if security.CheckCredentials("client", encoded, "client", "other") {
		t.Error("expected a wrong password to be refused")
	}
}
