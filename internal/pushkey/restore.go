// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package pushkey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/TwiN/logr"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// Restore of the push keys of a backup of the administration (fork, see the adminbackup package)

var (
	// ErrInvalidRestoredKey is returned when the name, the hash or the hint of a restored push key is invalid
	ErrInvalidRestoredKey = errors.New("invalid push key")

	// ErrHashInUse is returned when the hash of a restored push key is the hash of another push key or of the token of an
	// endpoint
	ErrHashInUse = errors.New("token hash in use")

	tokenHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hintPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{0,4}$`)
)

// EndpointTokenGuard runs fn while the tokens of the endpoints cannot change, passing whether a hash is the hash of the
// token of an endpoint. pushkey cannot import the managed endpoints, which provide it.
type EndpointTokenGuard func(fn func(isEndpointTokenHash func([sha256.Size]byte) bool) error) error

// ValidateRestoredKey validates the name, the hexadecimal SHA-256 hash of the token and the hint of a push key of a
// backup, and returns the decoded hash
func ValidateRestoredKey(name, tokenHash, hint string) ([sha256.Size]byte, error) {
	var hash [sha256.Size]byte
	if length := utf8.RuneCountInString(name); length == 0 || length > pushconfig.MaximumKeyNameLength || strings.TrimSpace(name) != name {
		return hash, fmt.Errorf("%w: the name must have between 1 and %d characters, without leading or trailing spaces", ErrInvalidRestoredKey, pushconfig.MaximumKeyNameLength)
	}
	if !tokenHashPattern.MatchString(tokenHash) {
		return hash, fmt.Errorf("%w: the token hash must be 64 lowercase hexadecimal characters", ErrInvalidRestoredKey)
	}
	if !hintPattern.MatchString(hint) {
		return hash, fmt.Errorf("%w: the hint must have at most 4 letters, digits, - or _", ErrInvalidRestoredKey)
	}
	decoded, _ := hex.DecodeString(tokenHash)
	copy(hash[:], decoded)
	return hash, nil
}

// Existing describes the push key that already uses a name or a hash
type Existing struct {
	Origin string
	Name   string
	Hash   [sha256.Size]byte
	// SameHash is whether the push key with the name has the given hash (only for a lookup by name)
	SameHash bool
}

// FindByName returns the push key with the given name, and whether its hash is hash
func FindByName(name string, hash [sha256.Size]byte) (*Existing, bool) {
	snap := current.Load()
	if snap == nil {
		return nil, false
	}
	for _, key := range snap.configKeys {
		if key.Name == name {
			return &Existing{Origin: OriginConfig, Name: name}, true
		}
	}
	for _, key := range snap.managedKeys {
		if key.Name == name {
			managedHash := snap.managedHashes[key.ID]
			return &Existing{Origin: OriginAdmin, Name: name, Hash: managedHash, SameHash: managedHash == hash}, true
		}
	}
	return nil, false
}

// IsHashInUse returns whether a push key, of the configuration file or created through the administration, has the hash
func IsHashInUse(hash [sha256.Size]byte) bool {
	snap := current.Load()
	if snap == nil {
		return false
	}
	_, exists := snap.hashes[hash]
	return exists
}

// IsManagedUnavailable returns whether the push keys created through the administration could not be loaded
func IsManagedUnavailable() bool {
	snap := current.Load()
	return snap != nil && snap.managedUnavailable
}

// Restore creates a push key of a backup with the hash and the hint of the backup, whose token is not known. The name
// must be unused, and the hash must not be the hash of another push key nor of the token of an endpoint, checked by
// guard, which serializes the check, the write and the publication with the changes of the managed endpoints.
func Restore(name, tokenHash, hint, author string, guard EndpointTokenGuard) (*Key, error) {
	hash, err := ValidateRestoredKey(name, tokenHash, hint)
	if err != nil {
		return nil, err
	}
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	pushKeyStore, ok := getPushKeyStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	var restored *Key
	err = guard(func(isEndpointTokenHash func([sha256.Size]byte) bool) error {
		if _, exists := FindByName(name, hash); exists {
			return fmt.Errorf("%w: %s", ErrNameInUse, name)
		}
		if IsHashInUse(hash) || isEndpointTokenHash(hash) {
			return ErrHashInUse
		}
		stored := &common.PushKey{Name: name, TokenHash: tokenHash, Hint: hint, CreatedBy: author}
		if err := pushKeyStore.CreatePushKey(stored, nil); err != nil {
			if errors.Is(err, common.ErrPushKeyAlreadyExists) {
				return fmt.Errorf("%w: %s", ErrNameInUse, name)
			}
			return err
		}
		// Published only after the commit, before the guard lets the managed endpoints change
		next := cloneCurrentSnapshot()
		restored = next.addManaged(stored, hash)
		current.Store(next)
		return nil
	})
	if err != nil {
		return nil, err
	}
	logr.Infof("[pushkey.Restore] Push key %s restored by %s", name, auditAuthor(author))
	return restored, nil
}
