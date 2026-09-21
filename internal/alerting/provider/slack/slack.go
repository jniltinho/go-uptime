// Package slack implements the alerting provider that sends alerts to Slack through an incoming webhook.
package slack

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

// Config is the configuration of the Slack provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	WebhookURL string `yaml:"webhook-url"`     // Slack webhook URL
	Title      string `yaml:"title,omitempty"` // Title of the message that will be sent
}

// Validate returns ErrWebhookURLNotSet when the webhook URL is empty.
func (cfg *Config) Validate() error {
	if len(cfg.WebhookURL) == 0 {
		return ErrWebhookURLNotSet
	}
	return nil
}

// Merge overwrites WebhookURL and Title with the values of override that are not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.WebhookURL) > 0 {
		cfg.WebhookURL = override.WebhookURL
	}
	if len(override.Title) > 0 {
		cfg.Title = override.Title
	}
}

// AlertProvider is the configuration necessary for sending an alert using Slack
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
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))
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
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return err
}

// Body is the JSON payload posted to the Slack webhook. Text is left empty: the alert goes in an Attachment.
type Body struct {
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

// Attachment is the coloured block of the message that carries the alert text: green when resolved, red when
// triggered. Its title is the configured one, or a Gatus default.
type Attachment struct {
	Title  string  `json:"title"`
	Text   string  `json:"text"`
	Short  bool    `json:"short"`
	Color  string  `json:"color"`
	Fields []Field `json:"fields,omitempty"`
}

// Field is a titled value inside an Attachment; the provider uses one to list the condition results.
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message, color string
	if resolved {
		message = fmt.Sprintf("An alert for *%s* has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
		color = "#36A64F"
	} else {
		message = fmt.Sprintf("An alert for *%s* has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
		color = "#DD0000"
	}
	var formattedConditionResults string
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = ":white_check_mark:"
		} else {
			prefix = ":x:"
		}
		formattedConditionResults += fmt.Sprintf("%s - `%s`\n", prefix, conditionResult.Condition)
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = ":\n> " + alertDescription
	}
	body := Body{
		Text: "",
		Attachments: []Attachment{
			{
				Title: cfg.Title,
				Text:  message + description,
				Short: false,
				Color: color,
			},
		},
	}
	if len(body.Attachments[0].Title) == 0 {
		body.Attachments[0].Title = ":helmet_with_white_cross: Go Uptime"
	}
	if len(formattedConditionResults) > 0 {
		body.Attachments[0].Fields = append(body.Attachments[0].Fields, Field{
			Title: "Condition results",
			Value: formattedConditionResults,
			Short: false,
		})
	}
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
