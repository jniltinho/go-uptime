// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

// mysqlTableOptions are the options of every table of the MySQL schema: InnoDB for foreign keys and transactions, and a
// binary collation so that unique keys compare like in PostgreSQL ("API" and "api" are different keys)
const mysqlTableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"

// createMySQLSchema creates the tables of the upstream schema in MySQL 8.4+ and MariaDB 10.11+ (fork). It differs from
// the PostgreSQL schema where MySQL requires it:
//   - foreign keys are table constraints, because MySQL 8.4 accepts and ignores REFERENCES in a column definition;
//   - unique keys are VARCHAR(768), the longest key that fits in an InnoDB index in utf8mb4, and there is no
//     UNIQUE(name, group), already enforced by the unique key derived from both;
//   - free texts are MEDIUMTEXT, because TEXT only stores 65535 bytes;
//   - times are DATETIME(6), without the time zone conversion and the 2038 limit of TIMESTAMP;
//   - indexes are declared in CREATE TABLE, because MySQL has no CREATE INDEX IF NOT EXISTS.
//
// The condition column is quoted, which requires the ANSI_QUOTES sql_mode set by newMySQLConfig.
func (s *Store) createMySQLSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suites (
			suite_id    BIGINT       AUTO_INCREMENT PRIMARY KEY,
			suite_key   VARCHAR(768) UNIQUE,
			suite_name  MEDIUMTEXT   NOT NULL,
			suite_group MEDIUMTEXT   NOT NULL
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS suite_results (
			suite_result_id BIGINT      AUTO_INCREMENT PRIMARY KEY,
			suite_id        BIGINT      NOT NULL,
			success         BOOLEAN     NOT NULL,
			errors          MEDIUMTEXT  NOT NULL,
			duration        BIGINT      NOT NULL,
			timestamp       DATETIME(6) NOT NULL,
			KEY suite_results_suite_id_idx (suite_id),
			CONSTRAINT fk_suite_results_suite_id FOREIGN KEY (suite_id) REFERENCES suites (suite_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoints (
			endpoint_id    BIGINT       AUTO_INCREMENT PRIMARY KEY,
			endpoint_key   VARCHAR(768) UNIQUE,
			endpoint_name  MEDIUMTEXT   NOT NULL,
			endpoint_group MEDIUMTEXT   NOT NULL
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoint_events (
			endpoint_event_id BIGINT      AUTO_INCREMENT PRIMARY KEY,
			endpoint_id       BIGINT      NOT NULL,
			event_type        MEDIUMTEXT  NOT NULL,
			event_timestamp   DATETIME(6) NOT NULL,
			KEY endpoint_events_endpoint_id_idx (endpoint_id),
			CONSTRAINT fk_endpoint_events_endpoint_id FOREIGN KEY (endpoint_id) REFERENCES endpoints (endpoint_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoint_results (
			endpoint_result_id     BIGINT      AUTO_INCREMENT PRIMARY KEY,
			endpoint_id            BIGINT      NOT NULL,
			success                BOOLEAN     NOT NULL,
			errors                 MEDIUMTEXT  NOT NULL,
			connected              BOOLEAN     NOT NULL,
			status                 BIGINT      NOT NULL,
			dns_rcode              MEDIUMTEXT  NOT NULL,
			certificate_expiration BIGINT      NOT NULL,
			domain_expiration      BIGINT      NOT NULL DEFAULT 0,
			hostname               MEDIUMTEXT  NOT NULL,
			ip                     MEDIUMTEXT  NOT NULL,
			duration               BIGINT      NOT NULL,
			timestamp              DATETIME(6) NOT NULL,
			suite_result_id        BIGINT,
			KEY idx_endpoint_results_endpoint_id (endpoint_id),
			KEY endpoint_results_suite_result_id_idx (suite_result_id),
			CONSTRAINT fk_endpoint_results_endpoint_id FOREIGN KEY (endpoint_id) REFERENCES endpoints (endpoint_id) ON DELETE CASCADE,
			CONSTRAINT fk_endpoint_results_suite_result_id FOREIGN KEY (suite_result_id) REFERENCES suite_results (suite_result_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoint_result_conditions (
			endpoint_result_condition_id BIGINT     AUTO_INCREMENT PRIMARY KEY,
			endpoint_result_id           BIGINT     NOT NULL,
			"condition"                  MEDIUMTEXT NOT NULL,
			success                      BOOLEAN    NOT NULL,
			KEY idx_endpoint_result_conditions_endpoint_result_id (endpoint_result_id),
			CONSTRAINT fk_endpoint_result_conditions_endpoint_result_id FOREIGN KEY (endpoint_result_id) REFERENCES endpoint_results (endpoint_result_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoint_uptimes (
			endpoint_uptime_id    BIGINT AUTO_INCREMENT PRIMARY KEY,
			endpoint_id           BIGINT NOT NULL,
			hour_unix_timestamp   BIGINT NOT NULL,
			total_executions      BIGINT NOT NULL,
			successful_executions BIGINT NOT NULL,
			total_response_time   BIGINT NOT NULL,
			UNIQUE KEY endpoint_uptimes_endpoint_id_hour_unix_timestamp_key (endpoint_id, hour_unix_timestamp),
			CONSTRAINT fk_endpoint_uptimes_endpoint_id FOREIGN KEY (endpoint_id) REFERENCES endpoints (endpoint_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
		`CREATE TABLE IF NOT EXISTS endpoint_alerts_triggered (
			endpoint_alert_trigger_id    BIGINT      AUTO_INCREMENT PRIMARY KEY,
			endpoint_id                  BIGINT      NOT NULL,
			configuration_checksum       VARCHAR(64) NOT NULL,
			resolve_key                  MEDIUMTEXT  NOT NULL,
			number_of_successes_in_a_row INTEGER     NOT NULL,
			UNIQUE KEY endpoint_alerts_triggered_endpoint_id_checksum_key (endpoint_id, configuration_checksum),
			CONSTRAINT fk_endpoint_alerts_triggered_endpoint_id FOREIGN KEY (endpoint_id) REFERENCES endpoints (endpoint_id) ON DELETE CASCADE
		) ` + mysqlTableOptions,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
