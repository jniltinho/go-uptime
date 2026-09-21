package managedendpoint

import (
	"sync"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

// AffectedStatusPage is a status page of the configuration file that selects the old key of a renamed managed endpoint
// by key, and that cannot be changed through the administration. It is output-only and travels in
// Detail.AffectedConfigStatusPages, in the response of PUT /api/v1/admin/endpoints/{key}.
type AffectedStatusPage struct {
	// Slug identifies the status page, as in its public URL.
	Slug string `json:"slug"`

	// Title is the display title of the status page. It may be empty.
	Title string `json:"title"`
}

// KeyRenamePlan is what a KeyRenameParticipant changes when a managed endpoint is renamed
type KeyRenamePlan struct {
	// StatusPages are the managed status pages written in the transaction of the rename
	StatusPages []*common.ManagedStatusPageUpdate

	// AffectedConfigStatusPages are the status pages of the configuration file that select the old key
	AffectedConfigStatusPages []AffectedStatusPage

	// Commit publishes the changes once the rename is committed, and Discard releases the participant when the rename
	// fails. Exactly one of them is called.
	Commit  func()
	Discard func()
}

// KeyRenameParticipant takes part in the rename of the key of a managed endpoint, e.g. to update the status pages that
// select it by key. It is registered by a package that cannot be imported by managedendpoint.
type KeyRenameParticipant interface {
	// PrepareKeyRename returns the changes for renaming oldKey to newKey. It is called with a lifecycle change in progress
	// and statesMutex held, and must neither acquire statesMutex nor use the storage.
	PrepareKeyRename(oldKey, newKey, author string) (*KeyRenamePlan, error)
}

var (
	keyRenameParticipant      KeyRenameParticipant
	keyRenameParticipantMutex sync.RWMutex
)

// RegisterKeyRenameParticipant registers the participant of the renames of managed endpoints, replacing the previous one
func RegisterKeyRenameParticipant(participant KeyRenameParticipant) {
	keyRenameParticipantMutex.Lock()
	defer keyRenameParticipantMutex.Unlock()
	keyRenameParticipant = participant
}

func registeredKeyRenameParticipant() KeyRenameParticipant {
	keyRenameParticipantMutex.RLock()
	defer keyRenameParticipantMutex.RUnlock()
	return keyRenameParticipant
}

// restorePersistedTriggeredAlertsOfKey restores into the endpoint the triggered alerts persisted for the endpoint named
// name in group, the name and group it had before being renamed. The persisted triggered alerts move to the new key with
// the endpoint data, in the transaction of the rename.
func restorePersistedTriggeredAlertsOfKey(parsed *Parsed, name, group string) {
	// Only the key and the alerts are read; the alerts are shared, so their restored state ends up in the endpoint
	lookup := &endpoint.Endpoint{Name: name, Group: group, Alerts: parsed.alerts()}
	if watchdog.RestorePersistedTriggeredAlerts(lookup) > 0 {
		parsed.setCounters(lookup.NumberOfSuccessesInARow, lookup.NumberOfFailuresInARow)
	}
}
