// Package watchdog runs the monitoring: one goroutine per endpoint and per suite, limited by the configured
// concurrency. Each execution stores its result, updates the metrics, handles the alerts and publishes the live
// update. Endpoints managed from the administration are started, stopped and restarted individually, and push
// endpoints are watched for missing heartbeats.
package watchdog

import (
	"context"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/metrics"
	"golang.org/x/sync/semaphore"
)

const (
	// UnlimitedConcurrencyWeight is the semaphore weight used when concurrency is set to 0 (unlimited).
	// This provides a practical upper limit while allowing very high concurrency for large deployments.
	UnlimitedConcurrencyWeight = 10000
)

var (
	// monitoringSemaphore is used to limit the number of endpoints/suites that can be evaluated concurrently.
	// Without this, conditions using response time may become inaccurate.
	monitoringSemaphore *semaphore.Weighted

	ctx        context.Context
	cancelFunc context.CancelFunc
)

// Monitor loops over each endpoint and starts a goroutine to monitor each endpoint separately
func Monitor(cfg *config.Config) {
	ctx, cancelFunc = context.WithCancel(context.Background())
	// Initialize semaphore based on concurrency configuration
	if cfg.Concurrency == 0 {
		// Unlimited concurrency - use a very high limit
		monitoringSemaphore = semaphore.NewWeighted(UnlimitedConcurrencyWeight)
	} else {
		// Limited concurrency based on configuration
		monitoringSemaphore = semaphore.NewWeighted(int64(cfg.Concurrency))
	}
	endpoints := newEndpointRegistry(ctx, cfg)
	registry.Store(endpoints)
	extraLabels := metrics.RegisteredExtraLabels()
	for _, endpoint := range cfg.Endpoints {
		if endpoint.IsEnabled() {
			// To prevent multiple requests from running at the same time, we'll wait for a little before each iteration
			time.Sleep(222 * time.Millisecond)
			if err := endpoints.start(endpoint, SourceConfig); err != nil {
				logr.Errorf("[watchdog.Monitor] Failed to start monitoring endpoint with key=%s: %s", endpoint.Key(), err.Error())
			}
		}
	}
	for _, externalEndpoint := range cfg.ExternalEndpoints {
		// Check if the external endpoint is enabled and is using heartbeat
		// If the external endpoint does not use heartbeat, then it does not need to be monitored periodically, because
		// alerting is checked every time an external endpoint is pushed to Go Uptime, unlike normal endpoints.
		if externalEndpoint.IsEnabled() && externalEndpoint.Heartbeat.Interval > 0 {
			// Fork: the heartbeat is in the registry, so that it can be stopped by key
			if err := endpoints.startExternal(externalEndpoint, SourceConfig); err != nil {
				logr.Errorf("[watchdog.Monitor] Failed to start the heartbeat of external endpoint with key=%s: %s", externalEndpoint.Key(), err.Error())
			}
		}
	}
	for _, suite := range cfg.Suites {
		if suite.IsEnabled() {
			time.Sleep(222 * time.Millisecond)
			go monitorSuite(suite, cfg, extraLabels, ctx)
		}
	}
}

// Shutdown stops monitoring all endpoints
func Shutdown(cfg *config.Config) {
	if endpoints := registry.Load(); endpoints != nil {
		endpoints.close()
	}
	// Stop in-flight HTTP connections
	for _, ep := range cfg.Endpoints {
		ep.Close()
	}
	for _, s := range cfg.Suites {
		for _, ep := range s.Endpoints {
			ep.Close()
		}
	}
	cancelFunc()
}
