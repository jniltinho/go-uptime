// Package ilert implements the alerting provider that sends alert events to ilert through the events REST API
// of its Gatus integration, identified by an integration key.
package ilert

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

const (
	// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): this is the path of the integration in the
	// API of iLert, not a name of ours
	restAPIUrl = "https://api.ilert.com/api/v1/events/gatus/"
)

// Errors returned by the validation of the configuration: ErrIntegrationKeyNotSet when the integration key is
// missing and ErrDuplicateGroupOverride when an override has an empty or already used group.
var (
	ErrIntegrationKeyNotSet   = errors.New("integration key is not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config holds the integration key, which is appended to the URL of the ilert events API.
type Config struct {
	IntegrationKey string `yaml:"integration-key"`
}

// Validate checks that the integration key is set.
func (cfg *Config) Validate() error {
	if len(cfg.IntegrationKey) == 0 {
		return ErrIntegrationKeyNotSet
	}
	return nil
}

// Merge replaces the integration key of cfg with the one of override, unless the latter is empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.IntegrationKey) > 0 {
		cfg.IntegrationKey = override.IntegrationKey
	}
}

// AlertProvider is the configuration necessary for sending an alert using ilert
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a case under which the default configuration is overridden for the endpoints of a group.
type Override struct {
	Group  string `yaml:"group"`
	Config `yaml:",inline"`
}

// Validate checks that every override has a non-empty group used only once, then validates the default
// configuration.
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

// Send posts the alert to the ilert events API with the status firing or resolved. It returns an error
// carrying the response body when the status code is 400 or above.
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%s", restAPIUrl, cfg.IntegrationKey), buffer)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.GetHTTPClient(nil).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}

	return err
}

// Body is the JSON payload posted to the ilert events API. Status is firing or resolved, and Details falls
// back to a placeholder text when the alert has no description.
type Body struct {
	Alert            alert.Alert                 `json:"alert"`
	Name             string                      `json:"name"`
	Group            string                      `json:"group"`
	Status           string                      `json:"status"`
	Title            string                      `json:"title"`
	Details          string                      `json:"details,omitempty"`
	ConditionResults []*endpoint.ConditionResult `json:"condition_results"`
	URL              string                      `json:"url"`
}

func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var details, status string
	if resolved {
		status = "resolved"
	} else {
		status = "firing"
	}

	if len(alert.GetDescription()) > 0 {
		details = alert.GetDescription()
	} else {
		details = "No description"
	}

	var body []byte
	body, _ = json.Marshal(Body{
		Alert:            *alert,
		Name:             ep.Name,
		Group:            ep.Group,
		Title:            ep.DisplayName(),
		Status:           status,
		Details:          details,
		ConditionResults: result.ConditionResults,
		URL:              ep.URL,
	})
	return body
}

// GetDefaultAlert returns the provider's default alert configuration, or nil when none is configured.
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}

// GetConfig returns the default configuration with the group override and then the alert's provider override
// merged into it. The merged configuration is validated and returned along with the validation error, if any.
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
