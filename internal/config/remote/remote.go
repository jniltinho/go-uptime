// Package remote models the remote section of the YAML configuration: other Gatus instances whose endpoint statuses
// are retrieved and merged into the ones of this instance. It validates the section and applies its defaults.
package remote

import (
	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/client"
)

// NOTICE: This is an experimental alpha feature and may be updated/removed in future versions.
// For more information, see https://github.com/TwiN/gatus/issues/64

// Config is the configuration of the remote instances, an experimental feature (see the notice above).
type Config struct {
	// Instances is a list of remote instances to retrieve endpoint statuses from.
	Instances []Instance `yaml:"instances,omitempty"`

	// ClientConfig is the configuration of the client used to communicate with the provider's target
	ClientConfig *client.Config `yaml:"client,omitempty"`
}

// Instance is a remote Gatus instance to retrieve endpoint statuses from.
type Instance struct {
	EndpointPrefix string `yaml:"endpoint-prefix"` // EndpointPrefix is prepended to the name of every endpoint retrieved from the instance
	URL            string `yaml:"url"`             // URL of the endpoint statuses API of the instance
}

// ValidateAndSetDefaults validates the client configuration, using the default one when there is none, and logs a
// warning about the experimental state of the feature when at least one instance is configured.
func (c *Config) ValidateAndSetDefaults() error {
	if c.ClientConfig == nil {
		c.ClientConfig = client.GetDefaultConfig()
	} else {
		if err := c.ClientConfig.ValidateAndSetDefaults(); err != nil {
			return err
		}
	}
	if len(c.Instances) > 0 {
		logr.Warn("WARNING: Your configuration is using 'remote', which is in alpha and may be updated/removed in future versions.")
		logr.Warn("WARNING: See https://github.com/TwiN/gatus/issues/64 for more information")
	}
	return nil
}
