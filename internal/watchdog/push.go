package watchdog

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/liveupdates"
	"github.com/jniltinho/go-uptime/v7/internal/metrics"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

var (
	// resultLocks serializes, per endpoint key, the processing of the results of an endpoint: its checks, its pushes and
	// its heartbeat, so that the counters of its alerts are never updated concurrently (fork)
	resultLocks sync.Map // map[string]*sync.Mutex

	// lastPushes keeps, per external endpoint key, the time of the last accepted push, in Unix nanoseconds. It is kept
	// across reloads, and starts at the first time the key is used, so that a heartbeat only fails after a full interval.
	lastPushes sync.Map // map[string]*atomic.Int64

	// retriesUsed keeps, per external endpoint key, the number of failures converted into pending results since the last
	// success (fork). It is only read and written with the lock of the key, and is kept across reloads like lastPushes.
	retriesUsed sync.Map // map[string]*int
)

// lockEndpointResults locks the processing of the results of the endpoint with the given key and returns the function
// that unlocks it
func lockEndpointResults(key string) func() {
	value, _ := resultLocks.LoadOrStore(key, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// lastPush returns the time of the last accepted push of the external endpoint with the given key
func lastPush(key string) *atomic.Int64 {
	initial := new(atomic.Int64)
	initial.Store(time.Now().UnixNano())
	value, _ := lastPushes.LoadOrStore(key, initial)
	return value.(*atomic.Int64)
}

// ForgetExternalEndpoint forgets the time of the last push and the retries used of the external endpoint with the given
// key (fork). It is called when a key stops existing: removal, renaming or reload of the configuration.
func ForgetExternalEndpoint(key string) {
	unlock := lockEndpointResults(key)
	defer unlock()
	lastPushes.Delete(key)
	retriesUsed.Delete(key)
}

// ForgetExternalEndpointsNotIn forgets the push state of every key that is not in keys (fork)
func ForgetExternalEndpointsNotIn(keys []string) {
	existing := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		existing[key] = struct{}{}
	}
	forget := func(key, _ any) bool {
		if _, exists := existing[key.(string)]; !exists {
			ForgetExternalEndpoint(key.(string))
		}
		return true
	}
	lastPushes.Range(forget)
	retriesUsed.Range(forget)
}

// applyRetries converts a failure of an external endpoint into a pending result while it has retries left, and resets
// the retries used on success (fork). A pending push does not change them, and neither does a failure during a
// maintenance window. The errors of a converted result become its message, and it keeps no errors. The lock of the
// key must be held.
func applyRetries(ee *endpoint.ExternalEndpoint, result *endpoint.Result, underMaintenance bool) {
	if result.Pending {
		return
	}
	key := ee.Key()
	if result.Success {
		retriesUsed.Delete(key)
		return
	}
	if ee.Heartbeat.Retries <= 0 || underMaintenance {
		return
	}
	value, _ := retriesUsed.LoadOrStore(key, new(int))
	used := value.(*int)
	if *used >= ee.Heartbeat.Retries {
		return
	}
	*used++
	result.Pending = true
	if len(result.Message) == 0 && len(result.Errors) > 0 {
		result.Message = endpoint.TruncateResultMessage(strings.Join(result.Errors, "; "))
	}
	result.Errors = nil
}

// startExternal starts monitoring the heartbeat of an external endpoint
func (r *endpointRegistry) startExternal(ee *endpoint.ExternalEndpoint, source Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrMonitoringStopped
	}
	key := ee.Key()
	if _, exists := r.entries[key]; exists {
		return ErrEndpointAlreadyMonitored
	}
	ctx, cancel := context.WithCancel(r.ctx)
	entry := &monitoredEndpoint{external: ee, source: source, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	r.entries[key] = entry
	go func() {
		defer close(entry.done)
		monitorExternalEndpointHeartbeat(ctx, ee, r.cfg)
	}()
	return nil
}

// StartExternalEndpoint starts monitoring the heartbeat of an external endpoint for the given source (fork). It is
// stopped with StopEndpoint.
func StartExternalEndpoint(ee *endpoint.ExternalEndpoint, source Source) error {
	endpoints := registry.Load()
	if endpoints == nil {
		return ErrMonitoringStopped
	}
	return endpoints.startExternal(ee, source)
}

// ProcessExternalEndpointResult stores a result of an external endpoint, publishes its metrics and handles its alerts,
// serialized with the other results of the endpoint (fork). A pushed result also restarts the interval of the heartbeat.
// It returns the error of the storage, in which case nothing else is done.
func ProcessExternalEndpointResult(ee *endpoint.ExternalEndpoint, result *endpoint.Result, cfg *config.Config, pushed bool) error {
	unlock := lockEndpointResults(ee.Key())
	defer unlock()
	return processExternalEndpointResult(ee, result, cfg, pushed)
}

// processExternalEndpointResult is ProcessExternalEndpointResult with the lock of the key already held
func processExternalEndpointResult(ee *endpoint.ExternalEndpoint, result *endpoint.Result, cfg *config.Config, pushed bool) error {
	key := ee.Key()
	underMaintenance := cfg.Maintenance.IsUnderMaintenance() || isUnderMaintenanceWindow(ee.MaintenanceWindows)
	applyRetries(ee, result, underMaintenance)
	convertedEndpoint := ee.ToEndpoint()
	if err := store.Get().InsertEndpointResult(convertedEndpoint, result); err != nil {
		return err
	}
	liveupdates.Publish(key)
	if pushed {
		lastPush(key).Store(time.Now().UnixNano())
	}
	if cfg.Metrics {
		metrics.PublishMetricsForEndpoint(convertedEndpoint, result, metrics.RegisteredExtraLabels())
	}
	if underMaintenance {
		logr.Debugf("[watchdog.ProcessExternalEndpointResult] Not handling alerting of key=%s because currently in the maintenance window", key)
		return nil
	}
	// Fork: a pending result neither triggers nor resolves alerts, and does not change their counters
	if result.Pending {
		return nil
	}
	HandleAlerting(convertedEndpoint, result, cfg.Alerting)
	ee.NumberOfSuccessesInARow = convertedEndpoint.NumberOfSuccessesInARow
	ee.NumberOfFailuresInARow = convertedEndpoint.NumberOfFailuresInARow
	return nil
}

// SubmitEndpointResult processes a result pushed to an endpoint monitored by the watchdog (fork): it is stored in the
// history of the endpoint, and its metrics and alerts are handled like those of its checks, one result at a time. It
// returns ErrEndpointNotMonitored when the endpoint is not monitored, or stopped while waiting for its lock.
func SubmitEndpointResult(key string, result *endpoint.Result) error {
	endpoints := registry.Load()
	if endpoints == nil {
		return ErrMonitoringStopped
	}
	endpoints.mu.Lock()
	entry, exists := endpoints.entries[key]
	endpoints.mu.Unlock()
	if !exists {
		return ErrEndpointNotMonitored
	}
	unlock := lockEndpointResults(key)
	defer unlock()
	if entry.ctx.Err() != nil {
		return ErrEndpointNotMonitored
	}
	// Fork: an external endpoint whose heartbeat is monitored, e.g. a push endpoint managed through the administration
	if entry.external != nil {
		return processExternalEndpointResult(entry.external, result, endpoints.cfg, true)
	}
	ep, cfg := entry.endpoint, endpoints.cfg
	if err := store.Get().InsertEndpointResult(ep, result); err != nil {
		return err
	}
	liveupdates.Publish(key)
	if cfg.Metrics {
		metrics.PublishMetricsForEndpoint(ep, result, metrics.RegisteredExtraLabels())
	}
	if cfg.Maintenance.IsUnderMaintenance() || isUnderMaintenanceWindow(ep.MaintenanceWindows) {
		logr.Debugf("[watchdog.SubmitEndpointResult] Not handling alerting of key=%s because currently in the maintenance window", key)
		return nil
	}
	if result.Pending {
		return nil
	}
	HandleAlerting(ep, result, cfg.Alerting)
	return nil
}

func isUnderMaintenanceWindow[T interface{ IsUnderMaintenance() bool }](maintenanceWindows []T) bool {
	for _, maintenanceWindow := range maintenanceWindows {
		if maintenanceWindow.IsUnderMaintenance() {
			return true
		}
	}
	return false
}
