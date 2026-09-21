// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TwiN/logr"
	"github.com/go-sql-driver/mysql"
)

const (
	// driverMySQL is the value of storage.type for MySQL 8.4+ and MariaDB 10.11+
	driverMySQL = "mysql"

	// mysqlSQLMode is the sql_mode of every session: ANSI_QUOTES lets the queries quote identifiers such as "condition"
	// like in SQLite and PostgreSQL, and NO_ZERO_DATE is left out so that the zero time.Time is stored as in the other
	// databases
	mysqlSQLMode = "ANSI_QUOTES,ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION"

	// mysqlLockWaitTimeout is the innodb_lock_wait_timeout of every session, in seconds, so that a lock wait becomes an
	// error that the store handles instead of blocking the watchdog for the default 50 seconds
	mysqlLockWaitTimeout = 10

	mysqlMaximumOpenConnections    = 25
	mysqlConnectionMaximumLifetime = 3 * time.Minute
	mysqlConnectionMaximumIdleTime = time.Minute

	// mysqlMinimumPageSize is the smallest innodb_page_size with which the unique VARCHAR(768) keys of the schema fit in
	// an index (3072 bytes)
	mysqlMinimumPageSize = 16384

	mysqlMinimumMajorVersion, mysqlMinimumMinorVersion     = 8, 4
	mariaDBMinimumMajorVersion, mariaDBMinimumMinorVersion = 10, 11
)

var (
	// ErrInvalidMySQLPath is returned when the path of a mysql store is not a valid DSN. It never includes the path,
	// which contains the password.
	ErrInvalidMySQLPath = errors.New("path is not a valid MySQL DSN")

	errMySQLPageSizeTooSmall = errors.New("the InnoDB page size of the MySQL server is too small: at least 16K is required")
)

// newMySQLConfig returns the configuration of the MySQL driver for the DSN, overriding what the store relies on
// whatever the DSN and the server configuration say: times in UTC, utf8mb4 with a binary collation, number of matched
// rows in RowsAffected and known session variables. TLS, timeouts and the other parameters of the DSN are kept.
func newMySQLConfig(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		// The error of the driver may quote the DSN, so it is not wrapped
		return nil, ErrInvalidMySQLPath
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.ClientFoundRows = true
	cfg.InterpolateParams = true
	cfg.CheckConnLiveness = true
	if err := cfg.Apply(mysql.Charset("utf8mb4", "utf8mb4_bin")); err != nil {
		return nil, err
	}
	params := make(map[string]string, len(cfg.Params)+3)
	for name, value := range cfg.Params {
		switch strings.ToLower(name) {
		case "time_zone", "sql_mode", "innodb_lock_wait_timeout":
			continue
		}
		params[name] = value
	}
	// Session variables are set with SET <name>=<value>, so string values must be quoted
	params["time_zone"] = "'+00:00'"
	params["sql_mode"] = "'" + mysqlSQLMode + "'"
	params["innodb_lock_wait_timeout"] = strconv.Itoa(mysqlLockWaitTimeout)
	cfg.Params = params
	return cfg, nil
}

// openMySQL opens a pool of connections to MySQL or MariaDB whose queries are translated (see mysqlConnector)
func openMySQL(dsn string) (*sql.DB, *mysql.Config, error) {
	cfg, err := newMySQLConfig(dsn)
	if err != nil {
		return nil, nil, err
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, nil, ErrInvalidMySQLPath
	}
	db := sql.OpenDB(&mysqlConnector{connector: connector})
	db.SetMaxOpenConns(mysqlMaximumOpenConnections)
	db.SetMaxIdleConns(mysqlMaximumOpenConnections)
	db.SetConnMaxLifetime(mysqlConnectionMaximumLifetime)
	db.SetConnMaxIdleTime(mysqlConnectionMaximumIdleTime)
	return db, cfg, nil
}

// openMySQLStore opens the pool of connections of a mysql store and checks the server, closing the pool if the check
// fails
func openMySQLStore(dsn string) (*sql.DB, error) {
	db, cfg, err := openMySQL(dsn)
	if err != nil {
		return nil, err
	}
	if err := checkMySQLServer(db, cfg); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// checkMySQLServer logs the version of the server, warns when it is older than the minimum supported versions and fails
// when its InnoDB page size is too small for the indexes of the schema
func checkMySQLServer(db *sql.DB, cfg *mysql.Config) error {
	var version string
	var pageSize int
	if err := db.QueryRow("SELECT VERSION(), @@innodb_page_size").Scan(&version, &pageSize); err != nil {
		return err
	}
	logr.Infof("[sql.NewStore] Connected to MySQL server version=%s address=%s database=%s", version, cfg.Addr, cfg.DBName)
	if supported, known := isSupportedMySQLVersion(version); !known {
		logr.Warnf("[sql.NewStore] Could not determine whether MySQL server version=%s is supported: the minimum supported versions are MySQL %d.%d and MariaDB %d.%d", version, mysqlMinimumMajorVersion, mysqlMinimumMinorVersion, mariaDBMinimumMajorVersion, mariaDBMinimumMinorVersion)
	} else if !supported {
		logr.Warnf("[sql.NewStore] MySQL server version=%s is older than the minimum supported versions (MySQL %d.%d and MariaDB %d.%d)", version, mysqlMinimumMajorVersion, mysqlMinimumMinorVersion, mariaDBMinimumMajorVersion, mariaDBMinimumMinorVersion)
	}
	if pageSize < mysqlMinimumPageSize {
		return fmt.Errorf("%w (innodb_page_size=%d)", errMySQLPageSizeTooSmall, pageSize)
	}
	return nil
}

// isSupportedMySQLVersion returns whether the version returned by SELECT VERSION() (for example 8.4.11 or
// 10.11.19-MariaDB-ubu2204) is at least MySQL 8.4 or MariaDB 10.11, and whether the version could be parsed
func isSupportedMySQLVersion(version string) (supported, known bool) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false, false
	}
	minorDigits := parts[1]
	if end := strings.IndexFunc(minorDigits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
		minorDigits = minorDigits[:end]
	}
	minor, err := strconv.Atoi(minorDigits)
	if err != nil {
		return false, false
	}
	minimumMajor, minimumMinor := mysqlMinimumMajorVersion, mysqlMinimumMinorVersion
	if strings.Contains(strings.ToLower(version), "mariadb") {
		minimumMajor, minimumMinor = mariaDBMinimumMajorVersion, mariaDBMinimumMinorVersion
	}
	return major > minimumMajor || (major == minimumMajor && minor >= minimumMinor), true
}
