package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

func TestAdminAPI_PushEndpoints(t *testing.T) {
	env := newAdminTestEnvironment(t)
	const conditions = "conditions: [\"[STATUS] == 200\"]\n"
	ifMatch := func(version string) map[string]string {
		return map[string]string{"If-Match": `"` + version + `"`}
	}

	// A push endpoint without token gets a generated one and its heartbeat is monitored
	_, created := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: backup\ngroup: jobs\n", nil, http.StatusCreated)
	token, _ := created["pushToken"].(string)
	if created["key"] != "jobs_backup" || created["type"] != managedendpoint.ItemTypePush || created["acceptsPush"] != true || len(token) != 32 || created["interval"] != "1m0s" {
		t.Fatalf("unexpected creation response: %v", created)
	}
	if document := created["definition"].(map[string]any)["json"].(map[string]any); document["token"] != managedendpoint.Mask {
		t.Errorf("expected the token to be masked in the definition, got %v", document)
	}
	if !watchdog.IsEndpointMonitored("jobs_backup") {
		t.Fatal("expected the heartbeat of the push endpoint to be monitored")
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+token+"?status=down&msg=Falha%20no%20backup", "", nil, http.StatusOK)
	if result, _ := latestResult(t, "jobs_backup"); result == nil || result.Success || result.Message != "Falha no backup" || result.Origin != endpoint.ResultOriginPush {
		t.Errorf("expected the push in the history of the push endpoint, got %+v", result)
	}
	_, options := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/options", "", nil, http.StatusOK)
	selectable := false
	for _, item := range options["endpoints"].([]any) {
		if item.(map[string]any)["key"] == "jobs_backup" {
			selectable = true
		}
	}
	if !selectable {
		t.Error("expected the push endpoint to be selectable by the status pages")
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: jobs\ntitle: Jobs\nendpoints: [jobs_backup]\n", nil, http.StatusCreated)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages/jobs/enable", "", ifMatch("1"), http.StatusOK)
	status, _, public := env.do(t, http.MethodGet, "/api/v1/status-pages/jobs", "", map[string]string{"Authorization": ""})
	publicPayload, _ := json.Marshal(public)
	if status != http.StatusOK || !strings.Contains(string(publicPayload), `"backup"`) || strings.Contains(string(publicPayload), "Falha no backup") {
		t.Errorf("expected the push endpoint on the public status page without the message of the push, got %d %s", status, publicPayload)
	}

	// A push endpoint with the token of an Uptime Kuma monitor keeps the URL of its scripts
	const kumaToken = "kUmA0123456789abcdefghijklmnopqr"
	_, kuma := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: kuma\ngroup: jobs\nheartbeat:\n  interval: 5m\ntoken: "+kumaToken+"\n", nil, http.StatusCreated)
	if kuma["pushToken"] != kumaToken || kuma["interval"] != "5m0s" {
		t.Fatalf("expected the pasted token and the heartbeat interval, got %v", kuma)
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+kumaToken+"?status=up&msg=OK&ping=", "", nil, http.StatusOK)

	// Rejections
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: copy\ngroup: jobs\ntoken: "+token+"\n", nil, http.StatusConflict)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "type: push\nname: bad\ngroup: jobs\ntoken: some-token-1\nurl: https://example.org\n", nil, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/test", "type: push\nname: tested\ngroup: jobs\ntoken: some-token-1\n", nil, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/jobs_backup", "name: backup\ngroup: jobs\nurl: "+env.serverURL+"\n"+conditions, ifMatch("1"), http.StatusBadRequest)

	// A disabled push endpoint rejects pushes
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/jobs_backup/disable", "", ifMatch("1"), http.StatusOK)
	if watchdog.IsEndpointMonitored("jobs_backup") {
		t.Error("expected the heartbeat to be stopped once disabled")
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+token, "", nil, http.StatusNotFound)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/jobs_backup/enable", "", ifMatch("2"), http.StatusOK)

	// Renaming keeps the token, sent back masked, and the history
	_, detail := env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/jobs_backup", "", nil, http.StatusOK)
	document := detail["definition"].(map[string]any)["json"].(map[string]any)
	document["group"] = "infra"
	payload, _ := json.Marshal(document)
	_, renamed := env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/jobs_backup", string(payload), map[string]string{"If-Match": `"3"`, "Content-Type": "application/json"}, http.StatusOK)
	if renamed["key"] != "infra_backup" || renamed["pushToken"] != token {
		t.Fatalf("expected the renamed endpoint to keep its token, got %v", renamed)
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+token+"?msg=Backup%20OK", "", nil, http.StatusOK)
	if result, count := latestResult(t, "infra_backup"); result == nil || !result.Success || result.Message != "Backup OK" || count < 2 {
		t.Errorf("expected the push with the history under the new key, got %+v (%d results)", result, count)
	}

	// An active endpoint with the push option accepts its token and the global keys; one without the option does not
	_, site := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: site\ngroup: erp\ninterval: 1h\nurl: "+env.serverURL+"\n"+conditions+"push:\n  enabled: true\n  token: erp-site-token\n", nil, http.StatusCreated)
	if site["acceptsPush"] != true || site["pushToken"] != "erp-site-token" {
		t.Fatalf("expected the active endpoint to receive push, got %v", site)
	}
	env.expectStatus(t, http.MethodGet, "/api/push/erp-site-token?status=down&msg=Latencia%20alta", "", nil, http.StatusOK)
	// The first check of the new endpoint can be stored after the push, so the push is looked up by origin
	if result := latestPushResult(t, "erp_site"); result == nil || result.Success || result.Message != "Latencia alta" {
		t.Errorf("expected the push in the history of the active endpoint, got %+v", result)
	}
	globalKey, err := pushkey.Create("akamai", "admin")
	if err != nil {
		t.Fatal(err)
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+globalKey.Token+"/erp_site?msg=Recuperado", "", nil, http.StatusOK)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: plain\ngroup: erp\ninterval: 1h\nurl: "+env.serverURL+"\n"+conditions, nil, http.StatusCreated)
	env.expectStatus(t, http.MethodGet, "/api/push/"+globalKey.Token+"/erp_plain", "", nil, http.StatusNotFound)

	// Removing the push endpoint stops its heartbeat and its pushes
	env.expectStatus(t, http.MethodDelete, "/api/v1/admin/endpoints/infra_backup", "", ifMatch("4"), http.StatusOK)
	if watchdog.IsEndpointMonitored("infra_backup") {
		t.Error("expected the heartbeat to be stopped once removed")
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+token, "", nil, http.StatusNotFound)
}
