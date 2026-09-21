// Package sendgrid implements the alerting provider that sends alerts by email through the SendGrid v3 Mail
// Send REST API, authenticated with a bearer API key.
package sendgrid

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// ApiURL is the SendGrid endpoint that emails are posted to.
const (
	ApiURL = "https://api.sendgrid.com/v3/mail/send"
)

// ErrAPIKeyNotSet is returned by Config.Validate when api-key is empty.
// ErrFromNotSet is returned by Config.Validate when the sender address, from, is empty.
// ErrToNotSet is returned by Config.Validate when the recipients, to, are empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrAPIKeyNotSet           = errors.New("api-key not set")
	ErrFromNotSet             = errors.New("from not set")
	ErrToNotSet               = errors.New("to not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the SendGrid provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override. To may hold several addresses separated by commas.
type Config struct {
	APIKey string `yaml:"api-key"`
	From   string `yaml:"from"`
	To     string `yaml:"to"`

	// ClientConfig is the configuration of the client used to communicate with the provider's target
	ClientConfig *client.Config `yaml:"client,omitempty"`
}

// Validate returns ErrAPIKeyNotSet, ErrFromNotSet or ErrToNotSet for the first of these fields that is empty.
func (cfg *Config) Validate() error {
	if len(cfg.APIKey) == 0 {
		return ErrAPIKeyNotSet
	}
	if len(cfg.From) == 0 {
		return ErrFromNotSet
	}
	if len(cfg.To) == 0 {
		return ErrToNotSet
	}
	return nil
}

// Merge overwrites APIKey, From and To with the values of override that are not empty, and ClientConfig when
// override has one.
func (cfg *Config) Merge(override *Config) {
	if override.ClientConfig != nil {
		cfg.ClientConfig = override.ClientConfig
	}
	if len(override.APIKey) > 0 {
		cfg.APIKey = override.APIKey
	}
	if len(override.From) > 0 {
		cfg.From = override.From
	}
	if len(override.To) > 0 {
		cfg.To = override.To
	}
}

// AlertProvider is the configuration necessary for sending an alert using SendGrid
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
	subject, body := provider.buildMessageSubjectAndBody(ep, alert, result, resolved)
	payload := provider.buildSendGridPayload(cfg, subject, body)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, ApiURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	response, err := client.GetHTTPClient(cfg.ClientConfig).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to sendgrid alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// SendGridPayload is the JSON payload posted to the SendGrid Mail Send API.
type SendGridPayload struct {
	Personalizations []Personalization `json:"personalizations"`
	From             Email             `json:"from"`
	Subject          string            `json:"subject"`
	Content          []Content         `json:"content"`
}

// Personalization lists the recipients of the email; the provider sends a single one with every address of
// Config.To.
type Personalization struct {
	To []Email `json:"to"`
}

// Email wraps an email address in the object form that the SendGrid API expects.
type Email struct {
	Email string `json:"email"`
}

// Content is one representation of the email body; the provider sends a text/plain and a text/html one.
type Content struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// buildSendGridPayload builds the SendGrid API payload
func (provider *AlertProvider) buildSendGridPayload(cfg *Config, subject, body string) SendGridPayload {
	toEmails := strings.Split(cfg.To, ",")
	var recipients []Email
	for _, email := range toEmails {
		recipients = append(recipients, Email{Email: strings.TrimSpace(email)})
	}
	return SendGridPayload{
		Personalizations: []Personalization{
			{
				To: recipients,
			},
		},
		From: Email{
			Email: cfg.From,
		},
		Subject: subject,
		Content: []Content{
			{
				Type:  "text/plain",
				Value: body,
			},
			{
				Type:  "text/html",
				Value: strings.ReplaceAll(body, "\n", "<br>"),
			},
		},
	}
}

// buildMessageSubjectAndBody builds the message subject and body
func (provider *AlertProvider) buildMessageSubjectAndBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) (string, string) {
	var subject, message string
	if resolved {
		subject = fmt.Sprintf("[%s] Alert resolved", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		subject = fmt.Sprintf("[%s] Alert triggered", ep.DisplayName())
		message = fmt.Sprintf("An alert for %s has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	var formattedConditionResults string
	if len(result.ConditionResults) > 0 {
		formattedConditionResults = "\n\nCondition results:\n"
		for _, conditionResult := range result.ConditionResults {
			var prefix string
			if conditionResult.Success {
				prefix = "✅"
			} else {
				prefix = "❌"
			}
			formattedConditionResults += fmt.Sprintf("%s %s\n", prefix, conditionResult.Condition)
		}
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = "\n\nAlert description: " + alertDescription
	}
	var extraLabels string
	if len(ep.ExtraLabels) > 0 {
		extraLabels = "\n\nExtra labels:\n"
		for key, value := range ep.ExtraLabels {
			extraLabels += fmt.Sprintf("  %s: %s\n", key, value)
		}
	}
	return subject, message + description + extraLabels + formattedConditionResults
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
