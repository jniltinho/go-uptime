// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

func initializeSQLiteStore(t *testing.T) store.ManagedEndpointStore {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() { store.Get().Close() })
	managedEndpointStore, ok := store.GetManagedEndpointStore()
	if !ok {
		t.Fatal("expected the sqlite store to support managed endpoints")
	}
	return managedEndpointStore
}

func createStoredEndpoint(t *testing.T, managedEndpointStore store.ManagedEndpointStore, key, definition string) {
	t.Helper()
	if err := managedEndpointStore.CreateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: definition}, nil); err != nil {
		t.Fatalf("failed to create managed endpoint %s: %v", key, err)
	}
}

func TestLoad_MemoryStorage(t *testing.T) {
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	keys, err := Load(newTestContext().Config)
	if err != nil || len(keys) != 0 || len(List()) != 0 {
		t.Errorf("expected nothing to load with the memory storage, got keys=%v err=%v", keys, err)
	}
}

func TestLoad(t *testing.T) {
	managedEndpointStore := initializeSQLiteStore(t)
	const conditions = "conditions: [\"[STATUS] == 200\"]\n"
	createStoredEndpoint(t, managedEndpointStore, "web_site", "name: site\ngroup: web\nurl: https://example.org\n"+conditions)
	createStoredEndpoint(t, managedEndpointStore, "core_api", "name: api\ngroup: core\nurl: https://example.org\n"+conditions)
	createStoredEndpoint(t, managedEndpointStore, "web_invalid", "name: invalid\ngroup: web\nurl: https://example.org\n"+conditions+"alerts:\n  - type: slack\n")
	keys, err := Load(newTestContext().Config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected the keys of every stored managed endpoint, got %v", keys)
	}
	if state := Get("web_site"); state == nil || state.Endpoint == nil || state.Endpoint.Interval != time.Minute || !strings.Contains(string(state.Effective), "interval: 1m0s") {
		t.Errorf("expected web_site to be valid with default values and its effective definition, got %+v", state)
	}
	if definition := configDefinition("core_api"); definition == nil {
		t.Error("expected the effective definition of the endpoint of the configuration file to be generated")
	}
	if state := Get("core_api"); state == nil || !state.InConflict() || state.Endpoint != nil {
		t.Errorf("expected core_api to be in conflict with the configuration file, got %+v", state)
	}
	if state := Get("web_invalid"); state == nil || state.Endpoint != nil || !errors.Is(state.Err, ErrAlertProviderNotConfigured) {
		t.Errorf("expected web_invalid to be invalid, got %+v", state)
	}
	if EndpointByKey("WEB_SITE") == nil {
		t.Error("expected EndpointByKey to ignore case")
	}
	if EndpointByKey("core_api") != nil {
		t.Error("expected no monitored endpoint for a managed endpoint in conflict")
	}
}

func TestStartMonitoring(t *testing.T) {
	managedEndpointStore := initializeSQLiteStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	createStoredEndpoint(t, managedEndpointStore, "web_enabled", "name: enabled\ngroup: web\ninterval: 1h\nurl: "+server.URL+"\nconditions: [\"[STATUS] == 200\"]\n")
	createStoredEndpoint(t, managedEndpointStore, "web_disabled", "name: disabled\ngroup: web\nenabled: false\ninterval: 1h\nurl: "+server.URL+"\nconditions: [\"[STATUS] == 200\"]\n")
	cfg := newTestContext().Config
	// Only managed endpoints are monitored in this test
	cfg.Endpoints, cfg.ExternalEndpoints, cfg.Suites = nil, nil, nil
	disabled := false
	cfg.Maintenance = &maintenance.Config{Enabled: &disabled}
	if _, err := Load(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	watchdog.Monitor(cfg)
	defer watchdog.Shutdown(cfg)
	StartMonitoring()
	if !watchdog.IsEndpointMonitored("web_enabled") {
		t.Error("expected the enabled managed endpoint to be monitored")
	}
	if watchdog.IsEndpointMonitored("web_disabled") {
		t.Error("expected the disabled managed endpoint not to be monitored")
	}
}
