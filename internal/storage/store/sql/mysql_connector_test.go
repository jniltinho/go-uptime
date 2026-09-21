// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlTestServers returns the DSNs of the MySQL and MariaDB test servers, from GO_UPTIME_TEST_MYSQL_URL and
// GO_UPTIME_TEST_MARIADB_URL. The DSN user must be allowed to create databases.
func mysqlTestServers() map[string]string {
	servers := make(map[string]string)
	if dsn := os.Getenv("GO_UPTIME_TEST_MYSQL_URL"); len(dsn) > 0 {
		servers["mysql"] = dsn
	}
	if dsn := os.Getenv("GO_UPTIME_TEST_MARIADB_URL"); len(dsn) > 0 {
		servers["mariadb"] = dsn
	}
	return servers
}

// newMySQLTestDatabase creates a database of its own for the test on the server of dsn, removed when the test ends,
// so that packages tested in parallel never share data, and returns its DSN
func newMySQLTestDatabase(t testing.TB, dsn string) string {
	t.Helper()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("invalid test DSN: %v", err)
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	database := "go_uptime_test_" + hex.EncodeToString(suffix)
	cfg.DBName = ""
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if _, err := admin.Exec("CREATE DATABASE " + database + " CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		t.Fatalf("failed to create the test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + database); err != nil {
			t.Errorf("failed to drop the test database %s: %v", database, err)
		}
	})
	cfg.DBName = database
	return cfg.FormatDSN()
}

// forEachMySQLTestServer runs test with a database of its own on each available MySQL and MariaDB test server, and
// skips when none is configured
func forEachMySQLTestServer(t *testing.T, test func(t *testing.T, dsn string)) {
	t.Helper()
	servers := mysqlTestServers()
	if len(servers) == 0 {
		t.Skip("GO_UPTIME_TEST_MYSQL_URL and GO_UPTIME_TEST_MARIADB_URL are not set")
	}
	for name, dsn := range servers {
		t.Run(name, func(t *testing.T) {
			test(t, newMySQLTestDatabase(t, dsn))
		})
	}
}

func openMySQLForTest(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, _, err := openMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`DROP TABLE IF EXISTS items`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE items (item_id BIGINT AUTO_INCREMENT PRIMARY KEY, amount INT NOT NULL, label VARCHAR(20) NOT NULL UNIQUE, "condition" MEDIUMTEXT, created_at DATETIME(6) NOT NULL) ENGINE=InnoDB`); err != nil {
		t.Fatalf("failed to create the test table: %v", err)
	}
	return db
}

func TestMySQLConnector_Placeholders(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		for _, interpolate := range []bool{true, false} {
			t.Run("interpolate="+map[bool]string{true: "true", false: "false"}[interpolate], func(t *testing.T) {
				db := openMySQLForTest(t, dsn)
				if !interpolate {
					// Without interpolation, the driver answers every statement with arguments with driver.ErrSkip
					db = reopenWithoutInterpolation(t, dsn)
				}
				now := time.Date(2026, 9, 14, 12, 0, 0, 123456000, time.UTC)
				for i, label := range []string{"a", "b", "c"} {
					if _, err := db.Exec(`INSERT INTO items (amount, label, "condition", created_at) VALUES ($1, $2, $3, $4)`, (i+1)*10, label, "[STATUS] == 200 costs $1", now); err != nil {
						t.Fatalf("insert failed: %v", err)
					}
				}
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE amount >= $1 AND amount >= $1 AND label <> $2`, 20, "z").Scan(&count); err != nil || count != 2 {
					t.Errorf("expected 2 rows with the repeated placeholder, got %d (err=%v)", count, err)
				}
				var condition string
				var createdAt time.Time
				if err := db.QueryRow(`SELECT "condition", created_at FROM items WHERE label IN ($2, $3) AND amount = $1`, 10, "a", "x").Scan(&condition, &createdAt); err != nil {
					t.Fatalf("query with reordered placeholders failed: %v", err)
				}
				if condition != "[STATUS] == 200 costs $1" || !createdAt.Equal(now) || createdAt.Location() != time.UTC {
					t.Errorf("unexpected row: condition=%q created_at=%v", condition, createdAt)
				}
				stmt, err := db.Prepare(`SELECT label FROM items WHERE amount = $1 OR amount = $1 + 0`)
				if err != nil {
					t.Fatalf("prepare failed: %v", err)
				}
				defer stmt.Close()
				for amount, expected := range map[int]string{10: "a", 30: "c"} {
					var label string
					if err := stmt.QueryRow(amount).Scan(&label); err != nil || label != expected {
						t.Errorf("prepared statement with %d: expected %s, got %s (err=%v)", amount, expected, label, err)
					}
				}
				if _, err := db.Exec(`SELECT $1`); !errors.Is(err, errArgumentCountInvalid) {
					t.Errorf("expected errArgumentCountInvalid for a missing argument, got %v", err)
				}
			})
		}
	})
}

func TestMySQLConnector_InsertReturning(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		openMySQLForTest(t, dsn)
		for dbIndex, db := range []*sql.DB{openMySQLForTestWithoutTable(t, dsn), reopenWithoutInterpolation(t, dsn)} {
			var ids []int64
			for i := 0; i < 2; i++ {
				var id int64
				label := "returning-" + strconv.Itoa(dbIndex) + "-" + strconv.Itoa(i)
				if err := db.QueryRow(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3) RETURNING item_id`, i, label, time.Now()).Scan(&id); err != nil {
					t.Fatalf("insert with RETURNING failed: %v", err)
				}
				ids = append(ids, id)
			}
			if ids[0] <= 0 || ids[1] != ids[0]+1 {
				t.Errorf("expected consecutive generated ids, got %v", ids)
			}
			var label string
			if err := db.QueryRow(`SELECT label FROM items WHERE item_id = $1`, ids[1]).Scan(&label); err != nil || label != "returning-"+strconv.Itoa(dbIndex)+"-1" {
				t.Errorf("expected the returned id to identify the inserted row, got %q (err=%v)", label, err)
			}
		}
		// A failed INSERT ... RETURNING aborts the transaction like any other statement
		db := openMySQLForTestWithoutTable(t, dsn)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := tx.QueryRow(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3) RETURNING item_id`, 1, strings.Repeat("x", 30), time.Now()).Scan(&id); err == nil {
			t.Fatal("expected the insert of a label longer than VARCHAR(20) to fail in strict mode")
		}
		if err := tx.Commit(); !errors.Is(err, errTransactionAborted) {
			t.Errorf("expected Commit to fail after the failed insert, got %v", err)
		}
	})
}

func openMySQLForTestWithoutTable(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, _, err := openMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func reopenWithoutInterpolation(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	cfg, err := newMySQLConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.InterpolateParams = false
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(&mysqlConnector{connector: connector})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMySQLConnector_AbortedTransaction(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		db := openMySQLForTest(t, dsn)
		now := time.Now()
		if _, err := db.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 1, "existing", now); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 2, "new", now); err != nil {
			t.Fatalf("first insert failed: %v", err)
		}
		_, err = tx.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 3, "existing", now)
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
			t.Fatalf("expected a duplicate key error, got %v", err)
		}
		if _, err := tx.Exec(`UPDATE items SET amount = $1 WHERE label = $2`, 4, "existing"); !errors.Is(err, errTransactionAborted) {
			t.Errorf("expected the following statement to be refused, got %v", err)
		}
		err = tx.Commit()
		if !errors.Is(err, errTransactionAborted) || !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
			t.Errorf("expected Commit to return the aborting error, got %v", err)
		}
		var labels []string
		rows, err := db.Query(`SELECT label FROM items ORDER BY label`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var label string
			_ = rows.Scan(&label)
			labels = append(labels, label)
		}
		_ = rows.Close()
		if !slices.Equal(labels, []string{"existing"}) {
			t.Errorf("expected the aborted transaction to be rolled back, got %v", labels)
		}
		var amount int
		if err := db.QueryRow(`SELECT amount FROM items WHERE label = $1`, "existing").Scan(&amount); err != nil || amount != 1 {
			t.Errorf("expected the connection to be usable again with amount=1, got %d (err=%v)", amount, err)
		}
	})
}

func TestMySQLConnector_FoundRows(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		db := openMySQLForTest(t, dsn)
		if _, err := db.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 7, "unchanged", time.Now()); err != nil {
			t.Fatal(err)
		}
		// Without clientFoundRows, MySQL counts only the rows whose values changed, and an optimistic update with the
		// same values would be taken as a version conflict
		result, err := db.Exec(`UPDATE items SET amount = $1 WHERE label = $2`, 7, "unchanged")
		if err != nil {
			t.Fatal(err)
		}
		if rowsAffected, err := result.RowsAffected(); err != nil || rowsAffected != 1 {
			t.Errorf("expected the matched row to be counted, got %d (err=%v)", rowsAffected, err)
		}
	})
}

func TestMySQLConnector_SessionAndTimes(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Loc, _ = time.LoadLocation("America/Sao_Paulo")
		cfg.ParseTime = false
		cfg.Params = map[string]string{"time_zone": "'-03:00'", "sql_mode": "''"}
		db := openMySQLForTest(t, cfg.FormatDSN())
		var timeZone, sqlMode string
		var lockWaitTimeout int
		if err := db.QueryRow(`SELECT @@session.time_zone, @@session.sql_mode, @@session.innodb_lock_wait_timeout`).Scan(&timeZone, &sqlMode, &lockWaitTimeout); err != nil {
			t.Fatal(err)
		}
		modes := strings.Split(sqlMode, ",")
		slices.Sort(modes)
		expectedModes := strings.Split(mysqlSQLMode, ",")
		slices.Sort(expectedModes)
		if timeZone != "+00:00" || !slices.Equal(modes, expectedModes) || lockWaitTimeout != mysqlLockWaitTimeout {
			t.Errorf("expected the fixed session variables, got time_zone=%s sql_mode=%s innodb_lock_wait_timeout=%d", timeZone, sqlMode, lockWaitTimeout)
		}
		// transaction_isolation only exists in MySQL and MariaDB 11.1+, tx_isolation in MariaDB
		var isolation string
		if err := db.QueryRow(`SELECT @@session.transaction_isolation`).Scan(&isolation); err != nil {
			if err := db.QueryRow(`SELECT @@session.tx_isolation`).Scan(&isolation); err != nil {
				t.Fatalf("failed to read the isolation level: %v", err)
			}
		}
		if isolation != "READ-COMMITTED" {
			t.Errorf("expected the READ-COMMITTED isolation level, got %s", isolation)
		}
		noon := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
		for label, value := range map[string]time.Time{"noon": noon, "noon-local": noon.In(cfg.Loc), "zero": {}} {
			if _, err := db.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 1, label, value); err != nil {
				t.Fatalf("%s: insert failed: %v", label, err)
			}
			var createdAt time.Time
			if err := db.QueryRow(`SELECT created_at FROM items WHERE label = $1`, label).Scan(&createdAt); err != nil {
				t.Fatalf("%s: query failed: %v", label, err)
			}
			if !createdAt.Equal(value) {
				t.Errorf("%s: expected %v, got %v", label, value, createdAt)
			}
		}
	})
}

func TestMySQLConnector_KilledConnection(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		db := openMySQLForTest(t, dsn)
		db.SetMaxOpenConns(1)
		var connectionID int64
		if err := db.QueryRow(`SELECT CONNECTION_ID()`).Scan(&connectionID); err != nil {
			t.Fatal(err)
		}
		admin, _, err := openMySQL(dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer admin.Close()
		if _, err := admin.Exec("KILL " + strconv.FormatInt(connectionID, 10)); err != nil {
			t.Fatalf("failed to kill the connection: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
		if _, err := db.Exec(`INSERT INTO items (amount, label, created_at) VALUES ($1, $2, $3)`, 1, "after-kill", time.Now()); err != nil {
			t.Errorf("expected the store to use a new connection after the server killed the previous one, got %v", err)
		}
	})
}
