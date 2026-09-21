package memory

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// CreateLoginSession persists a new login session. With the memory storage, sessions are lost when Gatus restarts.
func (s *Store) CreateLoginSession(session *common.LoginSession) error {
	s.loginSessionsMutex.Lock()
	defer s.loginSessionsMutex.Unlock()
	if s.loginSessions == nil {
		s.loginSessions = make(map[string]common.LoginSession)
	}
	s.loginSessions[session.TokenHash] = *session
	return nil
}

// GetLoginSession returns the login session with the given token hash
func (s *Store) GetLoginSession(tokenHash string) (*common.LoginSession, error) {
	s.loginSessionsMutex.RLock()
	defer s.loginSessionsMutex.RUnlock()
	session, exists := s.loginSessions[tokenHash]
	if !exists {
		return nil, common.ErrLoginSessionNotFound
	}
	return &session, nil
}

// DeleteLoginSession deletes the login session with the given token hash, if it exists
func (s *Store) DeleteLoginSession(tokenHash string) error {
	s.loginSessionsMutex.Lock()
	defer s.loginSessionsMutex.Unlock()
	delete(s.loginSessions, tokenHash)
	return nil
}

// DeleteExpiredLoginSessions deletes the login sessions expired at the given time
func (s *Store) DeleteExpiredLoginSessions(now time.Time) (int64, error) {
	s.loginSessionsMutex.Lock()
	defer s.loginSessionsMutex.Unlock()
	var deleted int64
	for tokenHash, session := range s.loginSessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.loginSessions, tokenHash)
			deleted++
		}
	}
	return deleted, nil
}
