// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package storage holds the storage section of the configuration: the type of the store (memory, sqlite, postgres or
// mysql), its path or DSN, caching and how many results and events are kept per endpoint.
package storage

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

const (
	// DefaultMaximumNumberOfResults and DefaultMaximumNumberOfEvents are how many results and events are kept per
	// endpoint when the storage configuration does not set maximum-number-of-results and maximum-number-of-events.
	DefaultMaximumNumberOfResults = 100
	DefaultMaximumNumberOfEvents  = 50

	// MySQLMaximumKeyLength is the maximum number of characters of an endpoint or suite key with the mysql storage: the
	// longest VARCHAR that fits in an InnoDB index in utf8mb4 (3072 bytes)
	MySQLMaximumKeyLength = 768
)

var (
	// ErrSQLStorageRequiresPath is returned when the type is sqlite, postgres or mysql and path is empty.
	ErrSQLStorageRequiresPath = errors.New("sql storage requires a non-empty path to be defined")
	// ErrMemoryStorageDoesNotSupportPath is returned when the type is memory and path is set.
	ErrMemoryStorageDoesNotSupportPath = errors.New("memory storage does not support persistence, use sqlite if you want persistence on file")

	// ErrMySQLStorageInvalidPath is returned when the path of a mysql storage is not a valid DSN. It never includes the
	// path, which contains the password.
	ErrMySQLStorageInvalidPath = errors.New("mysql storage requires storage.path to be a valid DSN, for example gatus:password@tcp(mariadb:3306)/gatus")
)

// Config is the configuration for storage
type Config struct {
	// Path is the path used by the store to achieve persistence
	// If blank, persistence is disabled.
	// Note that not all Type support persistence
	Path string `yaml:"path"`

	// Type of store
	// If blank, uses the default in-memory store
	Type Type `yaml:"type"`

	// Caching is whether to enable caching.
	// This is used to drastically decrease read latency by pre-emptively caching writes
	// as they happen, also known as the write-through caching strategy.
	// Does not apply if Config.Type is not TypePostgres, TypeSQLite or TypeMySQL.
	Caching bool `yaml:"caching,omitempty"`

	// MaximumNumberOfResults is the number of results each endpoint should be able to provide
	MaximumNumberOfResults int `yaml:"maximum-number-of-results,omitempty"`

	// MaximumNumberOfEvents is the number of events each endpoint should be able to provide
	MaximumNumberOfEvents int `yaml:"maximum-number-of-events,omitempty"`
}

// ValidateAndSetDefaults validates the configuration and sets the default values (if applicable)
func (c *Config) ValidateAndSetDefaults() error {
	if c.Type == "" {
		c.Type = TypeMemory
	}
	if (c.Type == TypePostgres || c.Type == TypeSQLite || c.Type == TypeMySQL) && len(c.Path) == 0 {
		return ErrSQLStorageRequiresPath
	}
	if c.Type == TypeMySQL {
		if _, err := mysql.ParseDSN(c.Path); err != nil {
			// The error of the driver may quote the DSN, so it is not wrapped
			return ErrMySQLStorageInvalidPath
		}
	}
	if c.Type == TypeMemory && len(c.Path) > 0 {
		return ErrMemoryStorageDoesNotSupportPath
	}
	if c.MaximumNumberOfResults <= 0 {
		c.MaximumNumberOfResults = DefaultMaximumNumberOfResults
	}
	if c.MaximumNumberOfEvents <= 0 {
		c.MaximumNumberOfEvents = DefaultMaximumNumberOfEvents
	}
	return nil
}
