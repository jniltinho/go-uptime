// Package cmd defines the commands of the gatus binary (serve, version, config validate, password hash and
// healthcheck). The configuration path and the log level come from the flag when it is passed, then from the environment
// (GATUS_CONFIG_PATH, GATUS_LOG_LEVEL), then from the defaults.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// GatusConfigPathEnvVar is the environment variable of the path of the configuration
	GatusConfigPathEnvVar = "GATUS_CONFIG_PATH"

	// GatusConfigFileEnvVar is deprecated in favor of GatusConfigPathEnvVar
	GatusConfigFileEnvVar = "GATUS_CONFIG_FILE"

	// GatusLogLevelEnvVar is the environment variable of the log level
	GatusLogLevelEnvVar = "GATUS_LOG_LEVEL"

	// GatusDelayStartEnvVar is the environment variable of the number of seconds to wait before starting
	GatusDelayStartEnvVar = "GATUS_DELAY_START_SECONDS"

	configFlagName   = "config"
	logLevelFlagName = "log-level"
)

var (
	// Version is injected at build time with -ldflags "-X github.com/jniltinho/go-uptime/v7/cmd.Version=x.y.z"
	Version = "dev"

	// GitCommit is injected at build time
	GitCommit = "unknown"

	// BuildDate is injected at build time
	BuildDate = "unknown"
)

// rootCmd runs the server when no command is given, so that the ENTRYPOINT of the image and everyone who runs the
// binary without arguments keep working
var rootCmd = &cobra.Command{
	Use:   "gatus",
	Short: "Gatus — health dashboard, status pages and alerting",
	Long: `Gatus monitors HTTP, ICMP, TCP, DNS and other services, evaluates conditions on their results, sends alerts and
serves a dashboard and public status pages.

Without a command, gatus starts the server, exactly like "gatus serve".`,
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runServe,
}

// Execute runs the command line. The error is printed once, here, because the commands are silent.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	// On the root and inherited by every command, so that `gatus --config x.yaml` is `gatus serve --config x.yaml`
	rootCmd.PersistentFlags().String(configFlagName, "", "path of the configuration file or directory (default: $"+GatusConfigPathEnvVar+", then config/config.yaml)")
	rootCmd.PersistentFlags().String(logLevelFlagName, "", "log level: DEBUG, INFO, WARN, ERROR or FATAL (default: $"+GatusLogLevelEnvVar+", then INFO)")
}
