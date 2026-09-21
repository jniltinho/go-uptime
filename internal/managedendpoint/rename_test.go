// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/custom"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

// Renaming during a slow execution must neither record anything under the old key nor lose the triggered alerts
func TestService_RenameDuringSlowExecution(t *testing.T) {
	t.Setenv("MOCK_ALERT_PROVIDER", "true")
	requests := make(chan struct{}, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	initializeSQLiteStore(t)
	disabled := false
	cfg := &config.Config{
		Alerting:    &alerting.Config{Custom: &custom.AlertProvider{DefaultConfig: custom.Config{URL: "https://example.org"}}},
		Storage:     &storage.Config{Type: storage.TypeSQLite},
		Maintenance: &maintenance.Config{Enabled: &disabled},
	}
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	watchdog.Monitor(cfg)
	t.Cleanup(func() { watchdog.Shutdown(cfg) })
	service := NewService(cfg)
	definition := "name: site\ngroup: web\ninterval: 1h\nurl: " + server.URL + "\nconditions: [\"[STATUS] == 200\"]\nalerts:\n  - type: custom\n    failure-threshold: 1\n    success-threshold: 5\n"
	if _, err := service.Create([]byte(definition), "ops@example.com"); err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	select {
	case <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first execution")
	}
	// Triggered alert of an ongoing incident, persisted under the old key with the same alert configuration
	incident, err := Prepare([]byte(definition), Context{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	incident.Endpoint.Alerts[0].Triggered, incident.Endpoint.Alerts[0].ResolveKey = true, "incident-1"
	if err := store.Get().UpsertTriggeredEndpointAlert(incident.Endpoint, incident.Endpoint.Alerts[0]); err != nil {
		t.Fatalf("failed to persist the triggered alert: %v", err)
	}

	detail, err := service.Update("web_site", []byte(strings.Replace(definition, "group: web", "group: clientes", 1)), 1, "ops@example.com")
	if err != nil {
		t.Fatalf("failed to rename: %v", err)
	}
	if detail.Key != "clientes_site" || detail.Version != 2 {
		t.Errorf("expected clientes_site at version 2, got %+v", detail.Item)
	}
	renamed := EndpointByKey("clientes_site")
	if renamed == nil || EndpointByKey("web_site") != nil || watchdog.IsEndpointMonitored("web_site") || !watchdog.IsEndpointMonitored("clientes_site") {
		t.Fatal("expected only the new key to be loaded and monitored")
	}
	// The first execution of the new key takes 2 seconds, so the restored state has not been changed yet
	if !renamed.Alerts[0].Triggered || renamed.Alerts[0].ResolveKey != "incident-1" {
		t.Errorf("expected the triggered alert to be restored into the renamed endpoint, got triggered=%v resolveKey=%s", renamed.Alerts[0].Triggered, renamed.Alerts[0].ResolveKey)
	}
	if exists, resolveKey, _, err := store.Get().GetTriggeredEndpointAlert(renamed, renamed.Alerts[0]); err != nil || !exists || resolveKey != "incident-1" {
		t.Errorf("expected the triggered alert under the new key, got exists=%v resolveKey=%s err=%v", exists, resolveKey, err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := store.Get().GetEndpointStatusByKey("web_site", paging.NewEndpointStatusParams().WithResults(1, 20)); !errors.Is(err, common.ErrEndpointNotFound) {
		t.Errorf("expected nothing to be recorded under the old key, got %v", err)
	}
}
