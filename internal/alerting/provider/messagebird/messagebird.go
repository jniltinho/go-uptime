// Package messagebird implements the alerting provider that sends alerts by SMS through the MessageBird
// REST API.
package messagebird

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

const restAPIURL = "https://rest.messagebird.com/messages"

// Errors returned by the validation of the configuration, each one when the field it names is empty:
// ErrorAccessKeyNotSet, ErrorOriginatorNotSet and ErrorRecipientsNotSet.
var (
	ErrorAccessKeyNotSet  = errors.New("access-key not set")
	ErrorOriginatorNotSet = errors.New("originator not set")
	ErrorRecipientsNotSet = errors.New("recipients not set")
)

// Config holds the access key, the sender shown on the SMS (Originator) and the phone numbers that receive it
// (Recipients, separated by commas).
type Config struct {
	AccessKey  string `yaml:"access-key"`
	Originator string `yaml:"originator"`
	Recipients string `yaml:"recipients"`
}

// Validate checks that AccessKey, Originator and Recipients are set.
func (cfg *Config) Validate() error {
	if len(cfg.AccessKey) == 0 {
		return ErrorAccessKeyNotSet
	}
	if len(cfg.Originator) == 0 {
		return ErrorOriginatorNotSet
	}
	if len(cfg.Recipients) == 0 {
		return ErrorRecipientsNotSet
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; empty fields of override leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if len(override.AccessKey) > 0 {
		cfg.AccessKey = override.AccessKey
	}
	if len(override.Originator) > 0 {
		cfg.Originator = override.Originator
	}
	if len(override.Recipients) > 0 {
		cfg.Recipients = override.Recipients
	}
}

// AlertProvider is the configuration necessary for sending an alert using Messagebird
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`
}

// Validate the provider's configuration
func (provider *AlertProvider) Validate() error {
	return provider.DefaultConfig.Validate()
}

// Send an alert using the provider
// Reference doc for messagebird: https://developers.messagebird.com/api/sms-messaging/#send-outbound-sms
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, restAPIURL, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("AccessKey %s", cfg.AccessKey))
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

// Body is the JSON payload posted to the messages endpoint of MessageBird; Body is the text of the SMS.
type Body struct {
	Originator string `json:"originator"`
	Recipients string `json:"recipients"`
	Body       string `json:"body"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message string
	if resolved {
		message = fmt.Sprintf("RESOLVED: %s - %s", ep.DisplayName(), alert.GetDescription())
	} else {
		message = fmt.Sprintf("TRIGGERED: %s - %s", ep.DisplayName(), alert.GetDescription())
	}
	body, _ := json.Marshal(Body{
		Originator: cfg.Originator,
		Recipients: cfg.Recipients,
		Body:       message,
	})
	return body
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
	// Validate the configuration
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
