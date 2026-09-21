// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package watchdog

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

func newRegistryTestConfig(t *testing.T, endpoints ...*endpoint.Endpoint) *config.Config {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	disabled := false
	return &config.Config{Endpoints: endpoints, Maintenance: &maintenance.Config{Enabled: &disabled}}
}

func newRegistryTestEndpoint(t *testing.T, name, url string) *endpoint.Endpoint {
	t.Helper()
	ep := &endpoint.Endpoint{Name: name, Group: "registry", URL: url, Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := ep.ValidateAndSetDefaults(); err != nil {
		t.Fatalf("invalid endpoint: %v", err)
	}
	return ep
}

// newRegistryTestServer returns a server that answers after the given delay and reports every request received
func newRegistryTestServer(t *testing.T, delay time.Duration) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	requests := make(chan struct{}, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func waitForRequest(t *testing.T, requests <-chan struct{}) {
	t.Helper()
	select {
	case <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the endpoint to be evaluated")
	}
}

func numberOfResults(key string) int {
	status, err := store.Get().GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 100))
	if err != nil || status == nil {
		return 0
	}
	return len(status.Results)
}

func TestStopEndpoint_DiscardsInFlightExecution(t *testing.T) {
	server, requests := newRegistryTestServer(t, 2*time.Second)
	ep := newRegistryTestEndpoint(t, "slow", server.URL)
	cfg := newRegistryTestConfig(t, ep)
	Monitor(cfg)
	defer Shutdown(cfg)
	waitForRequest(t, requests)
	stopStartedAt := time.Now()
	if err := StopEndpoint(ep.Key(), SourceConfig); err != nil {
		t.Fatalf("failed to stop endpoint: %v", err)
	}
	if elapsed := time.Since(stopStartedAt); elapsed < time.Second {
		t.Errorf("expected StopEndpoint to wait for the in-flight execution, returned after %s", elapsed)
	}
	if IsEndpointMonitored(ep.Key()) {
		t.Error("expected the endpoint not to be monitored anymore")
	}
	time.Sleep(500 * time.Millisecond)
	if n := numberOfResults(ep.Key()); n != 0 {
		t.Errorf("expected no result to be recorded for the stopped endpoint, got %d", n)
	}
}

func TestRestartEndpoint_DoesNotDuplicateResults(t *testing.T) {
	server, requests := newRegistryTestServer(t, time.Second)
	ep := newRegistryTestEndpoint(t, "restarted", server.URL)
	cfg := newRegistryTestConfig(t, ep)
	Monitor(cfg)
	defer Shutdown(cfg)
	waitForRequest(t, requests)
	replacement := newRegistryTestEndpoint(t, "restarted", server.URL)
	if err := RestartEndpoint(replacement, SourceConfig); err != nil {
		t.Fatalf("failed to restart endpoint: %v", err)
	}
	waitForRequest(t, requests)
	deadline := time.Now().Add(5 * time.Second)
	for numberOfResults(ep.Key()) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond)
	if n := numberOfResults(ep.Key()); n != 1 {
		t.Errorf("expected exactly 1 result after restarting during an execution, got %d", n)
	}
}

func TestStopEndpoint_DoesNotAffectOtherEndpoints(t *testing.T) {
	server, _ := newRegistryTestServer(t, 0)
	stopped := newRegistryTestEndpoint(t, "stopped", server.URL)
	other := newRegistryTestEndpoint(t, "other", server.URL)
	cfg := newRegistryTestConfig(t, stopped, other)
	Monitor(cfg)
	defer Shutdown(cfg)
	endpoints := registry.Load()
	endpoints.mu.Lock()
	otherEntry := endpoints.entries[other.Key()]
	endpoints.mu.Unlock()
	if otherEntry == nil {
		t.Fatal("expected the other endpoint to be monitored")
	}
	if err := StopEndpoint(stopped.Key(), SourceConfig); err != nil {
		t.Fatalf("failed to stop endpoint: %v", err)
	}
	endpoints.mu.Lock()
	currentOtherEntry := endpoints.entries[other.Key()]
	endpoints.mu.Unlock()
	if currentOtherEntry != otherEntry {
		t.Error("expected the other endpoint not to be restarted")
	}
	select {
	case <-otherEntry.done:
		t.Error("expected the other endpoint to still be monitored")
	default:
	}
}

func TestStopEndpoint_WrongSource(t *testing.T) {
	server, _ := newRegistryTestServer(t, 0)
	ep := newRegistryTestEndpoint(t, "from-config", server.URL)
	cfg := newRegistryTestConfig(t, ep)
	Monitor(cfg)
	defer Shutdown(cfg)
	if err := StopEndpoint(ep.Key(), SourceAdmin); !errors.Is(err, ErrEndpointNotMonitored) {
		t.Errorf("expected ErrEndpointNotMonitored, got %v", err)
	}
	if !IsEndpointMonitored(ep.Key()) {
		t.Error("expected the endpoint to still be monitored")
	}
}

func TestStartEndpoint(t *testing.T) {
	server, requests := newRegistryTestServer(t, 0)
	cfg := newRegistryTestConfig(t)
	Monitor(cfg)
	ep := newRegistryTestEndpoint(t, "managed", server.URL)
	if err := StartEndpoint(ep, SourceAdmin); err != nil {
		t.Fatalf("failed to start endpoint: %v", err)
	}
	waitForRequest(t, requests)
	if err := StartEndpoint(newRegistryTestEndpoint(t, "managed", server.URL), SourceAdmin); !errors.Is(err, ErrEndpointAlreadyMonitored) {
		t.Errorf("expected ErrEndpointAlreadyMonitored, got %v", err)
	}
	Shutdown(cfg)
	if err := StartEndpoint(newRegistryTestEndpoint(t, "after-shutdown", server.URL), SourceAdmin); !errors.Is(err, ErrMonitoringStopped) {
		t.Errorf("expected ErrMonitoringStopped after shutdown, got %v", err)
	}
}
