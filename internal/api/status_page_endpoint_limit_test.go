package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"

	"github.com/labstack/echo/v5"
)

// newEndpointLimitTestApp serves the pages infra (public) and clients (with a login), which both select the group core:
// alpha, bravo, charlie and delta, in this display order. A limit of 0 leaves maximum-endpoints-per-page unset.
func newEndpointLimitTestApp(t *testing.T, limit, rateLimit int) *echo.Echo {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	statusPages := protectedStatusPages(t, "page-secret")
	statusPages.RateLimit = &rateLimit
	if limit != 0 {
		statusPages.MaximumEndpointsPerPage = &limit
	}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	var endpoints []*endpoint.Endpoint
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		ep := &endpoint.Endpoint{Name: name, Group: "core", URL: "https://example.org", Conditions: []endpoint.Condition{"[STATUS] == 200"}}
		if err := ep.ValidateAndSetDefaults(); err != nil {
			t.Fatal(err)
		}
		if err := store.Get().InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: 10 * time.Millisecond}); err != nil {
			t.Fatal(err)
		}
		endpoints = append(endpoints, ep)
	}
	cfg := &config.Config{Security: statusPageBasicSecurity(t), Endpoints: endpoints, StatusPages: statusPages}
	statuspage.Load(cfg)
	t.Cleanup(func() { statuspage.ConfigureLimiter(0) })
	return New(cfg).Router()
}

// endpointRoutesOf returns the routes of a page that serve one endpoint. The stream is asked with HEAD, which goes
// through the same authorization and answers without opening a stream.
func endpointRoutesOf(slug, key string) []struct{ method, target string } {
	prefix := "/api/v1/status-pages/" + slug + "/endpoints/" + key
	return []struct{ method, target string }{
		{http.MethodGet, prefix},
		{http.MethodGet, prefix + "/response-time-chart?period=24h"},
		{http.MethodHead, prefix + "/events"},
		{http.MethodGet, prefix + "/health/badge.svg"},
		{http.MethodGet, prefix + "/response-times/24h/badge.svg"},
	}
}

func TestStatusPageEndpointLimit_RoutesBeyondTheCut(t *testing.T) {
	for _, scenario := range []struct {
		limit  int
		shown  []string
		hidden []string
	}{
		{limit: 0, shown: []string{"core_alpha", "core_delta"}},
		{limit: 1000, shown: []string{"core_alpha", "core_delta"}},
		{limit: 2, shown: []string{"core_alpha", "core_bravo"}, hidden: []string{"core_charlie", "core_delta"}},
	} {
		t.Run(fmt.Sprintf("limit-%d", scenario.limit), func(t *testing.T) {
			app := newEndpointLimitTestApp(t, scenario.limit, 0)
			for _, key := range scenario.shown {
				for _, route := range endpointRoutesOf("infra", key) {
					if response, body := doStatusPageRequest(t, app, route.method, route.target); response.StatusCode != http.StatusOK {
						t.Errorf("%s %s: expected 200 within the cut, got %d %s", route.method, route.target, response.StatusCode, body)
					}
				}
			}
			for _, key := range scenario.hidden {
				for _, route := range endpointRoutesOf("infra", key) {
					response, body := doStatusPageRequest(t, app, route.method, route.target)
					if response.StatusCode != http.StatusNotFound || response.Header.Get("Cache-Control") != "no-store" {
						t.Errorf("%s %s: expected the 404 of the status pages beyond the cut, got %d %s", route.method, route.target, response.StatusCode, body)
					}
					if route.method == http.MethodGet && body != statusPageNotFoundBody {
						t.Errorf("%s: expected the identical 404 body, got %s", route.target, body)
					}
				}
			}
			// The HTML of the details is the single page application in both cases: the 404 is the one of the API
			for _, key := range append(append([]string{}, scenario.shown...), scenario.hidden...) {
				response, body := doStatusPageRequest(t, app, http.MethodGet, "/status/infra/endpoints/"+key)
				if response.StatusCode != http.StatusOK || !strings.Contains(body, "<html") {
					t.Errorf("/status/infra/endpoints/%s: expected the single page application, got %d", key, response.StatusCode)
				}
			}
		})
	}
}

func TestStatusPageEndpointLimit_PayloadFollowsTheLimit(t *testing.T) {
	app := newEndpointLimitTestApp(t, 3, 0)
	response, body := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra")
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `"truncated":true`) || !strings.Contains(body, `"total":3`) {
		t.Errorf("expected a truncated payload with a total of 3, got %d %s", response.StatusCode, body)
	}
	if strings.Contains(body, `"name":"delta"`) || !strings.Contains(body, `"name":"charlie"`) {
		t.Errorf("expected charlie and not delta in the payload, got %s", body)
	}
}

// On a page with a login the cut tells nothing to who has no credential: 401 comes before any 404
func TestStatusPageEndpointLimit_LoginBeforeTheCut(t *testing.T) {
	app := newEndpointLimitTestApp(t, 2, 0)
	authorized := map[string]string{"Authorization": basicAuthorization("client", "page-secret")}
	for _, key := range []string{"core_alpha", "core_delta"} {
		for _, route := range endpointRoutesOf("clients", key) {
			// Only for the endpoint beyond the cut: every request without credential counts as a failed login of the
			// page, and ten of them in a minute would block the page for the requests below
			if key == "core_delta" {
				if response := doStatusPageRequestWithHeaders(t, app, route.method, route.target, nil); response.StatusCode != http.StatusUnauthorized {
					t.Errorf("%s %s: expected 401 without credential, got %d", route.method, route.target, response.StatusCode)
				}
			}
			expected := http.StatusOK
			if key == "core_delta" {
				expected = http.StatusNotFound
			}
			if response := doStatusPageRequestWithHeaders(t, app, route.method, route.target, authorized); response.StatusCode != expected {
				t.Errorf("%s %s: expected %d with the credential of the page, got %d", route.method, route.target, expected, response.StatusCode)
			}
		}
	}
}

// The 404s beyond the cut count in the rate limit of the 404s like any other, and the 429 is respected
func TestStatusPageEndpointLimit_RateLimitOfTheNotFound(t *testing.T) {
	app := newEndpointLimitTestApp(t, 2, 2)
	for i := 0; i < 2; i++ {
		if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra/endpoints/core_delta"); response.StatusCode != http.StatusNotFound {
			t.Fatalf("request %d: expected 404, got %d", i+1, response.StatusCode)
		}
	}
	response, body := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra/endpoints/core_charlie")
	if response.StatusCode != http.StatusTooManyRequests || body != statusPageTooManyRequestsBody || response.Header.Get("Retry-After") == "" {
		t.Errorf("expected 429 with Retry-After, got %d %s", response.StatusCode, body)
	}
	if response, _ := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra/endpoints/core_alpha"); response.StatusCode != http.StatusOK {
		t.Errorf("expected an endpoint within the cut not to be limited, got %d", response.StatusCode)
	}
}
