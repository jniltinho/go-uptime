// Package incidentio implements the alerting provider that sends alert events to incident.io through the
// HTTP alert source of its REST API.
package incidentio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"gopkg.in/yaml.v3"
)

const (
	restAPIUrl = "https://api.incident.io/v2/alert_events/http/"
)

// Errors returned by the validation of the configuration: ErrURLNotSet when the URL is missing,
// ErrURLNotPrefixedWithRestAPIURL when it does not start with the address of the alert events API,
// ErrAuthTokenNotSet when the token is missing, and ErrDuplicateGroupOverride when an override has an empty or
// already used group.
var (
	ErrURLNotSet                    = errors.New("url not set")
	ErrURLNotPrefixedWithRestAPIURL = fmt.Errorf("url must be prefixed with %s", restAPIUrl)
	ErrDuplicateGroupOverride       = errors.New("duplicate group override")
	ErrAuthTokenNotSet              = errors.New("auth-token not set")
)

// Config holds the URL of the alert source, whose last segment is the alert source config ID, the bearer
// token, and the optional source URL and metadata attached to each event.
type Config struct {
	URL       string                 `yaml:"url,omitempty"`
	AuthToken string                 `yaml:"auth-token,omitempty"`
	SourceURL string                 `yaml:"source-url,omitempty"`
	Metadata  map[string]interface{} `yaml:"metadata,omitempty"`
}

// Validate checks that URL and AuthToken are set and that URL points to the incident.io alert events API.
func (cfg *Config) Validate() error {
	if len(cfg.URL) == 0 {
		return ErrURLNotSet
	}
	if !strings.HasPrefix(cfg.URL, restAPIUrl) {
		return ErrURLNotPrefixedWithRestAPIURL
	}
	if len(cfg.AuthToken) == 0 {
		return ErrAuthTokenNotSet
	}
	return nil
}

// Merge copies every non-empty field of override over cfg; Metadata is replaced as a whole, not merged key
// by key.
func (cfg *Config) Merge(override *Config) {
	if len(override.URL) > 0 {
		cfg.URL = override.URL
	}
	if len(override.AuthToken) > 0 {
		cfg.AuthToken = override.AuthToken
	}
	if len(override.SourceURL) > 0 {
		cfg.SourceURL = override.SourceURL
	}
	if len(override.Metadata) > 0 {
		cfg.Metadata = override.Metadata
	}
}

// AlertProvider is the configuration necessary for sending an alert using incident.io
type AlertProvider struct {
	DefaultConfig Config `yaml:",inline"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a case under which the default configuration is overridden for the endpoints of a group.
type Override struct {
	Group  string `yaml:"group"`
	Config `yaml:",inline"`
}

// Validate checks that every override has a non-empty group used only once, then validates the default
// configuration.
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

// Send posts the alert event and stores the deduplication key of the response as the resolve key of the alert,
// so that the resolved event targets the same incident.io alert. An error is returned when the status code is
// 400 or above or when the response cannot be decoded.
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	cfg, err := provider.GetConfig(ep.Group, alert)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(provider.buildRequestBody(cfg, ep, alert, result, resolved))
	req, err := http.NewRequest(http.MethodPost, cfg.URL, buffer)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	response, err := client.GetHTTPClient(nil).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}
	incidentioResponse := Response{}
	err = json.NewDecoder(response.Body).Decode(&incidentioResponse)
	if err != nil {
		// Silently fail. We don't want to create tons of alerts just because we failed to parse the body.
		logr.Errorf("[incidentio.Send] Ran into error decoding pagerduty response: %s", err.Error())
	}
	alert.ResolveKey = incidentioResponse.DeduplicationKey
	return err
}

// Body is the JSON payload posted to the alert events API. Status is firing or resolved, and Metadata is the
// configured metadata plus the extra labels of the endpoint.
type Body struct {
	AlertSourceConfigID string                 `json:"alert_source_config_id"`
	Status              string                 `json:"status"`
	Title               string                 `json:"title"`
	DeduplicationKey    string                 `json:"deduplication_key,omitempty"`
	Description         string                 `json:"description,omitempty"`
	SourceURL           string                 `json:"source_url,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// Response is the part of the API answer the provider reads: the deduplication key of the alert.
type Response struct {
	DeduplicationKey string `json:"deduplication_key"`
}

func (provider *AlertProvider) buildRequestBody(cfg *Config, ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message, formattedConditionResults, status string
	if resolved {
		message = "An alert has been resolved after passing successfully " + strconv.Itoa(alert.SuccessThreshold) + " time(s) in a row"
		status = "resolved"
	} else {
		message = "An alert has been triggered due to having failed " + strconv.Itoa(alert.FailureThreshold) + " time(s) in a row"
		status = "firing"
	}
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = "🟢"
		} else {
			prefix = "🔴"
		}
		formattedConditionResults += fmt.Sprintf(" %s %s ", prefix, conditionResult.Condition)
	}
	if len(alert.GetDescription()) > 0 {
		message += " with the following description: " + alert.GetDescription()
	}
	message += fmt.Sprintf(" and the following conditions: %s ", formattedConditionResults)

	// Generate deduplication key if empty (first firing)
	if alert.ResolveKey == "" {
		// Generate unique key (endpoint key, alert type, timestamp)
		alert.ResolveKey = generateDeduplicationKey(ep, alert)
	}
	// Extract alert_source_config_id from URL
	alertSourceID := strings.TrimPrefix(cfg.URL, restAPIUrl)
	// Merge metadata: cfg.Metadata + ep.ExtraLabels (if present)
	mergedMetadata := map[string]interface{}{}
	// Copy cfg.Metadata
	maps.Copy(mergedMetadata, cfg.Metadata)
	// Add extra labels from endpoint (if present)
	if ep.ExtraLabels != nil && len(ep.ExtraLabels) > 0 {
		for k, v := range ep.ExtraLabels {
			mergedMetadata[k] = v
		}
	}

	body, _ := json.Marshal(Body{
		AlertSourceConfigID: alertSourceID,
		Title:               "Go Uptime: " + ep.DisplayName(),
		Status:              status,
		DeduplicationKey:    alert.ResolveKey,
		Description:         message,
		SourceURL:           cfg.SourceURL,
		Metadata:            mergedMetadata,
	})
	fmt.Printf("%v", string(body))
	return body
}

// GetConfig returns the default configuration with the group override and then the alert's provider override
// merged into it. The merged configuration is validated and returned along with the validation error, if any.
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

// GetDefaultAlert returns the provider's default alert configuration
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}

// ValidateOverrides validates the alert's provider override and, if present, the group override.
func (provider *AlertProvider) ValidateOverrides(group string, alert *alert.Alert) error {
	_, err := provider.GetConfig(group, alert)
	return err
}
