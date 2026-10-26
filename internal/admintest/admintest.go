// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package admintest limits the tests run from the administration (fork): the test of an endpoint definition and the
// test notification of an alerting provider share the same slots, because both make requests on behalf of an
// administrator.
package admintest

import "errors"

// MaximumConcurrentTests is the number of tests that can run at once
const MaximumConcurrentTests = 2

// ErrTooManyTests is returned when every slot is taken
var ErrTooManyTests = errors.New("too many tests in progress, try again later")

var slots = make(chan struct{}, MaximumConcurrentTests)

// TryAcquire takes a slot without waiting. When ok is true, release must be called once the test has really ended,
// even if its answer was sent earlier.
func TryAcquire() (release func(), ok bool) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		return nil, false
	}
}
