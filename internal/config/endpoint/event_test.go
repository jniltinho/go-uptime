// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

import (
	"testing"
)

func TestNewEventFromResult(t *testing.T) {
	if event := NewEventFromResult(&Result{Success: true}); event.Type != EventHealthy {
		t.Error("expected event.Type to be EventHealthy")
	}
	if event := NewEventFromResult(&Result{Success: false}); event.Type != EventUnhealthy {
		t.Error("expected event.Type to be EventUnhealthy")
	}
}
