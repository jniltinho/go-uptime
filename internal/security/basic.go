// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package security

import (
	"encoding/base64"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBasicSessionTTL is the default validity of the sessions of the login screen of security.basic
	DefaultBasicSessionTTL = 8 * time.Hour

	// MinimumBasicSessionTTL is the minimum value of security.basic.session-ttl
	MinimumBasicSessionTTL = 5 * time.Minute

	// MaximumBasicSessionTTL is the maximum value of security.basic.session-ttl
	MaximumBasicSessionTTL = 30 * 24 * time.Hour
)

// BasicConfig is the configuration for Basic authentication
type BasicConfig struct {
	// Username is the name which will need to be used for a successful authentication
	Username string `yaml:"username"`

	// PasswordBcryptHashBase64Encoded is the base64 encoded string of the Bcrypt hash of the password to use to
	// authenticate using basic auth.
	PasswordBcryptHashBase64Encoded string `yaml:"password-bcrypt-base64"`

	// SessionTTL is the validity of the sessions created by the login screen (fork). Defaults to 8 hours, and must be
	// between 5 minutes and 30 days.
	SessionTTL time.Duration `yaml:"session-ttl,omitempty"`
}

// isValid returns whether the basic security configuration is valid or not. The password must be the base64 of a bcrypt
// hash: a value that is not one can never match a password, and it used to be found only when the server started, as a
// panic, after go-uptime config validate had called the configuration valid.
func (c *BasicConfig) isValid() bool {
	return len(c.Username) > 0 && isBcryptHashInBase64(c.PasswordBcryptHashBase64Encoded) &&
		(c.SessionTTL == 0 || (c.SessionTTL >= MinimumBasicSessionTTL && c.SessionTTL <= MaximumBasicSessionTTL))
}

// isBcryptHashInBase64 returns whether the value is what go-uptime password hash prints: a bcrypt hash encoded in base64
// with the URL alphabet
func isBcryptHashInBase64(encoded string) bool {
	hash, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	_, err = bcrypt.Cost(hash)
	return err == nil
}

// validateAndSetDefaults returns whether the basic security configuration is valid or not and sets default values
func (c *BasicConfig) validateAndSetDefaults() bool {
	if !c.isValid() {
		return false
	}
	if c.SessionTTL == 0 {
		c.SessionTTL = DefaultBasicSessionTTL
	}
	return true
}

// sessionTTL returns the validity of the login sessions, with the default value when it is not set
func (c *BasicConfig) sessionTTL() time.Duration {
	if c.SessionTTL <= 0 {
		return DefaultBasicSessionTTL
	}
	return c.SessionTTL
}
