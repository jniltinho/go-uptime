package watchdog

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/metrics"
)

// Source identifies what an endpoint monitored by the watchdog comes from
type Source string

const (
	// SourceConfig is used for endpoints defined in the configuration file
	SourceConfig Source = "config"

	// SourceAdmin is used for endpoints managed through the administration API
	SourceAdmin Source = "admin"

	// stopGracePeriod is added to the client timeout when waiting for an in-flight execution to finish
	stopGracePeriod = 5 * time.Second

	// shutdownWaitTimeout is how long Shutdown waits for the monitoring goroutines to return
	shutdownWaitTimeout = 5 * time.Second
)

var (
	// ErrMonitoringStopped is returned when the watchdog is not running (not started yet, or shut down)
	ErrMonitoringStopped = errors.New("monitoring is not running")

	// ErrEndpointAlreadyMonitored is returned when an endpoint with the same key is already being monitored
	ErrEndpointAlreadyMonitored = errors.New("an endpoint with the same key is already being monitored")

	// ErrEndpointNotMonitored is returned when no endpoint with the given key is monitored for the given source
	ErrEndpointNotMonitored = errors.New("endpoint is not monitored for this source")

	// registry is the registry of the monitoring cycle started by the last call to Monitor
	registry atomic.Pointer[endpointRegistry]
)

// monitoredEndpoint is an endpoint being monitored by its own goroutine
type monitoredEndpoint struct {
	endpoint *endpoint.Endpoint
	// external is the external endpoint whose heartbeat is monitored, when endpoint is nil (fork)
	external *endpoint.ExternalEndpoint
	source   Source
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{} // closed once the monitoring goroutine has returned
}

// key returns the key of the monitored endpoint or external endpoint
func (entry *monitoredEndpoint) key() string {
	if entry.endpoint != nil {
		return entry.endpoint.Key()
	}
	return entry.external.Key()
}

// endpointRegistry keeps track of the endpoints monitored during a monitoring cycle, so that each one can be
// started, stopped or replaced individually without affecting the others.
//
// Monitored endpoint objects must never be modified: to change an endpoint, a new object must be created and
// passed to RestartEndpoint.
type endpointRegistry struct {
	mu      sync.Mutex
	ctx     context.Context
	cfg     *config.Config
	entries map[string]*monitoredEndpoint
	closed  bool
}

func newEndpointRegistry(ctx context.Context, cfg *config.Config) *endpointRegistry {
	return &endpointRegistry{ctx: ctx, cfg: cfg, entries: make(map[string]*monitoredEndpoint)}
}

func (r *endpointRegistry) start(ep *endpoint.Endpoint, source Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrMonitoringStopped
	}
	key := ep.Key()
	if _, exists := r.entries[key]; exists {
		return ErrEndpointAlreadyMonitored
	}
	ctx, cancel := context.WithCancel(r.ctx)
	entry := &monitoredEndpoint{endpoint: ep, source: source, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	r.entries[key] = entry
	go func() {
		defer close(entry.done)
		monitorEndpoint(ep, r.cfg, metrics.RegisteredExtraLabels(), ctx)
	}()
	return nil
}

// stop stops monitoring the endpoint and waits for its in-flight execution to finish. The lock is not held while
// waiting, so that other endpoints can be started or stopped in the meantime.
func (r *endpointRegistry) stop(key string, source Source) error {
	r.mu.Lock()
	entry, exists := r.entries[key]
	if !exists || entry.source != source {
		r.mu.Unlock()
		return ErrEndpointNotMonitored
	}
	delete(r.entries, key)
	r.mu.Unlock()
	entry.cancel()
	entry.waitForExecution()
	// Fork: waits for a result being submitted or pushed, which is processed under the lock of the key; the next ones
	// see the cancelled context and are discarded
	lockEndpointResults(key)()
	if entry.endpoint != nil {
		// Connections are only closed once the execution is done: closing them while the execution lazily creates the
		// HTTP client would be a data race
		entry.endpoint.Close()
	}
	return nil
}

// close cancels every monitored endpoint, prevents new ones from being started and waits, for at most
// shutdownWaitTimeout, for their monitoring goroutines to return, so that they do not use the state of the next
// monitoring cycle. Connections of endpoints from the configuration file are closed by Shutdown.
func (r *endpointRegistry) close() {
	r.mu.Lock()
	r.closed = true
	entries := make([]*monitoredEndpoint, 0, len(r.entries))
	for _, entry := range r.entries {
		entry.cancel()
		entries = append(entries, entry)
	}
	r.mu.Unlock()
	timer := time.NewTimer(shutdownWaitTimeout)
	defer timer.Stop()
	for _, entry := range entries {
		select {
		case <-entry.done:
			if entry.source != SourceConfig && entry.endpoint != nil {
				entry.endpoint.Close()
			}
		case <-timer.C:
			logr.Warnf("[watchdog.endpointRegistry.close] Timed out waiting for the monitored endpoints to stop; results of in-flight executions will be discarded")
			return
		}
	}
}

func (r *endpointRegistry) isMonitored(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.entries[key]
	return exists
}

// waitForExecution waits for the monitoring goroutine to return, for at most the client timeout plus a grace
// period. Even if the wait times out, the result of the in-flight execution is discarded because its context has
// been cancelled.
func (entry *monitoredEndpoint) waitForExecution() {
	timeout := client.GetDefaultConfig().Timeout
	if entry.endpoint != nil && entry.endpoint.ClientConfig != nil && entry.endpoint.ClientConfig.Timeout > 0 {
		timeout = entry.endpoint.ClientConfig.Timeout
	}
	timer := time.NewTimer(timeout + stopGracePeriod)
	defer timer.Stop()
	select {
	case <-entry.done:
	case <-timer.C:
		logr.Warnf("[watchdog.waitForExecution] Timed out waiting for the in-flight execution of endpoint with key=%s; its result will be discarded", entry.key())
	}
}

// StartEndpoint starts monitoring an endpoint for the given source.
func StartEndpoint(ep *endpoint.Endpoint, source Source) error {
	endpoints := registry.Load()
	if endpoints == nil {
		return ErrMonitoringStopped
	}
	return endpoints.start(ep, source)
}

// StopEndpoint stops monitoring the endpoint with the given key if it is monitored for the given source, and waits
// for its in-flight execution to finish. No result, metric or alert of that endpoint is recorded afterward.
func StopEndpoint(key string, source Source) error {
	endpoints := registry.Load()
	if endpoints == nil {
		return ErrMonitoringStopped
	}
	return endpoints.stop(key, source)
}

// RestartEndpoint replaces the monitored endpoint that has the same key as ep, for the given source.
func RestartEndpoint(ep *endpoint.Endpoint, source Source) error {
	if err := StopEndpoint(ep.Key(), source); err != nil {
		return err
	}
	return StartEndpoint(ep, source)
}

// IsEndpointMonitored returns whether an endpoint with the given key is currently monitored
func IsEndpointMonitored(key string) bool {
	endpoints := registry.Load()
	return endpoints != nil && endpoints.isMonitored(key)
}
