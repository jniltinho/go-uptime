package store

import (
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var _ ManagedEndpointStore = (*sql.Store)(nil)

// ManagedEndpointStore persists the endpoints managed through the administration API.
//
// Every write runs in a transaction and accepts an apply function, called after the write and before the commit:
// if apply returns an error, the transaction is rolled back and that error is returned. If the commit itself fails
// after apply succeeded, the error is returned and the caller must undo what apply did. apply must not wait for
// other writes to the store (with SQLite, the transaction holds the only connection).
type ManagedEndpointStore interface {
	// ListManagedEndpoints returns every managed endpoint, ordered by key
	ListManagedEndpoints() ([]*common.ManagedEndpoint, error)

	// GetManagedEndpoint returns the managed endpoint with the given key, or common.ErrManagedEndpointNotFound
	GetManagedEndpoint(key string) (*common.ManagedEndpoint, error)

	// CreateManagedEndpoint persists a new managed endpoint with version 1, or returns
	// common.ErrManagedEndpointAlreadyExists. On success, Version, CreatedAt and UpdatedAt are set.
	CreateManagedEndpoint(managedEndpoint *common.ManagedEndpoint, apply func() error) error

	// UpdateManagedEndpoint replaces the definition of the managed endpoint if its current version is
	// expectedVersion, or returns common.ErrManagedEndpointVersionMismatch. On success, Version, CreatedAt and
	// UpdatedAt are set.
	UpdateManagedEndpoint(managedEndpoint *common.ManagedEndpoint, expectedVersion int64, apply func() error) error

	// RenameManagedEndpoint replaces the definition of the managed endpoint stored under rename.OldKey, if its current
	// version is expectedVersion, and stores it under managedEndpoint.Key, in a single transaction:
	//   - when the key changes and a managed endpoint or endpoint data already exists under the new key, it returns
	//     common.ErrEndpointKeyInUse;
	//   - when rename.MoveHistory is true, the statuses, results, events, uptimes and triggered alerts of the old key
	//     move to the new key, with rename.Name and rename.Group, even if the key does not change;
	//   - every rename.StatusPages is written, or common.ErrManagedStatusPageVersionMismatch is returned if one of them
	//     was changed.
	//
	// On success, Version, CreatedAt and UpdatedAt of managedEndpoint and of the status pages are set.
	RenameManagedEndpoint(managedEndpoint *common.ManagedEndpoint, expectedVersion int64, rename *common.ManagedEndpointRename, apply func() error) error

	// DeleteManagedEndpoint deletes the managed endpoint if its current version is expectedVersion. When
	// deleteEndpointData is true, the statuses, results, events, uptimes and triggered alerts of the key are deleted in
	// the same transaction.
	DeleteManagedEndpoint(key string, expectedVersion int64, deleteEndpointData bool, apply func() error) error
}

// GetManagedEndpointStore returns the storage provider as a ManagedEndpointStore, if it supports managed endpoints
func GetManagedEndpointStore() (ManagedEndpointStore, bool) {
	managedEndpointStore, ok := Get().(ManagedEndpointStore)
	return managedEndpointStore, ok
}
