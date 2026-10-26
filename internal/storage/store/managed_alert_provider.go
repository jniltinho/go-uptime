// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package store

import (
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var _ ManagedAlertProviderStore = (*sql.Store)(nil)

// ManagedAlertProviderStore persists the alerting providers managed through the administration API.
//
// Writes follow the same contract as ManagedEndpointStore: apply is called after the write and before the commit, a
// failing apply rolls the transaction back, and the caller must undo what apply did if the commit itself fails.
type ManagedAlertProviderStore interface {
	// ListManagedAlertProviders returns every managed alerting provider, ordered by type
	ListManagedAlertProviders() ([]*common.ManagedAlertProvider, error)

	// GetManagedAlertProvider returns the managed alerting provider with the given type, or common.ErrManagedAlertProviderNotFound
	GetManagedAlertProvider(alertType string) (*common.ManagedAlertProvider, error)

	// CreateManagedAlertProvider persists a new managed alerting provider with version 1, or returns
	// common.ErrManagedAlertProviderAlreadyExists. On success, Version, CreatedAt and UpdatedAt are set.
	CreateManagedAlertProvider(managedAlertProvider *common.ManagedAlertProvider, apply func() error) error

	// UpdateManagedAlertProvider replaces the definition of the managed alerting provider if its current version is
	// expectedVersion, or returns common.ErrManagedAlertProviderVersionMismatch. On success, Version, CreatedAt and
	// UpdatedAt are set.
	UpdateManagedAlertProvider(managedAlertProvider *common.ManagedAlertProvider, expectedVersion int64, apply func() error) error

	// DeleteManagedAlertProvider deletes the managed alerting provider if its current version is expectedVersion
	DeleteManagedAlertProvider(alertType string, expectedVersion int64, apply func() error) error
}

// GetManagedAlertProviderStore returns the storage provider as a ManagedAlertProviderStore, if it supports managed alerting providers
func GetManagedAlertProviderStore() (ManagedAlertProviderStore, bool) {
	managedAlertProviderStore, ok := Get().(ManagedAlertProviderStore)
	return managedAlertProviderStore, ok
}
