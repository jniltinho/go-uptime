package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

type adminTestEnvironment struct {
	app        *echo.Echo
	serverURL  string
	slowURL    string
	slowServed chan struct{}
}

func newAdminTestEnvironment(t *testing.T) *adminTestEnvironment {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	slowServed := make(chan struct{}, 10)
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowServed <- struct{}{}
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slowServer.Close)
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	yamlEndpoint := &endpoint.Endpoint{Name: "api", Group: "core", URL: server.URL, Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := yamlEndpoint.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	disabled := false
	cfg := &config.Config{
		Security:    &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}},
		Admin:       &admin.Config{Enabled: true},
		Storage:     &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "gatus.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		Maintenance: &maintenance.Config{Enabled: &disabled},
		Endpoints:   []*endpoint.Endpoint{yamlEndpoint},
	}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		// Other tests of the package use the default memory store
		_ = store.Initialize(nil)
		_, _ = managedendpoint.Load(&config.Config{})
		pushkey.Load(&config.Config{})
	})
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatalf("failed to load managed endpoints: %v", err)
	}
	pushkey.Load(cfg)
	watchdog.Monitor(cfg)
	t.Cleanup(func() { watchdog.Shutdown(cfg) })
	return &adminTestEnvironment{app: New(cfg).Router(), serverURL: server.URL, slowURL: slowServer.URL, slowServed: slowServed}
}

func (env *adminTestEnvironment) do(t *testing.T, method, path, body string, headers map[string]string) (int, http.Header, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "status.example.com"
	request.SetBasicAuth("admin", "secret")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/yaml")
	}
	for name, value := range headers {
		if len(value) == 0 {
			request.Header.Del(name)
		} else {
			request.Header.Set(name, value)
		}
	}
	response, err := testHTTP(env.app, request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var decoded map[string]any
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &decoded)
	}
	return response.StatusCode, response.Header, decoded
}

func (env *adminTestEnvironment) expectStatus(t *testing.T, method, path, body string, headers map[string]string, expected int) (http.Header, map[string]any) {
	t.Helper()
	status, header, decoded := env.do(t, method, path, body, headers)
	if status != expected {
		t.Fatalf("%s %s: expected %d, got %d (%v)", method, path, expected, status, decoded)
	}
	return header, decoded
}

func TestAdminAPI(t *testing.T) {
	env := newAdminTestEnvironment(t)
	const conditions = "conditions: [\"[STATUS] == 200\"]\n"
	siteDefinition := "name: site\ngroup: web\ninterval: 1h\nurl: " + env.serverURL + "\nheaders:\n  Authorization: Bearer abc123\n" + conditions

	t.Run("requires-authentication", func(t *testing.T) {
		if status, _, _ := env.do(t, http.MethodGet, "/api/v1/admin/endpoints", "", map[string]string{"Authorization": ""}); status != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", status)
		}
	})
	t.Run("metadata", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/metadata", "", nil, http.StatusOK)
		if _, ok := body["alertTypes"].([]any); !ok {
			t.Errorf("expected alertTypes in metadata, got %v", body)
		}
	})
	t.Run("create", func(t *testing.T) {
		header, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", siteDefinition, nil, http.StatusCreated)
		if header.Get("ETag") != `"1"` || body["key"] != "web_site" || body["source"] != "admin" {
			t.Errorf("unexpected creation response: etag=%s body=%v", header.Get("ETag"), body)
		}
		if !watchdog.IsEndpointMonitored("web_site") {
			t.Error("expected the created endpoint to be monitored")
		}
	})
	t.Run("create-rejections", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", siteDefinition, nil, http.StatusConflict)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: api\ngroup: core\nurl: https://example.org\n"+conditions, nil, http.StatusConflict)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: nocondition\ngroup: web\nurl: https://example.org\n", nil, http.StatusBadRequest)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: typo\ngroup: web\nintervall: 1m\nurl: https://example.org\n"+conditions, nil, http.StatusBadRequest)
	})
	t.Run("list-and-get", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/endpoints", http.NoBody)
		request.Host = "status.example.com"
		request.SetBasicAuth("admin", "secret")
		response, err := testHTTP(env.app, request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var items []map[string]any
		if err := json.NewDecoder(response.Body).Decode(&items); err != nil {
			t.Fatal(err)
		}
		sources := map[string]string{}
		for _, item := range items {
			sources[item["key"].(string)] = item["source"].(string)
		}
		if len(items) != 2 || sources["core_api"] != "config" || sources["web_site"] != "admin" {
			t.Errorf("unexpected list: %v", items)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/unknown_endpoint", "", nil, http.StatusNotFound)
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_site", "", nil, http.StatusOK)
		headers := body["definition"].(map[string]any)["json"].(map[string]any)["headers"].(map[string]any)
		if headers["Authorization"] != managedendpoint.Mask {
			t.Errorf("expected the Authorization header to be masked, got %v", headers)
		}
	})
	t.Run("validate-does-not-persist", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/validate", "name: validated\ngroup: web\nurl: https://example.org\n"+conditions, nil, http.StatusOK)
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_validated", "", nil, http.StatusNotFound)
	})
	t.Run("parse-returns-submitted-document", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/parse", "name: typed\ngroup: web\nheaders:\n  Authorization: Bearer typed\n", nil, http.StatusOK)
		headers, _ := body["json"].(map[string]any)["headers"].(map[string]any)
		if headers["Authorization"] != "Bearer typed" {
			t.Errorf("expected the submitted secret to be returned as is, got %v", body)
		}
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/parse", "name: [\n", nil, http.StatusBadRequest)
	})
	t.Run("spa-routes", func(t *testing.T) {
		for _, path := range []string{"/admin", "/admin/endpoints/new", "/admin/endpoints/web_site/edit", "/admin/status-pages", "/admin/status-pages/new", "/admin/status-pages/infra/edit"} {
			request := httptest.NewRequest(http.MethodGet, path, http.NoBody)
			response, err := testHTTP(env.app, request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
				t.Errorf("expected %s to serve the application, got %d %s", path, response.StatusCode, response.Header.Get("Content-Type"))
			}
		}
	})
	t.Run("update-rejections", func(t *testing.T) {
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", siteDefinition, nil, http.StatusPreconditionRequired)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", siteDefinition, map[string]string{"If-Match": `"5"`}, http.StatusPreconditionFailed)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/core_api", "name: api\ngroup: core\nurl: https://example.org\n"+conditions, map[string]string{"If-Match": `"1"`}, http.StatusConflict)
	})
	t.Run("update-keeps-masked-secret", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_site", "", nil, http.StatusOK)
		document := body["definition"].(map[string]any)["json"].(map[string]any)
		document["interval"] = "2h"
		payload, _ := json.Marshal(document)
		header, _ := env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", string(payload), map[string]string{"If-Match": `"1"`, "Content-Type": "application/json"}, http.StatusOK)
		if header.Get("ETag") != `"2"` {
			t.Errorf("expected version 2, got %s", header.Get("ETag"))
		}
		managedEndpointStore, _ := store.GetManagedEndpointStore()
		stored, err := managedEndpointStore.GetManagedEndpoint("web_site")
		if err != nil || !strings.Contains(stored.Definition, "Bearer abc123") || !strings.Contains(stored.Definition, "2h") {
			t.Errorf("expected the secret to be kept and the interval to be updated, got %+v (err=%v)", stored, err)
		}
	})
	t.Run("disable-and-enable", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/web_site/disable", "", map[string]string{"If-Match": `"2"`}, http.StatusOK)
		if watchdog.IsEndpointMonitored("web_site") {
			t.Error("expected the disabled endpoint not to be monitored")
		}
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/web_site/enable", "", map[string]string{"If-Match": `"3"`}, http.StatusOK)
		if !watchdog.IsEndpointMonitored("web_site") {
			t.Error("expected the enabled endpoint to be monitored")
		}
	})
	t.Run("rename", func(t *testing.T) {
		params := paging.NewEndpointStatusParams().WithResults(1, 100)
		if err := store.Get().InsertEndpointResult(&endpoint.Endpoint{Name: "site", Group: "web"}, &endpoint.Result{Success: true, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		before, err := store.Get().GetEndpointStatusByKey("web_site", params)
		if err != nil || len(before.Results) == 0 {
			t.Fatalf("expected results before the rename, got %v (err=%v)", before, err)
		}
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_site", "", nil, http.StatusOK)
		document := body["definition"].(map[string]any)["json"].(map[string]any)
		renamedTo := func(name, group string) string {
			document["name"], document["group"] = name, group
			payload, _ := json.Marshal(document)
			return string(payload)
		}
		ifMatch := func(version string) map[string]string {
			return map[string]string{"If-Match": `"` + version + `"`, "Content-Type": "application/json"}
		}
		// Keys already in use: another managed endpoint, an endpoint of the configuration file and stored data
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints", "name: other\ngroup: web\ninterval: 1h\nurl: "+env.serverURL+"\n"+conditions, nil, http.StatusCreated)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", renamedTo("other", "web"), ifMatch("4"), http.StatusConflict)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", renamedTo("api", "core"), ifMatch("4"), http.StatusConflict)
		if err := store.Get().InsertEndpointResult(&endpoint.Endpoint{Name: "orphan", Group: "web"}, &endpoint.Result{Success: true, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", renamedTo("orphan", "web"), ifMatch("4"), http.StatusConflict)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", renamedTo("site", "clientes"), ifMatch("3"), http.StatusPreconditionFailed)
		if !watchdog.IsEndpointMonitored("web_site") {
			t.Fatal("expected the endpoint to be monitored again after the rejected renames")
		}

		header, body := env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/web_site", renamedTo("site", "clientes"), ifMatch("4"), http.StatusOK)
		if header.Get("ETag") != `"5"` || body["key"] != "clientes_site" || body["group"] != "clientes" {
			t.Errorf("unexpected rename response: etag=%s body=%v", header.Get("ETag"), body)
		}
		if watchdog.IsEndpointMonitored("web_site") || !watchdog.IsEndpointMonitored("clientes_site") {
			t.Error("expected only the new key to be monitored")
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_site", "", nil, http.StatusNotFound)
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/clientes_site", "", nil, http.StatusOK)
		after, err := store.Get().GetEndpointStatusByKey("clientes_site", params)
		if err != nil || after.Group != "clientes" || len(after.Results) < len(before.Results) {
			t.Errorf("expected the history under the new key, got %+v (err=%v)", after, err)
		}
		if _, err := store.Get().GetEndpointStatusByKey("web_site", params); !errors.Is(err, common.ErrEndpointNotFound) {
			t.Errorf("expected no status under the old key, got %v", err)
		}
		managedEndpointStore, _ := store.GetManagedEndpointStore()
		if stored, err := managedEndpointStore.GetManagedEndpoint("clientes_site"); err != nil || !strings.Contains(stored.Definition, "Bearer abc123") {
			t.Errorf("expected the masked secret to be kept under the new key, got %+v (err=%v)", stored, err)
		}

		header, body = env.expectStatus(t, http.MethodPut, "/api/v1/admin/endpoints/clientes_site", renamedTo("site", "web"), ifMatch("5"), http.StatusOK)
		if header.Get("ETag") != `"6"` || body["key"] != "web_site" {
			t.Errorf("unexpected response when renaming back: etag=%s body=%v", header.Get("ETag"), body)
		}
		if after, err := store.Get().GetEndpointStatusByKey("web_site", params); err != nil || len(after.Results) < len(before.Results) {
			t.Errorf("expected the history back under the original key, got %+v (err=%v)", after, err)
		}
	})
	t.Run("test-endpoint", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/endpoints/test", "name: tested\ngroup: web\nurl: "+env.serverURL+"\n"+conditions, nil, http.StatusOK)
		results, _ := body["conditionResults"].([]any)
		if body["success"] != true || len(results) != 1 {
			t.Errorf("unexpected test result: %v", body)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_tested", "", nil, http.StatusNotFound)
	})
	t.Run("too-many-tests", func(t *testing.T) {
		slowDefinition := "name: slow\ngroup: web\nurl: " + env.slowURL + "\n" + conditions
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				env.do(t, http.MethodPost, "/api/v1/admin/endpoints/test", slowDefinition, nil)
			}()
		}
		for i := 0; i < 2; i++ {
			select {
			case <-env.slowServed:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for the slow tests to start")
			}
		}
		if status, _, _ := env.do(t, http.MethodPost, "/api/v1/admin/endpoints/test", slowDefinition, nil); status != http.StatusTooManyRequests {
			t.Errorf("expected 429 with 2 tests in progress, got %d", status)
		}
		wg.Wait()
	})
	t.Run("write-during-cycle", func(t *testing.T) {
		lifecycle.BeginCycle()
		status, _, _ := env.do(t, http.MethodPost, "/api/v1/admin/endpoints", "name: during-cycle\ngroup: web\nurl: https://example.org\n"+conditions, nil)
		lifecycle.EndCycle()
		if status != http.StatusServiceUnavailable {
			t.Errorf("expected 503 during a cycle, got %d", status)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_during-cycle", "", nil, http.StatusNotFound)
	})
	t.Run("delete", func(t *testing.T) {
		env.expectStatus(t, http.MethodDelete, "/api/v1/admin/endpoints/core_api", "", map[string]string{"If-Match": `"1"`}, http.StatusConflict)
		_, body := env.expectStatus(t, http.MethodDelete, "/api/v1/admin/endpoints/web_site", "", map[string]string{"If-Match": `"6"`}, http.StatusOK)
		if body["triggeredAlerts"] != float64(0) {
			t.Errorf("unexpected deletion response: %v", body)
		}
		if watchdog.IsEndpointMonitored("web_site") {
			t.Error("expected the deleted endpoint not to be monitored")
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/endpoints/web_site", "", nil, http.StatusNotFound)
	})
}
