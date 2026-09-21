// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const pushKeyColumns = "push_key_id, name, token_hash, hint, created_at, created_by"

// createPushKeysSchema creates the table of the global push keys created through the administration API (fork). Only
// the hash of the tokens is stored, and timestamps are stored as Unix milliseconds.
func (s *Store) createPushKeysSchema() error {
	switch s.driver {
	case "sqlite":
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS push_keys (
				push_key_id INTEGER PRIMARY KEY,
				name        TEXT    NOT NULL UNIQUE,
				token_hash  TEXT    NOT NULL UNIQUE,
				hint        TEXT    NOT NULL,
				created_at  INTEGER NOT NULL,
				created_by  TEXT    NOT NULL
			)
		`)
		return err
	case driverMySQL:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS push_keys (
				push_key_id BIGINT      AUTO_INCREMENT PRIMARY KEY,
				name        VARCHAR(64) NOT NULL UNIQUE,
				token_hash  CHAR(64)    NOT NULL UNIQUE,
				hint        VARCHAR(8)  NOT NULL,
				created_at  BIGINT      NOT NULL,
				created_by  MEDIUMTEXT  NOT NULL
			) ` + mysqlTableOptions)
		return err
	default:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS push_keys (
				push_key_id BIGSERIAL PRIMARY KEY,
				name        TEXT      NOT NULL UNIQUE,
				token_hash  TEXT      NOT NULL UNIQUE,
				hint        TEXT      NOT NULL,
				created_at  BIGINT    NOT NULL,
				created_by  TEXT      NOT NULL
			)
		`)
		return err
	}
}

// ListPushKeys returns every push key, ordered by name
func (s *Store) ListPushKeys() ([]*common.PushKey, error) {
	rows, err := s.db.Query("SELECT " + pushKeyColumns + " FROM push_keys ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pushKeys []*common.PushKey
	for rows.Next() {
		var pushKey common.PushKey
		var createdAt int64
		if err := rows.Scan(&pushKey.ID, &pushKey.Name, &pushKey.TokenHash, &pushKey.Hint, &createdAt, &pushKey.CreatedBy); err != nil {
			return nil, err
		}
		pushKey.CreatedAt = time.UnixMilli(createdAt)
		pushKeys = append(pushKeys, &pushKey)
	}
	return pushKeys, rows.Err()
}

// CreatePushKey persists a new push key
func (s *Store) CreatePushKey(pushKey *common.PushKey, apply func() error) error {
	now := time.Now().UnixMilli()
	var id int64
	err := s.inTransaction(func(tx *sql.Tx) error {
		err := tx.QueryRow(
			"INSERT INTO push_keys (name, token_hash, hint, created_at, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING push_key_id",
			pushKey.Name, pushKey.TokenHash, pushKey.Hint, now, pushKey.CreatedBy,
		).Scan(&id)
		if isUniqueViolation(err) {
			return common.ErrPushKeyAlreadyExists
		}
		return err
	}, apply)
	if err != nil {
		return err
	}
	pushKey.ID, pushKey.CreatedAt = id, time.UnixMilli(now)
	return nil
}

// DeletePushKey deletes the push key with the given id
func (s *Store) DeletePushKey(id int64, apply func() error) error {
	return s.inTransaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM push_keys WHERE push_key_id = $1", id)
		if err != nil {
			return err
		}
		if rowsAffected, err := result.RowsAffected(); err != nil {
			return err
		} else if rowsAffected == 0 {
			return common.ErrPushKeyNotFound
		}
		return nil
	}, apply)
}
