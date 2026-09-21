package cmd

import (
	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
)

// loadUpdatedConfiguration loads and validates a modified configuration while the current one is still running.
//
// It returns the updated configuration and true when it can be applied. When the updated configuration is invalid
// and SkipInvalidConfigUpdate is enabled, it logs the error, keeps the current configuration running (nothing has
// been stopped yet), marks the modification as processed and returns false. When SkipInvalidConfigUpdate is
// disabled, an invalid configuration still panics.
func loadUpdatedConfiguration(current *config.Config, load func() (*config.Config, error)) (*config.Config, bool) {
	updatedConfig, err := load()
	if err == nil {
		return updatedConfig, true
	}
	if !current.SkipInvalidConfigUpdate {
		panic(err)
	}
	logr.Errorf("[cmd.loadUpdatedConfiguration] Failed to load new configuration: %s", err.Error())
	logr.Error("[cmd.loadUpdatedConfiguration] The configuration file was updated, but it is not valid. The current configuration will continue being used.")
	// Update the last file modification time to avoid trying to process the same invalid configuration again
	current.UpdateLastFileModTime()
	return nil, false
}
