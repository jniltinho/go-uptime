// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package splunk implements the alerting provider that sends alerts to Splunk as events through the HTTP
// Event Collector (HEC), authenticated with a HEC token.
package splunk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// ErrHecURLNotSet is returned by Config.Validate when hec-url is empty.
// ErrHecTokenNotSet is returned by Config.Validate when hec-token is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrHecURLNotSet           = errors.New("hec-url not set")
	ErrHecTokenNotSet         = errors.New("hec-token not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Splunk provider. HecURL is the base URL of the collector, without the
// /services/collector/event path. Config is both the default configuration and the shape of the group overrides
// and of an alert's provider-override.
type Config struct {
	HecURL     string `yaml:"hec-url"`              // Splunk HEC (HTTP Event Collector) URL
	HecToken   string `yaml:"hec-token"`            // Splunk HEC token
	Source     string `yaml:"source,omitempty"`     // Event source
	SourceType string `yaml:"sourcetype,omitempty"` // Event source type
	Index      string `yaml:"index,omitempty"`      // Splunk index
}

// Validate returns ErrHecURLNotSet or ErrHecTokenNotSet when the corresponding field is empty.
func (cfg *Config) Validate() error {
	if len(cfg.HecURL) == 0 {
		return ErrHecURLNotSet
	}
	if len(cfg.HecToken) == 0 {
		return ErrHecTokenNotSet
	}
	return nil
}

// Merge overwrites the fields of cfg with the fields of override that are not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.HecURL) > 0 {
		cfg.HecURL = override.HecURL
	}
	if len(override.HecToken) > 0 {
		cfg.HecToken = override.HecToken
	}
	if len(override.Source) > 0 {
		cfg.Source = override.Source
	}
	if len(override.SourceType) > 0 {
		cfg.SourceType = override.SourceType
	}
	if len(override.Index) > 0 {
		cfg.Index = override.Index
	}
}

// AlertProvider is the configuration necessary for sending an alert using Splunk
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
	body, err := provider.buildRequestBody(cfg, ep, alert, result, resolved)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(body)
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/services/collector/event", cfg.HecURL), buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Splunk %s", cfg.HecToken))
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to splunk alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// Body is the JSON payload posted to the HEC. Source defaults to "gatus" and SourceType to "gatus:alert";
// Index is omitted when not configured, which leaves the choice to the token's default index.
type Body struct {
	Time       int64  `json:"time"`
	Source     string `json:"source,omitempty"`
	SourceType string `json:"sourcetype,omitempty"`
	Index      string `json:"index,omitempty"`
	Event      Event  `json:"event"`
}

// Event is the alert as indexed by Splunk. AlertType is "triggered" or "resolved" and Status is "critical"
// or "ok".
type Event struct {
	AlertType   string                      `json:"alert_type"`
	Endpoint    string                      `json:"endpoint"`
	Group       string                      `json:"group,omitempty"`
	Status      string                      `json:"status"`
	Message     string                      `json:"message"`
	Description string                      `json:"description,omitempty"`
	Conditions  []*endpoint.ConditionResult `json:"conditions,omitempty"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) ([]byte, error) {
	var alertType, status, message string
	if resolved {
		alertType = "resolved"
		status = "ok"
		message = fmt.Sprintf("Alert for %s has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		alertType = "triggered"
		status = "critical"
		message = fmt.Sprintf("Alert for %s has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	event := Event{
		AlertType:   alertType,
		Endpoint:    ep.DisplayName(),
		Group:       ep.Group,
		Status:      status,
		Message:     message,
		Description: alert.GetDescription(),
	}
	if len(result.ConditionResults) > 0 {
		event.Conditions = result.ConditionResults
	}
	body := Body{
		Time:  time.Now().Unix(),
		Event: event,
	}
	// Set optional fields
	if cfg.Source != "" {
		body.Source = cfg.Source
	} else {
		// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): the searches and indexes of Splunk
		// written on v6 filter by this source and by the sourcetype below; both are configurable
		body.Source = "gatus"
	}
	if cfg.SourceType != "" {
		body.SourceType = cfg.SourceType
	} else {
		body.SourceType = "gatus:alert"
	}
	if cfg.Index != "" {
		body.Index = cfg.Index
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
