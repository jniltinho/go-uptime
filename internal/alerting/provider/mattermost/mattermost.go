// Package mattermost implements the alerting provider that sends alerts to a Mattermost channel through an
// incoming webhook.
package mattermost

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

// Errors returned by the validation of the configuration: ErrWebhookURLNotSet when the webhook URL is missing
// and ErrDuplicateGroupOverride when an override has no webhook URL or an empty or already used group.
var (
	ErrWebhookURLNotSet       = errors.New("webhook URL not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config holds the webhook URL, the optional channel that replaces the default one of the webhook, and the
// optional configuration of the HTTP client.
type Config struct {
	WebhookURL   string         `yaml:"webhook-url"`
	Channel      string         `yaml:"channel,omitempty"`
	ClientConfig *client.Config `yaml:"client,omitempty"`
}

// Validate checks that the webhook URL is set.
func (cfg *Config) Validate() error {
	if len(cfg.WebhookURL) == 0 {
		return ErrWebhookURLNotSet
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; empty fields of override leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if override.ClientConfig != nil {
		cfg.ClientConfig = override.ClientConfig
	}
	if len(override.WebhookURL) > 0 {
		cfg.WebhookURL = override.WebhookURL
	}
	if len(override.Channel) > 0 {
		cfg.Channel = override.Channel
	}
}

// AlertProvider is the configuration necessary for sending an alert using Mattermost
type AlertProvider struct {
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
	if provider.Overrides != nil {
		registeredGroups := make(map[string]bool)
		for _, override := range provider.Overrides {
			if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" || len(override.WebhookURL) == 0 {
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
	buffer := bytes.NewBuffer([]byte(provider.buildRequestBody(cfg, ep, alert, result, resolved)))
	request, err := http.NewRequest(http.MethodPost, cfg.WebhookURL, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.GetHTTPClient(cfg.ClientConfig).Do(request)
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

// Body is the JSON payload posted to the Mattermost webhook. The alert itself is carried by Attachments, and
// Username and IconURL identify Gatus as the author.
type Body struct {
	Channel     string       `json:"channel,omitempty"` // Optional channel override
	Text        string       `json:"text"`
	Username    string       `json:"username"`
	IconURL     string       `json:"icon_url"`
	Attachments []Attachment `json:"attachments"`
}

// Attachment is the coloured block of a Mattermost message that carries the alert text and its fields.
type Attachment struct {
	Title    string  `json:"title"`
	Fallback string  `json:"fallback"`
	Text     string  `json:"text"`
	Short    bool    `json:"short"`
	Color    string  `json:"color"`
	Fields   []Field `json:"fields"`
}

// Field is a titled entry of an Attachment, used to list the condition results.
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
	if len(result.ConditionResults) > 0 {
		for _, conditionResult := range result.ConditionResults {
			var prefix string
			if conditionResult.Success {
				prefix = ":white_check_mark:"
			} else {
				prefix = ":x:"
			}
			formattedConditionResults += fmt.Sprintf("%s - `%s`\n", prefix, conditionResult.Condition)
		}
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = ":\n> " + alertDescription
	}
	body := Body{
		Channel:  cfg.Channel,
		Text:     "",
		Username: "gatus",
		IconURL:  "https://raw.githubusercontent.com/TwiN/gatus/master/.github/assets/logo.png",
		Attachments: []Attachment{
			{
				Title:    ":helmet_with_white_cross: Gatus",
				Fallback: "Gatus - " + message,
				Text:     message + description,
				Short:    false,
				Color:    color,
			},
		},
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
