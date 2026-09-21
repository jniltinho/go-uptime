// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
)

func TestStatusPage_EndpointDetails(t *testing.T) {
	enabled, rateLimit := true, 0
	statusPages := &pageconfig.Config{Enabled: &enabled, RateLimit: &rateLimit, Pages: []*pageconfig.Page{
		{Slug: "infra", Title: "Infra", Groups: []string{"core"}, Featured: []string{"core_api"}, Charts: []string{"core_api"}},
	}}
	app := newStatusPageTestApp(t, statusPageBasicSecurity(t), statusPages)

	response, body := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra")
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `"featured":[{"name":"api"`) || strings.Contains(body, "chart") {
		t.Fatalf("expected the page payload with api featured and without chart, got %d: %s", response.StatusCode, body)
	}

	response, body = doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra/endpoints/core_api")
	if response.StatusCode != http.StatusOK || response.Header.Get("WWW-Authenticate") != "" || response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("expected the endpoint details without authentication, got %d %v: %s", response.StatusCode, response.Header, body)
	}
	var details statuspage.EndpointDetailsPayload
	if err := json.Unmarshal([]byte(body), &details); err != nil || details.Page.Slug != "infra" || details.Page.Title != "Infra" || details.Name != "api" || details.Group != "core" || details.Status != statuspage.StatusUp || len(details.Results) != 1 || len(details.Events) == 0 {
		t.Errorf("unexpected endpoint details: %s (err=%v)", body, err)
	}
	for _, sensitive := range []string{"core_api", "10.0.0.5", "secret-error", "example.org"} {
		if strings.Contains(body, sensitive) {
			t.Errorf("expected %q not to be published, got %s", sensitive, body)
		}
	}

	// The details page is served by the HTML route, and its chart and badges by the routes by key of the original Go Uptime
	for _, target := range []string{"/status/infra/endpoints/core_api", "/api/v1/endpoints/core_api/response-times/24h/history", "/api/v1/endpoints/core_api/health/badge.svg"} {
		if response, body := doStatusPageRequest(t, app, http.MethodGet, target); response.StatusCode != http.StatusOK || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("%s: expected 200 without authentication, got %d: %s", target, response.StatusCode, body)
		}
	}

	reference, referenceBody := doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/missing")
	for _, target := range []string{
		"/api/v1/status-pages/infra/endpoints/core_missing",
		"/api/v1/status-pages/missing/endpoints/core_api",
		"/api/v1/status-pages/infra/endpoints/core_api/extra",
		"/api/v1/status-pages/infra/endpoints/core_api%2Fx",
		"/api/v1/status-pages/infra/response-times/24h",
	} {
		response, body := doStatusPageRequest(t, app, http.MethodGet, target)
		if response.StatusCode != http.StatusNotFound || body != referenceBody || response.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("%s: expected the identical 404, got %d %s", target, response.StatusCode, body)
		}
		for _, name := range comparedStatusPageHeaders {
			if response.Header.Get(name) != reference.Header.Get(name) {
				t.Errorf("%s: header %s: expected %q, got %q", target, name, reference.Header.Get(name), response.Header.Get(name))
			}
		}
	}
}
