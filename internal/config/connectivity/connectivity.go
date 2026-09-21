// Package connectivity models the connectivity section of the YAML configuration: a checker that tells whether Gatus
// itself can reach the internet, so that endpoints are not evaluated (and alerts not sent) while it cannot. It
// validates the section, applies its defaults and performs the check.
package connectivity

import (
	"errors"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/client"
)

var (
	// ErrInvalidInterval is returned by Config.ValidateAndSetDefaults when the interval of the checker is set but
	// lower than 5 seconds.
	ErrInvalidInterval = errors.New("connectivity.checker.interval must be 5s or higher")

	// ErrInvalidDNSTarget is returned by Config.ValidateAndSetDefaults when the target of the checker does not end
	// with :53.
	ErrInvalidDNSTarget = errors.New("connectivity.checker.target must be suffixed with :53")
)

// Config is the configuration for the connectivity checker.
type Config struct {
	// Checker is the configuration of the connectivity checker. Connectivity is not checked when it is nil.
	Checker *Checker `yaml:"checker,omitempty"`
}

// ValidateAndSetDefaults validates the connectivity configuration and defaults the interval of the checker to 60
// seconds. It returns ErrInvalidInterval or ErrInvalidDNSTarget, and does nothing when there is no checker.
func (c *Config) ValidateAndSetDefaults() error {
	if c.Checker != nil {
		if c.Checker.Interval == 0 {
			c.Checker.Interval = 60 * time.Second
		} else if c.Checker.Interval < 5*time.Second {
			return ErrInvalidInterval
		}
		if !strings.HasSuffix(c.Checker.Target, ":53") {
			return ErrInvalidDNSTarget
		}
	}
	return nil
}

// Checker is the configuration for making sure Gatus has access to the internet.
type Checker struct {
	Target   string        `yaml:"target"`             // e.g. 1.1.1.1:53
	Interval time.Duration `yaml:"interval,omitempty"` // Interval is the minimum time between two checks; defaults to 60s, at least 5s

	isConnected bool
	lastCheck   time.Time
}

// Check returns whether a TCP connection to the target can be established within 5 seconds. It always performs the
// check, without using or updating the cached state.
func (c *Checker) Check() bool {
	connected, _ := client.CanCreateNetworkConnection("tcp", c.Target, "", &client.Config{Timeout: 5 * time.Second})
	return connected
}

// IsConnected returns whether Gatus has connectivity. The result of Check is cached: a new check is only made when
// more than Interval has elapsed since the previous one.
func (c *Checker) IsConnected() bool {
	if now := time.Now(); now.After(c.lastCheck.Add(c.Interval)) {
		c.lastCheck, c.isConnected = now, c.Check()
	}
	return c.isConnected
}
