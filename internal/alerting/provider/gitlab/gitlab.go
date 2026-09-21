// Package gitlab implements the alerting provider that creates and resolves GitLab alerts through the HTTP
// endpoint (webhook) of a GitLab alert integration.
package gitlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// Defaults applied by Config.Validate: DefaultSeverity when no severity is set and DefaultMonitoringTool when
// no monitoring tool name is set.
const (
	DefaultSeverity = "critical"
	// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): GitLab groups and deduplicates the alerts by
	// monitoring_tool, and the title of the alert derives from it; it is configurable (monitoring-tool)
	DefaultMonitoringTool = "gatus"
)

// Errors returned by the validation of the configuration: ErrInvalidWebhookURL when the webhook URL is empty
// or cannot be parsed and ErrAuthorizationKeyNotSet when the authorization key is missing.
var (
	ErrInvalidWebhookURL      = fmt.Errorf("invalid webhook-url")
	ErrAuthorizationKeyNotSet = fmt.Errorf("authorization-key not set")
)

// Config holds the webhook URL and the authorization key of the GitLab alert integration and the values sent
// with each alert.
type Config struct {
	WebhookURL       string `yaml:"webhook-url"`                // The webhook url provided by GitLab
	AuthorizationKey string `yaml:"authorization-key"`          // The authorization key provided by GitLab
	Severity         string `yaml:"severity,omitempty"`         // Severity can be one of: critical, high, medium, low, info, unknown. Defaults to critical
	MonitoringTool   string `yaml:"monitoring-tool,omitempty"`  // MonitoringTool overrides the name sent to gitlab. Defaults to gatus
	EnvironmentName  string `yaml:"environment-name,omitempty"` // EnvironmentName is the name of the associated GitLab environment. Required to display alerts on a dashboard.
	Service          string `yaml:"service,omitempty"`          // Service affected. Defaults to the endpoint's display name
}

// Validate checks the webhook URL and the authorization key, and fills Severity and MonitoringTool with their
// defaults when they are empty.
func (cfg *Config) Validate() error {
	if len(cfg.WebhookURL) == 0 {
		return ErrInvalidWebhookURL
	} else if _, err := url.Parse(cfg.WebhookURL); err != nil {
		return ErrInvalidWebhookURL
	}
	if len(cfg.AuthorizationKey) == 0 {
		return ErrAuthorizationKeyNotSet
	}
	if len(cfg.Severity) == 0 {
		cfg.Severity = DefaultSeverity
	}
	if len(cfg.MonitoringTool) == 0 {
		cfg.MonitoringTool = DefaultMonitoringTool
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; empty fields of override leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if len(override.WebhookURL) > 0 {
		cfg.WebhookURL = override.WebhookURL
	}
	if len(override.AuthorizationKey) > 0 {
		cfg.AuthorizationKey = override.AuthorizationKey
	}
	if len(override.Severity) > 0 {
		cfg.Severity = override.Severity
	}
	if len(override.MonitoringTool) > 0 {
		cfg.MonitoringTool = override.MonitoringTool
	}
	if len(override.EnvironmentName) > 0 {
		cfg.EnvironmentName = override.EnvironmentName
	}
	if len(override.Service) > 0 {
		cfg.Service = override.Service
	}
}

// AlertProvider is the configuration necessary for sending an alert using GitLab
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`
}

// Validate the provider's configuration
func (provider *AlertProvider) Validate() error {
	return provider.DefaultConfig.Validate()
}

// Send posts the alert to the GitLab alert endpoint. A resolve key is generated when the alert has none, and
// GitLab uses it as the fingerprint that links the resolved alert to the triggered one.
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	if len(alert.ResolveKey) == 0 {
		alert.ResolveKey = uuid.NewString()
	}
	buffer := bytes.NewBuffer(provider.buildAlertBody(cfg, ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, cfg.WebhookURL, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AuthorizationKey))
	response, err := client.GetHTTPClient(nil).Do(request)
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

// AlertBody is the JSON payload posted to the GitLab alert endpoint. Fingerprint carries the resolve key of the
// alert, and EndTime is only set when the alert is resolved.
type AlertBody struct {
	Title                 string `json:"title,omitempty"`                   // The title of the alert.
	Description           string `json:"description,omitempty"`             // A high-level summary of the problem.
	StartTime             string `json:"start_time,omitempty"`              // The time of the alert. If none is provided, a current time is used.
	EndTime               string `json:"end_time,omitempty"`                // The resolution time of the alert. If provided, the alert is resolved.
	Service               string `json:"service,omitempty"`                 // The affected service.
	MonitoringTool        string `json:"monitoring_tool,omitempty"`         // The name of the associated monitoring tool.
	Hosts                 string `json:"hosts,omitempty"`                   // One or more hosts, as to where this incident occurred.
	Severity              string `json:"severity,omitempty"`                // The severity of the alert. Case-insensitive. Can be one of: critical, high, medium, low, info, unknown. Defaults to critical if missing or value is not in this list.
	Fingerprint           string `json:"fingerprint,omitempty"`             // The unique identifier of the alert. This can be used to group occurrences of the same alert.
	GitlabEnvironmentName string `json:"gitlab_environment_name,omitempty"` // The name of the associated GitLab environment. Required to display alerts on a dashboard.
}

// buildAlertBody builds the body of the alert
func (provider *AlertProvider) buildAlertBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	service := cfg.Service
	if len(service) == 0 {
		service = ep.DisplayName()
	}
	body := AlertBody{
		Title:                 fmt.Sprintf("alert(%s): %s", cfg.MonitoringTool, service),
		StartTime:             result.Timestamp.Format(time.RFC3339),
		Service:               service,
		MonitoringTool:        cfg.MonitoringTool,
		Hosts:                 ep.URL,
		GitlabEnvironmentName: cfg.EnvironmentName,
		Severity:              cfg.Severity,
		Fingerprint:           alert.ResolveKey,
	}
	if resolved {
		body.EndTime = result.Timestamp.Format(time.RFC3339)
	}
	var formattedConditionResults string
	if len(result.ConditionResults) > 0 {
		formattedConditionResults = "\n\n## Condition results\n"
		for _, conditionResult := range result.ConditionResults {
			var prefix string
			if conditionResult.Success {
				prefix = ":white_check_mark:"
			} else {
				prefix = ":x:"
			}
			formattedConditionResults += fmt.Sprintf("- %s - `%s`\n", prefix, conditionResult.Condition)
		}
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = ":\n> " + alertDescription
	}
	var message string
	if resolved {
		message = fmt.Sprintf("An alert for *%s* has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		message = fmt.Sprintf("An alert for *%s* has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	body.Description = message + description + formattedConditionResults
	bodyAsJSON, _ := json.Marshal(body)
	return bodyAsJSON
}

// GetDefaultAlert returns the provider's default alert configuration
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}

// GetConfig returns the configuration for the provider with the overrides applied
func (provider *AlertProvider) GetConfig(group string, alert *alert.Alert) (*Config, error) {
	cfg := provider.DefaultConfig
	// Handle alert overrides
	if len(alert.ProviderOverride) != 0 {
		overrideConfig := Config{}
		if err := yaml.Unmarshal(alert.ProviderOverrideAsBytes(), &overrideConfig); err != nil {
			return nil, err
		}
		cfg.Merge(&overrideConfig)
	}
	// Validate the configuration (we're returning the cfg here even if there's an error mostly for testing purposes)
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
