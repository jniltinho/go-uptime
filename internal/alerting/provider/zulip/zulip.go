// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package zulip implements the alerting provider that posts alerts to a Zulip channel through the messages
// REST API, authenticated as a bot with basic authentication.
package zulip

import (
	"bytes"
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

// ErrBotEmailNotSet is returned by Config.Validate when bot-email is empty.
// ErrBotAPIKeyNotSet is returned by Config.Validate when bot-api-key is empty.
// ErrDomainNotSet is returned by Config.Validate when domain is empty.
// ErrChannelIDNotSet is returned by Config.Validate when channel-id is empty.
// ErrDuplicateGroupOverride is returned by AlertProvider.Validate when two overrides share the same group or an
// override has no group.
var (
	ErrBotEmailNotSet         = errors.New("bot-email not set")
	ErrBotAPIKeyNotSet        = errors.New("bot-api-key not set")
	ErrDomainNotSet           = errors.New("domain not set")
	ErrChannelIDNotSet        = errors.New("channel-id not set")
	ErrDuplicateGroupOverride = errors.New("duplicate group override")
)

// Config is the configuration of the Zulip provider. It is both the default configuration and the shape of the
// group overrides and of an alert's provider-override.
type Config struct {
	BotEmail  string `yaml:"bot-email"`       // Email of the bot user
	BotAPIKey string `yaml:"bot-api-key"`     // API key of the bot user
	Domain    string `yaml:"domain"`          // Domain of the Zulip server
	ChannelID string `yaml:"channel-id"`      // ID of the channel to send the message to
	Topic     string `yaml:"topic,omitempty"` // Topic to send the message to; defaults to "Gatus" when not set

	// The following placeholders are supported in Topic:
	//   [ENDPOINT_NAME], [ENDPOINT_GROUP], [ALERT_DESCRIPTION]
}

// Validate returns ErrBotEmailNotSet, ErrBotAPIKeyNotSet, ErrDomainNotSet or ErrChannelIDNotSet for the first
// of these fields that is empty.
func (cfg *Config) Validate() error {
	if len(cfg.BotEmail) == 0 {
		return ErrBotEmailNotSet
	}
	if len(cfg.BotAPIKey) == 0 {
		return ErrBotAPIKeyNotSet
	}
	if len(cfg.Domain) == 0 {
		return ErrDomainNotSet
	}
	if len(cfg.ChannelID) == 0 {
		return ErrChannelIDNotSet
	}
	return nil
}

// Merge overwrites the fields of cfg with the fields of override that are not empty.
func (cfg *Config) Merge(override *Config) {
	if len(override.BotEmail) > 0 {
		cfg.BotEmail = override.BotEmail
	}
	if len(override.BotAPIKey) > 0 {
		cfg.BotAPIKey = override.BotAPIKey
	}
	if len(override.Domain) > 0 {
		cfg.Domain = override.Domain
	}
	if len(override.ChannelID) > 0 {
		cfg.ChannelID = override.ChannelID
	}
	if len(override.Topic) > 0 {
		cfg.Topic = override.Topic
	}
}

// AlertProvider is the configuration necessary for sending an alert using Zulip
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
	buffer := bytes.NewBufferString(provider.buildRequestBody(cfg, ep, alert, result, resolved))
	zulipEndpoint := fmt.Sprintf("https://%s/api/v1/messages", cfg.Domain)
	request, err := http.NewRequest(http.MethodPost, zulipEndpoint, buffer)
	if err != nil {
		return err
	}
	request.SetBasicAuth(cfg.BotEmail, cfg.BotAPIKey)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "go-uptime/1.0")
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) string {
	var message string
	if resolved {
		message = fmt.Sprintf("An alert for **%s** has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
	} else {
		message = fmt.Sprintf("An alert for **%s** has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
	}
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		message += "\n> " + alertDescription + "\n"
	}
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = ":check:"
		} else {
			prefix = ":cross_mark:"
		}
		message += fmt.Sprintf("\n%s - `%s`", prefix, conditionResult.Condition)
	}
	topic := cfg.Topic
	if len(topic) == 0 {
		// Kept from Gatus on purpose (see AGENTS.md, "Names kept from Gatus"): the topic is where the messages of who
		// did not configure one are already grouped; topic changes it
		topic = "Gatus"
	}
	topic = strings.ReplaceAll(topic, "[ENDPOINT_NAME]", ep.Name)
	topic = strings.ReplaceAll(topic, "[ENDPOINT_GROUP]", ep.Group)
	topic = strings.ReplaceAll(topic, "[ALERT_DESCRIPTION]", alert.GetDescription())
	return url.Values{
		"type":    {"channel"},
		"to":      {cfg.ChannelID},
		"topic":   {topic},
		"content": {message},
	}.Encode()
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
