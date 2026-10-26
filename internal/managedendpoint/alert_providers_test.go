// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alertprovider"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

// setupAlertProviders starts a cycle with only managed endpoints and providers, and returns its configuration
func setupAlertProviders(t *testing.T) *config.Config {
	t.Helper()
	initializeSQLiteStore(t)
	disabled := false
	cfg := &config.Config{Maintenance: &maintenance.Config{Enabled: &disabled}}
	alertprovider.Load(cfg)
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	watchdog.Monitor(cfg)
	t.Cleanup(func() { watchdog.Shutdown(cfg) })
	return cfg
}

func customProviderDefinition(url string, failureThreshold int) []byte {
	return []byte(`{"type": "custom", "url": "` + url + `", "default-alert": {"failure-threshold": ` + string(rune('0'+failureThreshold)) + `}}`)
}

func TestAlertProviders_EndpointUsesManagedProvider(t *testing.T) {
	cfg := setupAlertProviders(t)
	service := NewService(cfg)
	pushEndpoint := []byte("type: push\nname: backup\ngroup: jobs\ntoken: tok_0123456789abcdefABCDEF01\nheartbeat:\n  interval: 1h\nalerts:\n  - type: custom\n")
	if _, err := service.Create(pushEndpoint, "admin"); !errors.Is(err, ErrAlertProviderNotConfigured) {
		t.Fatalf("expected the alert to be refused without provider, got %v", err)
	}
	if _, err := alertprovider.Create(customProviderDefinition("https://example.org/hook", 3), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(pushEndpoint, "admin"); err != nil {
		t.Fatalf("expected the alert of a managed provider to be accepted, got %v", err)
	}
	state := Get("jobs_backup")
	if got := state.Push.Alerts[0].FailureThreshold; got != 3 {
		t.Errorf("expected the failure threshold of the default alert, got %d", got)
	}
	// Changing the default alert restarts the endpoint with the new inherited value
	result, err := alertprovider.Update("custom", customProviderDefinition(alertprovider.MaskedValue, 5), 1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Restarted, []string{"jobs_backup"}) {
		t.Errorf("expected jobs_backup to be restarted, got %v", result.Restarted)
	}
	if got := Get("jobs_backup").Push.Alerts[0].FailureThreshold; got != 5 {
		t.Errorf("expected the new failure threshold, got %d", got)
	}
	if !watchdog.IsEndpointMonitored("jobs_backup") {
		t.Error("expected the restarted endpoint to be monitored")
	}
	// A provider in use cannot be deleted
	var inUse *alertprovider.InUseError
	if err := alertprovider.Delete("custom", 2, "admin"); !errors.As(err, &inUse) || !slices.Equal(inUse.Keys, []string{"jobs_backup"}) {
		t.Fatalf("expected the deletion to be refused because of jobs_backup, got %v", err)
	}
}

func TestAlertProviders_DisabledEndpointCountsAsUser(t *testing.T) {
	cfg := setupAlertProviders(t)
	if _, err := alertprovider.Create(customProviderDefinition("https://example.org/hook", 3), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(cfg).Create([]byte("name: old\ngroup: web\nenabled: false\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\nalerts:\n  - type: custom\n"), "admin"); err != nil {
		t.Fatal(err)
	}
	var inUse *alertprovider.InUseError
	if err := alertprovider.Delete("custom", 1, "admin"); !errors.As(err, &inUse) || !slices.Equal(inUse.Keys, []string{"web_old"}) {
		t.Fatalf("expected the disabled endpoint to block the deletion, got %v", err)
	}
}

func TestAlertProviders_FileTakesPrecedence(t *testing.T) {
	initializeSQLiteStore(t)
	cfg := newTestContext().Config
	alertprovider.Load(cfg)
	if _, err := alertprovider.Create(customProviderDefinition("https://example.org/hook", 3), "admin"); !errors.Is(err, alertprovider.ErrConfiguredInFile) {
		t.Fatalf("expected a type of the file to be refused, got %v", err)
	}
}

func TestAlertProviders_TestNotification(t *testing.T) {
	setupAlertProviders(t)
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	result, err := alertprovider.Test([]byte(`{"type": "custom", "url": "`+server.URL+`/hook?token=secret-token-123"}`), "", "", true)
	if err != nil || !result.Success || !result.Resolved || received.Load() != 2 {
		t.Fatalf("expected the test notification and its resolution to be delivered, got result=%+v err=%v received=%d", result, err, received.Load())
	}
	failed, err := alertprovider.Test([]byte(`{"type": "custom", "url": "http://127.0.0.1:1/hook?token=secret-token-123"}`), "", "", true)
	if err != nil || failed.Success || failed.Resolved {
		t.Fatalf("expected a failed test, got result=%+v err=%v", failed, err)
	}
	if strings.Contains(failed.Error, "secret-token-123") {
		t.Errorf("expected the error to be redacted, got %q", failed.Error)
	}
	if _, err := alertprovider.Test(nil, string(alert.TypeTelegram), "", false); !errors.Is(err, alertprovider.ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured for a type without provider, got %v", err)
	}
}
