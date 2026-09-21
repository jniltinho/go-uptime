// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package lifecycle

import (
	"testing"
	"time"
)

func TestTryBeginChange(t *testing.T) {
	end, ok := TryBeginChange()
	if !ok {
		t.Fatal("expected a change to be allowed when no cycle is in progress")
	}
	end()

	BeginCycle()
	if _, ok := TryBeginChange(); ok {
		t.Fatal("expected a change to be refused while a cycle is in progress")
	}
	EndCycle()

	end, ok = TryBeginChange()
	if !ok {
		t.Fatal("expected a change to be allowed after the cycle ended")
	}
	end()
}

func TestBeginCycle_WaitsForInFlightChange(t *testing.T) {
	end, ok := TryBeginChange()
	if !ok {
		t.Fatal("expected a change to be allowed when no cycle is in progress")
	}
	cycleStarted := make(chan struct{})
	go func() {
		BeginCycle()
		close(cycleStarted)
	}()
	select {
	case <-cycleStarted:
		t.Fatal("expected the cycle to wait for the in-flight change")
	case <-time.After(100 * time.Millisecond):
	}
	// A pending cycle must refuse new changes
	if _, ok := TryBeginChange(); ok {
		t.Fatal("expected a change to be refused while a cycle is pending")
	}
	end()
	select {
	case <-cycleStarted:
	case <-time.After(time.Second):
		t.Fatal("expected the cycle to start once the in-flight change ended")
	}
	EndCycle()
}
