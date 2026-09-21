// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const managedEndpointColumns = "endpoint_key, definition, version, created_at, updated_at, updated_by"

// createManagedEndpointsSchema creates the table of the endpoints managed through the administration API.
// Timestamps are stored as Unix milliseconds so that both drivers behave the same way.
func (s *Store) createManagedEndpointsSchema() error {
	if s.driver == "sqlite" {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_endpoints (
				managed_endpoint_id INTEGER PRIMARY KEY,
				endpoint_key        TEXT    NOT NULL UNIQUE,
				definition          TEXT    NOT NULL,
				version             INTEGER NOT NULL,
				created_at          INTEGER NOT NULL,
				updated_at          INTEGER NOT NULL,
				updated_by          TEXT    NOT NULL
			)
		`)
		return err
	}
	if s.driver == driverMySQL {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_endpoints (
				managed_endpoint_id BIGINT       AUTO_INCREMENT PRIMARY KEY,
				endpoint_key        VARCHAR(768) NOT NULL UNIQUE,
				definition          MEDIUMTEXT   NOT NULL,
				version             BIGINT       NOT NULL,
				created_at          BIGINT       NOT NULL,
				updated_at          BIGINT       NOT NULL,
				updated_by          MEDIUMTEXT   NOT NULL
			) ` + mysqlTableOptions)
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS managed_endpoints (
			managed_endpoint_id BIGSERIAL PRIMARY KEY,
			endpoint_key        TEXT      NOT NULL UNIQUE,
			definition          TEXT      NOT NULL,
			version             BIGINT    NOT NULL,
			created_at          BIGINT    NOT NULL,
			updated_at          BIGINT    NOT NULL,
			updated_by          TEXT      NOT NULL
		)
	`)
	return err
}

// ListManagedEndpoints returns every managed endpoint, ordered by key
func (s *Store) ListManagedEndpoints() ([]*common.ManagedEndpoint, error) {
	rows, err := s.db.Query("SELECT " + managedEndpointColumns + " FROM managed_endpoints ORDER BY endpoint_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var managedEndpoints []*common.ManagedEndpoint
	for rows.Next() {
		managedEndpoint, err := scanManagedEndpoint(rows)
		if err != nil {
			return nil, err
		}
		managedEndpoints = append(managedEndpoints, managedEndpoint)
	}
	return managedEndpoints, rows.Err()
}

// GetManagedEndpoint returns the managed endpoint with the given key, or common.ErrManagedEndpointNotFound
func (s *Store) GetManagedEndpoint(key string) (*common.ManagedEndpoint, error) {
	managedEndpoint, err := scanManagedEndpoint(s.db.QueryRow("SELECT "+managedEndpointColumns+" FROM managed_endpoints WHERE endpoint_key = $1", key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrManagedEndpointNotFound
	}
	return managedEndpoint, err
}

// CreateManagedEndpoint persists a new managed endpoint with version 1
func (s *Store) CreateManagedEndpoint(managedEndpoint *common.ManagedEndpoint, apply func() error) error {
	now := time.Now().UnixMilli()
	err := s.inTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO managed_endpoints ("+managedEndpointColumns+") VALUES ($1, $2, $3, $4, $5, $6)",
			managedEndpoint.Key, managedEndpoint.Definition, 1, now, now, managedEndpoint.UpdatedBy,
		)
		if isUniqueViolation(err) {
			return common.ErrManagedEndpointAlreadyExists
		}
		return err
	}, apply)
	if err != nil {
		return err
	}
	managedEndpoint.Version = 1
	managedEndpoint.CreatedAt, managedEndpoint.UpdatedAt = time.UnixMilli(now), time.UnixMilli(now)
	return nil
}

// UpdateManagedEndpoint replaces the definition of the managed endpoint if its current version is expectedVersion
func (s *Store) UpdateManagedEndpoint(managedEndpoint *common.ManagedEndpoint, expectedVersion int64, apply func() error) error {
	now := time.Now().UnixMilli()
	var createdAt int64
	err := s.inTransaction(func(tx *sql.Tx) error {
		var version int64
		err := tx.QueryRow("SELECT version, created_at FROM managed_endpoints WHERE endpoint_key = $1", managedEndpoint.Key).Scan(&version, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return common.ErrManagedEndpointNotFound
		}
		if err != nil {
			return err
		}
		if version != expectedVersion {
			return common.ErrManagedEndpointVersionMismatch
		}
		result, err := tx.Exec(
			"UPDATE managed_endpoints SET definition = $1, version = $2, updated_at = $3, updated_by = $4 WHERE endpoint_key = $5 AND version = $6",
			managedEndpoint.Definition, expectedVersion+1, now, managedEndpoint.UpdatedBy, managedEndpoint.Key, expectedVersion,
		)
		if err != nil {
			return err
		}
		// Another transaction may have changed the row between the SELECT and the UPDATE
		if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
			return common.ErrManagedEndpointVersionMismatch
		}
		return nil
	}, apply)
	if err != nil {
		return err
	}
	managedEndpoint.Version = expectedVersion + 1
	managedEndpoint.CreatedAt, managedEndpoint.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(now)
	return nil
}

// RenameManagedEndpoint replaces the definition of the managed endpoint stored under rename.OldKey and stores it under
// managedEndpoint.Key, moving the endpoint data when rename.MoveHistory is true
func (s *Store) RenameManagedEndpoint(managedEndpoint *common.ManagedEndpoint, expectedVersion int64, rename *common.ManagedEndpointRename, apply func() error) error {
	now := time.Now().UnixMilli()
	var createdAt int64
	err := s.inTransaction(func(tx *sql.Tx) error {
		var version int64
		err := tx.QueryRow("SELECT version, created_at FROM managed_endpoints WHERE endpoint_key = $1", rename.OldKey).Scan(&version, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return common.ErrManagedEndpointNotFound
		}
		if err != nil {
			return err
		}
		if version != expectedVersion {
			return common.ErrManagedEndpointVersionMismatch
		}
		if managedEndpoint.Key != rename.OldKey {
			// Data under the new key may exist without a definition, e.g. the history of an endpoint removed from the
			// configuration file before the next reload: it is neither merged nor deleted
			for _, query := range []string{"SELECT COUNT(*) FROM managed_endpoints WHERE endpoint_key = $1", "SELECT COUNT(*) FROM endpoints WHERE endpoint_key = $1"} {
				var count int64
				if err := tx.QueryRow(query, managedEndpoint.Key).Scan(&count); err != nil {
					return err
				}
				if count > 0 {
					return common.ErrEndpointKeyInUse
				}
			}
		}
		result, err := tx.Exec(
			"UPDATE managed_endpoints SET endpoint_key = $1, definition = $2, version = $3, updated_at = $4, updated_by = $5 WHERE endpoint_key = $6 AND version = $7",
			managedEndpoint.Key, managedEndpoint.Definition, expectedVersion+1, now, managedEndpoint.UpdatedBy, rename.OldKey, expectedVersion,
		)
		if isUniqueViolation(err) {
			return common.ErrEndpointKeyInUse
		}
		if err != nil {
			return err
		}
		// Another transaction may have changed the row between the SELECT and the UPDATE
		if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
			return common.ErrManagedEndpointVersionMismatch
		}
		if rename.MoveHistory {
			// Events, results, uptimes and triggered alerts reference endpoint_id, so they follow the renamed row
			_, err := tx.Exec(
				"UPDATE endpoints SET endpoint_key = $1, endpoint_name = $2, endpoint_group = $3 WHERE endpoint_key = $4",
				managedEndpoint.Key, rename.Name, rename.Group, rename.OldKey,
			)
			if isUniqueViolation(err) {
				return common.ErrEndpointKeyInUse
			}
			if err != nil {
				return err
			}
		}
		for _, update := range rename.StatusPages {
			page := update.StatusPage
			result, err := tx.Exec(
				"UPDATE managed_status_pages SET definition = $1, version = $2, updated_at = $3, updated_by = $4 WHERE slug = $5 AND version = $6",
				page.Definition, update.ExpectedVersion+1, now, page.UpdatedBy, page.Slug, update.ExpectedVersion,
			)
			if err != nil {
				return err
			}
			if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
				return common.ErrManagedStatusPageVersionMismatch
			}
		}
		return nil
	}, apply)
	if err != nil {
		return err
	}
	if s.writeThroughCache != nil {
		_ = s.writeThroughCache.DeleteKeysByPattern(rename.OldKey + "*")
		_ = s.writeThroughCache.DeleteKeysByPattern(managedEndpoint.Key + "*")
	}
	managedEndpoint.Version = expectedVersion + 1
	managedEndpoint.CreatedAt, managedEndpoint.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(now)
	for _, update := range rename.StatusPages {
		update.StatusPage.Version, update.StatusPage.UpdatedAt = update.ExpectedVersion+1, time.UnixMilli(now)
	}
	return nil
}

// DeleteManagedEndpoint deletes the managed endpoint if its current version is expectedVersion and, when
// deleteEndpointData is true, the statuses, results, events, uptimes and triggered alerts of its key
func (s *Store) DeleteManagedEndpoint(key string, expectedVersion int64, deleteEndpointData bool, apply func() error) error {
	err := s.inTransaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM managed_endpoints WHERE endpoint_key = $1 AND version = $2", key, expectedVersion)
		if err != nil {
			return err
		}
		if rowsAffected, err := result.RowsAffected(); err != nil {
			return err
		} else if rowsAffected == 0 {
			var version int64
			err := tx.QueryRow("SELECT version FROM managed_endpoints WHERE endpoint_key = $1", key).Scan(&version)
			if errors.Is(err, sql.ErrNoRows) {
				return common.ErrManagedEndpointNotFound
			}
			if err != nil {
				return err
			}
			return common.ErrManagedEndpointVersionMismatch
		}
		if deleteEndpointData {
			// Results, events, uptimes and triggered alerts are deleted through ON DELETE CASCADE
			if _, err := tx.Exec("DELETE FROM endpoints WHERE endpoint_key = $1", key); err != nil {
				return err
			}
		}
		return nil
	}, apply)
	if err == nil && deleteEndpointData && s.writeThroughCache != nil {
		_ = s.writeThroughCache.DeleteKeysByPattern(key + "*")
	}
	return err
}

// inTransaction runs write in a transaction, then apply, and only commits if both succeed
func (s *Store) inTransaction(write func(tx *sql.Tx) error, apply func() error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err = write(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if apply != nil {
		if err = apply(); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanManagedEndpoint(row rowScanner) (*common.ManagedEndpoint, error) {
	var managedEndpoint common.ManagedEndpoint
	var createdAt, updatedAt int64
	if err := row.Scan(&managedEndpoint.Key, &managedEndpoint.Definition, &managedEndpoint.Version, &createdAt, &updatedAt, &managedEndpoint.UpdatedBy); err != nil {
		return nil, err
	}
	managedEndpoint.CreatedAt, managedEndpoint.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(updatedAt)
	return &managedEndpoint, nil
}

// isUniqueViolation returns whether err is a unique constraint violation, for SQLite, PostgreSQL, MySQL and MariaDB
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == mysqlErrorDuplicateEntry
	}
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed") || strings.Contains(message, "duplicate key value violates unique constraint")
}
