// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package push contains the configuration of the push monitoring (fork): the global push keys and the endpoints of the
// configuration file that receive push, in addition to the external endpoints
package push

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// MinimumKeyTokenLength is the minimum length of the token of a global push key
	MinimumKeyTokenLength = 16

	// MinimumEndpointTokenLength is the minimum length of the push token of an endpoint
	MinimumEndpointTokenLength = 8

	// MaximumTokenLength is the maximum length of every push token
	MaximumTokenLength = 128

	// MaximumKeyNameLength is the maximum length, in characters, of the name of a global push key
	MaximumKeyNameLength = 64

	// hintLength is the number of trailing characters of a token shown as its hint
	hintLength = 4
)

var (
	// ErrInvalidKeyName is returned when the name of a global push key is empty or too long
	ErrInvalidKeyName = fmt.Errorf("push.keys[].name must have between 1 and %d characters", MaximumKeyNameLength)

	// ErrDuplicateKeyName is returned when two global push keys have the same name
	ErrDuplicateKeyName = errors.New("push.keys[].name must be unique")

	// ErrInvalidKeyToken is returned when the token of a global push key does not have the allowed length or characters
	ErrInvalidKeyToken = fmt.Errorf("push.keys[].token must have between %d and %d letters, digits, '-' or '_'", MinimumKeyTokenLength, MaximumTokenLength)

	// ErrInvalidEndpointToken is returned when the push token of an endpoint does not have the allowed length or characters
	ErrInvalidEndpointToken = fmt.Errorf("push tokens of endpoints must have between %d and %d letters, digits, '-' or '_'", MinimumEndpointTokenLength, MaximumTokenLength)

	// ErrDuplicateToken is returned when a push token is used more than once
	ErrDuplicateToken = errors.New("push tokens must be unique")

	// ErrEndpointKeyRequired is returned when an entry of push.endpoints has no key
	ErrEndpointKeyRequired = errors.New("push.endpoints[].key is required")

	// ErrDuplicateEndpoint is returned when the same endpoint key is listed more than once in push.endpoints
	ErrDuplicateEndpoint = errors.New("push.endpoints[].key must be unique")

	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// Config is the configuration of the push monitoring
type Config struct {
	// Keys are the global push keys: each one accepts push for every enabled endpoint that receives push
	Keys []*Key `yaml:"keys,omitempty"`

	// Endpoints are the active endpoints of the configuration file that receive push. External endpoints always do.
	Endpoints []*Endpoint `yaml:"endpoints,omitempty"`
}

// Key is a global push key
type Key struct {
	// Name identifies who uses the key, e.g. akamai
	Name string `yaml:"name"`

	// Token is the secret of the key, used in /api/push/<token>/<endpoint-key>
	Token string `yaml:"token"`

	hash [sha256.Size]byte
}

// Hash returns the SHA-256 hash of the token, set by Config.ValidateAndSetDefaults
func (key *Key) Hash() [sha256.Size]byte {
	return key.hash
}

// Hint returns the trailing characters of the token, shown instead of the token
func (key *Key) Hint() string {
	return TokenHint(key.Token)
}

// Endpoint is an active endpoint of the configuration file that receives push
type Endpoint struct {
	// Key is the key of the endpoint (group and name)
	Key string `yaml:"key"`

	// Token is the optional push token of the endpoint, used in /api/push/<token>
	Token string `yaml:"token,omitempty"`
}

// HashToken returns the SHA-256 hash of a push token
func HashToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// ValidToken returns whether a push token has between minimumLength and MaximumTokenLength letters, digits, '-' or '_'
func ValidToken(token string, minimumLength int) bool {
	return len(token) >= minimumLength && len(token) <= MaximumTokenLength && tokenPattern.MatchString(token)
}

// TokenHint returns the trailing characters of a token, or an empty string if the token is too short to have a hint
func TokenHint(token string) string {
	if len(token) <= hintLength {
		return ""
	}
	return token[len(token)-hintLength:]
}

// Tokens returns every token of the configuration: the tokens of the global keys and of the endpoints
func (c *Config) Tokens() []string {
	if c == nil {
		return nil
	}
	tokens := make([]string, 0, len(c.Keys)+len(c.Endpoints))
	for _, key := range c.Keys {
		tokens = append(tokens, key.Token)
	}
	for _, endpoint := range c.Endpoints {
		if len(endpoint.Token) > 0 {
			tokens = append(tokens, endpoint.Token)
		}
	}
	return tokens
}

// ValidateAndSetDefaults validates the keys and the endpoints, trims the names of the keys, converts the endpoint keys
// to lowercase and computes the hashes of the key tokens
func (c *Config) ValidateAndSetDefaults() error {
	names := make(map[string]struct{}, len(c.Keys))
	tokens := make(map[string]struct{}, len(c.Keys)+len(c.Endpoints))
	for _, key := range c.Keys {
		key.Name = strings.TrimSpace(key.Name)
		if length := utf8.RuneCountInString(key.Name); length == 0 || length > MaximumKeyNameLength {
			return ErrInvalidKeyName
		}
		if _, exists := names[key.Name]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateKeyName, key.Name)
		}
		names[key.Name] = struct{}{}
		if !ValidToken(key.Token, MinimumKeyTokenLength) {
			return fmt.Errorf("%w: %s", ErrInvalidKeyToken, key.Name)
		}
		if _, exists := tokens[key.Token]; exists {
			return fmt.Errorf("%w: push.keys %s", ErrDuplicateToken, key.Name)
		}
		tokens[key.Token] = struct{}{}
		key.hash = HashToken(key.Token)
	}
	endpointKeys := make(map[string]struct{}, len(c.Endpoints))
	for _, endpoint := range c.Endpoints {
		endpoint.Key = strings.ToLower(strings.TrimSpace(endpoint.Key))
		if len(endpoint.Key) == 0 {
			return ErrEndpointKeyRequired
		}
		if _, exists := endpointKeys[endpoint.Key]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateEndpoint, endpoint.Key)
		}
		endpointKeys[endpoint.Key] = struct{}{}
		if len(endpoint.Token) == 0 {
			continue
		}
		if !ValidToken(endpoint.Token, MinimumEndpointTokenLength) {
			return fmt.Errorf("%w: push.endpoints %s", ErrInvalidEndpointToken, endpoint.Key)
		}
		if _, exists := tokens[endpoint.Token]; exists {
			return fmt.Errorf("%w: push.endpoints %s", ErrDuplicateToken, endpoint.Key)
		}
		tokens[endpoint.Token] = struct{}{}
	}
	return nil
}
