package cmd

import (
	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/spf13/cobra"
)

// resolveConfigPath returns the path of the configuration: the flag when it was passed, otherwise the environment. The
// flag is tested with Changed and not by its value, because an empty default would shadow the environment. A path
// typed by the operator must exist: config.LoadConfiguration falls back on the default paths when it does not, which
// would silently load another file.
func resolveConfigPath(cmd *cobra.Command) (string, error) {
	if flag := cmd.Flags().Lookup(configFlagName); flag != nil && flag.Changed {
		configPath := flag.Value.String()
		if err := config.RequireConfigPath(configPath); err != nil {
			return "", err
		}
		return configPath, nil
	}
	return lookupEnvironment(ConfigPathEnvVar, LegacyConfigPathEnvVar, LegacyConfigFileEnvVar), nil
}

// resolveLogLevel returns the log level of the flag when it was passed, otherwise of the environment
func resolveLogLevel(cmd *cobra.Command) string {
	if flag := cmd.Flags().Lookup(logLevelFlagName); flag != nil && flag.Changed {
		return flag.Value.String()
	}
	return lookupEnvironment(LogLevelEnvVar, LegacyLogLevelEnvVar)
}

func configureLogging(logLevelAsString string) {
	if logLevel, err := logr.LevelFromString(logLevelAsString); err != nil {
		logr.SetThreshold(logr.LevelInfo)
		if len(logLevelAsString) == 0 {
			logr.Infof("[cmd.configureLogging] Defaulting log level to %s", logr.LevelInfo)
		} else {
			logr.Warnf("[cmd.configureLogging] Invalid log level '%s', defaulting to %s", logLevelAsString, logr.LevelInfo)
		}
	} else {
		logr.SetThreshold(logLevel)
		logr.Infof("[cmd.configureLogging] Log Level is set to %s", logr.GetThreshold())
	}
}
