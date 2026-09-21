// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package ssh holds the ssh section of an endpoint of the YAML configuration, i.e. the credentials used by the
// endpoints whose URL starts with ssh://, and validates them.
package ssh

import (
	"errors"
)

var (
	// ErrEndpointWithoutSSHUsername is the error with which Go Uptime will panic if an endpoint with SSH monitoring is configured without a user.
	ErrEndpointWithoutSSHUsername = errors.New("you must specify a username for each SSH endpoint")

	// ErrEndpointWithoutSSHAuth is the error with which Go Uptime will panic if an endpoint with SSH monitoring is configured without a password or private key.
	ErrEndpointWithoutSSHAuth = errors.New("you must specify a password or private-key for each SSH endpoint")
)

// Config is the configuration of the credentials of an Endpoint of type SSH. When Username, Password and PrivateKey
// are all empty, the endpoint only checks the SSH banner instead of logging in and running a command.
type Config struct {
	Username   string `yaml:"username,omitempty"`    // Username to log in with; required when Password or PrivateKey is set
	Password   string `yaml:"password,omitempty"`    // Password of the user
	PrivateKey string `yaml:"private-key,omitempty"` // PrivateKey of the user in PEM format, tried before Password when both are set
}

// Validate the SSH configuration. No credentials at all is valid (banner check). Otherwise it returns
// ErrEndpointWithoutSSHUsername if there is no username and ErrEndpointWithoutSSHAuth if there is neither a password
// nor a private key.
func (cfg *Config) Validate() error {
	// If there's no username, password, or private key, this endpoint can still check the SSH banner, so the endpoint is still valid
	if len(cfg.Username) == 0 && len(cfg.Password) == 0 && len(cfg.PrivateKey) == 0 {
		return nil
	}
	// If any authentication method is provided (password or private key), a username is required
	if len(cfg.Username) == 0 {
		return ErrEndpointWithoutSSHUsername
	}
	// If a username is provided, require at least a password or a private key
	if len(cfg.Password) == 0 && len(cfg.PrivateKey) == 0 {
		return ErrEndpointWithoutSSHAuth
	}
	return nil
}
