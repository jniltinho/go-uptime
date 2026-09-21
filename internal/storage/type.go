// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package storage

// Type of the store.
type Type string

const (
	TypeMemory   Type = "memory"   // In-memory store
	TypeSQLite   Type = "sqlite"   // SQLite store
	TypePostgres Type = "postgres" // Postgres store
	TypeMySQL    Type = "mysql"    // MySQL 8.4+ and MariaDB 10.11+ store (fork)
)
