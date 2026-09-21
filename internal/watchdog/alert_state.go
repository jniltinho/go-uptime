// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package watchdog

import (
	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

// RestorePersistedTriggeredAlerts deletes the persisted triggered alerts of ep whose configuration no longer matches
// one of its enabled alerts, and restores the triggered state (triggered flag, resolve key and counters) of the
// remaining ones into ep. It returns the number of triggered alerts restored.
//
// It is used when an endpoint object is (re)created: on startup, on configuration reload and when a managed endpoint
// is replaced at runtime.
func RestorePersistedTriggeredAlerts(ep *endpoint.Endpoint) int {
	var checksums []string
	for _, alert := range ep.Alerts {
		if alert.IsEnabled() {
			checksums = append(checksums, alert.Checksum())
		}
	}
	if deleted := store.Get().DeleteAllTriggeredAlertsNotInChecksumsByEndpoint(ep, checksums); deleted > 0 {
		logr.Debugf("[watchdog.RestorePersistedTriggeredAlerts] Deleted %d triggered alerts for endpoint with key=%s because their configurations have been changed or deleted", deleted, ep.Key())
	}
	restored := 0
	for _, alert := range ep.Alerts {
		exists, resolveKey, numberOfSuccessesInARow, err := store.Get().GetTriggeredEndpointAlert(ep, alert)
		if err != nil {
			logr.Errorf("[watchdog.RestorePersistedTriggeredAlerts] Failed to get triggered alert for endpoint with key=%s: %s", ep.Key(), err.Error())
			continue
		}
		if exists {
			alert.Triggered, alert.ResolveKey = true, resolveKey
			ep.NumberOfSuccessesInARow, ep.NumberOfFailuresInARow = numberOfSuccessesInARow, alert.FailureThreshold
			restored++
		}
	}
	return restored
}
