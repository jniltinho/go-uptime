// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package heartbeat holds the heartbeat section of an external endpoint of the YAML configuration: the interval
// within which a result must be pushed and the number of retries before the absence of a push counts as a failure.
// The values are validated by the endpoint package.
package heartbeat

import "time"

// Config used to check if the external endpoint has received new results when it should have.
// This configuration is used to trigger alerts when an external endpoint has no new results for a defined period of time
type Config struct {
	// Interval is the time interval at which Go Uptime verifies whether the external endpoint has received new results
	// If no new result is received within the interval, the endpoint is marked as failed and alerts are triggered
	Interval time.Duration `yaml:"interval"`

	// Retries is the number of consecutive failures (down pushes or intervals without push) recorded as pending before
	// a failure is recorded (fork). It requires Interval and defaults to 0.
	Retries int `yaml:"retries,omitempty"`
}
