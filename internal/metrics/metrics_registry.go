// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package metrics

import (
	"slices"

	"github.com/prometheus/client_golang/prometheus"
)

// RegisteredExtraLabels returns a copy of the extra labels the metrics were registered with by the last call to
// InitializePrometheusMetrics, or nil if the metrics are not initialized.
func RegisteredExtraLabels() []string {
	labels := registeredExtraLabels.Load()
	if labels == nil {
		return nil
	}
	return slices.Clone(*labels)
}

// DeleteMetricsForEndpointKey removes every endpoint metric series of the endpoint with the given key, so that a
// removed or replaced endpoint does not leave stale series behind. It returns the number of series deleted.
func DeleteMetricsForEndpointKey(key string) int {
	if !metricsInitialized {
		return 0
	}
	labels := prometheus.Labels{"key": key}
	deleted := 0
	for _, vec := range []*prometheus.CounterVec{resultTotal, resultConnectedTotal, resultCodeTotal} {
		if vec != nil {
			deleted += vec.DeletePartialMatch(labels)
		}
	}
	for _, vec := range []*prometheus.GaugeVec{resultDurationSeconds, resultCertificateExpirationSeconds, resultDomainExpirationSeconds, resultEndpointSuccess} {
		if vec != nil {
			deleted += vec.DeletePartialMatch(labels)
		}
	}
	return deleted
}
