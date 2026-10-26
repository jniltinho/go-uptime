// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAdminAlertProvidersAPI(t *testing.T) {
	env := newAdminTestEnvironment(t)
	var received atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	secretURL := receiver.URL + "/hook?token=very-secret-token"
	definition := `{"type": "custom", "url": "` + secretURL + `", "default-alert": {"failure-threshold": 2}}`

	_, listing := env.expectStatus(t, http.MethodGet, "/api/v1/admin/alert-providers", "", nil, http.StatusOK)
	if providers, _ := listing["providers"].([]any); len(providers) != 41 || listing["storageSupported"] != true {
		t.Fatalf("expected the 41 types and a supported storage, got %d providers", len(providers))
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers/validate", `{"type": "slack"}`, jsonHeaders, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers", `{"type": "carrier-pigeon"}`, jsonHeaders, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers", `{"type": "telegram", "token": "t", "id": "1", "client": {"tunnel": "x"}}`, jsonHeaders, http.StatusBadRequest)

	// Test of a definition that is not stored, triggered and resolved
	_, tested := env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers/test?resolve=true", definition, jsonHeaders, http.StatusOK)
	if tested["success"] != true || tested["resolved"] != true || received.Load() != 2 {
		t.Fatalf("expected the notification and its resolution, got %v (received %d)", tested, received.Load())
	}

	header, created := env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers", definition, jsonHeaders, http.StatusCreated)
	if header.Get("ETag") != `"1"` || created["source"] != "web" || strings.Contains(responseText(created), "very-secret-token") {
		t.Fatalf("expected a masked web provider at version 1, got %v (ETag %q)", created, header.Get("ETag"))
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers", definition, jsonHeaders, http.StatusConflict)
	_, detail := env.expectStatus(t, http.MethodGet, "/api/v1/admin/alert-providers/custom", "", nil, http.StatusOK)
	if strings.Contains(responseText(detail), "very-secret-token") {
		t.Fatalf("expected the detail to mask the URL, got %v", detail)
	}
	env.expectStatus(t, http.MethodGet, "/api/v1/admin/alert-providers/carrier-pigeon", "", nil, http.StatusNotFound)

	// A managed endpoint uses the provider, which then cannot be deleted
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: backup\ngroup: jobs\ntoken: tok_0123456789abcdefABCDEF01\nheartbeat:\n  interval: 1h\nalerts:\n  - type: custom\n", nil, http.StatusCreated)

	// Update keeping the masked URL: If-Match is required, a stale version is refused
	update := `{"url": "********", "default-alert": {"failure-threshold": 3}}`
	env.expectStatus(t, http.MethodPut, "/api/v1/admin/alert-providers/custom", update, jsonHeaders, http.StatusPreconditionRequired)
	env.expectStatus(t, http.MethodPut, "/api/v1/admin/alert-providers/custom", update, map[string]string{"Content-Type": "application/json", "If-Match": `"7"`}, http.StatusPreconditionFailed)
	_, updated := env.expectStatus(t, http.MethodPut, "/api/v1/admin/alert-providers/custom", update, map[string]string{"Content-Type": "application/json", "If-Match": `"1"`}, http.StatusOK)
	if restarted, _ := updated["restarted"].([]any); len(restarted) != 1 || restarted[0] != "jobs_backup" {
		t.Errorf("expected jobs_backup to be restarted, got %v", updated["restarted"])
	}
	received.Store(0)
	_, configured := env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers/custom/test", "", nil, http.StatusOK)
	if configured["success"] != true || received.Load() != 1 {
		t.Errorf("expected the stored URL to be kept and tested, got %v (received %d)", configured, received.Load())
	}
	_, inUse := env.expectStatus(t, http.MethodDelete, "/api/v1/admin/alert-providers/custom", "", map[string]string{"If-Match": `"2"`}, http.StatusConflict)
	if endpoints, _ := inUse["endpoints"].([]any); len(endpoints) != 1 || endpoints[0] != "jobs_backup" {
		t.Errorf("expected the conflict to name jobs_backup, got %v", inUse)
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/alert-providers/telegram/test", "", nil, http.StatusConflict)
	if status, _, _ := env.do(t, http.MethodGet, "/api/v1/admin/alert-providers", "", map[string]string{"Authorization": ""}); status != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", status)
	}
}

// responseText returns a decoded JSON answer as text, to look for a value anywhere in it
func responseText(body map[string]any) string {
	encoded, _ := json.Marshal(body)
	return string(encoded)
}
