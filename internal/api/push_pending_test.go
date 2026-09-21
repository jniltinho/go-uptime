// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"

	"github.com/labstack/echo/v5"
)

// A push with status=pending records a pending result, an extension of the fork (fork)
func TestPush_PendingStatus(t *testing.T) {
	router := newPushTestRouter(t)
	status, _, body := doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken+"?status=pending&msg=Aguardando")
	if status != http.StatusOK || body != `{"ok":true}` {
		t.Fatalf("expected 200 with {\"ok\":true}, got %d %s", status, body)
	}
	result, _ := latestResult(t, "jobs_backup")
	if result == nil || result.Success || !result.Pending || result.Message != "Aguardando" || len(result.Errors) != 0 || result.Origin != endpoint.ResultOriginPush {
		t.Fatalf("expected a pending result with its message and without errors, got %+v", result)
	}
}

// The protected status of an endpoint has the fields of the details page of the dashboard (fork)
func TestEndpointStatus_DetailsSummaryFields(t *testing.T) {
	router := newPushTestRouter(t)
	doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken+"?status=up&ping=120")
	doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken+"?status=down&ping=80")
	doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken+"?status=up")
	// The router of the push tests has no storage configuration, which the status route needs
	cfg := &config.Config{
		Storage:           &storage.Config{MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{{Name: "backup", Group: "jobs", Token: pushTestExternalToken}},
	}
	statusRouter := echo.New()
	statusRouter.GET("/api/v1/endpoints/:key/statuses", EndpointStatus(cfg))
	fetch := func(path string) map[string]any {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		response, err := testHTTP(statusRouter, request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d %s", path, response.StatusCode, data)
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	decoded := fetch("/api/v1/endpoints/jobs_backup/statuses")
	uptime, _ := decoded["uptime"].(map[string]any)
	responseTime, _ := decoded["responseTime"].(map[string]any)
	if decoded["push"] != true || uptime["24h"] == nil || responseTime["24h"] == nil {
		t.Fatalf("expected push, uptime and response time, got %v", decoded)
	}
	if ratio := uptime["24h"].(float64); ratio < 0.66 || ratio > 0.67 {
		t.Errorf("expected an uptime of 2/3 over 24 hours, got %v", ratio)
	}
	// The last push has no ping
	if decoded["currentResponseTime"] != nil {
		t.Errorf("expected no current response time for a last result without duration, got %v", decoded["currentResponseTime"])
	}
	// The current response time does not depend on the page of results requested
	doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken+"?status=up&ping=42")
	if second := fetch("/api/v1/endpoints/jobs_backup/statuses?page=2&pageSize=1"); second["currentResponseTime"] != float64(42) {
		t.Errorf("expected the response time of the last result on the second page, got %v", second["currentResponseTime"])
	}
	if active := fetch("/api/v1/endpoints/erp_site/statuses"); active["push"] != false {
		t.Errorf("expected push=false for an active endpoint, got %v", active["push"])
	}
}
