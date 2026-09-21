// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package pushkey keeps the global push keys (fork): the keys of the configuration file and the keys created through the
// administration API, whose tokens are only stored as hashes
package pushkey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const (
	// OriginConfig is a push key of the configuration file, read-only in the administration
	OriginConfig = "config"

	// OriginAdmin is a push key created through the administration API
	OriginAdmin = "admin"
)

var (
	// ErrStorageNotSupported is returned when the storage does not support push keys
	ErrStorageNotSupported = errors.New("the storage does not support push keys managed through the administration")

	// ErrCycleInProgress is returned when a start or configuration reload is in progress
	ErrCycleInProgress = errors.New("a start or configuration reload is in progress, try again later")

	// ErrNameInUse is returned when a push key with the same name already exists
	ErrNameInUse = errors.New("a push key with the same name already exists")

	// ErrNotFound is returned when no push key created through the administration has the given id
	ErrNotFound = errors.New("push key not found")

	// getPushKeyStore is replaced in tests
	getPushKeyStore = store.GetPushKeyStore

	// current is an immutable snapshot of the push keys, replaced on every change (copy-on-write)
	current atomic.Pointer[snapshot]

	// mutex serializes the changes of the snapshot
	mutex sync.Mutex
)

// Key describes a global push key, without its token. It is an item of the keys field of Listing, answered by
// GET /api/v1/admin/push-keys, and its fields are at the top level of Created. It is output-only: the token and its
// SHA-256 hash are never part of it.
type Key struct {
	// ID is the id of a push key created through the administration, 0 for the configuration file
	ID int64 `json:"id,omitempty"`

	// Name identifies who uses the key, e.g. the name of the sender. It is trimmed and has 1 to 64 characters, and a key
	// cannot be created with the name of an existing key of either origin.
	Name string `json:"name"`

	// Hint is the last 4 characters of the token, shown instead of the token so that a key can be recognized.
	Hint string `json:"hint"`

	// Origin is where the key is defined: "config" for a key of the configuration file, read-only in the
	// administration, and "admin" for a key created through the administration API.
	Origin string `json:"origin"`

	// CreatedAt is the instant at which the key was created through the administration, as a RFC 3339 timestamp. It is
	// omitted for a key of the configuration file.
	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// CreatedBy is the author of the key: the username of the basic authentication or the OIDC subject. It is omitted
	// for a key of the configuration file and when the author is unknown.
	CreatedBy string `json:"createdBy,omitempty"`
}

// Created is a push key that was just created, with its token, which is never available again. It is the body of the
// 201 answer of POST /api/v1/admin/push-keys, whose request is a JSON object with the name of the key, and has the
// fields of Key at the same level.
type Created struct {
	Key

	// Token is the generated secret of the key, 32 random letters and digits, used in the path of the push API,
	// /api/push/{token}/{endpoint-key}. It is only answered here: only its SHA-256 hash is stored, so it cannot be read
	// again.
	Token string `json:"token"`
}

// Listing is the administration list of the push keys, the body of GET /api/v1/admin/push-keys
type Listing struct {
	// ManagedUnavailable is whether the push keys created through the administration could not be loaded
	ManagedUnavailable bool `json:"managedUnavailable"`

	// Keys are the keys of the configuration file, then the keys created through the administration, each ordered by
	// name. It is an empty array, never null, without key.
	Keys []*Key `json:"keys"`
}

type snapshot struct {
	configKeys         []*Key
	managedKeys        []*Key
	hashes             map[[sha256.Size]byte]string
	managedHashes      map[int64][sha256.Size]byte
	managedUnavailable bool
}

// Load publishes the push keys of cfg and the push keys persisted in the storage, replacing the previous ones. If the
// persisted push keys cannot be listed, only the keys of the configuration file are accepted.
func Load(cfg *config.Config) {
	mutex.Lock()
	defer mutex.Unlock()
	next := &snapshot{hashes: make(map[[sha256.Size]byte]string), managedHashes: make(map[int64][sha256.Size]byte)}
	if cfg.Push != nil {
		for _, key := range cfg.Push.Keys {
			next.configKeys = append(next.configKeys, &Key{Name: key.Name, Hint: key.Hint(), Origin: OriginConfig})
			next.hashes[key.Hash()] = key.Name
		}
	}
	if pushKeyStore, ok := getPushKeyStore(); ok {
		storedKeys, err := pushKeyStore.ListPushKeys()
		if err != nil {
			next.managedUnavailable = true
			logr.Errorf("[pushkey.Load] Failed to load the push keys created through the administration, so only the push keys of the configuration file are accepted: %s", err.Error())
		}
		for _, stored := range storedKeys {
			decoded, err := hex.DecodeString(stored.TokenHash)
			if err != nil || len(decoded) != sha256.Size {
				logr.Errorf("[pushkey.Load] Ignoring push key %s because its stored hash is invalid", stored.Name)
				continue
			}
			var hash [sha256.Size]byte
			copy(hash[:], decoded)
			next.addManaged(stored, hash)
		}
	}
	current.Store(next)
	if total := len(next.configKeys) + len(next.managedKeys); total > 0 {
		logr.Infof("[pushkey.Load] Loaded %d push keys", total)
	}
}

// Lookup returns the name of the push key whose token is token
func Lookup(token string) (string, bool) {
	snap := current.Load()
	if snap == nil {
		return "", false
	}
	name, exists := snap.hashes[pushconfig.HashToken(token)]
	return name, exists
}

// List returns the push keys: the keys of the configuration file, then the keys created through the administration,
// each ordered by name
func List() *Listing {
	listing := &Listing{Keys: []*Key{}}
	snap := current.Load()
	if snap == nil {
		return listing
	}
	listing.ManagedUnavailable = snap.managedUnavailable
	listing.Keys = append(append(listing.Keys, sortedKeys(snap.configKeys)...), sortedKeys(snap.managedKeys)...)
	return listing
}

// Create creates a push key with a generated token and returns it with the token, which is only available in the
// response
func Create(name, author string) (*Created, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	name = strings.TrimSpace(name)
	if length := utf8.RuneCountInString(name); length == 0 || length > pushconfig.MaximumKeyNameLength {
		return nil, pushconfig.ErrInvalidKeyName
	}
	if snap := current.Load(); snap != nil {
		for _, key := range append(append([]*Key{}, snap.configKeys...), snap.managedKeys...) {
			if key.Name == name {
				return nil, fmt.Errorf("%w: %s", ErrNameInUse, name)
			}
		}
	}
	pushKeyStore, ok := getPushKeyStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	token, err := pushconfig.GenerateToken()
	if err != nil {
		return nil, err
	}
	hash := pushconfig.HashToken(token)
	stored := &common.PushKey{Name: name, TokenHash: hex.EncodeToString(hash[:]), Hint: pushconfig.TokenHint(token), CreatedBy: author}
	if err := pushKeyStore.CreatePushKey(stored, nil); err != nil {
		if errors.Is(err, common.ErrPushKeyAlreadyExists) {
			return nil, fmt.Errorf("%w: %s", ErrNameInUse, name)
		}
		return nil, err
	}
	// Published only after the commit
	next := cloneCurrentSnapshot()
	key := next.addManaged(stored, hash)
	current.Store(next)
	logr.Infof("[pushkey.Create] Push key %s created by %s", name, auditAuthor(author))
	return &Created{Key: *key, Token: token}, nil
}

// Delete deletes the push key created through the administration with the given id. Pushes with its token are rejected
// as soon as it returns.
func Delete(id int64, author string) error {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	snap := current.Load()
	var name string
	if snap != nil {
		for _, key := range snap.managedKeys {
			if key.ID == id {
				name = key.Name
			}
		}
	}
	if len(name) == 0 {
		return ErrNotFound
	}
	pushKeyStore, ok := getPushKeyStore()
	if !ok {
		return ErrStorageNotSupported
	}
	if err := pushKeyStore.DeletePushKey(id, nil); err != nil {
		if errors.Is(err, common.ErrPushKeyNotFound) {
			return ErrNotFound
		}
		return err
	}
	next := cloneCurrentSnapshot()
	if hash, exists := next.managedHashes[id]; exists {
		delete(next.hashes, hash)
		delete(next.managedHashes, id)
	}
	managedKeys := next.managedKeys[:0]
	for _, key := range next.managedKeys {
		if key.ID != id {
			managedKeys = append(managedKeys, key)
		}
	}
	next.managedKeys = managedKeys
	// A key of the configuration file with the same token keeps being accepted
	if snap := current.Load(); snap != nil {
		for hash, keyName := range snap.hashes {
			if _, managed := next.hashes[hash]; !managed && keyName != name {
				next.hashes[hash] = keyName
			}
		}
	}
	current.Store(next)
	logr.Infof("[pushkey.Delete] Push key %s deleted by %s", name, auditAuthor(author))
	return nil
}

// addManaged adds a push key created through the administration to the snapshot, which must not be published yet
func (snap *snapshot) addManaged(stored *common.PushKey, hash [sha256.Size]byte) *Key {
	createdAt := stored.CreatedAt
	key := &Key{ID: stored.ID, Name: stored.Name, Hint: stored.Hint, Origin: OriginAdmin, CreatedAt: &createdAt, CreatedBy: stored.CreatedBy}
	snap.managedKeys = append(snap.managedKeys, key)
	snap.managedHashes[stored.ID] = hash
	snap.hashes[hash] = stored.Name
	return key
}

// cloneCurrentSnapshot returns a copy of the current snapshot, to be changed and published. mutex must be held.
func cloneCurrentSnapshot() *snapshot {
	next := &snapshot{hashes: make(map[[sha256.Size]byte]string), managedHashes: make(map[int64][sha256.Size]byte)}
	if snap := current.Load(); snap != nil {
		next.configKeys = append(next.configKeys, snap.configKeys...)
		next.managedKeys = append(next.managedKeys, snap.managedKeys...)
		next.managedUnavailable = snap.managedUnavailable
		for hash, name := range snap.hashes {
			next.hashes[hash] = name
		}
		for id, hash := range snap.managedHashes {
			next.managedHashes[id] = hash
		}
	}
	return next
}

func sortedKeys(keys []*Key) []*Key {
	sorted := append([]*Key{}, keys...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

func auditAuthor(author string) string {
	if len(author) == 0 {
		return "unknown"
	}
	return author
}
