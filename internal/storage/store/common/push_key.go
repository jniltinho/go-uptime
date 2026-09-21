// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"errors"
	"time"
)

var (
	// ErrPushKeyNotFound is returned when no push key exists with the given id
	ErrPushKeyNotFound = errors.New("push key not found")

	// ErrPushKeyAlreadyExists is returned when a push key with the same name or token already exists
	ErrPushKeyAlreadyExists = errors.New("a push key with the same name or token already exists")
)

// PushKey is a global push key created through the administration API (fork). Only the hash of its token is stored.
type PushKey struct {
	ID int64

	// Name identifies who uses the key, e.g. akamai
	Name string

	// TokenHash is the SHA-256 hash of the token, in hexadecimal
	TokenHash string

	// Hint is the last characters of the token, shown instead of the token
	Hint string

	CreatedAt time.Time

	// CreatedBy is the author of the key (basic auth username or OIDC subject)
	CreatedBy string
}
