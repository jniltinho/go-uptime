// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider"
	"gopkg.in/yaml.v3"
)

// Actions of RestoreItem, the same as those of the items of a restore plan
const (
	restoreCreate    = "create"
	restoreUpdate    = "update"
	restoreUnchanged = "unchanged"
	restoreSkip      = "skip"
)

// RestoreItem is what restoring the provider of a backup would do
type RestoreItem struct {
	// Action is "create", "update", "unchanged" or "skip"
	Action string

	// Reason explains a skip, in English
	Reason string

	// Version is the version of the managed provider of the type, 0 without one
	Version int64

	// CurrentSHA256 is the SHA-256 hash of the stored definition of the managed provider of the type, empty without one
	CurrentSHA256 string

	// Definition is the definition to apply, as a document with its type key, YAML encoded
	Definition []byte

	// Users are the keys of the managed endpoints that use the type, restarted by an update
	Users []string

	provider provider.AlertProvider
	alert    alert.Type
}

// IsManagedUnavailable returns whether the managed providers could not be loaded, so that a backup or a restore does
// not mistake an unavailable list for an empty one
func IsManagedUnavailable() bool {
	snap := current.Load()
	return snap != nil && snap.unavailable
}

// PlanRestore returns what restoring the provider of alertType with definition, as stored in a backup (YAML without the
// type key), would do. A type declared in the configuration file is skipped, so is a definition with a masked secret;
// an existing provider with another definition is only updated with overwrite.
func PlanRestore(name, definition string, overwrite bool) *RestoreItem {
	alertType := alertTypeOf(name)
	item := &RestoreItem{alert: alertType}
	skip := func(reason string) *RestoreItem {
		item.Action, item.Reason = restoreSkip, reason
		return item
	}
	if !IsType(alertType) {
		return skip(fmt.Sprintf("%s: %s", ErrUnknownType.Error(), alertType))
	}
	snap := current.Load()
	if snap == nil {
		return skip(ErrCycleInProgress.Error())
	}
	if entry, exists := snap.managed[alertType]; exists {
		item.Version = entry.stored.Version
		item.CurrentSHA256 = sha256Hex([]byte(entry.stored.Definition))
	}
	if snap.declared[alertType] {
		return skip("configured in the configuration file of this installation")
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(definition), &document); err != nil {
		return skip(ErrInvalidDefinition.Error())
	}
	if document == nil {
		document = map[string]any{}
	}
	if hasMaskedSecret(alertType, document, "") {
		return skip("a secret is masked in the backup")
	}
	prepared, err := PrepareDocument(alertType, copyDocument(document))
	if err != nil {
		return skip(redactError(alertType, document, err).Error())
	}
	withType := copyDocument(prepared.Document)
	withType[typeField] = string(alertType)
	if item.Definition, err = yaml.Marshal(withType); err != nil {
		return skip(err.Error())
	}
	item.provider = prepared.Provider
	entry, exists := snap.managed[alertType]
	if !exists {
		item.Action = restoreCreate
		return item
	}
	if storedDocument := entry.document; storedDocument != nil {
		if stored, err := yaml.Marshal(storedDocument); err == nil && string(stored) == string(prepared.Definition) {
			item.Action = restoreUnchanged
			return item
		}
	}
	if !overwrite {
		return skip("already exists")
	}
	item.Action = restoreUpdate
	if participant := getDependents(); participant != nil {
		item.Users = participant.Users(alertType)
	}
	return item
}

// RestoredEffective returns the effective configuration the restore of items would leave: the current one with the
// providers that the items create or update
func RestoredEffective(items []*RestoreItem) *alerting.Config {
	snap := current.Load()
	if snap == nil {
		return nil
	}
	next := snap.clone()
	for _, item := range items {
		if item.provider != nil && (item.Action == restoreCreate || item.Action == restoreUpdate) {
			next.managed[item.alert] = &managedEntry{stored: nil, prepared: &Prepared{Type: item.alert, Provider: item.provider}}
		}
	}
	return next.compose()
}

// ManagedFingerprint returns the type and the version of every managed provider, sorted, for the fingerprint of a
// restore: the endpoints of the plan are validated against them, even when they are not in the backup
func ManagedFingerprint() []string {
	snap := current.Load()
	if snap == nil {
		return []string{}
	}
	entries := make([]string, 0, len(snap.managed))
	for alertType, entry := range snap.managed {
		entries = append(entries, fmt.Sprintf("%s:%d:%s", alertType, entry.stored.Version, sha256Hex([]byte(entry.stored.Definition))))
	}
	sort.Strings(entries)
	return entries
}

// hasMaskedSecret returns whether a secret field of document holds MaskedValue
func hasMaskedSecret(alertType alert.Type, value any, path string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if hasMaskedSecret(alertType, child, joinPath(path, key)) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasMaskedSecret(alertType, child, path+"[]") {
				return true
			}
		}
	case string:
		return typed == MaskedValue && isSecretPath(alertType, path)
	}
	return false
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
