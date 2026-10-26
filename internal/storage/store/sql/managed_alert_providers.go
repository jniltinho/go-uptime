// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const managedAlertProviderColumns = "alert_type, definition, version, created_at, updated_at, updated_by"

// createManagedAlertProvidersSchema creates the table of the alerting providers managed through the administration API,
// one row per alert type. Timestamps are stored as Unix milliseconds, like managed_endpoints.
func (s *Store) createManagedAlertProvidersSchema() error {
	if s.driver == "sqlite" {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_alert_providers (
				managed_alert_provider_id INTEGER PRIMARY KEY,
				alert_type                TEXT    NOT NULL UNIQUE,
				definition                TEXT    NOT NULL,
				version                   INTEGER NOT NULL,
				created_at                INTEGER NOT NULL,
				updated_at                INTEGER NOT NULL,
				updated_by                TEXT    NOT NULL
			)
		`)
		return err
	}
	if s.driver == driverMySQL {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_alert_providers (
				managed_alert_provider_id BIGINT      AUTO_INCREMENT PRIMARY KEY,
				alert_type                VARCHAR(64) NOT NULL UNIQUE,
				definition                MEDIUMTEXT  NOT NULL,
				version                   BIGINT      NOT NULL,
				created_at                BIGINT      NOT NULL,
				updated_at                BIGINT      NOT NULL,
				updated_by                MEDIUMTEXT  NOT NULL
			) ` + mysqlTableOptions)
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS managed_alert_providers (
			managed_alert_provider_id BIGSERIAL PRIMARY KEY,
			alert_type                TEXT      NOT NULL UNIQUE,
			definition                TEXT      NOT NULL,
			version                   BIGINT    NOT NULL,
			created_at                BIGINT    NOT NULL,
			updated_at                BIGINT    NOT NULL,
			updated_by                TEXT      NOT NULL
		)
	`)
	return err
}

// ListManagedAlertProviders returns every managed alerting provider, ordered by type
func (s *Store) ListManagedAlertProviders() ([]*common.ManagedAlertProvider, error) {
	rows, err := s.db.Query("SELECT " + managedAlertProviderColumns + " FROM managed_alert_providers ORDER BY alert_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var managedAlertProviders []*common.ManagedAlertProvider
	for rows.Next() {
		managedAlertProvider, err := scanManagedAlertProvider(rows)
		if err != nil {
			return nil, err
		}
		managedAlertProviders = append(managedAlertProviders, managedAlertProvider)
	}
	return managedAlertProviders, rows.Err()
}

// GetManagedAlertProvider returns the managed alerting provider with the given type, or common.ErrManagedAlertProviderNotFound
func (s *Store) GetManagedAlertProvider(alertType string) (*common.ManagedAlertProvider, error) {
	managedAlertProvider, err := scanManagedAlertProvider(s.db.QueryRow("SELECT "+managedAlertProviderColumns+" FROM managed_alert_providers WHERE alert_type = $1", alertType))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrManagedAlertProviderNotFound
	}
	return managedAlertProvider, err
}

// CreateManagedAlertProvider persists a new managed alerting provider with version 1
func (s *Store) CreateManagedAlertProvider(managedAlertProvider *common.ManagedAlertProvider, apply func() error) error {
	now := time.Now().UnixMilli()
	err := s.inTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO managed_alert_providers ("+managedAlertProviderColumns+") VALUES ($1, $2, $3, $4, $5, $6)",
			managedAlertProvider.Type, managedAlertProvider.Definition, 1, now, now, managedAlertProvider.UpdatedBy,
		)
		if isUniqueViolation(err) {
			return common.ErrManagedAlertProviderAlreadyExists
		}
		return err
	}, apply)
	if err != nil {
		return err
	}
	managedAlertProvider.Version = 1
	managedAlertProvider.CreatedAt, managedAlertProvider.UpdatedAt = time.UnixMilli(now), time.UnixMilli(now)
	return nil
}

// UpdateManagedAlertProvider replaces the definition of the managed alerting provider if its current version is expectedVersion
func (s *Store) UpdateManagedAlertProvider(managedAlertProvider *common.ManagedAlertProvider, expectedVersion int64, apply func() error) error {
	now := time.Now().UnixMilli()
	var createdAt int64
	err := s.inTransaction(func(tx *sql.Tx) error {
		var version int64
		err := tx.QueryRow("SELECT version, created_at FROM managed_alert_providers WHERE alert_type = $1", managedAlertProvider.Type).Scan(&version, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return common.ErrManagedAlertProviderNotFound
		}
		if err != nil {
			return err
		}
		if version != expectedVersion {
			return common.ErrManagedAlertProviderVersionMismatch
		}
		result, err := tx.Exec(
			"UPDATE managed_alert_providers SET definition = $1, version = $2, updated_at = $3, updated_by = $4 WHERE alert_type = $5 AND version = $6",
			managedAlertProvider.Definition, expectedVersion+1, now, managedAlertProvider.UpdatedBy, managedAlertProvider.Type, expectedVersion,
		)
		if err != nil {
			return err
		}
		// Another transaction may have changed the row between the SELECT and the UPDATE
		if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
			return common.ErrManagedAlertProviderVersionMismatch
		}
		return nil
	}, apply)
	if err != nil {
		return err
	}
	managedAlertProvider.Version = expectedVersion + 1
	managedAlertProvider.CreatedAt, managedAlertProvider.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(now)
	return nil
}

// DeleteManagedAlertProvider deletes the managed alerting provider if its current version is expectedVersion
func (s *Store) DeleteManagedAlertProvider(alertType string, expectedVersion int64, apply func() error) error {
	return s.inTransaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM managed_alert_providers WHERE alert_type = $1 AND version = $2", alertType, expectedVersion)
		if err != nil {
			return err
		}
		if rowsAffected, err := result.RowsAffected(); err != nil {
			return err
		} else if rowsAffected == 0 {
			var version int64
			err := tx.QueryRow("SELECT version FROM managed_alert_providers WHERE alert_type = $1", alertType).Scan(&version)
			if errors.Is(err, sql.ErrNoRows) {
				return common.ErrManagedAlertProviderNotFound
			}
			if err != nil {
				return err
			}
			return common.ErrManagedAlertProviderVersionMismatch
		}
		return nil
	}, apply)
}

func scanManagedAlertProvider(row rowScanner) (*common.ManagedAlertProvider, error) {
	var managedAlertProvider common.ManagedAlertProvider
	var createdAt, updatedAt int64
	if err := row.Scan(&managedAlertProvider.Type, &managedAlertProvider.Definition, &managedAlertProvider.Version, &createdAt, &updatedAt, &managedAlertProvider.UpdatedBy); err != nil {
		return nil, err
	}
	managedAlertProvider.CreatedAt, managedAlertProvider.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(updatedAt)
	return &managedAlertProvider, nil
}
