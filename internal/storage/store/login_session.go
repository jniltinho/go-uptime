// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package store

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/memory"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var (
	_ LoginSessionStore = (*sql.Store)(nil)
	_ LoginSessionStore = (*memory.Store)(nil)
)

// LoginSessionStore persists the sessions of the login screen of security.basic (fork)
type LoginSessionStore interface {
	// CreateLoginSession persists a new login session
	CreateLoginSession(session *common.LoginSession) error

	// GetLoginSession returns the login session with the given token hash, or common.ErrLoginSessionNotFound
	GetLoginSession(tokenHash string) (*common.LoginSession, error)

	// DeleteLoginSession deletes the login session with the given token hash, if it exists
	DeleteLoginSession(tokenHash string) error

	// DeleteExpiredLoginSessions deletes the login sessions expired at the given time and returns how many were deleted
	DeleteExpiredLoginSessions(now time.Time) (int64, error)
}

// GetLoginSessionStore returns the storage provider as a LoginSessionStore, if it supports login sessions
func GetLoginSessionStore() (LoginSessionStore, bool) {
	loginSessionStore, ok := Get().(LoginSessionStore)
	return loginSessionStore, ok
}
