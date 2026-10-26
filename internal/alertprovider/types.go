// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"reflect"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider"
)

// alertingConfigType is the type of the alerting section, whose fields are the providers, one per alert type
var alertingConfigType = reflect.TypeOf(alerting.Config{})

// Types returns every alert type that has a provider, in the order of the fields of alerting.Config, which is
// alphabetical by provider
func Types() []alert.Type {
	types := make([]alert.Type, 0, alertingConfigType.NumField())
	for i := 0; i < alertingConfigType.NumField(); i++ {
		if tag := yamlName(alertingConfigType.Field(i)); len(tag) > 0 {
			types = append(types, alert.Type(tag))
		}
	}
	return types
}

// IsType returns whether alertType is the type of an alerting provider
func IsType(alertType alert.Type) bool {
	_, ok := fieldIndex(alertType)
	return ok
}

// fieldIndex returns the index of the field of alerting.Config that holds the provider of alertType
func fieldIndex(alertType alert.Type) (int, bool) {
	for i := 0; i < alertingConfigType.NumField(); i++ {
		if yamlName(alertingConfigType.Field(i)) == string(alertType) {
			return i, true
		}
	}
	return 0, false
}

// providerType returns the struct type of the provider of alertType, e.g. slack.AlertProvider
func providerType(alertType alert.Type) (reflect.Type, bool) {
	index, ok := fieldIndex(alertType)
	if !ok {
		return nil, false
	}
	return alertingConfigType.Field(index).Type.Elem(), true
}

// providerOf returns the provider of alertType configured in alertingConfig, or nil
func providerOf(alertingConfig *alerting.Config, alertType alert.Type) provider.AlertProvider {
	index, ok := fieldIndex(alertType)
	if !ok || alertingConfig == nil {
		return nil
	}
	field := reflect.ValueOf(alertingConfig).Elem().Field(index)
	if field.IsNil() {
		return nil
	}
	return field.Interface().(provider.AlertProvider)
}

// compose returns a shallow copy of base, or of an empty configuration, with the providers of managed set for their
// types. base is never changed.
func compose(base *alerting.Config, managed map[alert.Type]provider.AlertProvider) *alerting.Config {
	composed := &alerting.Config{}
	if base != nil {
		*composed = *base
	}
	value := reflect.ValueOf(composed).Elem()
	for alertType, alertProvider := range managed {
		if index, ok := fieldIndex(alertType); ok {
			value.Field(index).Set(reflect.ValueOf(alertProvider))
		}
	}
	return composed
}

func yamlName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	if name == "-" {
		return ""
	}
	return name
}

// alertTypeOf converts a type name, as sent by the administration, to an alert.Type
func alertTypeOf(name string) alert.Type {
	return alert.Type(strings.ToLower(strings.TrimSpace(name)))
}
