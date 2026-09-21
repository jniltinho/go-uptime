// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package lifecycle coordinates the start and reload cycles of the application with runtime changes, such as
// endpoints managed through the administration API.
//
// A cycle (initial start, configuration reload or shutdown) holds the lock exclusively. Runtime changes must not
// wait for a cycle to finish: they try to acquire the lock in shared mode and give up immediately if a cycle is
// in progress or pending.
package lifecycle

import "sync"

var mu sync.RWMutex

// BeginCycle blocks until in-flight runtime changes are done, then prevents new ones until EndCycle is called.
func BeginCycle() {
	mu.Lock()
}

// EndCycle allows runtime changes again. It must be called exactly once for each BeginCycle.
func EndCycle() {
	mu.Unlock()
}

// TryBeginChange starts a runtime change if no cycle is in progress or pending.
//
// When ok is true, the caller must call end once the change has been applied. When ok is false, a cycle is in
// progress and the change must not be applied (for instance, the API responds with 503).
func TryBeginChange() (end func(), ok bool) {
	if !mu.TryRLock() {
		return nil, false
	}
	return mu.RUnlock, true
}
