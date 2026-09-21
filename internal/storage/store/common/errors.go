// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package common holds the errors and the types shared by the implementations of the store and by their callers.
package common

import "errors"

var (
	ErrEndpointNotFound = errors.New("endpoint not found")               // When an endpoint does not exist in the store
	ErrSuiteNotFound    = errors.New("suite not found")                  // When a suite does not exist in the store
	ErrInvalidTimeRange = errors.New("'from' cannot be older than 'to'") // When an invalid time range is provided
)
