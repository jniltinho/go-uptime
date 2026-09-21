// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"errors"
	"time"
)

var (
	// ErrManagedEndpointNotFound is returned when no managed endpoint exists with the given key
	ErrManagedEndpointNotFound = errors.New("managed endpoint not found")

	// ErrManagedEndpointAlreadyExists is returned when a managed endpoint with the same key already exists
	ErrManagedEndpointAlreadyExists = errors.New("a managed endpoint with the same key already exists")

	// ErrManagedEndpointVersionMismatch is returned when the managed endpoint was changed since the version provided
	ErrManagedEndpointVersionMismatch = errors.New("managed endpoint version does not match")

	// ErrEndpointKeyInUse is returned when renaming a managed endpoint to a key that already has a managed endpoint or
	// stored endpoint data
	ErrEndpointKeyInUse = errors.New("the new endpoint key already has a managed endpoint or stored data")
)

// ManagedEndpointRename describes how a managed endpoint is renamed (see ManagedEndpointStore.RenameManagedEndpoint)
type ManagedEndpointRename struct {
	// OldKey is the key under which the managed endpoint is currently stored
	OldKey string

	// Name and Group are the new name and group of the endpoint, stored with its data when MoveHistory is true
	Name  string
	Group string

	// MoveHistory is whether the statuses, results, events, uptimes and triggered alerts of OldKey move to the new key.
	// It is false for a managed endpoint in conflict with the configuration file, whose key data belongs to the
	// endpoint of the configuration file.
	MoveHistory bool

	// StatusPages are the managed status pages whose definition changes in the same transaction
	StatusPages []*ManagedStatusPageUpdate
}

// ManagedEndpoint is the persisted definition of an endpoint managed through the administration API
type ManagedEndpoint struct {
	// Key is the key of the endpoint (group and name)
	Key string

	// Definition is the endpoint definition in YAML, as submitted and without default values
	Definition string

	// Version starts at 1 and is incremented on every change
	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time

	// UpdatedBy is the author of the last change (basic auth username or OIDC subject)
	UpdatedBy string
}
