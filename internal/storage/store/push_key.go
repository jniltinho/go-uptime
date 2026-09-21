// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package store

import (
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var _ PushKeyStore = (*sql.Store)(nil)

// PushKeyStore persists the global push keys created through the administration API (fork).
//
// Writes follow the same contract as ManagedEndpointStore: apply is called after the write and before the commit, and
// a failing apply rolls the transaction back.
type PushKeyStore interface {
	// ListPushKeys returns every push key, ordered by name
	ListPushKeys() ([]*common.PushKey, error)

	// CreatePushKey persists a new push key, or returns common.ErrPushKeyAlreadyExists. On success, ID and CreatedAt are
	// set.
	CreatePushKey(pushKey *common.PushKey, apply func() error) error

	// DeletePushKey deletes the push key with the given id, or returns common.ErrPushKeyNotFound
	DeletePushKey(id int64, apply func() error) error
}

// GetPushKeyStore returns the storage provider as a PushKeyStore, if it supports push keys
func GetPushKeyStore() (PushKeyStore, bool) {
	pushKeyStore, ok := Get().(PushKeyStore)
	return pushKeyStore, ok
}
