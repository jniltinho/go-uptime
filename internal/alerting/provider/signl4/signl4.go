// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package signl4 implements the alerting provider that sends alerts to SIGNL4 through the team's inbound
// webhook, whose URL is built from the team secret.
package signl4

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

// ErrTeamSecretNotSet is returned by Config.Validate when team-secret is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrTeamSecretNotSet       = errors.New("team-secret not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the SIGNL4 provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	TeamSecret string `yaml:"team-secret"` // SIGNL4 team secret
}

// Validate returns ErrTeamSecretNotSet when the team secret is empty.
func (cfg *Config) Validate() error {
	if len(cfg.TeamSecret) == 0 {
		return ErrTeamSecretNotSet
	}
	return nil
}

// Merge overwrites TeamSecret with the one of override when it is not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.TeamSecret) > 0 {
		cfg.TeamSecret = override.TeamSecret
	}
}

// AlertProvider is the configuration necessary for sending an alert using SIGNL4
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
	webhookURL := fmt.Sprintf("https://connect.signl4.com/webhook/%s", cfg.TeamSecret)
	request, err := http.NewRequest(http.MethodPost, webhookURL, buffer)
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
		return fmt.Errorf("call to signl4 alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// Body is the JSON payload posted to the SIGNL4 webhook. XS4Status is "new" or "resolved", and
// XS4ExternalID derives from the endpoint key so that the resolution matches the alert it closes.
type Body struct {
	Title         string `json:"Title"`
	Message       string `json:"Message"`
	XS4Service    string `json:"X-S4-Service"`
	XS4Status     string `json:"X-S4-Status"`
	XS4ExternalID string `json:"X-S4-ExternalID"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) ([]byte, error) {
	var title, message, status string
	if resolved {
		title = fmt.Sprintf("RESOLVED: %s", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
		status = "resolved"
	} else {
		title = fmt.Sprintf("TRIGGERED: %s", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
		status = "new"
	}
	var conditionResults string
	if len(result.ConditionResults) > 0 {
		conditionResults = "\n\nCondition results:\n"
		for _, conditionResult := range result.ConditionResults {
			var prefix string
			if conditionResult.Success {
				prefix = "✓"
			} else {
				prefix = "✗"
			}
			conditionResults += fmt.Sprintf("%s %s\n", prefix, conditionResult.Condition)
		}
	}
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		message += "\n\nDescription: " + alertDescription
	}
	message += conditionResults
	body := Body{
		Title:      title,
		Message:    message,
		XS4Service: ep.DisplayName(),
		XS4Status:  status,
		// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): the resolution is matched to the alert
		// by this identifier, so another prefix would leave the alerts opened by v6 unresolved
		XS4ExternalID: fmt.Sprintf("gatus-%s", ep.Key()),
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
