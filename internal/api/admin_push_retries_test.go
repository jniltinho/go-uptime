// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"net/http"
	"strings"
	"testing"
)

// Push endpoints accept heartbeat retries between 0 and 100, and status pages accept show-messages (fork)
func TestAdminAPI_PushRetriesAndShowMessages(t *testing.T) {
	env := newAdminTestEnvironment(t)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: late\ngroup: jobs\nheartbeat:\n  interval: 1m\n  retries: 101\n", nil, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\nheartbeat:\n  retries: 2\n", nil, http.StatusBadRequest)
	_, created := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: late\ngroup: jobs\nheartbeat:\n  interval: 1m\n  retries: 2\n", nil, http.StatusCreated)
	yamlDefinition, _ := created["definition"].(map[string]any)["yaml"].(string)
	if !strings.Contains(yamlDefinition, "retries: 2") {
		t.Errorf("expected the definition to keep the retries, got %v", created["definition"])
	}
	_, page := env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: late-jobs\ntitle: Late jobs\nendpoints: [jobs_late]\nshow-messages: true\n", nil, http.StatusCreated)
	pageDefinition, _ := page["definition"].(map[string]any)
	if pageDefinition == nil || pageDefinition["show-messages"] != true {
		t.Errorf("expected the status page to keep show-messages, got %v", page)
	}
}
