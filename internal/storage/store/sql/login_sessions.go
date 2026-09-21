package sql

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// createLoginSessionsSchema creates the table of the sessions of the login screen of security.basic (fork). Only the
// hash of the tokens is stored, and timestamps are stored as Unix milliseconds.
func (s *Store) createLoginSessionsSchema() error {
	switch s.driver {
	case "sqlite":
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS login_sessions (
				token_hash             TEXT    PRIMARY KEY,
				username               TEXT    NOT NULL,
				credential_fingerprint TEXT    NOT NULL,
				created_at             INTEGER NOT NULL,
				expires_at             INTEGER NOT NULL
			)
		`); err != nil {
			return err
		}
		_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS login_sessions_expires_at_idx ON login_sessions (expires_at)`)
		return err
	case driverMySQL:
		// MySQL has no CREATE INDEX IF NOT EXISTS, so the index is declared in CREATE TABLE
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS login_sessions (
				token_hash             CHAR(64)     PRIMARY KEY,
				username               VARCHAR(255) NOT NULL,
				credential_fingerprint CHAR(64)     NOT NULL,
				created_at             BIGINT       NOT NULL,
				expires_at             BIGINT       NOT NULL,
				INDEX login_sessions_expires_at_idx (expires_at)
			) ` + mysqlTableOptions)
		return err
	default:
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS login_sessions (
				token_hash             CHAR(64) PRIMARY KEY,
				username               TEXT     NOT NULL,
				credential_fingerprint CHAR(64) NOT NULL,
				created_at             BIGINT   NOT NULL,
				expires_at             BIGINT   NOT NULL
			)
		`); err != nil {
			return err
		}
		_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS login_sessions_expires_at_idx ON login_sessions (expires_at)`)
		return err
	}
}

// CreateLoginSession persists a new login session
func (s *Store) CreateLoginSession(session *common.LoginSession) error {
	_, err := s.db.Exec(
		"INSERT INTO login_sessions (token_hash, username, credential_fingerprint, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)",
		session.TokenHash, session.Username, session.CredentialFingerprint, session.CreatedAt.UnixMilli(), session.ExpiresAt.UnixMilli(),
	)
	return err
}

// GetLoginSession returns the login session with the given token hash
func (s *Store) GetLoginSession(tokenHash string) (*common.LoginSession, error) {
	session := &common.LoginSession{TokenHash: tokenHash}
	var createdAt, expiresAt int64
	err := s.db.QueryRow(
		"SELECT username, credential_fingerprint, created_at, expires_at FROM login_sessions WHERE token_hash = $1",
		tokenHash,
	).Scan(&session.Username, &session.CredentialFingerprint, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrLoginSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	session.CreatedAt, session.ExpiresAt = time.UnixMilli(createdAt), time.UnixMilli(expiresAt)
	return session, nil
}

// DeleteLoginSession deletes the login session with the given token hash, if it exists
func (s *Store) DeleteLoginSession(tokenHash string) error {
	_, err := s.db.Exec("DELETE FROM login_sessions WHERE token_hash = $1", tokenHash)
	return err
}

// DeleteExpiredLoginSessions deletes the login sessions expired at the given time
func (s *Store) DeleteExpiredLoginSessions(now time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM login_sessions WHERE expires_at <= $1", now.UnixMilli())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
