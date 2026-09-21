package cmd

import (
	"os"
	"sync"

	"github.com/TwiN/logr"
)

const (
	// ConfigPathEnvVar is the environment variable of the path of the configuration
	ConfigPathEnvVar = "GO_UPTIME_CONFIG_PATH"

	// LogLevelEnvVar is the environment variable of the log level
	LogLevelEnvVar = "GO_UPTIME_LOG_LEVEL"

	// DelayStartEnvVar is the environment variable of the number of seconds to wait before starting
	DelayStartEnvVar = "GO_UPTIME_DELAY_START_SECONDS"

	// LegacyConfigPathEnvVar, LegacyLogLevelEnvVar and LegacyDelayStartEnvVar are the names that the variables had while
	// the project was called Gatus. They stay accepted as aliases during the 7.x series, so that the compose files and the
	// units of v6 keep working: the new name wins, and the old one logs a warning.
	LegacyConfigPathEnvVar = "GATUS_CONFIG_PATH"
	LegacyLogLevelEnvVar   = "GATUS_LOG_LEVEL"
	LegacyDelayStartEnvVar = "GATUS_DELAY_START_SECONDS"

	// LegacyConfigFileEnvVar was already deprecated in favor of GATUS_CONFIG_PATH before the project was renamed. It gets no
	// new name: it stays accepted, last in line.
	LegacyConfigFileEnvVar = "GATUS_CONFIG_FILE"
)

// warnedLegacyVariables holds the legacy variables that were already warned about, because the configuration path is
// resolved again on every reload
var warnedLegacyVariables sync.Map

// lookupEnvironment returns the value of the first variable that is set, name first and then its legacy names, in
// order. An empty value counts as not set, so that an empty GO_UPTIME_* does not hide a GATUS_* that has a value. The
// value of a variable that lost to an earlier one is ignored. A legacy variable that is used is warned about once.
func lookupEnvironment(name string, legacyNames ...string) string {
	if value := os.Getenv(name); len(value) > 0 {
		return value
	}
	for _, legacyName := range legacyNames {
		value := os.Getenv(legacyName)
		if len(value) == 0 {
			continue
		}
		if _, warned := warnedLegacyVariables.LoadOrStore(legacyName, struct{}{}); !warned {
			logr.Warnf("[cmd.lookupEnvironment] %s is deprecated, use %s instead", legacyName, name)
		}
		return value
	}
	return ""
}
