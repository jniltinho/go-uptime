// Package pagerduty implements the alerting provider that triggers and resolves PagerDuty incidents through
// the Events API v2, keeping the dedup key returned by PagerDuty as the alert's resolve key.
package pagerduty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

const (
	restAPIURL = "https://events.pagerduty.com/v2/enqueue"
)

// ErrIntegrationKeyNotSet is returned by Config.Validate when integration-key is not exactly 32 characters
// long, which includes not being set at all.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrIntegrationKeyNotSet   = errors.New("integration-key must have exactly 32 characters")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the PagerDuty provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	IntegrationKey string `yaml:"integration-key"`
}

// Validate returns ErrIntegrationKeyNotSet unless the integration key has exactly 32 characters.
func (cfg *Config) Validate() error {
	if len(cfg.IntegrationKey) != 32 {
		return ErrIntegrationKeyNotSet
	}
	return nil
}

// Merge overwrites IntegrationKey with the one of override when it is not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.IntegrationKey) > 0 {
		cfg.IntegrationKey = override.IntegrationKey
	}
}

// AlertProvider is the configuration necessary for sending an alert using PagerDuty
type AlertProvider struct {
	// DefaultConfig is the configuration used when no group override and no alert provider-override changes it.
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a case under which the default integration is overridden
type Override struct {
	Group  string `yaml:"group"`
	Config `yaml:",inline"`
}

// Validate the provider's configuration
func (provider *AlertProvider) Validate() error {
	registeredGroups := make(map[string]bool)
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" {
				return ErrDuplicateGroupOverride
			}
			registeredGroups[override.Group] = true
		}
	}
	// Either the default integration key has the right length, or there are overrides who are properly configured.
	return provider.DefaultConfig.Validate()
}

// Send an alert using the provider
//
// Relevant: https://developer.pagerduty.com/docs/events-api-v2/trigger-events/
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, restAPIURL, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}
	if alert.IsSendingOnResolved() {
		if resolved {
			// The alert has been resolved and there's no error, so we can clear the alert's ResolveKey
			alert.ResolveKey = ""
		} else {
			// We need to retrieve the resolve key from the response
			var payload pagerDutyResponsePayload
			if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
				// Silently fail. We don't want to create tons of alerts just because we failed to parse the body.
				logr.Errorf("[pagerduty.Send] Ran into error decoding pagerduty response: %s", err.Error())
			} else {
				alert.ResolveKey = payload.DedupKey
			}
		}
	}
	return nil
}

// Body is the event sent to the PagerDuty Events API v2. DedupKey is empty when triggering and carries the
// alert's ResolveKey when resolving.
type Body struct {
	RoutingKey  string  `json:"routing_key"`
	DedupKey    string  `json:"dedup_key"`
	EventAction string  `json:"event_action"`
	Payload     Payload `json:"payload"`
}

// Payload describes the incident inside Body. Source is always "Gatus" and Severity is always "critical".
type Payload struct {
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	Severity string `json:"severity"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message, eventAction, resolveKey string
	if resolved {
		message = fmt.Sprintf("RESOLVED: %s - %s", ep.DisplayName(), alert.GetDescription())
		eventAction = "resolve"
		resolveKey = alert.ResolveKey
	} else {
		message = fmt.Sprintf("TRIGGERED: %s - %s", ep.DisplayName(), alert.GetDescription())
		eventAction = "trigger"
		resolveKey = ""
	}
	body, _ := json.Marshal(Body{
		RoutingKey:  cfg.IntegrationKey,
		DedupKey:    resolveKey,
		EventAction: eventAction,
		Payload: Payload{
			Summary: message,
			// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): rules and filters of PagerDuty match
			// the source of the event
			Source:   "Gatus",
			Severity: "critical",
		},
	})
	return body
}

// GetDefaultAlert returns the provider's default alert configuration
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}

// GetConfig returns the configuration for the provider with the overrides applied
func (provider *AlertProvider) GetConfig(group string, alert *alert.Alert) (*Config, error) {
	cfg := provider.DefaultConfig
	// Handle group overrides
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if group == override.Group {
				cfg.Merge(&override.Config)
				break
			}
		}
	}
	// Handle alert overrides
	if len(alert.ProviderOverride) != 0 {
		overrideConfig := Config{}
		if err := yaml.Unmarshal(alert.ProviderOverrideAsBytes(), &overrideConfig); err != nil {
			return nil, err
		}
		cfg.Merge(&overrideConfig)
	}
	// Validate the configuration
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}

type pagerDutyResponsePayload struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	DedupKey string `json:"dedup_key"`
}
