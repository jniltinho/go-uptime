// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package managedendpoint parses, validates and serializes the endpoints managed through the administration API.
package managedendpoint

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/key"
	"gopkg.in/yaml.v3"
)

var (
	// ErrEmptyDefinition is returned when the definition has no content
	ErrEmptyDefinition = errors.New("the endpoint definition is empty")

	// ErrInvalidDefinition is returned when the definition cannot be decoded into an endpoint
	ErrInvalidDefinition = errors.New("invalid endpoint definition")

	// ErrFieldNotAllowed is returned when the definition uses a field that managed endpoints cannot use
	ErrFieldNotAllowed = errors.New("field is not allowed in managed endpoints")

	// ErrAlertProviderNotConfigured is returned when an alert type has no configured provider
	ErrAlertProviderNotConfigured = errors.New("alert provider is not configured")

	// ErrInvalidAlertOverride is returned when the provider-override of an alert is invalid
	ErrInvalidAlertOverride = errors.New("invalid provider-override")

	// ErrKeyConflict is returned when the key of the endpoint is already used
	ErrKeyConflict = errors.New("the endpoint key is already in use")

	// ErrExtraLabelNotAllowed is returned when an extra label is not registered for the Prometheus metrics
	ErrExtraLabelNotAllowed = errors.New("extra label is not allowed")
)

// Context holds what a managed endpoint is validated against
type Context struct {
	// Config is the loaded configuration
	Config *config.Config

	// ManagedKeys are the keys of the other managed endpoints (excluding the endpoint being validated)
	ManagedKeys []string

	// AllowedExtraLabels are the extra labels registered for the Prometheus metrics
	AllowedExtraLabels []string

	// PushTokens are the push tokens used by the configuration file and by the other managed endpoints, with a
	// description of what uses them (fork)
	PushTokens map[string]string

	// IsPushKeyToken returns whether a token is the token of a global push key (fork)
	IsPushKeyToken func(token string) bool
}

// Prepared is a validated managed endpoint
type Prepared struct {
	// Parsed is the validated endpoint, with default values, ready to be monitored
	Parsed

	// Definition is the YAML definition to persist, without default values
	Definition []byte
}

// Parse decodes a YAML or JSON definition of an active endpoint, rejecting unknown fields and push endpoints (see
// ParseDefinition). Unlike the configuration file, environment variables are not expanded.
func Parse(definition []byte) (*endpoint.Endpoint, error) {
	parsed, err := ParseDefinition(definition)
	if err != nil {
		return nil, err
	}
	if parsed.Push != nil {
		return nil, fmt.Errorf("%w: a push endpoint is not an active endpoint", ErrInvalidDefinition)
	}
	return parsed.Endpoint, nil
}

// Prepare parses and validates a definition, returning the endpoint to monitor and the definition to persist
func Prepare(definition []byte, ctx Context) (*Prepared, error) {
	parsed, err := ParseDefinition(definition)
	if err != nil {
		return nil, err
	}
	// The definition is persisted from the submitted document rather than from the endpoint struct, so that neither
	// default values nor zero values of fields without omitempty (e.g. alerts[].failure-threshold) are persisted
	document, err := ToDocument(definition)
	if err != nil {
		return nil, err
	}
	normalizedDefinition, err := FromDocument(document)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if parsed.Push != nil {
		err = validatePush(parsed.Push, ctx)
	} else {
		err = validate(parsed.Endpoint, ctx)
	}
	if err != nil {
		return nil, err
	}
	if err := validatePushToken(parsed, ctx); err != nil {
		return nil, err
	}
	return &Prepared{Parsed: *parsed, Definition: normalizedDefinition}, nil
}

// Effective returns the YAML definition of a validated endpoint, including its default values
func Effective(ep *endpoint.Endpoint) ([]byte, error) {
	return yaml.Marshal(ep)
}

// ToDocument converts a YAML definition into a generic document whose keys are the YAML keys
func ToDocument(definition []byte) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(definition, &document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if document == nil {
		document = make(map[string]any)
	}
	return document, nil
}

// FromDocument converts a generic document back into a YAML definition
func FromDocument(document map[string]any) ([]byte, error) {
	return yaml.Marshal(document)
}

// ConfigKeyOrigin returns a description of what uses endpointKey in the configuration file, if anything
func ConfigKeyOrigin(cfg *config.Config, endpointKey string) (origin string, used bool) {
	for _, ep := range cfg.Endpoints {
		if ep.Key() == endpointKey {
			return "an endpoint of the configuration file", true
		}
	}
	for _, ee := range cfg.ExternalEndpoints {
		if ee.Key() == endpointKey {
			return "an external endpoint of the configuration file", true
		}
	}
	for _, s := range cfg.Suites {
		if s.Key() == endpointKey {
			return "a suite of the configuration file", true
		}
		for _, ep := range s.Endpoints {
			if key.ConvertGroupAndNameToKey(s.Group, ep.Name) == endpointKey {
				return "an endpoint of a suite of the configuration file", true
			}
		}
	}
	return "", false
}

func validate(ep *endpoint.Endpoint, ctx Context) error {
	if err := checkAllowedFields(ep); err != nil {
		return err
	}
	// Provider default alerts must be merged before the endpoint defaults are set, like for the configuration file
	if err := validateAlerts(ep.Alerts, ep.Group, ctx.Config); err != nil {
		return err
	}
	if err := ep.ValidateAndSetDefaults(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if err := config.ResolveTunnelForClientConfig(ctx.Config, ep.ClientConfig); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if err := validateKey(ep.Key(), ctx); err != nil {
		return err
	}
	return checkExtraLabels(ep, ctx.AllowedExtraLabels)
}

// validateKey checks that the key of an endpoint can be stored and is not used by the configuration file or another
// managed endpoint
func validateKey(endpointKey string, ctx Context) error {
	// Fork: MySQL and MariaDB cannot index longer keys
	if err := config.CheckStorageKeyLength(ctx.Config.Storage, "endpoint", endpointKey); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if origin, used := ConfigKeyOrigin(ctx.Config, endpointKey); used {
		return fmt.Errorf("%w by %s: %s", ErrKeyConflict, origin, endpointKey)
	}
	if slices.Contains(ctx.ManagedKeys, endpointKey) {
		return fmt.Errorf("%w by another managed endpoint: %s", ErrKeyConflict, endpointKey)
	}
	return nil
}

func checkAllowedFields(ep *endpoint.Endpoint) error {
	if clientConfig := ep.ClientConfig; clientConfig != nil {
		if clientConfig.IAPConfig != nil {
			return fieldNotAllowed("client.identity-aware-proxy")
		}
		if clientConfig.TLS != nil && len(clientConfig.TLS.CertificateFile) > 0 {
			return fieldNotAllowed("client.tls.certificate-file")
		}
		if clientConfig.TLS != nil && len(clientConfig.TLS.PrivateKeyFile) > 0 {
			return fieldNotAllowed("client.tls.private-key-file")
		}
	}
	if len(ep.Store) > 0 {
		return fieldNotAllowed("store")
	}
	if ep.AlwaysRun {
		return fieldNotAllowed("always-run")
	}
	return nil
}

func fieldNotAllowed(field string) error {
	return fmt.Errorf("%w: %s", ErrFieldNotAllowed, field)
}

func validateAlerts(alerts []*alert.Alert, group string, cfg *config.Config) error {
	for _, endpointAlert := range alerts {
		if endpointAlert == nil {
			return fmt.Errorf("%w: empty alert", ErrInvalidDefinition)
		}
		var alertProvider provider.AlertProvider
		if cfg.Alerting != nil {
			alertProvider = cfg.Alerting.GetAlertingProviderByAlertType(endpointAlert.Type)
		}
		if alertProvider == nil {
			return fmt.Errorf("%w: %s", ErrAlertProviderNotConfigured, endpointAlert.Type)
		}
		if defaultAlert := alertProvider.GetDefaultAlert(); defaultAlert != nil {
			provider.MergeProviderDefaultAlertIntoEndpointAlert(defaultAlert, endpointAlert)
		}
		if len(endpointAlert.ProviderOverride) > 0 {
			if err := alertProvider.ValidateOverrides(group, endpointAlert); err != nil {
				return fmt.Errorf("%w for alert of type %s: %w", ErrInvalidAlertOverride, endpointAlert.Type, err)
			}
		}
	}
	return nil
}

func checkExtraLabels(ep *endpoint.Endpoint, allowedExtraLabels []string) error {
	labels := make([]string, 0, len(ep.ExtraLabels))
	for label := range ep.ExtraLabels {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		if !slices.Contains(allowedExtraLabels, label) {
			return fmt.Errorf("%w: %s (allowed extra labels: %v)", ErrExtraLabelNotAllowed, label, allowedExtraLabels)
		}
	}
	return nil
}
