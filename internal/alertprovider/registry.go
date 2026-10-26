// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
	"gopkg.in/yaml.v3"
)

var (
	// getManagedAlertProviderStore is replaced in tests
	getManagedAlertProviderStore = store.GetManagedAlertProviderStore

	// current is an immutable snapshot of the providers, replaced on every change (copy-on-write)
	current atomic.Pointer[snapshot]

	// mutex serializes the changes of the snapshot. See RegisterDependents for the lock order.
	mutex sync.Mutex
)

// managedEntry is a provider managed through the administration, as stored and as prepared
type managedEntry struct {
	stored *common.ManagedAlertProvider

	// document is the stored definition, decoded; nil when it cannot be decoded
	document map[string]any

	// prepared is the validated provider; nil when the stored definition is no longer valid
	prepared *Prepared

	// err is why the stored definition is not valid, redacted
	err error
}

// snapshot is the state of the alerting providers of a cycle, from the configuration file and the administration
type snapshot struct {
	cfg *config.Config

	// declared are the types whose provider the configuration file declares, valid or not
	declared map[alert.Type]bool

	managed map[alert.Type]*managedEntry

	// effective is the alerting configuration of the managed endpoints: the providers of the file and the valid managed
	// providers of the types the file does not declare
	effective *alerting.Config

	// unavailable is whether the managed providers could not be listed
	unavailable bool
}

// Load publishes the providers of cfg and the providers persisted in the storage, replacing those of the previous
// cycle, and hands the effective configuration to the watchdog. It must run after the storage is initialized and
// before the managed endpoints are loaded. A stored provider that is no longer valid is kept and listed as invalid.
func Load(cfg *config.Config) {
	mutex.Lock()
	defer mutex.Unlock()
	next := &snapshot{cfg: cfg, declared: make(map[alert.Type]bool), managed: make(map[alert.Type]*managedEntry)}
	for _, alertType := range cfg.DeclaredAlertTypes() {
		next.declared[alertType] = true
	}
	if managedStore, ok := getManagedAlertProviderStore(); ok {
		stored, err := managedStore.ListManagedAlertProviders()
		if err != nil {
			next.unavailable = true
			logr.Errorf("[alertprovider.Load] Failed to load the alerting providers managed through the administration, so only the providers of the configuration file are used: %s", err.Error())
		}
		for _, row := range stored {
			entry := newManagedEntry(row)
			next.managed[alertTypeOf(row.Type)] = entry
			if entry.err != nil {
				logr.Errorf("[alertprovider.Load] The alerting provider %s managed through the administration is invalid: %s", row.Type, entry.err.Error())
			} else if next.declared[alertTypeOf(row.Type)] {
				logr.Warnf("[alertprovider.Load] The alerting provider %s managed through the administration is shadowed by the one of the configuration file", row.Type)
			}
		}
	}
	next.effective = next.compose()
	publishSnapshot(next)
	if len(next.managed) > 0 {
		logr.Infof("[alertprovider.Load] Loaded %d alerting providers managed through the administration", len(next.managed))
	}
}

// Effective returns the alerting configuration of the endpoints managed through the administration, or nil before the
// first Load
func Effective() *alerting.Config {
	if snap := current.Load(); snap != nil {
		return snap.effective
	}
	return nil
}

// EffectiveFor returns the effective configuration when it was loaded for cfg, and the alerting section of cfg
// otherwise: a configuration of another cycle never sees the managed providers of this one
func EffectiveFor(cfg *config.Config) *alerting.Config {
	if snap := current.Load(); snap != nil && snap.cfg == cfg {
		return snap.effective
	}
	if cfg == nil {
		return nil
	}
	return cfg.Alerting
}

// newManagedEntry decodes and prepares a stored provider
func newManagedEntry(stored *common.ManagedAlertProvider) *managedEntry {
	entry := &managedEntry{stored: stored}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(stored.Definition), &document); err != nil {
		entry.err = errors.New("the stored definition cannot be decoded")
		return entry
	}
	if document == nil {
		document = map[string]any{}
	}
	entry.document = document
	prepared, err := PrepareDocument(alertTypeOf(stored.Type), copyDocument(document))
	if err != nil {
		entry.err = redactError(alertTypeOf(stored.Type), document, err)
		return entry
	}
	entry.prepared = prepared
	return entry
}

// compose builds the effective configuration of the snapshot
func (snap *snapshot) compose() *alerting.Config {
	managed := make(map[alert.Type]provider.AlertProvider)
	for alertType, entry := range snap.managed {
		if entry.prepared != nil && !snap.declared[alertType] {
			managed[alertType] = entry.prepared.Provider
		}
	}
	var base *alerting.Config
	if snap.cfg != nil {
		base = snap.cfg.Alerting
	}
	return compose(base, managed)
}

// clone returns a copy of the snapshot whose managed map can be changed
func (snap *snapshot) clone() *snapshot {
	next := &snapshot{cfg: snap.cfg, declared: snap.declared, managed: make(map[alert.Type]*managedEntry, len(snap.managed)), unavailable: snap.unavailable}
	for alertType, entry := range snap.managed {
		next.managed[alertType] = entry
	}
	return next
}

func publishSnapshot(next *snapshot) {
	current.Store(next)
	watchdog.SetManagedAlertingConfig(next.effective)
}

// copyDocument returns a deep copy of a decoded definition
func copyDocument(document map[string]any) map[string]any {
	copied, _ := copyValue(document).(map[string]any)
	return copied
}

func copyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, child := range typed {
			copied[key] = copyValue(child)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, child := range typed {
			copied[i] = copyValue(child)
		}
		return copied
	default:
		return typed
	}
}
