// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"reflect"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

// DeclaredAlertTypes returns the alert types whose provider the configuration file declares, including those discarded
// by the validation because they are invalid (fork). A declared type takes precedence over a provider managed through
// the administration. For a configuration that was not parsed from a file, they are the providers of its alerting
// section.
func (config *Config) DeclaredAlertTypes() []alert.Type {
	if config.declaredAlertTypes == nil {
		return declaredAlertTypes(config.Alerting)
	}
	return append([]alert.Type{}, config.declaredAlertTypes...)
}

// declaredAlertTypes returns the alert types whose field of alertingConfig is set
func declaredAlertTypes(alertingConfig *alerting.Config) []alert.Type {
	if alertingConfig == nil {
		return nil
	}
	var types []alert.Type
	value := reflect.ValueOf(alertingConfig).Elem()
	for i := 0; i < value.NumField(); i++ {
		if value.Field(i).Kind() == reflect.Pointer && !value.Field(i).IsNil() {
			name, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("yaml"), ",")
			types = append(types, alert.Type(name))
		}
	}
	return types
}
