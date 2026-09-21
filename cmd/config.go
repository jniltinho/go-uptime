package cmd

import (
	"fmt"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Work with the configuration",
}

// configValidateCmd loads and validates the configuration exactly as the server does, and nothing else: no storage is
// opened, no server is started and no request is made
var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the configuration without starting anything",
	Long: `Loads and validates the configuration exactly as the server does, without opening the storage, without starting the
server and without making any request. Exits with 0 when the configuration is valid and with 1, showing the error, when
it is not.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) (err error) {
		configPath, err := resolveConfigPath(cmd)
		if err != nil {
			return err
		}
		// Only the result matters here: the informational logs of the load would bury it
		previousThreshold := logr.GetThreshold()
		logr.SetThreshold(logr.LevelError)
		defer logr.SetThreshold(previousThreshold)
		// Some invalid documents make the validators of the load panic, e.g. `endpoints: [null]`. This command promises
		// an error and the exit code 1 for an invalid configuration, never a stack trace.
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("invalid configuration: %v", recovered)
			}
		}()
		cfg, err := config.LoadConfiguration(configPath)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "The configuration is valid: %d endpoints, %d external endpoints, %d suites\n", len(cfg.Endpoints), len(cfg.ExternalEndpoints), len(cfg.Suites))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}
