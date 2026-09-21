// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package push resolves the pushes received through /api/push (fork): the push token of an endpoint identifies it,
// and a global push key accepts push for every endpoint that receives push
package push

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
)

// Target is an endpoint that receives a push
type Target struct {
	// Key is the key of the endpoint
	Key string

	// External is the external endpoint that receives the push, or nil for an active endpoint monitored by Go Uptime
	External *endpoint.ExternalEndpoint

	// Managed is whether the endpoint is managed through the administration, in which case the push is only accepted
	// while its monitoring runs
	Managed bool
}

// Resolver resolves the endpoint of a push. It is immutable once created.
type Resolver struct {
	// targets are the enabled endpoints that receive push, by key
	targets map[string]Target

	// tokens are the push tokens used by exactly one endpoint, with the key of that endpoint
	tokens map[string]string

	// endpointTokens are the push tokens of the endpoints, by key, including the tokens used by more than one endpoint
	endpointTokens map[string]string

	// globalKeys are the names of the global push keys, by hash of their token
	globalKeys map[[sha256.Size]byte]string
}

// NewResolver returns the resolver of the endpoints of cfg that receive push: the external endpoints and the endpoints
// listed in push.endpoints
func NewResolver(cfg *config.Config) *Resolver {
	resolver := &Resolver{
		targets:        make(map[string]Target),
		tokens:         make(map[string]string),
		endpointTokens: make(map[string]string),
		globalKeys:     make(map[[sha256.Size]byte]string),
	}
	ambiguous := make(map[string]struct{})
	addToken := func(token, key string) {
		resolver.endpointTokens[key] = token
		if _, isAmbiguous := ambiguous[token]; isAmbiguous {
			return
		}
		if other, used := resolver.tokens[token]; used {
			logr.Warnf("[push.NewResolver] The endpoints with key=%s and key=%s have the same push token: /api/push/<token> ignores it, use /api/push/<token>/<endpoint-key>", other, key)
			delete(resolver.tokens, token)
			ambiguous[token] = struct{}{}
			return
		}
		resolver.tokens[token] = key
	}
	for _, externalEndpoint := range cfg.ExternalEndpoints {
		if !externalEndpoint.IsEnabled() {
			continue
		}
		key := externalEndpoint.Key()
		resolver.targets[key] = Target{Key: key, External: externalEndpoint}
		if len(externalEndpoint.Token) > 0 {
			addToken(externalEndpoint.Token, key)
		}
	}
	if cfg.Push != nil {
		for _, pushEndpoint := range cfg.Push.Endpoints {
			ep := cfg.GetEndpointByKey(pushEndpoint.Key)
			if ep == nil || !ep.IsEnabled() {
				continue
			}
			key := ep.Key()
			resolver.targets[key] = Target{Key: key}
			if len(pushEndpoint.Token) > 0 {
				addToken(pushEndpoint.Token, key)
			}
		}
		for _, globalKey := range cfg.Push.Keys {
			resolver.globalKeys[globalKey.Hash()] = globalKey.Name
		}
	}
	return resolver
}

// Resolve returns the endpoint of a push to /api/push/<token>, or to /api/push/<token>/<endpointKey> when endpointKey
// is not empty. The second form accepts the push token of the endpoint or a global push key. The second return value is
// the name of the global key used, if any.
func (resolver *Resolver) Resolve(token, endpointKey string) (Target, string, bool) {
	if len(token) == 0 {
		return Target{}, "", false
	}
	if len(endpointKey) == 0 {
		if key, exists := resolver.tokens[token]; exists {
			return resolver.targets[key], "", true
		}
		// Fork: endpoints managed through the administration
		if managed, exists := managedendpoint.PushTargetByToken(token); exists {
			return Target{Key: managed.Key, External: managed.Push, Managed: true}, "", true
		}
		return Target{}, "", false
	}
	key := strings.ToLower(endpointKey)
	target, exists := resolver.targets[key]
	endpointToken := resolver.endpointTokens[key]
	if !exists {
		managed, managedExists := managedendpoint.PushTargetByKey(key)
		if !managedExists {
			return Target{}, "", false
		}
		target, endpointToken = Target{Key: managed.Key, External: managed.Push, Managed: true}, managedendpoint.PushTokenOf(key)
	}
	if len(endpointToken) > 0 && subtle.ConstantTimeCompare([]byte(endpointToken), []byte(token)) == 1 {
		return target, "", true
	}
	if name, isGlobalKey := resolver.globalKeys[pushconfig.HashToken(token)]; isGlobalKey {
		return target, name, true
	}
	// Global keys created through the administration
	if name, isGlobalKey := pushkey.Lookup(token); isGlobalKey {
		return target, name, true
	}
	return Target{}, "", false
}
