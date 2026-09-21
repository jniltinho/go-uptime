// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package watchdog

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/heartbeat"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
)

func processRetriesTestResult(t *testing.T, ee *endpoint.ExternalEndpoint, result *endpoint.Result) *endpoint.Result {
	t.Helper()
	cfg := newRegistryTestConfig(t)
	cfg.Alerting = &alerting.Config{}
	result.Timestamp = time.Now()
	if err := ProcessExternalEndpointResult(ee, result, cfg, true); err != nil {
		t.Fatal(err)
	}
	return result
}

// Failures of an external endpoint with retries are pending until the retries are used, and a success resets them
func TestProcessExternalEndpointResult_Retries(t *testing.T) {
	ee := &endpoint.ExternalEndpoint{Name: "retries", Group: "watchdog", Token: "token", Heartbeat: heartbeat.Config{Interval: time.Minute, Retries: 2}}
	ForgetExternalEndpoint(ee.Key())
	t.Cleanup(func() { ForgetExternalEndpoint(ee.Key()) })
	down := func() *endpoint.Result {
		return processRetriesTestResult(t, ee, &endpoint.Result{Success: false, Errors: []string{"timeout"}})
	}
	for i := 1; i <= 2; i++ {
		result := down()
		if !result.Pending || len(result.Errors) != 0 || result.Message != "timeout" {
			t.Fatalf("expected the failure %d to be pending with the errors as message, got %+v", i, result)
		}
		if ee.NumberOfFailuresInARow != 0 {
			t.Fatalf("expected a pending result not to count as a failure, got %d", ee.NumberOfFailuresInARow)
		}
	}
	if result := down(); result.Pending || ee.NumberOfFailuresInARow != 1 {
		t.Fatalf("expected a failure once the retries are used, got %+v with %d failures", result, ee.NumberOfFailuresInARow)
	}
	if result := down(); result.Pending || ee.NumberOfFailuresInARow != 2 {
		t.Fatalf("expected the next failures to stay failures, got %+v", result)
	}
	// An explicit pending push changes neither the retries nor the counters of the alerts
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Pending: true, Message: "Aguardando"}); !result.Pending || ee.NumberOfFailuresInARow != 2 {
		t.Fatalf("expected an explicit pending push to leave the counters as they were, got %+v", result)
	}
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: true}); result.Pending || ee.NumberOfFailuresInARow != 0 {
		t.Fatalf("expected a success to reset the failures, got %+v", result)
	}
	if result := down(); !result.Pending {
		t.Fatalf("expected a success to reset the retries, got %+v", result)
	}
}

// Without retries, a failure is recorded immediately
func TestProcessExternalEndpointResult_WithoutRetries(t *testing.T) {
	ee := &endpoint.ExternalEndpoint{Name: "no-retries", Group: "watchdog", Token: "token", Heartbeat: heartbeat.Config{Interval: time.Minute}}
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: false, Errors: []string{"down"}}); result.Pending || len(result.Errors) != 1 {
		t.Fatalf("expected an immediate failure, got %+v", result)
	}
}

// Forgetting an external endpoint forgets the retries used
func TestForgetExternalEndpoint(t *testing.T) {
	ee := &endpoint.ExternalEndpoint{Name: "forgotten", Group: "watchdog", Token: "token", Heartbeat: heartbeat.Config{Interval: time.Minute, Retries: 1}}
	processRetriesTestResult(t, ee, &endpoint.Result{Success: false})
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: false}); result.Pending {
		t.Fatalf("expected the retries to be used, got %+v", result)
	}
	ForgetExternalEndpointsNotIn([]string{"other_key"})
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: false}); !result.Pending {
		t.Fatalf("expected the retries to be forgotten, got %+v", result)
	}
	ForgetExternalEndpoint(ee.Key())
	if _, exists := lastPushes.Load(ee.Key()); exists {
		t.Error("expected the last push to be forgotten")
	}
}

// During a maintenance window, a failure is recorded as a failure without using the retries
func TestProcessExternalEndpointResult_RetriesDuringMaintenance(t *testing.T) {
	window := &maintenance.Config{Start: time.Now().UTC().Add(-time.Hour).Format("15:04"), Duration: 4 * time.Hour, Timezone: "UTC"}
	if err := window.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	if !window.IsUnderMaintenance() {
		t.Skip("the maintenance window crosses midnight in an unsupported way at this time")
	}
	ee := &endpoint.ExternalEndpoint{Name: "maintenance", Group: "watchdog", Token: "token", Heartbeat: heartbeat.Config{Interval: time.Minute, Retries: 1}, MaintenanceWindows: []*maintenance.Config{window}}
	t.Cleanup(func() { ForgetExternalEndpoint(ee.Key()) })
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: false}); result.Pending {
		t.Fatalf("expected a failure during the maintenance window, got %+v", result)
	}
	ee.MaintenanceWindows = nil
	if result := processRetriesTestResult(t, ee, &endpoint.Result{Success: false}); !result.Pending {
		t.Fatalf("expected the retries not to be used during the maintenance window, got %+v", result)
	}
}
