package managedendpoint

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
	"gopkg.in/yaml.v3"
)

const (
	// TypePush is the type of the managed endpoints that receive their results through /api/push (fork)
	TypePush = "push"

	// ItemTypePush is the type of the push endpoints in the administration list
	ItemTypePush = "PUSH"

	// DefaultPushHeartbeatInterval is the heartbeat interval of a push endpoint without one
	DefaultPushHeartbeatInterval = time.Minute

	typeField = "type"
	pushField = "push"
)

var (
	// ErrPushNotTestable is returned when testing a push endpoint, which Go Uptime does not check
	ErrPushNotTestable = errors.New("push endpoints cannot be tested: send a push to their URL instead")

	// ErrPushTokenInUse is returned when the push token of an endpoint is already used
	ErrPushTokenInUse = errors.New("the push token is already used")

	// ErrTypeChanged is returned when an update changes an active endpoint into a push endpoint, or the opposite
	ErrTypeChanged = errors.New("the type of a managed endpoint cannot be changed")
)

// PushOption is the option of an active managed endpoint that also receives push (fork)
type PushOption struct {
	// Enabled is whether the endpoint accepts push
	Enabled bool `yaml:"enabled"`

	// Token is the optional push token of the endpoint, used in /api/push/<token>
	Token string `yaml:"token,omitempty"`
}

// Parsed is a decoded managed endpoint definition: an active endpoint, which may receive push, or a push endpoint.
// Exactly one of Endpoint and Push is set, except for a Parsed built from a state in conflict or invalid.
type Parsed struct {
	// Endpoint is the active endpoint, checked by Go Uptime
	Endpoint *endpoint.Endpoint

	// PushOption is the push option of the active endpoint, if any
	PushOption *PushOption

	// Push is the push endpoint, whose results are pushed
	Push *endpoint.ExternalEndpoint
}

// IsValid returns whether the definition has an endpoint
func (parsed *Parsed) IsValid() bool {
	return parsed.Endpoint != nil || parsed.Push != nil
}

// IsPush returns whether the definition is a push endpoint
func (parsed *Parsed) IsPush() bool {
	return parsed.Push != nil
}

// Name returns the name of the endpoint
func (parsed *Parsed) Name() string {
	if parsed.Push != nil {
		return parsed.Push.Name
	}
	if parsed.Endpoint != nil {
		return parsed.Endpoint.Name
	}
	return ""
}

// Group returns the group of the endpoint
func (parsed *Parsed) Group() string {
	if parsed.Push != nil {
		return parsed.Push.Group
	}
	if parsed.Endpoint != nil {
		return parsed.Endpoint.Group
	}
	return ""
}

// Key returns the key of the endpoint
func (parsed *Parsed) Key() string {
	if parsed.Push != nil {
		return parsed.Push.Key()
	}
	if parsed.Endpoint != nil {
		return parsed.Endpoint.Key()
	}
	return ""
}

// IsEnabled returns whether the endpoint is enabled
func (parsed *Parsed) IsEnabled() bool {
	if parsed.Push != nil {
		return parsed.Push.IsEnabled()
	}
	return parsed.Endpoint != nil && parsed.Endpoint.IsEnabled()
}

// ReceivesPush returns whether the endpoint receives push: push endpoints always do, active endpoints with the option
func (parsed *Parsed) ReceivesPush() bool {
	return parsed.Push != nil || (parsed.Endpoint != nil && parsed.PushOption != nil && parsed.PushOption.Enabled)
}

// PushToken returns the push token of the endpoint, or an empty string
func (parsed *Parsed) PushToken() string {
	if parsed.Push != nil {
		return parsed.Push.Token
	}
	if parsed.PushOption != nil {
		return parsed.PushOption.Token
	}
	return ""
}

func (parsed *Parsed) alerts() []*alert.Alert {
	if parsed.Push != nil {
		return parsed.Push.Alerts
	}
	if parsed.Endpoint != nil {
		return parsed.Endpoint.Alerts
	}
	return nil
}

// setCounters sets the numbers of successes and failures in a row of the endpoint
func (parsed *Parsed) setCounters(successes, failures int) {
	if parsed.Push != nil {
		parsed.Push.NumberOfSuccessesInARow, parsed.Push.NumberOfFailuresInARow = successes, failures
	} else if parsed.Endpoint != nil {
		parsed.Endpoint.NumberOfSuccessesInARow, parsed.Endpoint.NumberOfFailuresInARow = successes, failures
	}
}

// ParseDefinition decodes a YAML or JSON definition, rejecting unknown fields. A definition with type: push is a push
// endpoint, with the fields of an external endpoint; the other definitions are active endpoints, which may have the push
// option. Unlike the configuration file, environment variables are not expanded.
func ParseDefinition(definition []byte) (*Parsed, error) {
	if len(bytes.TrimSpace(definition)) == 0 {
		return nil, ErrEmptyDefinition
	}
	if err := checkSingleDocument(definition); err != nil {
		return nil, err
	}
	document, err := ToDocument(definition)
	if err != nil {
		return nil, err
	}
	endpointType, _ := document[typeField].(string)
	if value, exists := document[typeField]; exists && value != nil && endpointType != "" && endpointType != TypePush {
		return nil, fmt.Errorf("%w: unknown type %v", ErrInvalidDefinition, value)
	}
	rawPushOption, hasPushOption := document[pushField]
	delete(document, typeField)
	delete(document, pushField)
	remaining, err := FromDocument(document)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if endpointType == TypePush {
		if hasPushOption {
			return nil, fieldNotAllowed(pushField)
		}
		var pushEndpoint endpoint.ExternalEndpoint
		if err := decodeStrict(remaining, &pushEndpoint); err != nil {
			return nil, err
		}
		return &Parsed{Push: &pushEndpoint}, nil
	}
	var ep endpoint.Endpoint
	if err := decodeStrict(remaining, &ep); err != nil {
		return nil, err
	}
	parsed := &Parsed{Endpoint: &ep}
	if hasPushOption && rawPushOption != nil {
		encoded, err := yaml.Marshal(rawPushOption)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
		}
		var option PushOption
		if err := decodeStrict(encoded, &option); err != nil {
			return nil, fmt.Errorf("%w (push)", err)
		}
		parsed.PushOption = &option
	}
	return parsed, nil
}

func checkSingleDocument(definition []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(definition))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrEmptyDefinition
		}
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	var extraDocument yaml.Node
	if err := decoder.Decode(&extraDocument); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: a single document is expected", ErrInvalidDefinition)
	}
	return nil
}

func decodeStrict(definition []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(definition))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrEmptyDefinition
		}
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	return nil
}

// validatePush validates a push endpoint, setting its default heartbeat interval
func validatePush(pushEndpoint *endpoint.ExternalEndpoint, ctx Context) error {
	if err := validateAlerts(pushEndpoint.Alerts, pushEndpoint.Group, ctx.Config); err != nil {
		return err
	}
	if pushEndpoint.Heartbeat.Interval == 0 {
		pushEndpoint.Heartbeat.Interval = DefaultPushHeartbeatInterval
	}
	if err := pushEndpoint.ValidateAndSetDefaults(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	return validateKey(pushEndpoint.Key(), ctx)
}

// validatePushToken validates the push token of the endpoint, and that it is not used by another endpoint or push key
func validatePushToken(parsed *Parsed, ctx Context) error {
	token := parsed.PushToken()
	if len(token) == 0 {
		return nil
	}
	if !pushconfig.ValidToken(token, pushconfig.MinimumEndpointTokenLength) {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, pushconfig.ErrInvalidEndpointToken)
	}
	if !parsed.ReceivesPush() {
		return nil
	}
	if owner, used := ctx.PushTokens[token]; used {
		return fmt.Errorf("%w by %s", ErrPushTokenInUse, owner)
	}
	if ctx.IsPushKeyToken != nil && ctx.IsPushKeyToken(token) {
		return fmt.Errorf("%w by a global push key", ErrPushTokenInUse)
	}
	return nil
}

// effectiveDefinition returns the YAML definition of a validated endpoint, including its default values
func effectiveDefinition(parsed *Parsed) ([]byte, error) {
	if parsed.Push != nil {
		return yaml.Marshal(parsed.Push)
	}
	return Effective(parsed.Endpoint)
}

// withGeneratedPushToken returns the definition with a generated token if it is a push endpoint without one
func withGeneratedPushToken(definition []byte) ([]byte, error) {
	parsed, err := ParseDefinition(definition)
	if err != nil || parsed.Push == nil || len(parsed.Push.Token) > 0 {
		return definition, err
	}
	document, err := ToDocument(definition)
	if err != nil {
		return nil, err
	}
	token, err := pushconfig.GenerateToken()
	if err != nil {
		return nil, err
	}
	document["token"] = token
	return FromDocument(document)
}

// restoreTriggeredAlerts restores the triggered alerts persisted for the key of the endpoint
func restoreTriggeredAlerts(parsed *Parsed) {
	if parsed.Push != nil {
		converted := parsed.Push.ToEndpoint()
		if watchdog.RestorePersistedTriggeredAlerts(converted) > 0 {
			parsed.setCounters(converted.NumberOfSuccessesInARow, converted.NumberOfFailuresInARow)
		}
		return
	}
	watchdog.RestorePersistedTriggeredAlerts(parsed.Endpoint)
}

// startMonitoring starts monitoring an enabled endpoint: the checks of an active endpoint, or the heartbeat of a push
// endpoint
func startMonitoring(parsed *Parsed) error {
	if !parsed.IsEnabled() {
		return nil
	}
	var err error
	if parsed.Push != nil {
		err = watchdog.StartExternalEndpoint(parsed.Push, watchdog.SourceAdmin)
	} else {
		err = watchdog.StartEndpoint(parsed.Endpoint, watchdog.SourceAdmin)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrApplyFailed, err)
	}
	return nil
}
