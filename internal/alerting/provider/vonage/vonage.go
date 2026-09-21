// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package vonage implements the alerting provider that sends alerts as SMS through the Vonage (formerly
// Nexmo) SMS REST API, one request for each recipient.
package vonage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// ApiURL is the Vonage SMS endpoint that messages are posted to.
const ApiURL = "https://rest.nexmo.com/sms/json"

// ErrAPIKeyNotSet is returned by Config.Validate when api-key is empty.
// ErrAPISecretNotSet is returned by Config.Validate when api-secret is empty.
// ErrFromNotSet is returned by Config.Validate when the sender, from, is empty.
// ErrToNotSet is returned by Config.Validate when the list of recipients, to, is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrAPIKeyNotSet           = errors.New("api-key not set")
	ErrAPISecretNotSet        = errors.New("api-secret not set")
	ErrFromNotSet             = errors.New("from not set")
	ErrToNotSet               = errors.New("to not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Vonage provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	APIKey    string   `yaml:"api-key"`
	APISecret string   `yaml:"api-secret"`
	From      string   `yaml:"from"`
	To        []string `yaml:"to"`
}

// Validate returns ErrAPIKeyNotSet, ErrAPISecretNotSet, ErrFromNotSet or ErrToNotSet for the first of these
// fields that is empty.
func (cfg *Config) Validate() error {
	if len(cfg.APIKey) == 0 {
		return ErrAPIKeyNotSet
	}
	if len(cfg.APISecret) == 0 {
		return ErrAPISecretNotSet
	}
	if len(cfg.From) == 0 {
		return ErrFromNotSet
	}
	if len(cfg.To) == 0 {
		return ErrToNotSet
	}
	return nil
}

// Merge overwrites the fields of cfg with the fields of override that are not empty. The list of recipients
// is replaced as a whole, not appended.
func (cfg *Config) Merge(override *Config) {
	if len(override.APIKey) > 0 {
		cfg.APIKey = override.APIKey
	}
	if len(override.APISecret) > 0 {
		cfg.APISecret = override.APISecret
	}
	if len(override.From) > 0 {
		cfg.From = override.From
	}
	if len(override.To) > 0 {
		cfg.To = override.To
	}
}

// AlertProvider is the configuration necessary for sending an alert using Vonage
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
	message := provider.buildMessage(cfg, ep, alert, result, resolved)

	// Send SMS to each recipient
	for _, recipient := range cfg.To {
		if err := provider.sendSMS(cfg, recipient, message); err != nil {
			return err
		}
	}
	return nil
}

// sendSMS sends an individual SMS message
func (provider *AlertProvider) sendSMS(cfg *Config, to, message string) error {
	data := url.Values{}
	data.Set("api_key", cfg.APIKey)
	data.Set("api_secret", cfg.APISecret)
	data.Set("from", cfg.From)
	data.Set("to", to)
	data.Set("text", message)
	request, err := http.NewRequest(http.MethodPost, ApiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	// Read response body once and use it for both error handling and JSON processing
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("call to vonage alert returned status code %d: %s", response.StatusCode, string(body))
	}
	// Check response for errors in messages array
	var vonageResponse Response
	if err := json.Unmarshal(body, &vonageResponse); err != nil {
		return err
	}
	// Check if any message failed
	for _, msg := range vonageResponse.Messages {
		if msg.Status != "0" {
			return fmt.Errorf("vonage SMS failed with status %s: %s", msg.Status, msg.ErrorText)
		}
	}
	return nil
}

// Response is the body returned by the Vonage SMS API. The API answers 200 even when a message is rejected,
// so Send fails when any Message has a Status other than "0".
type Response struct {
	MessageCount string    `json:"message-count"`
	Messages     []Message `json:"messages"`
}

// Message is the delivery report of one SMS inside Response; Status "0" means success.
type Message struct {
	To               string `json:"to"`
	MessageID        string `json:"message-id"`
	Status           string `json:"status"`
	ErrorText        string `json:"error-text"`
	RemainingBalance string `json:"remaining-balance"`
	MessagePrice     string `json:"message-price"`
	Network          string `json:"network"`
}

// buildMessage builds the SMS message content
func (provider *AlertProvider) buildMessage(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) string {
	if resolved {
		return fmt.Sprintf("RESOLVED: %s - %s", ep.DisplayName(), alert.GetDescription())
	} else {
		return fmt.Sprintf("TRIGGERED: %s - %s", ep.DisplayName(), alert.GetDescription())
	}
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
