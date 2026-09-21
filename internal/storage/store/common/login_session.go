// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"errors"
	"time"
)

// ErrLoginSessionNotFound is returned when no login session exists with the given token hash
var ErrLoginSessionNotFound = errors.New("login session not found")

// LoginSession is a session created by the login screen of security.basic (fork). Only the hash of its token is stored.
type LoginSession struct {
	// TokenHash is the SHA-256 hash of the token of the session cookie, in hexadecimal
	TokenHash string

	// Username is the basic auth username that logged in
	Username string

	// CredentialFingerprint is the SHA-256 hash, in hexadecimal, of the configured username and password hash when the
	// session was created: a session stops being valid as soon as the credential changes
	CredentialFingerprint string

	CreatedAt time.Time

	ExpiresAt time.Time
}
