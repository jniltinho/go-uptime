package store

import (
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var _ ManagedStatusPageStore = (*sql.Store)(nil)

// ManagedStatusPageStore persists the public status pages managed through the administration API.
//
// Writes follow the same contract as ManagedEndpointStore: apply is called after the write and before the commit, a
// failing apply rolls the transaction back, and the caller must undo what apply did if the commit itself fails.
type ManagedStatusPageStore interface {
	// ListManagedStatusPages returns every managed status page, ordered by slug
	ListManagedStatusPages() ([]*common.ManagedStatusPage, error)

	// GetManagedStatusPage returns the managed status page with the given slug, or common.ErrManagedStatusPageNotFound
	GetManagedStatusPage(slug string) (*common.ManagedStatusPage, error)

	// CreateManagedStatusPage persists a new managed status page with version 1, or returns
	// common.ErrManagedStatusPageAlreadyExists. On success, Version, CreatedAt and UpdatedAt are set.
	CreateManagedStatusPage(managedStatusPage *common.ManagedStatusPage, apply func() error) error

	// UpdateManagedStatusPage replaces the definition of the managed status page if its current version is
	// expectedVersion, or returns common.ErrManagedStatusPageVersionMismatch. On success, Version, CreatedAt and
	// UpdatedAt are set.
	UpdateManagedStatusPage(managedStatusPage *common.ManagedStatusPage, expectedVersion int64, apply func() error) error

	// DeleteManagedStatusPage deletes the managed status page if its current version is expectedVersion
	DeleteManagedStatusPage(slug string, expectedVersion int64, apply func() error) error
}

// GetManagedStatusPageStore returns the storage provider as a ManagedStatusPageStore, if it supports managed status pages
func GetManagedStatusPageStore() (ManagedStatusPageStore, bool) {
	managedStatusPageStore, ok := Get().(ManagedStatusPageStore)
	return managedStatusPageStore, ok
}
