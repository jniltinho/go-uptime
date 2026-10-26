// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package resend implements the alerting provider that sends alerts by email through the Resend API, authenticated with
// a bearer API key.
package resend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// MaximumRecipients is the number of addresses the Resend API accepts in the to field of an email
const MaximumRecipients = 50

// APIURL is the Resend endpoint that emails are posted to. It is a variable so that the tests can check it.
var APIURL = "https://api.resend.com/emails"

// ErrAPIKeyNotSet is returned by Config.Validate when api-key is empty.
// ErrFromNotSet is returned by Config.Validate when the sender address, from, is empty.
// ErrToNotSet is returned by Config.Validate when the recipients, to, are empty.
// ErrTooManyRecipients is returned by Config.Validate when to has more than MaximumRecipients addresses.
// ErrInvalidAddress is returned by Config.Validate when from or an address of to is not an email address, either
// "user@example.com" or "Name <user@example.com>".
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrAPIKeyNotSet           = errors.New("api-key not set")
	ErrFromNotSet             = errors.New("from not set")
	ErrToNotSet               = errors.New("to not set")
	ErrTooManyRecipients      = fmt.Errorf("to has more than %d recipients, the maximum of Resend", MaximumRecipients)
	ErrInvalidAddress         = errors.New("invalid email address")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Resend provider. It is both the default configuration and the shape of the group
// overrides and of an alert's provider-override. To may hold several addresses separated by commas.
type Config struct {
	APIKey string `yaml:"api-key"` // API key of Resend, sent as a bearer token
	From   string `yaml:"from"`    // Sender, e.g. "Go Uptime <alerts@example.com>", of a domain verified in Resend
	To     string `yaml:"to"`      // Recipients separated by commas, at most MaximumRecipients

	// ClientConfig is the configuration of the client used to communicate with the provider's target
	ClientConfig *client.Config `yaml:"client,omitempty"`
}

// Validate returns ErrAPIKeyNotSet, ErrFromNotSet or ErrToNotSet for the first of these fields that is empty or only
// spaces, ErrTooManyRecipients when to has more addresses than Resend accepts, and ErrInvalidAddress when from or an
// address of to cannot be parsed, so that a mistake is found when the configuration is loaded, not at the first alert.
func (cfg *Config) Validate() error {
	if len(strings.TrimSpace(cfg.APIKey)) == 0 {
		return ErrAPIKeyNotSet
	}
	if len(strings.TrimSpace(cfg.From)) == 0 {
		return ErrFromNotSet
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return fmt.Errorf("%w in from: %s", ErrInvalidAddress, cfg.From)
	}
	recipients := cfg.recipients()
	if len(recipients) == 0 {
		return ErrToNotSet
	}
	if len(recipients) > MaximumRecipients {
		return ErrTooManyRecipients
	}
	for _, recipient := range recipients {
		if _, err := mail.ParseAddress(recipient); err != nil {
			return fmt.Errorf("%w in to: %s", ErrInvalidAddress, recipient)
		}
	}
	return nil
}

// Merge overwrites APIKey, From and To with the values of override that are not empty, and ClientConfig when override
// has one.
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

// recipients returns the addresses of To, trimmed, without the empty ones
func (cfg *Config) recipients() []string {
	var recipients []string
	for _, address := range strings.Split(cfg.To, ",") {
		if address = strings.TrimSpace(address); len(address) > 0 {
			recipients = append(recipients, address)
		}
	}
	return recipients
}

// AlertProvider is the configuration necessary for sending an alert using Resend
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

// Validate the provider's configuration: the default configuration and each group override merged over it, so that an
// override with too many recipients or an invalid address is refused when the configuration is loaded
func (provider *AlertProvider) Validate() error {
	registeredGroups := make(map[string]bool)
	for _, override := range provider.Overrides {
		if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" {
			return ErrDuplicateGroupOverride
		}
		registeredGroups[override.Group] = true
	}
	if err := provider.DefaultConfig.Validate(); err != nil {
		return err
	}
	for _, override := range provider.Overrides {
		merged := provider.DefaultConfig
		merged.Merge(&override.Config)
		if err := merged.Validate(); err != nil {
			return fmt.Errorf("override of group %s: %w", override.Group, err)
		}
	}
	return nil
}

// Send an alert using the provider
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	subject, body := provider.buildMessageSubjectAndBody(ep, alert, result, resolved)
	payload, err := json.Marshal(buildPayload(cfg, subject, body))
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, APIURL, bytes.NewBuffer(payload))
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
		return fmt.Errorf("call to resend alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// Payload is the JSON body posted to the Resend API: the sender, the recipients, the subject and the message as text
// and as HTML
type Payload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

// buildPayload builds the body of the request to the Resend API. The HTML is the text escaped, so that a name, a
// description or a condition with < > or & is shown as it is, with its line breaks as <br>.
func buildPayload(cfg *Config, subject, body string) Payload {
	return Payload{
		From:    cfg.From,
		To:      cfg.recipients(),
		Subject: subject,
		Text:    body,
		HTML:    strings.ReplaceAll(html.EscapeString(body), "\n", "<br>"),
	}
}

// buildMessageSubjectAndBody builds the message subject and body, like the other email providers
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
			prefix := "❌"
			if conditionResult.Success {
				prefix = "✅"
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
	for _, override := range provider.Overrides {
		if group == override.Group {
			cfg.Merge(&override.Config)
			break
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
