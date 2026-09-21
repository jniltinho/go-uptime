// Package homeassistant implements the alerting provider that fires a gatus_alert event in Home Assistant
// through its REST API, authenticated with a long-lived access token.
package homeassistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

// Errors returned by the validation of the configuration: ErrURLNotSet when the URL of the Home Assistant
// instance is missing, ErrTokenNotSet when the access token is missing, and ErrDuplicateGroupOverride
// when an override has an empty or already used group.
var (
	ErrURLNotSet              = errors.New("url not set")
	ErrTokenNotSet            = errors.New("token not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
	ErrInvalidEventType       = errors.New("event-type must have only letters, digits, underscores, hyphens and dots")
)

// DefaultEventType is the event fired when event-type is not set. It is kept from Gatus on purpose (see AGENTS.md,
// "Names kept from Gatus"): the automations of Home Assistant written on v6 listen to this event.
const DefaultEventType = "gatus_alert"

// eventTypePattern is what an event type may have, since it is part of the path of the request
var eventTypePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Config holds the base URL of the Home Assistant instance, the access token sent as a bearer token and, optionally,
// the type of the event to fire.
type Config struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`

	// EventType is the type of the event fired in Home Assistant, in the path of the request and in the event_type of
	// its body. It defaults to DefaultEventType.
	EventType string `yaml:"event-type,omitempty"`
}

// GetEventType returns the type of the event to fire
func (cfg *Config) GetEventType() string {
	if len(cfg.EventType) == 0 {
		return DefaultEventType
	}
	return cfg.EventType
}

// Validate checks that URL and Token are set and that the event type, when set, can be part of a path.
func (cfg *Config) Validate() error {
	if len(cfg.URL) == 0 {
		return ErrURLNotSet
	}
	if len(cfg.Token) == 0 {
		return ErrTokenNotSet
	}
	if len(cfg.EventType) > 0 && !eventTypePattern.MatchString(cfg.EventType) {
		return ErrInvalidEventType
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; empty fields of override leave cfg untouched.
func (cfg *Config) Merge(override *Config) {
	if len(override.URL) > 0 {
		cfg.URL = override.URL
	}
	if len(override.Token) > 0 {
		cfg.Token = override.Token
	}
	if len(override.EventType) > 0 {
		cfg.EventType = override.EventType
	}
}

// AlertProvider is the configuration necessary for sending an alert using HomeAssistant
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
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg.GetEventType(), ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/events/%s", cfg.URL, cfg.GetEventType()), buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+cfg.Token)
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

// Body is the JSON payload posted to the events endpoint of Home Assistant. EventData carries the state of the
// alert, with FailureCount set for a triggered alert and SuccessCount for a resolved one.
type Body struct {
	EventType string `json:"event_type"`
	EventData struct {
		Status      string `json:"status"`
		Endpoint    string `json:"endpoint"`
		Description string `json:"description,omitempty"`
		Conditions  []struct {
			Condition string `json:"condition"`
			Success   bool   `json:"success"`
		} `json:"conditions,omitempty"`
		FailureCount int `json:"failure_count,omitempty"`
		SuccessCount int `json:"success_count,omitempty"`
	} `json:"event_data"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(eventType string, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	body := Body{
		EventType: eventType,
		EventData: struct {
			Status      string `json:"status"`
			Endpoint    string `json:"endpoint"`
			Description string `json:"description,omitempty"`
			Conditions  []struct {
				Condition string `json:"condition"`
				Success   bool   `json:"success"`
			} `json:"conditions,omitempty"`
			FailureCount int `json:"failure_count,omitempty"`
			SuccessCount int `json:"success_count,omitempty"`
		}{
			Status:   "resolved",
			Endpoint: ep.DisplayName(),
		},
	}

	if !resolved {
		body.EventData.Status = "triggered"
		body.EventData.FailureCount = alert.FailureThreshold
	} else {
		body.EventData.SuccessCount = alert.SuccessThreshold
	}

	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		body.EventData.Description = alertDescription
	}

	if len(result.ConditionResults) > 0 {
		for _, conditionResult := range result.ConditionResults {
			body.EventData.Conditions = append(body.EventData.Conditions, struct {
				Condition string `json:"condition"`
				Success   bool   `json:"success"`
			}{
				Condition: conditionResult.Condition,
				Success:   conditionResult.Success,
			})
		}
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
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if group == override.Group {
				cfg.Merge(&override.Config)
				break
			}
		}
	}
	if len(alert.ProviderOverride) != 0 {
		overrideConfig := Config{}
		if err := yaml.Unmarshal(alert.ProviderOverrideAsBytes(), &overrideConfig); err != nil {
			return nil, err
		}
		cfg.Merge(&overrideConfig)
	}
	err := cfg.Validate()
	return &cfg, err
}

// ValidateOverrides validates the alert's provider override and, if present, the group override
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
