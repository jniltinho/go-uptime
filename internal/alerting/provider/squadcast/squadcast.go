// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package squadcast implements the alerting provider that triggers and resolves Squadcast incidents through
// an incoming webhook.
package squadcast

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// ErrWebhookURLNotSet is returned by Config.Validate when webhook-url is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrWebhookURLNotSet       = errors.New("webhook-url not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Squadcast provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	WebhookURL string `yaml:"webhook-url"` // Squadcast webhook URL
}

// Validate returns ErrWebhookURLNotSet when the webhook URL is empty.
func (cfg *Config) Validate() error {
	if len(cfg.WebhookURL) == 0 {
		return ErrWebhookURLNotSet
	}
	return nil
}

// Merge overwrites WebhookURL with the one of override when it is not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.WebhookURL) > 0 {
		cfg.WebhookURL = override.WebhookURL
	}
}

// AlertProvider is the configuration necessary for sending an alert using Squadcast
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
	return provider.DefaultConfig.Validate()
}

// Send an alert using the provider
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	body, err := provider.buildRequestBody(ep, alert, result, resolved)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(body)
	request, err := http.NewRequest(http.MethodPost, cfg.WebhookURL, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to squadcast alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// Body is the JSON payload posted to the Squadcast webhook. Status is "trigger" or "resolve", and EventID
// derives from the endpoint key so that the resolution matches the incident it closes.
type Body struct {
	Message     string            `json:"message"`
	Description string            `json:"description,omitempty"`
	EventID     string            `json:"event_id"`
	Status      string            `json:"status"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) ([]byte, error) {
	var message, status string
	// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): the resolve event is matched to the incident
	// by this identifier, so another prefix would leave the incidents opened by v6 unresolved; the source tag below is
	// kept for the routing rules
	eventID := fmt.Sprintf("gatus-%s", ep.Key())
	if resolved {
		message = fmt.Sprintf("RESOLVED: %s", ep.DisplayName())
		status = "resolve"
	} else {
		message = fmt.Sprintf("ALERT: %s", ep.DisplayName())
		status = "trigger"
	}
	description := fmt.Sprintf("Endpoint: %s\n", ep.DisplayName())
	if resolved {
		description += fmt.Sprintf("Alert has been resolved after passing successfully %d time(s) in a row\n", alert.SuccessThreshold)
	} else {
		description += fmt.Sprintf("Endpoint has failed %d time(s) in a row\n", alert.FailureThreshold)
	}
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description += fmt.Sprintf("\nDescription: %s", alertDescription)
	}
	if len(result.ConditionResults) > 0 {
		description += "\n\nCondition Results:"
		for _, conditionResult := range result.ConditionResults {
			var status string
			if conditionResult.Success {
				status = "✅"
			} else {
				status = "❌"
			}
			description += fmt.Sprintf("\n%s %s", status, conditionResult.Condition)
		}
	}
	body := Body{
		Message:     message,
		Description: description,
		EventID:     eventID,
		Status:      status,
		Tags: map[string]string{
			"endpoint": ep.Name,
			"group":    ep.Group,
			"source":   "gatus",
		},
	}
	bodyAsJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bodyAsJSON, nil
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
