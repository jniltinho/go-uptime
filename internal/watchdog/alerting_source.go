// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package watchdog

import (
	"sync/atomic"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

// sendErrorRedactor removes the secrets of the provider of an alert type from the text of a delivery error (fork).
// Providers often quote their URL, which can carry the credential, in their errors.
var sendErrorRedactor atomic.Pointer[func(alertType alert.Type, err error) string]

// SetSendErrorRedactor sets the function that turns a delivery error into the text logged by the watchdog
func SetSendErrorRedactor(redactor func(alertType alert.Type, err error) string) {
	sendErrorRedactor.Store(&redactor)
}

// describeSendError returns the text of a delivery error of a provider of alertType, without its secrets
func describeSendError(alertType alert.Type, err error) string {
	if redactor := sendErrorRedactor.Load(); redactor != nil {
		return (*redactor)(alertType, err)
	}
	return err.Error()
}

// managedAlerting is the alerting configuration of the endpoints managed through the administration: the providers of
// the configuration file and the providers managed through the administration (fork). Nil until one is set.
var managedAlerting atomic.Pointer[alerting.Config]

// SetManagedAlertingConfig sets the alerting configuration used by the endpoints monitored for SourceAdmin from their
// next result on. Endpoints of the configuration file keep using the alerting section of the file.
func SetManagedAlertingConfig(alertingConfig *alerting.Config) {
	managedAlerting.Store(alertingConfig)
}

// alertingConfigFor returns the alerting configuration of the endpoint with the given key: the managed one for an
// endpoint monitored for SourceAdmin, when set, and the alerting section of cfg otherwise
func alertingConfigFor(key string, cfg *config.Config) *alerting.Config {
	if endpoints := registry.Load(); endpoints != nil && endpoints.sourceOf(key) == SourceAdmin {
		if managed := managedAlerting.Load(); managed != nil {
			return managed
		}
	}
	return cfg.Alerting
}

func (r *endpointRegistry) sourceOf(key string) Source {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, exists := r.entries[key]; exists {
		return entry.source
	}
	return ""
}

// MigrateTriggeredAlerts moves the persisted triggered alerts of previous to the alerts of next at the same position
// and of the same type, with the checksum of the alert of next, before next is monitored (fork).
//
// RestorePersistedTriggeredAlerts deletes a triggered alert whose checksum changed, and the checksum covers the values
// an alert inherits from the default alert of its provider: without this, a change of the provider would make an open
// incident forget its resolve key and send a second notification. Both endpoints must have the same key.
func MigrateTriggeredAlerts(previous, next *endpoint.Endpoint) int {
	if previous == nil || next == nil || previous.Key() != next.Key() {
		return 0
	}
	migrated := 0
	for i, previousAlert := range previous.Alerts {
		if i >= len(next.Alerts) || next.Alerts[i] == nil || previousAlert == nil || next.Alerts[i].Type != previousAlert.Type {
			continue
		}
		nextAlert := next.Alerts[i]
		if previousAlert.Checksum() == nextAlert.Checksum() || !nextAlert.IsEnabled() {
			continue
		}
		exists, resolveKey, numberOfSuccessesInARow, err := store.Get().GetTriggeredEndpointAlert(previous, previousAlert)
		if err != nil || !exists {
			continue
		}
		migratedAlert := *nextAlert
		migratedAlert.Triggered, migratedAlert.ResolveKey = true, resolveKey
		migratingEndpoint := *next
		migratingEndpoint.NumberOfSuccessesInARow = numberOfSuccessesInARow
		if err := store.Get().UpsertTriggeredEndpointAlert(&migratingEndpoint, &migratedAlert); err != nil {
			logr.Errorf("[watchdog.MigrateTriggeredAlerts] Failed to migrate a triggered alert of endpoint with key=%s: %s", next.Key(), err.Error())
			continue
		}
		migrated++
	}
	return migrated
}
