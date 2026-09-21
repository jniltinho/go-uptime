// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package plivo implements the alerting provider that sends alerts as SMS through the Plivo Message REST API,
// one request for each recipient, using basic authentication.
package plivo

import (
	"bytes"
	"encoding/base64"
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

// ErrAuthIDNotSet is returned by Config.Validate when auth-id is empty.
// ErrAuthTokenNotSet is returned by Config.Validate when auth-token is empty.
// ErrFromNotSet is returned by Config.Validate when the sender number, from, is empty.
// ErrToNotSet is returned by Config.Validate when the list of recipients, to, is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrAuthIDNotSet           = errors.New("auth-id not set")
	ErrAuthTokenNotSet        = errors.New("auth-token not set")
	ErrFromNotSet             = errors.New("from not set")
	ErrToNotSet               = errors.New("to not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Plivo provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	AuthID    string   `yaml:"auth-id"`
	AuthToken string   `yaml:"auth-token"`
	From      string   `yaml:"from"`
	To        []string `yaml:"to"`
}

// Validate returns ErrAuthIDNotSet, ErrAuthTokenNotSet, ErrFromNotSet or ErrToNotSet for the first of these
// fields that is empty.
func (cfg *Config) Validate() error {
	if len(cfg.AuthID) == 0 {
		return ErrAuthIDNotSet
	}
	if len(cfg.AuthToken) == 0 {
		return ErrAuthTokenNotSet
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
	if len(override.AuthID) > 0 {
		cfg.AuthID = override.AuthID
	}
	if len(override.AuthToken) > 0 {
		cfg.AuthToken = override.AuthToken
	}
	if len(override.From) > 0 {
		cfg.From = override.From
	}
	if len(override.To) > 0 {
		cfg.To = override.To
	}
}

// AlertProvider is the configuration necessary for sending an alert using Plivo
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
	// Send individual SMS messages to each recipient
	for _, to := range cfg.To {
		if err := provider.sendSMS(cfg, to, message); err != nil {
			return err
		}
	}
	return nil
}

// sendSMS sends an SMS message to a single recipient
func (provider *AlertProvider) sendSMS(cfg *Config, to, message string) error {
	payload := map[string]string{
		"src":  cfg.From,
		"dst":  to,
		"text": message,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Message/", cfg.AuthID), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(cfg.AuthID+":"+cfg.AuthToken))))
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to plivo alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// buildMessage builds the message for the provider
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
