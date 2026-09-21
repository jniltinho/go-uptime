// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"slices"
	"testing"
)

func TestNewStore_MySQLSchema(t *testing.T) {
	forEachMySQLTestServer(t, func(t *testing.T, dsn string) {
		store, err := NewStore(driverMySQL, dsn, false, 100, 50)
		if err != nil {
			t.Fatalf("failed to create the store: %v", err)
		}
		defer store.Close()
		var tables []string
		// Sorted in Go: the collation of information_schema sorts underscores differently in MySQL and MariaDB
		rows, err := store.db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND engine = 'InnoDB'`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var table string
			_ = rows.Scan(&table)
			tables = append(tables, table)
		}
		_ = rows.Close()
		slices.Sort(tables)
		expectedTables := []string{"endpoint_alerts_triggered", "endpoint_events", "endpoint_response_time_buckets", "endpoint_result_conditions", "endpoint_result_messages", "endpoint_results", "endpoint_uptimes", "endpoints", "login_sessions", "managed_endpoints", "managed_status_pages", "push_keys", "suite_results", "suites"}
		if !slices.Equal(tables, expectedTables) {
			t.Errorf("expected the InnoDB tables %v, got %v", expectedTables, tables)
		}
		var cascadingForeignKeys int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() AND delete_rule = 'CASCADE'`).Scan(&cascadingForeignKeys); err != nil || cascadingForeignKeys != 9 {
			t.Errorf("expected 9 foreign keys with ON DELETE CASCADE, got %d (err=%v)", cascadingForeignKeys, err)
		}
		// Removing an endpoint removes its rows in every table that references it
		if _, err := store.db.Exec(`INSERT INTO endpoints (endpoint_key, endpoint_name, endpoint_group) VALUES ($1, $2, $3)`, "core_api", "api", "core"); err != nil {
			t.Fatal(err)
		}
		statements := []string{
			`INSERT INTO endpoint_results (endpoint_id, success, errors, connected, status, dns_rcode, certificate_expiration, domain_expiration, hostname, ip, duration, timestamp) SELECT endpoint_id, TRUE, '', TRUE, 200, '', 0, 0, '', '', 1, NOW(6) FROM endpoints WHERE endpoint_key = 'core_api'`,
			`INSERT INTO endpoint_result_conditions (endpoint_result_id, "condition", success) SELECT MAX(endpoint_result_id), '[STATUS] == 200', TRUE FROM endpoint_results`,
			`INSERT INTO endpoint_events (endpoint_id, event_type, event_timestamp) SELECT endpoint_id, 'START', NOW(6) FROM endpoints WHERE endpoint_key = 'core_api'`,
			`INSERT INTO endpoint_uptimes (endpoint_id, hour_unix_timestamp, total_executions, successful_executions, total_response_time) SELECT endpoint_id, 0, 1, 1, 1 FROM endpoints WHERE endpoint_key = 'core_api'`,
			`INSERT INTO endpoint_alerts_triggered (endpoint_id, configuration_checksum, resolve_key, number_of_successes_in_a_row) SELECT endpoint_id, REPEAT('a', 64), '', 0 FROM endpoints WHERE endpoint_key = 'core_api'`,
			`DELETE FROM endpoints WHERE endpoint_key = 'core_api'`,
		}
		for _, statement := range statements {
			if _, err := store.db.Exec(statement); err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
		}
		for _, table := range []string{"endpoint_results", "endpoint_result_conditions", "endpoint_events", "endpoint_uptimes", "endpoint_alerts_triggered"} {
			var count int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
				t.Errorf("expected the rows of %s to be removed in cascade, got %d (err=%v)", table, count, err)
			}
		}
		// Keys that differ only by case are different, like in PostgreSQL
		for _, key := range []string{"core_API", "core_api"} {
			if _, err := store.db.Exec(`INSERT INTO endpoints (endpoint_key, endpoint_name, endpoint_group) VALUES ($1, $2, $3)`, key, "api", "core"); err != nil {
				t.Errorf("expected %s to be a different key, got %v", key, err)
			}
		}
		// Starting again on the existing schema neither fails nor removes data
		store.Close()
		store, err = NewStore(driverMySQL, dsn, false, 100, 50)
		if err != nil {
			t.Fatalf("failed to create the store again on the existing schema: %v", err)
		}
		var endpoints int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpoints); err != nil || endpoints != 2 {
			t.Errorf("expected the 2 endpoints to be kept, got %d (err=%v)", endpoints, err)
		}
	})
}
