// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider"
	"gopkg.in/yaml.v3"
)

const typeField = "type"

var (
	// ErrEmptyDefinition is returned when the definition has no content
	ErrEmptyDefinition = errors.New("the alerting provider definition is empty")

	// ErrInvalidDefinition is returned when the definition cannot be decoded into an alerting provider or is invalid
	ErrInvalidDefinition = errors.New("invalid alerting provider definition")

	// ErrUnknownType is returned when the type of the definition is not the type of an alerting provider
	ErrUnknownType = errors.New("unknown alerting provider type")

	// ErrFieldNotAllowed is returned when the definition uses a field that reads a file or a credential of the server
	ErrFieldNotAllowed = errors.New("field is not allowed in alerting providers managed through the administration")
)

// forbiddenClientPaths are the fields of a client section that read a file or a credential of the server, or use one
// of its SSH tunnels. They are refused wherever a client section appears.
var forbiddenClientPaths = [][]string{
	{"identity-aware-proxy"},
	{"tls", "certificate-file"},
	{"tls", "private-key-file"},
	{"tunnel"},
}

// Prepared is a validated alerting provider definition
type Prepared struct {
	// Type is the alert type of the provider
	Type alert.Type

	// Provider is the provider, with its default values, ready to send alerts
	Provider provider.AlertProvider

	// Document is the submitted definition without the type key and without default values
	Document map[string]any

	// Definition is the YAML of Document, as persisted
	Definition []byte
}

// Prepare decodes a YAML or JSON definition with a type key and the keys of the type's block in the alerting section,
// rejecting unknown keys, and validates the provider, each group override merged over its default configuration and
// its default alert. Unlike the configuration file, environment variables are not expanded.
func Prepare(raw []byte) (*Prepared, error) {
	document, err := decodeDocument(raw)
	if err != nil {
		return nil, err
	}
	name, _ := document[typeField].(string)
	if len(strings.TrimSpace(name)) == 0 {
		return nil, fmt.Errorf("%w: the type is required", ErrInvalidDefinition)
	}
	alertType := alertTypeOf(name)
	delete(document, typeField)
	return PrepareDocument(alertType, document)
}

// PrepareDocument validates the document of a provider of alertType, without its type key. See Prepare.
func PrepareDocument(alertType alert.Type, document map[string]any) (*Prepared, error) {
	structType, ok := providerType(alertType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownType, alertType)
	}
	if document == nil {
		document = map[string]any{}
	}
	if err := checkForbiddenFields(document, ""); err != nil {
		return nil, err
	}
	definition, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	value := reflect.New(structType)
	if len(document) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(definition))
		decoder.KnownFields(true)
		if err := decoder.Decode(value.Interface()); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
		}
	}
	alertProvider := value.Interface().(provider.AlertProvider)
	if err := validate(alertType, value); err != nil {
		return nil, err
	}
	return &Prepared{Type: alertType, Provider: alertProvider, Document: document, Definition: definition}, nil
}

// decodeDocument decodes a single YAML or JSON mapping
func decodeDocument(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrEmptyDefinition
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrEmptyDefinition
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: a single document is expected", ErrInvalidDefinition)
	}
	if document == nil {
		return nil, ErrEmptyDefinition
	}
	return document, nil
}

// checkForbiddenFields refuses, at any depth of value, the fields of a client section that use resources of the server
func checkForbiddenFields(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := joinPath(path, key)
			if key == "client" {
				if client, ok := child.(map[string]any); ok {
					for _, forbidden := range forbiddenClientPaths {
						if hasPath(client, forbidden) {
							return &forbiddenFieldError{path: childPath + "." + strings.Join(forbidden, ".")}
						}
					}
				}
			}
			if err := checkForbiddenFields(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := checkForbiddenFields(child, path+"[]"); err != nil {
				return err
			}
		}
	}
	return nil
}

// ForbiddenFieldPath returns the path of the first field of a client section that uses a resource of the server, at any
// depth of document, e.g. the provider-override of an alert of a managed endpoint
func ForbiddenFieldPath(document map[string]any) (string, bool) {
	if err := checkForbiddenFields(document, ""); err != nil {
		var forbidden *forbiddenFieldError
		if errors.As(err, &forbidden) {
			return forbidden.path, true
		}
	}
	return "", false
}

// forbiddenFieldError is the error of checkForbiddenFields
type forbiddenFieldError struct {
	path string
}

func (err *forbiddenFieldError) Error() string {
	return ErrFieldNotAllowed.Error() + ": " + err.path
}

func (err *forbiddenFieldError) Unwrap() error {
	return ErrFieldNotAllowed
}

func hasPath(document map[string]any, path []string) bool {
	value, exists := document[path[0]]
	if !exists {
		return false
	}
	if len(path) == 1 {
		return true
	}
	child, ok := value.(map[string]any)
	return ok && hasPath(child, path[1:])
}

// validate validates the provider, each group override merged over the default configuration, and the default alert
func validate(alertType alert.Type, value reflect.Value) error {
	alertProvider := value.Interface().(provider.AlertProvider)
	if err := alertProvider.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if alertType == alert.TypeAWSSES {
		if err := validateAWSSESKeys(value); err != nil {
			return err
		}
	}
	element := value.Elem()
	if defaultAlert, ok := element.FieldByName("DefaultAlert").Interface().(*alert.Alert); ok && defaultAlert != nil {
		copied := *defaultAlert
		copied.Type = alertType
		if err := copied.ValidateAndSetDefaults(); err != nil {
			return fmt.Errorf("%w: default-alert: %w", ErrInvalidDefinition, err)
		}
	}
	defaultConfig, overrides := element.FieldByName("DefaultConfig"), element.FieldByName("Overrides")
	if !defaultConfig.IsValid() || !overrides.IsValid() || overrides.Kind() != reflect.Slice {
		return nil
	}
	for i := 0; i < overrides.Len(); i++ {
		override := overrides.Index(i)
		overrideConfig := override.FieldByName("Config")
		if !overrideConfig.IsValid() {
			continue
		}
		merged := reflect.New(defaultConfig.Type())
		merged.Elem().Set(defaultConfig)
		mergeMethod := merged.MethodByName("Merge")
		validateMethod, ok := merged.Interface().(interface{ Validate() error })
		if !mergeMethod.IsValid() || !ok {
			continue
		}
		overridePointer := reflect.New(overrideConfig.Type())
		overridePointer.Elem().Set(overrideConfig)
		mergeMethod.Call([]reflect.Value{overridePointer})
		if err := validateMethod.Validate(); err != nil {
			group, _ := override.FieldByName("Group").Interface().(string)
			return fmt.Errorf("%w: override of group %q: %w", ErrInvalidDefinition, group, err)
		}
	}
	return nil
}

// validateAWSSESKeys requires explicit keys: without them, the AWS SDK falls back on the credentials of the machine
func validateAWSSESKeys(value reflect.Value) error {
	defaultConfig := value.Elem().FieldByName("DefaultConfig")
	accessKeyID, _ := defaultConfig.FieldByName("AccessKeyID").Interface().(string)
	secretAccessKey, _ := defaultConfig.FieldByName("SecretAccessKey").Interface().(string)
	if len(accessKeyID) == 0 || len(secretAccessKey) == 0 {
		return fmt.Errorf("%w: access-key-id and secret-access-key are required, the credentials of the server cannot be used", ErrFieldNotAllowed)
	}
	return nil
}

// Parse decodes a single YAML or JSON mapping and returns it as it was sent, without validating nor masking anything.
// The form of the administration uses it to switch from the YAML to the form.
func Parse(raw []byte) (map[string]any, error) {
	return decodeDocument(raw)
}
