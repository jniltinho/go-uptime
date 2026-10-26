// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"errors"
	"time"
)

var (
	// ErrManagedAlertProviderNotFound is returned when no managed alerting provider exists with the given type
	ErrManagedAlertProviderNotFound = errors.New("managed alerting provider not found")

	// ErrManagedAlertProviderAlreadyExists is returned when a managed alerting provider with the same type already exists
	ErrManagedAlertProviderAlreadyExists = errors.New("a managed alerting provider with the same type already exists")

	// ErrManagedAlertProviderVersionMismatch is returned when the managed alerting provider was changed since the given
	// version
	ErrManagedAlertProviderVersionMismatch = errors.New("managed alerting provider version does not match")
)

// ManagedAlertProvider is the persisted definition of an alerting provider managed through the administration API
type ManagedAlertProvider struct {
	// Type is the alert type of the provider, e.g. "slack", unique among the managed providers
	Type string

	// Definition is the provider definition in YAML, without the type key, as submitted and without default values
	Definition string

	// Version starts at 1 and is incremented on every change
	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time

	// UpdatedBy is the author of the last change (basic auth username or OIDC subject)
	UpdatedBy string
}
