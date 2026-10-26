// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"sort"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alertprovider"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
	"gopkg.in/yaml.v3"
)

func init() {
	alertprovider.RegisterDependents(alertProviderDependents{})
}

// alertProviderDependents makes the managed endpoints follow the changes of the alerting providers managed through the
// administration (fork). See alertprovider.Dependents for the lock order.
type alertProviderDependents struct{}

// Lock takes statesMutex, which every write of a managed endpoint holds
func (alertProviderDependents) Lock() func() {
	statesMutex.Lock()
	return statesMutex.Unlock
}

// Users returns the keys of the managed endpoints whose stored definition has an alert of alertType
func (alertProviderDependents) Users(alertType alert.Type) []string {
	var keys []string
	for _, state := range List() {
		if definitionUsesAlertType(state.Stored.Definition, alertType) {
			keys = append(keys, state.Stored.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// Plan prepares again the managed endpoints that use alertType against alertingConfig. An endpoint in conflict with the
// configuration file is left alone. An endpoint that was valid and would become invalid fails the plan; one that was
// already invalid is prepared again, so that recreating a deleted provider brings it back.
func (alertProviderDependents) Plan(alertingConfig *alerting.Config, alertType alert.Type) (alertprovider.DependentsPlan, error) {
	cfg := loadedConfig.Load()
	if cfg == nil {
		return &alertProviderPlan{}, nil
	}
	allowedExtraLabels := cfg.GetUniqueExtraMetricLabels()
	keys := Keys()
	plan := &alertProviderPlan{}
	failures := make(map[string]string)
	for _, state := range List() {
		if state.InConflict() || !definitionUsesAlertType(state.Stored.Definition, alertType) {
			continue
		}
		next := prepareState(state.Stored, cfg, keysExcept(keys, state.Stored.Key), allowedExtraLabels, alertingConfig)
		if next.Err != nil && state.Err == nil {
			failures[state.Stored.Key] = next.Err.Error()
			continue
		}
		plan.changes = append(plan.changes, &alertProviderChange{previous: state, next: next})
	}
	if len(failures) > 0 {
		return nil, &alertprovider.DependentsError{Errors: failures}
	}
	sort.Slice(plan.changes, func(i, j int) bool {
		return plan.changes[i].previous.Stored.Key < plan.changes[j].previous.Stored.Key
	})
	return plan, nil
}

// alertProviderPlan restarts the managed endpoints affected by a change of an alerting provider
type alertProviderPlan struct {
	changes []*alertProviderChange
}

type alertProviderChange struct {
	previous, next *State
	wasMonitored   bool
}

// Keys returns the keys of the endpoints of the plan
func (plan *alertProviderPlan) Keys() []string {
	keys := make([]string, 0, len(plan.changes))
	for _, change := range plan.changes {
		keys = append(keys, change.previous.Stored.Key)
	}
	return keys
}

// Stop stops the monitored endpoints of the plan
func (plan *alertProviderPlan) Stop() {
	for _, change := range plan.changes {
		key := change.previous.Stored.Key
		if !change.previous.Parsed().IsValid() || !watchdog.IsEndpointMonitored(key) {
			continue
		}
		if err := watchdog.StopEndpoint(key, watchdog.SourceAdmin); err != nil && !errors.Is(err, watchdog.ErrEndpointNotMonitored) {
			logr.Errorf("[managedendpoint.alertProviderPlan.Stop] Failed to stop managed endpoint with key=%s: %s", key, err.Error())
			continue
		}
		change.wasMonitored = true
	}
}

// Start moves the triggered alerts to the new endpoints, publishes their states and starts the enabled ones
func (plan *alertProviderPlan) Start() []string {
	var warnings []string
	for _, change := range plan.changes {
		key := change.next.Stored.Key
		nextParsed := change.next.Parsed()
		if nextParsed.IsValid() {
			watchdog.MigrateTriggeredAlerts(alertingEndpointOf(change.previous.Parsed()), alertingEndpointOf(nextParsed))
			restoreTriggeredAlerts(nextParsed)
		}
		putState(change.next)
		if change.next.Err != nil {
			warnings = append(warnings, key+": "+change.next.Err.Error())
			continue
		}
		if !nextParsed.IsEnabled() {
			continue
		}
		if err := startMonitoring(nextParsed); err != nil {
			warnings = append(warnings, key+": "+err.Error())
			logr.Errorf("[managedendpoint.alertProviderPlan.Start] Failed to start managed endpoint with key=%s: %s", key, err.Error())
		}
	}
	return warnings
}

// alertingEndpointOf returns the endpoint whose triggered alerts are persisted for a parsed definition, or nil
func alertingEndpointOf(parsed *Parsed) *endpoint.Endpoint {
	if parsed.Push != nil {
		return parsed.Push.ToEndpoint()
	}
	return parsed.Endpoint
}

// definitionUsesAlertType returns whether a stored definition has an alert of alertType. The raw definition is read,
// so that disabled and invalid definitions count too.
func definitionUsesAlertType(definition string, alertType alert.Type) bool {
	var document struct {
		Alerts []struct {
			Type string `yaml:"type"`
		} `yaml:"alerts"`
	}
	if err := yaml.Unmarshal([]byte(definition), &document); err != nil {
		return false
	}
	for _, definitionAlert := range document.Alerts {
		if alert.Type(strings.ToLower(strings.TrimSpace(definitionAlert.Type))) == alertType {
			return true
		}
	}
	return false
}
