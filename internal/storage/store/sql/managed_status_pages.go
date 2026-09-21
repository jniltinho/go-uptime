package sql

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const managedStatusPageColumns = "slug, definition, version, created_at, updated_at, updated_by"

// createManagedStatusPagesSchema creates the table of the public status pages managed through the administration API.
// Timestamps are stored as Unix milliseconds, like managed_endpoints.
func (s *Store) createManagedStatusPagesSchema() error {
	if s.driver == "sqlite" {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_status_pages (
				managed_status_page_id INTEGER PRIMARY KEY,
				slug                   TEXT    NOT NULL UNIQUE,
				definition             TEXT    NOT NULL,
				version                INTEGER NOT NULL,
				created_at             INTEGER NOT NULL,
				updated_at             INTEGER NOT NULL,
				updated_by             TEXT    NOT NULL
			)
		`)
		return err
	}
	if s.driver == driverMySQL {
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS managed_status_pages (
				managed_status_page_id BIGINT      AUTO_INCREMENT PRIMARY KEY,
				slug                   VARCHAR(64) NOT NULL UNIQUE,
				definition             MEDIUMTEXT  NOT NULL,
				version                BIGINT      NOT NULL,
				created_at             BIGINT      NOT NULL,
				updated_at             BIGINT      NOT NULL,
				updated_by             MEDIUMTEXT  NOT NULL
			) ` + mysqlTableOptions)
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS managed_status_pages (
			managed_status_page_id BIGSERIAL PRIMARY KEY,
			slug                   TEXT      NOT NULL UNIQUE,
			definition             TEXT      NOT NULL,
			version                BIGINT    NOT NULL,
			created_at             BIGINT    NOT NULL,
			updated_at             BIGINT    NOT NULL,
			updated_by             TEXT      NOT NULL
		)
	`)
	return err
}

// ListManagedStatusPages returns every managed status page, ordered by slug
func (s *Store) ListManagedStatusPages() ([]*common.ManagedStatusPage, error) {
	rows, err := s.db.Query("SELECT " + managedStatusPageColumns + " FROM managed_status_pages ORDER BY slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var managedStatusPages []*common.ManagedStatusPage
	for rows.Next() {
		managedStatusPage, err := scanManagedStatusPage(rows)
		if err != nil {
			return nil, err
		}
		managedStatusPages = append(managedStatusPages, managedStatusPage)
	}
	return managedStatusPages, rows.Err()
}

// GetManagedStatusPage returns the managed status page with the given slug, or common.ErrManagedStatusPageNotFound
func (s *Store) GetManagedStatusPage(slug string) (*common.ManagedStatusPage, error) {
	managedStatusPage, err := scanManagedStatusPage(s.db.QueryRow("SELECT "+managedStatusPageColumns+" FROM managed_status_pages WHERE slug = $1", slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrManagedStatusPageNotFound
	}
	return managedStatusPage, err
}

// CreateManagedStatusPage persists a new managed status page with version 1
func (s *Store) CreateManagedStatusPage(managedStatusPage *common.ManagedStatusPage, apply func() error) error {
	now := time.Now().UnixMilli()
	err := s.inTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO managed_status_pages ("+managedStatusPageColumns+") VALUES ($1, $2, $3, $4, $5, $6)",
			managedStatusPage.Slug, managedStatusPage.Definition, 1, now, now, managedStatusPage.UpdatedBy,
		)
		if isUniqueViolation(err) {
			return common.ErrManagedStatusPageAlreadyExists
		}
		return err
	}, apply)
	if err != nil {
		return err
	}
	managedStatusPage.Version = 1
	managedStatusPage.CreatedAt, managedStatusPage.UpdatedAt = time.UnixMilli(now), time.UnixMilli(now)
	return nil
}

// UpdateManagedStatusPage replaces the definition of the managed status page if its current version is expectedVersion
func (s *Store) UpdateManagedStatusPage(managedStatusPage *common.ManagedStatusPage, expectedVersion int64, apply func() error) error {
	now := time.Now().UnixMilli()
	var createdAt int64
	err := s.inTransaction(func(tx *sql.Tx) error {
		var version int64
		err := tx.QueryRow("SELECT version, created_at FROM managed_status_pages WHERE slug = $1", managedStatusPage.Slug).Scan(&version, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return common.ErrManagedStatusPageNotFound
		}
		if err != nil {
			return err
		}
		if version != expectedVersion {
			return common.ErrManagedStatusPageVersionMismatch
		}
		result, err := tx.Exec(
			"UPDATE managed_status_pages SET definition = $1, version = $2, updated_at = $3, updated_by = $4 WHERE slug = $5 AND version = $6",
			managedStatusPage.Definition, expectedVersion+1, now, managedStatusPage.UpdatedBy, managedStatusPage.Slug, expectedVersion,
		)
		if err != nil {
			return err
		}
		// Another transaction may have changed the row between the SELECT and the UPDATE
		if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
			return common.ErrManagedStatusPageVersionMismatch
		}
		return nil
	}, apply)
	if err != nil {
		return err
	}
	managedStatusPage.Version = expectedVersion + 1
	managedStatusPage.CreatedAt, managedStatusPage.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(now)
	return nil
}

// DeleteManagedStatusPage deletes the managed status page if its current version is expectedVersion
func (s *Store) DeleteManagedStatusPage(slug string, expectedVersion int64, apply func() error) error {
	return s.inTransaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM managed_status_pages WHERE slug = $1 AND version = $2", slug, expectedVersion)
		if err != nil {
			return err
		}
		if rowsAffected, err := result.RowsAffected(); err != nil {
			return err
		} else if rowsAffected == 0 {
			var version int64
			err := tx.QueryRow("SELECT version FROM managed_status_pages WHERE slug = $1", slug).Scan(&version)
			if errors.Is(err, sql.ErrNoRows) {
				return common.ErrManagedStatusPageNotFound
			}
			if err != nil {
				return err
			}
			return common.ErrManagedStatusPageVersionMismatch
		}
		return nil
	}, apply)
}

func scanManagedStatusPage(row rowScanner) (*common.ManagedStatusPage, error) {
	var managedStatusPage common.ManagedStatusPage
	var createdAt, updatedAt int64
	if err := row.Scan(&managedStatusPage.Slug, &managedStatusPage.Definition, &managedStatusPage.Version, &createdAt, &updatedAt, &managedStatusPage.UpdatedBy); err != nil {
		return nil, err
	}
	managedStatusPage.CreatedAt, managedStatusPage.UpdatedAt = time.UnixMilli(createdAt), time.UnixMilli(updatedAt)
	return &managedStatusPage, nil
}
