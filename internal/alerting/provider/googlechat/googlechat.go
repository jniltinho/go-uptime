// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package googlechat implements the alerting provider that sends alerts to a Google Chat space through an
// incoming webhook.
package googlechat

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
	ErrWebhookURLNotSet       = errors.New("webhook-url not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config holds the webhook URL of the space and the optional configuration of the HTTP client.
type Config struct {
	WebhookURL   string         `yaml:"webhook-url"`
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
}

// AlertProvider is the configuration necessary for sending an alert using Google chat
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
	buffer := bytes.NewBuffer(provider.buildRequestBody(ep, alert, result, resolved))
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

// Body is the JSON payload posted to the Google Chat webhook, in the cards message format.
type Body struct {
	Cards []Cards `json:"cards"`
}

// Cards is a card of the message, made of sections.
type Cards struct {
	Sections []Sections `json:"sections"`
}

// Sections is a section of a card, made of widgets.
type Sections struct {
	Widgets []Widgets `json:"widgets"`
}

// Widgets is a widget of a section: either a key-value block or a row of buttons.
type Widgets struct {
	KeyValue *KeyValue `json:"keyValue,omitempty"`
	Buttons  []Buttons `json:"buttons,omitempty"`
}

// KeyValue is the widget that shows a labelled text, used for the alert message and the condition results.
type KeyValue struct {
	TopLabel         string `json:"topLabel,omitempty"`
	Content          string `json:"content,omitempty"`
	ContentMultiline string `json:"contentMultiline,omitempty"`
	BottomLabel      string `json:"bottomLabel,omitempty"`
	Icon             string `json:"icon,omitempty"`
}

// Buttons is a button of a widget; only text buttons are used.
type Buttons struct {
	TextButton TextButton `json:"textButton"`
}

// TextButton is a button with a label and the action run when it is clicked.
type TextButton struct {
	Text    string  `json:"text"`
	OnClick OnClick `json:"onClick"`
}

// OnClick is the action of a button, which always opens a link.
type OnClick struct {
	OpenLink OpenLink `json:"openLink"`
}

// OpenLink holds the URL opened by a button, set to the URL of the endpoint.
type OpenLink struct {
	URL string `json:"url"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message, color string
	if resolved {
		color = "#36A64F"
		message = fmt.Sprintf("<font color='%s'>An alert has been resolved after passing successfully %d time(s) in a row</font>", color, alert.SuccessThreshold)
	} else {
		color = "#DD0000"
		message = fmt.Sprintf("<font color='%s'>An alert has been triggered due to having failed %d time(s) in a row</font>", color, alert.FailureThreshold)
	}
	var formattedConditionResults string
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = "✅"
		} else {
			prefix = "❌"
		}
		formattedConditionResults += fmt.Sprintf("%s   %s<br>", prefix, conditionResult.Condition)
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = ":: " + alertDescription
	}
	payload := Body{
		Cards: []Cards{
			{
				Sections: []Sections{
					{
						Widgets: []Widgets{
							{
								KeyValue: &KeyValue{
									TopLabel:         ep.DisplayName(),
									Content:          message,
									ContentMultiline: "true",
									BottomLabel:      description,
									Icon:             "BOOKMARK",
								},
							},
						},
					},
				},
			},
		},
	}
	if len(formattedConditionResults) > 0 {
		payload.Cards[0].Sections[0].Widgets = append(payload.Cards[0].Sections[0].Widgets, Widgets{
			KeyValue: &KeyValue{
				TopLabel:         "Condition results",
				Content:          formattedConditionResults,
				ContentMultiline: "true",
				Icon:             "DESCRIPTION",
			},
		})
	}
	if ep.Type() == endpoint.TypeHTTP {
		// We only include a button targeting the URL if the endpoint is an HTTP endpoint
		// If the URL isn't prefixed with https://, Google Chat will just display a blank message aynways.
		// See https://github.com/TwiN/gatus/issues/362
		payload.Cards[0].Sections[0].Widgets = append(payload.Cards[0].Sections[0].Widgets, Widgets{
			Buttons: []Buttons{
				{
					TextButton: TextButton{
						Text:    "URL",
						OnClick: OnClick{OpenLink: OpenLink{URL: ep.URL}},
					},
				},
			},
		})
	}
	bodyAsJSON, _ := json.Marshal(payload)
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
