// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"crypto/sha256"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/TwiN/logr"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/metrics"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

// Restore of the managed endpoints of a backup of the administration (fork, see the adminbackup package)

var (
	// ErrMaskedSecret is returned when a definition of a backup has a secret masked by the administration API
	ErrMaskedSecret = errors.New("masked secret")

	// ErrPushWithoutToken is returned when a push endpoint of a backup has no token, which a restore never generates
	ErrPushWithoutToken = errors.New("push endpoint without token")

	// managedUnavailable is whether the last Load could not list the managed endpoints
	managedUnavailable atomic.Bool
)

// IsManagedUnavailable returns whether the managed endpoints could not be loaded from the storage, in which case the
// list of managed endpoints is empty but not trustworthy
func IsManagedUnavailable() bool {
	return managedUnavailable.Load()
}

// RestoreContext accumulates what the items already planned by a restore will create
type RestoreContext struct {
	// PlannedKeys are the keys of the endpoints planned to be created
	PlannedKeys []string

	// PlannedTokens are the push tokens of the endpoints planned to be created or updated
	PlannedTokens map[string]string

	// PlannedKeyHashes are the hashes of the tokens of the push keys planned to be created
	PlannedKeyHashes map[[sha256.Size]byte]bool
}

// AlertCount returns the number of alerts of the endpoint
func (parsed *Parsed) AlertCount() int {
	return len(parsed.alerts())
}

// HasMaskedSecret returns whether the document has Mask where MaskSecrets writes it: sensitive headers, the password or
// sensitive query parameters of the URL, client.oauth2.client-secret, ssh.password, ssh.private-key, the push tokens and
// the leaves of alerts[].provider-override
func HasMaskedSecret(document map[string]any) bool {
	if headers, ok := document["headers"].(map[string]any); ok {
		for name, value := range headers {
			if isSensitiveName(name) && value == Mask {
				return true
			}
		}
	}
	if rawURL, ok := document["url"].(string); ok && strings.Contains(rawURL, Mask) {
		return true
	}
	for _, path := range [][]string{{"client", "oauth2", "client-secret"}, {"ssh", "password"}, {"ssh", "private-key"}, {"token"}, {pushField, "token"}} {
		if parent := nestedMap(document, path[:len(path)-1]...); parent != nil && parent[path[len(path)-1]] == Mask {
			return true
		}
	}
	if alerts, ok := document["alerts"].([]any); ok {
		for _, item := range alerts {
			if alert, ok := item.(map[string]any); ok && hasMaskedLeaf(alert["provider-override"]) {
				return true
			}
		}
	}
	return false
}

func hasMaskedLeaf(value any) bool {
	switch typedValue := value.(type) {
	case map[string]any:
		for _, v := range typedValue {
			if hasMaskedLeaf(v) {
				return true
			}
		}
	case []any:
		for _, v := range typedValue {
			if hasMaskedLeaf(v) {
				return true
			}
		}
	default:
		return value == Mask
	}
	return false
}

// DefinitionPushToken returns the push token written in a definition, even when the definition is invalid
func DefinitionPushToken(definition []byte) string {
	if parsed, err := ParseDefinition(definition); err == nil {
		return parsed.PushToken()
	}
	document, err := ToDocument(definition)
	if err != nil {
		return ""
	}
	if token, ok := document["token"].(string); ok {
		return token
	}
	if token, ok := nestedMap(document, pushField)["token"].(string); ok {
		return token
	}
	return ""
}

// StoredPushTokens returns the push tokens written in the stored definitions of every managed endpoint, including the
// endpoints in conflict and the invalid ones
func StoredPushTokens() []string {
	var tokens []string
	for _, state := range List() {
		if token := DefinitionPushToken([]byte(state.Stored.Definition)); len(token) > 0 {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// NormalizeDefinition returns the stored form of a definition, to compare it with a stored definition
func NormalizeDefinition(definition []byte) ([]byte, error) {
	document, err := ToDocument(definition)
	if err != nil {
		return nil, err
	}
	return FromDocument(document)
}

// ValidateRestore validates, without effects, a definition of a backup against the current managed endpoints and what
// the restore already planned. Unlike Validate, the masked secrets are never restored from the stored definition and
// the push token of a push endpoint is never generated. The type of an existing managed endpoint cannot change.
func (s *Service) ValidateRestore(raw []byte, ctx *RestoreContext) (*Prepared, error) {
	document, err := ToDocument(raw)
	if err != nil {
		return nil, err
	}
	if HasMaskedSecret(document) {
		return nil, ErrMaskedSecret
	}
	parsed, err := ParseDefinition(raw)
	if err != nil {
		return nil, err
	}
	if parsed.IsPush() && len(parsed.PushToken()) == 0 {
		return nil, ErrPushWithoutToken
	}
	key := parsed.Key()
	if state := Get(key); state != nil {
		if storedParsed, err := ParseDefinition([]byte(state.Stored.Definition)); err == nil && storedParsed.IsPush() != parsed.IsPush() {
			return nil, ErrTypeChanged
		}
	}
	managedKeys := append(keysExcept(Keys(), key), ctx.PlannedKeys...)
	tokens := s.pushTokens(key)
	for token, description := range ctx.PlannedTokens {
		tokens[token] = description
	}
	return Prepare(raw, Context{
		Config:             s.cfg,
		ManagedKeys:        managedKeys,
		AllowedExtraLabels: metrics.RegisteredExtraLabels(),
		PushTokens:         tokens,
		IsPushKeyToken: func(token string) bool {
			return isPushKeyToken(token) || ctx.PlannedKeyHashes[pushconfig.HashToken(token)]
		},
	})
}

// NameOrGroupChanges returns whether the definition changes the name or the group of the managed endpoint with the
// same key, which an update applies without changing the key
func NameOrGroupChanges(parsed *Parsed) bool {
	state := Get(parsed.Key())
	if state == nil {
		return false
	}
	storedParsed, err := ParseDefinition([]byte(state.Stored.Definition))
	return err == nil && (storedParsed.Name() != parsed.Name() || storedParsed.Group() != parsed.Group())
}

// RestoreCreate creates a managed endpoint of a backup, like Create but without generating a push token
func (s *Service) RestoreCreate(raw []byte, author string) (*Detail, error) {
	return s.create(raw, author, false)
}

// EndpointTokenGuard returns the guard of pushkey.Restore: it runs fn with the changes of the managed endpoints
// serialized, so that no endpoint takes the token of a push key being restored, passing whether a hash is the hash of
// the token of an endpoint of the configuration file or of any managed endpoint
func (s *Service) EndpointTokenGuard() pushkey.EndpointTokenGuard {
	return func(fn func(isEndpointTokenHash func([sha256.Size]byte) bool) error) error {
		statesMutex.Lock()
		defer statesMutex.Unlock()
		hashes := make(map[[sha256.Size]byte]bool)
		for _, token := range s.EndpointPushTokens() {
			hashes[pushconfig.HashToken(token)] = true
		}
		return fn(func(hash [sha256.Size]byte) bool {
			return hashes[hash]
		})
	}
}

// create creates a managed endpoint, generating the push token of a push endpoint without token when generateToken is
// true
func (s *Service) create(raw []byte, author string, generateToken bool) (*Detail, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	statesMutex.Lock()
	defer statesMutex.Unlock()
	managedEndpointStore, ok := store.GetManagedEndpointStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	var err error
	if generateToken {
		if raw, err = withGeneratedPushToken(raw); err != nil {
			return nil, err
		}
	}
	prepared, err := s.prepare(raw, "")
	if err != nil {
		return nil, err
	}
	key := prepared.Key()
	// Generated before the endpoint starts being monitored (see State.Effective)
	effective, err := effectiveDefinition(&prepared.Parsed)
	if err != nil {
		return nil, err
	}
	// Uses the storage outside of the transaction, so it must run before it
	restoreTriggeredAlerts(&prepared.Parsed)
	stored := &common.ManagedEndpoint{Key: key, Definition: string(prepared.Definition), UpdatedBy: author}
	started := false
	err = managedEndpointStore.CreateManagedEndpoint(stored, func() error {
		if err := startMonitoring(&prepared.Parsed); err != nil {
			return err
		}
		started = prepared.IsEnabled()
		return nil
	})
	if err != nil {
		if started {
			_ = watchdog.StopEndpoint(key, watchdog.SourceAdmin)
		}
		return nil, err
	}
	state := newStateFromPrepared(stored, prepared, effective)
	putState(state)
	logr.Infof("[managedendpoint.Create] Managed endpoint with key=%s created by %s", key, auditAuthor(author))
	return adminDetail(state)
}

// EndpointPushTokens returns the push tokens of the configuration file and of every managed endpoint, including the
// endpoints in conflict and the invalid ones
func (s *Service) EndpointPushTokens() []string {
	seen := make(map[string]bool)
	var tokens []string
	for token := range s.pushTokens("") {
		if !seen[token] {
			seen[token] = true
			tokens = append(tokens, token)
		}
	}
	for _, token := range StoredPushTokens() {
		if !seen[token] {
			seen[token] = true
			tokens = append(tokens, token)
		}
	}
	return tokens
}
