package watchdog

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

func newAlertStateTestEndpoint(alertEnabled bool) *endpoint.Endpoint {
	description := "api is down"
	return &endpoint.Endpoint{
		Name:  "api",
		Group: "core",
		URL:   "https://example.org",
		Alerts: []*alert.Alert{{
			Type:             alert.TypeCustom,
			Enabled:          &alertEnabled,
			FailureThreshold: 3,
			SuccessThreshold: 2,
			Description:      &description,
		}},
	}
}

func TestRestorePersistedTriggeredAlerts(t *testing.T) {
	err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50})
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Get().Close()
	// Persist a triggered alert, as the watchdog does when an alert is sent
	running := newAlertStateTestEndpoint(true)
	if err := store.Get().InsertEndpointResult(running, &endpoint.Result{Timestamp: time.Now()}); err != nil {
		t.Fatalf("failed to insert result: %v", err)
	}
	running.Alerts[0].Triggered, running.Alerts[0].ResolveKey = true, "incident-1"
	running.NumberOfSuccessesInARow = 1
	if err := store.Get().UpsertTriggeredEndpointAlert(running, running.Alerts[0]); err != nil {
		t.Fatalf("failed to persist triggered alert: %v", err)
	}

	t.Run("same-alert-configuration-is-restored", func(t *testing.T) {
		replacement := newAlertStateTestEndpoint(true)
		if restored := RestorePersistedTriggeredAlerts(replacement); restored != 1 {
			t.Fatalf("expected 1 triggered alert to be restored, got %d", restored)
		}
		restoredAlert := replacement.Alerts[0]
		if !restoredAlert.Triggered || restoredAlert.ResolveKey != "incident-1" {
			t.Errorf("expected triggered alert with resolve key incident-1, got triggered=%v resolveKey=%q", restoredAlert.Triggered, restoredAlert.ResolveKey)
		}
		if replacement.NumberOfSuccessesInARow != 1 || replacement.NumberOfFailuresInARow != 3 {
			t.Errorf("expected counters successes=1 failures=3, got successes=%d failures=%d", replacement.NumberOfSuccessesInARow, replacement.NumberOfFailuresInARow)
		}
	})

	t.Run("disabled-alert-is-cleared", func(t *testing.T) {
		replacement := newAlertStateTestEndpoint(false)
		if restored := RestorePersistedTriggeredAlerts(replacement); restored != 0 {
			t.Fatalf("expected no triggered alert to be restored, got %d", restored)
		}
		exists, _, _, err := store.Get().GetTriggeredEndpointAlert(running, running.Alerts[0])
		if err != nil {
			t.Fatalf("failed to get triggered alert: %v", err)
		}
		if exists {
			t.Error("expected the persisted triggered alert to be deleted")
		}
	})
}
