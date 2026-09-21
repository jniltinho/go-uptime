package endpoint

import (
	"errors"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/heartbeat"
)

func TestExternalEndpoint_ValidateAndSetDefaultsWithHeartbeatRetries(t *testing.T) {
	scenarios := map[string]struct {
		heartbeat heartbeat.Config
		expected  error
	}{
		"without-retries":  {heartbeat: heartbeat.Config{Interval: time.Minute}},
		"maximum":          {heartbeat: heartbeat.Config{Interval: time.Minute, Retries: MaximumHeartbeatRetries}},
		"above-maximum":    {heartbeat: heartbeat.Config{Interval: time.Minute, Retries: MaximumHeartbeatRetries + 1}, expected: ErrExternalEndpointHeartbeatRetriesOutOfRange},
		"negative":         {heartbeat: heartbeat.Config{Interval: time.Minute, Retries: -1}, expected: ErrExternalEndpointHeartbeatRetriesOutOfRange},
		"without-interval": {heartbeat: heartbeat.Config{Retries: 2}, expected: ErrExternalEndpointHeartbeatRetriesWithoutInterval},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			externalEndpoint := &ExternalEndpoint{Name: "backup", Group: "jobs", Token: "token", Heartbeat: scenario.heartbeat}
			if err := externalEndpoint.ValidateAndSetDefaults(); !errors.Is(err, scenario.expected) {
				t.Errorf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
}
