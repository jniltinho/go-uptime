package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
)

func TestAdminPushKeysAPI(t *testing.T) {
	env := newAdminTestEnvironment(t)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	// A key of the configuration file, listed without id so that it cannot be revoked through the web
	yamlKeys := &push.Config{Keys: []*push.Key{{Name: "yaml-key", Token: "yaml-key-token-0123456789"}}}
	if err := yamlKeys.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	pushkey.Load(&config.Config{Push: yamlKeys})

	_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/push-keys", "", nil, http.StatusOK)
	keys, _ := body["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("expected only the push key of the configuration file, got %v", body)
	}
	if listed, _ := keys[0].(map[string]any); listed["origin"] != pushkey.OriginConfig || listed["name"] != "yaml-key" || listed["id"] != nil || listed["hint"] != "6789" {
		t.Errorf("expected the push key of the configuration file without id, got %v", listed)
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/push-keys", `{"name": "yaml-key"}`, jsonHeaders, http.StatusConflict)

	header, created := env.expectStatus(t, http.MethodPost, "/api/v1/admin/push-keys", `{"name": " akamai "}`, jsonHeaders, http.StatusCreated)
	token, _ := created["token"].(string)
	if created["name"] != "akamai" || created["origin"] != pushkey.OriginAdmin || len(token) != 32 || created["hint"] != token[len(token)-4:] || header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected creation response: %v (Cache-Control=%q)", created, header.Get("Cache-Control"))
	}
	if name, ok := pushkey.Lookup(token); !ok || name != "akamai" {
		t.Errorf("expected the created key to be accepted, got %q (ok=%v)", name, ok)
	}
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/push-keys", `{"name": "akamai"}`, jsonHeaders, http.StatusConflict)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/push-keys", `{"name": " "}`, jsonHeaders, http.StatusBadRequest)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/push-keys", `{"name": "`+strings.Repeat("n", 65)+`"}`, jsonHeaders, http.StatusBadRequest)

	_, body = env.expectStatus(t, http.MethodGet, "/api/v1/admin/push-keys", "", nil, http.StatusOK)
	keys, _ = body["keys"].([]any)
	var listed map[string]any
	for _, key := range keys {
		if item, _ := key.(map[string]any); item["origin"] == pushkey.OriginAdmin {
			listed = item
		}
	}
	if len(keys) != 2 || listed == nil {
		t.Fatalf("expected the created push key in the list, got %v", body)
	}
	if _, hasToken := listed["token"]; hasToken || listed["hint"] != token[len(token)-4:] || listed["createdBy"] != "admin" {
		t.Errorf("expected the list to have the hint and the author, without the token, got %v", listed)
	}

	// The key is used by an active endpoint that accepts push, and rejected on the push route once revoked
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: site\ngroup: cdn\ninterval: 1h\nurl: "+env.serverURL+"\nconditions: [\"[STATUS] == 200\"]\npush:\n  enabled: true\n", nil, http.StatusCreated)
	env.expectStatus(t, http.MethodGet, "/api/push/"+token+"/cdn_site?status=up&msg=OK", "", nil, http.StatusOK)
	id := fmt.Sprintf("%v", created["id"])
	env.expectStatus(t, http.MethodDelete, "/api/v1/admin/push-keys/"+id, "", nil, http.StatusOK)
	if _, ok := pushkey.Lookup(token); ok {
		t.Error("expected the deleted key to be rejected")
	}
	env.expectStatus(t, http.MethodGet, "/api/push/"+token+"/cdn_site?status=up&msg=OK", "", nil, http.StatusNotFound)
	env.expectStatus(t, http.MethodGet, "/api/push/yaml-key-token-0123456789/cdn_site?status=up", "", nil, http.StatusOK)
	env.expectStatus(t, http.MethodDelete, "/api/v1/admin/push-keys/"+id, "", nil, http.StatusNotFound)
	env.expectStatus(t, http.MethodDelete, "/api/v1/admin/push-keys/abc", "", nil, http.StatusNotFound)
	if status, _, _ := env.do(t, http.MethodGet, "/api/v1/admin/push-keys", "", map[string]string{"Authorization": ""}); status != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", status)
	}
}
