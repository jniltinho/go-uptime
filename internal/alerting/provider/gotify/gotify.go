// Package gotify implements the alerting provider that sends alerts as messages to a Gotify server through
// its REST API, authenticated with an application token.
package gotify

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

// DefaultPriority is the priority given to the messages when the configuration sets none.
const DefaultPriority = 5

// Errors returned by the validation of the configuration: ErrServerURLNotSet when the URL of the Gotify
// server is missing and ErrTokenNotSet when the application token is missing.
var (
	ErrServerURLNotSet = errors.New("server URL not set")
	ErrTokenNotSet     = errors.New("token not set")
)

// Config holds the address and the application token of the Gotify server and the priority and title of the
// messages. An empty Title means Gatus followed by the display name of the endpoint.
type Config struct {
	ServerURL string `yaml:"server-url"`         // URL of the Gotify server
	Token     string `yaml:"token"`              // Token to use when sending a message to the Gotify server
	Priority  int    `yaml:"priority,omitempty"` // Priority of the message. Defaults to DefaultPriority.
	Title     string `yaml:"title,omitempty"`    // Title of the message that will be sent
}

// Validate sets Priority to DefaultPriority when it is zero and checks that ServerURL and Token are set.
func (cfg *Config) Validate() error {
	if cfg.Priority == 0 {
		cfg.Priority = DefaultPriority
	}
	if len(cfg.ServerURL) == 0 {
		return ErrServerURLNotSet
	}
	if len(cfg.Token) == 0 {
		return ErrTokenNotSet
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; a Priority of zero leaves cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if len(override.ServerURL) > 0 {
		cfg.ServerURL = override.ServerURL
	}
	if len(override.Token) > 0 {
		cfg.Token = override.Token
	}
	if override.Priority != 0 {
		cfg.Priority = override.Priority
	}
	if len(override.Title) > 0 {
		cfg.Title = override.Title
	}
}

// AlertProvider is the configuration necessary for sending an alert using Gotify
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
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, cfg.ServerURL+"/message?token="+cfg.Token, buffer)
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
		return fmt.Errorf("failed to send alert to Gotify: %s", string(body))
	}
	return nil
}

// Body is the JSON payload posted to the message endpoint of the Gotify server.
type Body struct {
	Message  string `json:"message"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message string
	if resolved {
		message = fmt.Sprintf("An alert for `%s` has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		message = fmt.Sprintf("An alert for `%s` has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	var formattedConditionResults string
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = "✓"
		} else {
			prefix = "✕"
		}
		formattedConditionResults += fmt.Sprintf("\n%s - %s", prefix, conditionResult.Condition)
	}
	if len(alert.GetDescription()) > 0 {
		message += " with the following description: " + alert.GetDescription()
	}
	message += formattedConditionResults
	title := "Go Uptime: " + ep.DisplayName()
	if cfg.Title != "" {
		title = cfg.Title
	}
	bodyAsJSON, _ := json.Marshal(Body{
		Message:  message,
		Title:    title,
		Priority: cfg.Priority,
	})
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
	// Validate the configuration
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
