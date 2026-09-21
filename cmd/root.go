// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package cmd defines the commands of the go-uptime binary (serve, version, config validate, password hash and
// healthcheck). The configuration path and the log level come from the flag when it is passed, then from the environment
// (GO_UPTIME_CONFIG_PATH, GO_UPTIME_LOG_LEVEL, then their GATUS_* aliases, see environment.go), then from the defaults.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
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
	Use:   "go-uptime",
	Short: "Go Uptime — health dashboard, status pages and alerting",
	Long: `Go Uptime monitors HTTP, ICMP, TCP, DNS and other services, evaluates conditions on their results, sends alerts and
serves a dashboard and public status pages.

Without a command, go-uptime starts the server, exactly like "go-uptime serve".`,
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
	// On the root and inherited by every command, so that `go-uptime --config x.yaml` is `go-uptime serve --config x.yaml`
	rootCmd.PersistentFlags().String(configFlagName, "", "path of the configuration file or directory (default: $"+ConfigPathEnvVar+", then config/config.yaml)")
	rootCmd.PersistentFlags().String(logLevelFlagName, "", "log level: DEBUG, INFO, WARN, ERROR or FATAL (default: $"+LogLevelEnvVar+", then INFO)")
}
