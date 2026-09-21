package watchdog

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/custom"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/heartbeat"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

func endpointResults(t *testing.T, key string) []*endpoint.Result {
	t.Helper()
	status, err := store.Get().GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 100))
	if err != nil || status == nil {
		return nil
	}
	return status.Results
}

// A heartbeat must record one failure for every full interval without push, including consecutive intervals
func TestExternalEndpointHeartbeat_OneFailurePerInterval(t *testing.T) {
	ee := &endpoint.ExternalEndpoint{Name: "consecutive", Group: "heartbeat", Token: "token", Heartbeat: heartbeat.Config{Interval: 300 * time.Millisecond}}
	cfg := newRegistryTestConfig(t)
	cfg.ExternalEndpoints = []*endpoint.ExternalEndpoint{ee}
	Monitor(cfg)
	defer Shutdown(cfg)
	if !IsEndpointMonitored(ee.Key()) {
		t.Fatal("expected the heartbeat to be in the registry")
	}
	// Ticks at 300, 600 and 900 ms: before the fork, the failure of a tick counted as a new result for the next one
	deadline := time.Now().Add(1150 * time.Millisecond)
	for len(endpointResults(t, ee.Key())) < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	results := endpointResults(t, ee.Key())
	if len(results) < 3 {
		t.Fatalf("expected a failure for each of the 3 intervals without push, got %d results", len(results))
	}
	for _, result := range results {
		if result.Success || len(result.Errors) != 1 {
			t.Errorf("expected heartbeat failures, got %+v", result)
		}
	}
	if err := StopEndpoint(ee.Key(), SourceConfig); err != nil {
		t.Fatalf("expected the heartbeat to be stopped by key, got %v", err)
	}
	count := len(endpointResults(t, ee.Key()))
	time.Sleep(700 * time.Millisecond)
	if after := len(endpointResults(t, ee.Key())); after != count {
		t.Errorf("expected no failure after stopping the heartbeat, got %d new results", after-count)
	}
}

// Pushes within the interval must prevent any heartbeat failure
func TestExternalEndpointHeartbeat_PushRestartsTheInterval(t *testing.T) {
	ee := &endpoint.ExternalEndpoint{Name: "pushed", Group: "heartbeat", Token: "token", Heartbeat: heartbeat.Config{Interval: 400 * time.Millisecond}}
	cfg := newRegistryTestConfig(t)
	cfg.ExternalEndpoints = []*endpoint.ExternalEndpoint{ee}
	Monitor(cfg)
	defer Shutdown(cfg)
	for i := 0; i < 8; i++ {
		if err := ProcessExternalEndpointResult(ee, &endpoint.Result{Success: true, Timestamp: time.Now()}, cfg, true); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
	}
	for _, result := range endpointResults(t, ee.Key()) {
		if !result.Success {
			t.Fatalf("expected no heartbeat failure while pushes arrive within the interval, got %+v", result)
		}
	}
}

// A failed push must update the counters of the external endpoint and trigger its alert
func TestProcessExternalEndpointResult_Alerting(t *testing.T) {
	t.Setenv("MOCK_ALERT_PROVIDER", "true")
	enabled := true
	ee := &endpoint.ExternalEndpoint{Name: "alerting", Group: "push", Token: "token", Alerts: []*alert.Alert{{Type: alert.TypeCustom, Enabled: &enabled, FailureThreshold: 1, SuccessThreshold: 1}}}
	cfg := newRegistryTestConfig(t)
	cfg.Alerting = &alerting.Config{Custom: &custom.AlertProvider{DefaultConfig: custom.Config{URL: "https://example.org"}}}
	if err := ProcessExternalEndpointResult(ee, &endpoint.Result{Success: false, Timestamp: time.Now(), Errors: []string{"disco cheio"}}, cfg, true); err != nil {
		t.Fatal(err)
	}
	if ee.NumberOfFailuresInARow != 1 || !ee.Alerts[0].Triggered {
		t.Errorf("expected 1 failure in a row and the alert triggered, got failures=%d triggered=%v", ee.NumberOfFailuresInARow, ee.Alerts[0].Triggered)
	}
}

// Pushes to an active endpoint must be processed with its checks, without races, and rejected once it is stopped
func TestSubmitEndpointResult(t *testing.T) {
	server, _ := newRegistryTestServer(t, 0)
	ep := newRegistryTestEndpoint(t, "submitted", server.URL)
	ep.Interval = 20 * time.Millisecond
	cfg := newRegistryTestConfig(t, ep)
	Monitor(cfg)
	defer Shutdown(cfg)
	if err := SubmitEndpointResult("registry_unknown", &endpoint.Result{Success: true, Timestamp: time.Now()}); !errors.Is(err, ErrEndpointNotMonitored) {
		t.Errorf("expected ErrEndpointNotMonitored for an unknown key, got %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := &endpoint.Result{Success: false, Timestamp: time.Now(), Errors: []string{"Latencia alta"}, Message: "Latencia alta", Origin: endpoint.ResultOriginPush}
			if err := SubmitEndpointResult(ep.Key(), result); err != nil {
				t.Errorf("expected the push to be accepted, got %v", err)
			}
		}()
	}
	wg.Wait()
	pushed := 0
	for _, result := range endpointResults(t, ep.Key()) {
		if result.Origin == endpoint.ResultOriginPush {
			pushed++
		}
	}
	if pushed != 20 {
		t.Errorf("expected the 20 pushes in the history of the endpoint, got %d", pushed)
	}
	if err := StopEndpoint(ep.Key(), SourceConfig); err != nil {
		t.Fatal(err)
	}
	if err := SubmitEndpointResult(ep.Key(), &endpoint.Result{Success: true, Timestamp: time.Now()}); !errors.Is(err, ErrEndpointNotMonitored) {
		t.Errorf("expected ErrEndpointNotMonitored once the endpoint is stopped, got %v", err)
	}
}
