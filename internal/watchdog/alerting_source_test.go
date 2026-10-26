// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package watchdog

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

func TestMigrateTriggeredAlerts(t *testing.T) {
	err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50})
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Get().Close()
	previous := newAlertStateTestEndpoint(true)
	if err := store.Get().InsertEndpointResult(previous, &endpoint.Result{Timestamp: time.Now()}); err != nil {
		t.Fatalf("failed to insert result: %v", err)
	}
	previous.Alerts[0].Triggered, previous.Alerts[0].ResolveKey = true, "incident-1"
	previous.NumberOfSuccessesInARow = 1
	if err := store.Get().UpsertTriggeredEndpointAlert(previous, previous.Alerts[0]); err != nil {
		t.Fatalf("failed to persist triggered alert: %v", err)
	}
	// The provider changed its default alert: the inherited thresholds of the alert, and so its checksum, changed
	next := newAlertStateTestEndpoint(true)
	next.Alerts[0].FailureThreshold, next.Alerts[0].SuccessThreshold = 5, 1
	if migrated := MigrateTriggeredAlerts(previous, next); migrated != 1 {
		t.Fatalf("expected 1 triggered alert to be migrated, got %d", migrated)
	}
	if restored := RestorePersistedTriggeredAlerts(next); restored != 1 {
		t.Fatalf("expected the migrated alert to be restored, got %d", restored)
	}
	if !next.Alerts[0].Triggered || next.Alerts[0].ResolveKey != "incident-1" || next.NumberOfSuccessesInARow != 1 {
		t.Errorf("expected the triggered state to survive, got triggered=%v resolveKey=%q successes=%d", next.Alerts[0].Triggered, next.Alerts[0].ResolveKey, next.NumberOfSuccessesInARow)
	}
	// An alert of another type at the same position is not migrated
	other := newAlertStateTestEndpoint(true)
	other.Alerts[0].Type = alert.TypeSlack
	if migrated := MigrateTriggeredAlerts(next, other); migrated != 0 {
		t.Errorf("expected nothing to be migrated to another type, got %d", migrated)
	}
}

func TestDescribeSendError(t *testing.T) {
	defer sendErrorRedactor.Store(nil)
	if got := describeSendError(alert.TypeSlack, errors.New("raw")); got != "raw" {
		t.Errorf("expected the raw error without redactor, got %q", got)
	}
	SetSendErrorRedactor(func(alertType alert.Type, err error) string { return string(alertType) + ": redacted" })
	if got := describeSendError(alert.TypeSlack, errors.New("https://hooks.example.com/secret")); got != "slack: redacted" {
		t.Errorf("expected the redactor to be used, got %q", got)
	}
}
