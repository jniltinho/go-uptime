package watchdog

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// monitorExternalEndpointHeartbeat records a failure for every full heartbeat interval without an accepted push, until
// ctx is cancelled
func monitorExternalEndpointHeartbeat(ctx context.Context, ee *endpoint.ExternalEndpoint, cfg *config.Config) {
	last := lastPush(ee.Key())
	ticker := time.NewTicker(ee.Heartbeat.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logr.Warnf("[watchdog.monitorExternalEndpointHeartbeat] Canceling current execution of group=%s; endpoint=%s; key=%s", ee.Group, ee.Name, ee.Key())
			return
		case <-ticker.C:
			executeExternalEndpointHeartbeat(ctx, ee, cfg, last)
		}
	}
}

func executeExternalEndpointHeartbeat(ctx context.Context, ee *endpoint.ExternalEndpoint, cfg *config.Config, last *atomic.Int64) {
	// Acquire semaphore to limit concurrent external endpoint monitoring
	if err := monitoringSemaphore.Acquire(ctx, 1); err != nil {
		// Only fails if context is cancelled (during shutdown, or because the heartbeat was stopped)
		logr.Debugf("[watchdog.executeExternalEndpointHeartbeat] Context cancelled, skipping execution: %s", err.Error())
		return
	}
	defer monitoringSemaphore.Release(1)
	// If there's a connectivity checker configured, check if Go Uptime has internet connectivity
	if cfg.Connectivity != nil && cfg.Connectivity.Checker != nil && !cfg.Connectivity.Checker.IsConnected() {
		logr.Infof("[watchdog.executeExternalEndpointHeartbeat] No connectivity; skipping execution")
		return
	}
	logr.Debugf("[watchdog.executeExternalEndpointHeartbeat] Checking heartbeat for group=%s; endpoint=%s; key=%s", ee.Group, ee.Name, ee.Key())
	// Fork: the heartbeat depends on the last accepted push, not on the last result, which would also count the failures
	// recorded by the heartbeat itself and record a failure only every other interval
	if time.Since(time.Unix(0, last.Load())) < ee.Heartbeat.Interval {
		logr.Infof("[watchdog.executeExternalEndpointHeartbeat] Checked heartbeat for group=%s; endpoint=%s; key=%s; success=true; errors=0", ee.Group, ee.Name, ee.Key())
		return
	}
	// Fork: the text is also the message of the result, so that it can be shown on the public status pages, which never
	// publish errors
	heartbeatMessage := endpoint.HeartbeatMessagePrefix + ee.Heartbeat.Interval.String()
	result := &endpoint.Result{
		Timestamp: time.Now(),
		Success:   false,
		Errors:    []string{heartbeatMessage},
		Message:   heartbeatMessage,
	}
	if ctx.Err() != nil {
		return
	}
	if err := ProcessExternalEndpointResult(ee, result, cfg, false); err != nil {
		logr.Errorf("[watchdog.executeExternalEndpointHeartbeat] Failed to insert result in storage: %s", err.Error())
		return
	}
	logr.Infof("[watchdog.executeExternalEndpointHeartbeat] Checked heartbeat for group=%s; endpoint=%s; key=%s; success=false; errors=1", ee.Group, ee.Name, ee.Key())
}
