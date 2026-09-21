// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"errors"
	"time"
)

var (
	// ErrManagedStatusPageNotFound is returned when no managed status page exists with the given slug
	ErrManagedStatusPageNotFound = errors.New("managed status page not found")

	// ErrManagedStatusPageAlreadyExists is returned when a managed status page with the same slug already exists
	ErrManagedStatusPageAlreadyExists = errors.New("a managed status page with the same slug already exists")

	// ErrManagedStatusPageVersionMismatch is returned when the managed status page was changed since the version provided
	ErrManagedStatusPageVersionMismatch = errors.New("managed status page version does not match")
)

// ManagedStatusPage is the persisted definition of a public status page managed through the administration API
type ManagedStatusPage struct {
	// Slug identifies the status page in its public path (/status/<slug>)
	Slug string

	// Definition is the status page definition in YAML, as submitted and without default values
	Definition string

	// Version starts at 1 and is incremented on every change
	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time

	// UpdatedBy is the author of the last change (basic auth username or OIDC subject)
	UpdatedBy string
}

// ManagedStatusPageUpdate is a change of the definition of a managed status page applied in the transaction of another
// write, if the current version of the page is ExpectedVersion
type ManagedStatusPageUpdate struct {
	// StatusPage is the changed status page: Slug, Definition and UpdatedBy are written. On success, Version and
	// UpdatedAt are set.
	StatusPage *ManagedStatusPage

	ExpectedVersion int64
}
