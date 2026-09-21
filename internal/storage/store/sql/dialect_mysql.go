// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/TwiN/logr"
	"github.com/go-sql-driver/mysql"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
)

// Numbers of the MySQL errors handled by the store
const (
	mysqlErrorDuplicateEntry  = 1062
	mysqlErrorDuplicateColumn = 1060
	mysqlErrorLockWaitTimeout = 1205
	mysqlErrorDeadlock        = 1213
)

const (
	// mysqlMaximumAttempts is the number of attempts of an insertion of results that fails with a deadlock or a lock wait
	// timeout
	mysqlMaximumAttempts = 3

	// mysqlRetryDelay is the base of the delay between two attempts, multiplied by the number of attempts and increased by
	// a random delay of up to mysqlRetryDelay, so that the transactions that deadlocked do not retry at the same time
	mysqlRetryDelay = 20 * time.Millisecond
)

// SQL of the upstream queries that MySQL and MariaDB do not support as written for PostgreSQL and SQLite (fork). The
// placeholders are written as $N, like the rest of the store, and translated by mysqlConnector.
const (
	// mysqlUpsertTriggeredEndpointAlertQuery replaces the resolve key and the number of successes in a row of an existing
	// triggered alert, like the ON CONFLICT clause of UpsertTriggeredEndpointAlert
	mysqlUpsertTriggeredEndpointAlertQuery = `
		INSERT INTO endpoint_alerts_triggered (endpoint_id, configuration_checksum, resolve_key, number_of_successes_in_a_row)
		VALUES ($1, $2, $3, $4)
		ON DUPLICATE KEY UPDATE
			resolve_key = VALUES(resolve_key),
			number_of_successes_in_a_row = VALUES(number_of_successes_in_a_row)
	`

	// mysqlUpsertHourlyUptimeQuery adds the execution to the uptime entry of the hour, like the ON CONFLICT clause of
	// updateEndpointUptime
	mysqlUpsertHourlyUptimeQuery = `
		INSERT INTO endpoint_uptimes (endpoint_id, hour_unix_timestamp, total_executions, successful_executions, total_response_time)
		VALUES ($1, $2, $3, $4, $5)
		ON DUPLICATE KEY UPDATE
			total_executions = total_executions + VALUES(total_executions),
			successful_executions = successful_executions + VALUES(successful_executions),
			total_response_time = total_response_time + VALUES(total_response_time)
	`

	// mysqlUpsertDailyUptimeQuery replaces the daily uptime entry built from the merged hourly entries, like the ON
	// CONFLICT clause of mergeHourlyUptimeEntriesOlderThanMergeThresholdIntoDailyUptimeEntries
	mysqlUpsertDailyUptimeQuery = `
		INSERT INTO endpoint_uptimes (endpoint_id, hour_unix_timestamp, total_executions, successful_executions, total_response_time)
		VALUES ($1, $2, $3, $4, $5)
		ON DUPLICATE KEY UPDATE
			total_executions = VALUES(total_executions),
			successful_executions = VALUES(successful_executions),
			total_response_time = VALUES(total_response_time)
	`

	// The deletions below keep the $2 most recent rows by id, like the upstream NOT IN (SELECT ... LIMIT $2), which MySQL
	// rejects (errors 1235 and 1093): the row at offset $2 is the most recent one to delete, and the derived table with a
	// LIMIT is materialized, so it may read the table the rows are deleted from. Without such a row, the comparison with
	// NULL deletes nothing.

	mysqlDeleteOldEndpointEventsQuery = `
		DELETE FROM endpoint_events
		WHERE endpoint_id = $1 AND endpoint_event_id <= (
			SELECT id FROM (
				SELECT endpoint_event_id AS id FROM endpoint_events
				WHERE endpoint_id = $1 ORDER BY endpoint_event_id DESC LIMIT 1 OFFSET $2
			) AS cutoff
		)
	`

	mysqlDeleteOldEndpointResultsQuery = `
		DELETE FROM endpoint_results
		WHERE endpoint_id = $1 AND endpoint_result_id <= (
			SELECT id FROM (
				SELECT endpoint_result_id AS id FROM endpoint_results
				WHERE endpoint_id = $1 ORDER BY endpoint_result_id DESC LIMIT 1 OFFSET $2
			) AS cutoff
		)
	`

	mysqlDeleteOldSuiteResultsQuery = `
		DELETE FROM suite_results
		WHERE suite_id = $1 AND suite_result_id <= (
			SELECT id FROM (
				SELECT suite_result_id AS id FROM suite_results
				WHERE suite_id = $1 ORDER BY suite_result_id DESC LIMIT 1 OFFSET $2
			) AS cutoff
		)
	`
)

// dialectQuery returns mysqlQuery with MySQL and MariaDB, and query with the other databases
func (s *Store) dialectQuery(mysqlQuery, query string) string {
	if s.driver == driverMySQL {
		return mysqlQuery
	}
	return query
}

// InsertEndpointResult inserts the result of an endpoint, see insertEndpointResultWithoutRetry. With MySQL and MariaDB,
// a deadlock, a lock wait timeout or a certification conflict of Galera fails the transaction (see mysqlTx), which is
// then attempted once more.
//
// Fork: once the result is committed, it is added to the buckets of the response time chart in a short transaction of
// its own (see insertResponseTimeBuckets), which never changes the error returned.
func (s *Store) InsertEndpointResult(ep *endpoint.Endpoint, result *endpoint.Result) error {
	err := s.retryOnTransientMySQLError("InsertEndpointResult", func() error {
		return s.insertEndpointResultWithoutRetry(ep, result)
	})
	if err == nil {
		s.insertResponseTimeBuckets(ep, result)
	}
	return err
}

// InsertSuiteResult inserts the result of a suite, see insertSuiteResultWithoutRetry and InsertEndpointResult
func (s *Store) InsertSuiteResult(su *suite.Suite, result *suite.Result) error {
	return s.retryOnTransientMySQLError("InsertSuiteResult", func() error {
		return s.insertSuiteResultWithoutRetry(su, result)
	})
}

// retryOnTransientMySQLError runs attempt and, with MySQL and MariaDB, runs it again while it fails with a deadlock or a
// lock wait timeout, up to mysqlMaximumAttempts attempts, waiting a growing and random delay between them. Each attempt
// is a whole transaction, fully rolled back when it fails.
func (s *Store) retryOnTransientMySQLError(operation string, attempt func() error) error {
	err := attempt()
	for attempts := 1; attempts < mysqlMaximumAttempts && s.driver == driverMySQL && isTransientMySQLError(err); attempts++ {
		delay := time.Duration(attempts)*mysqlRetryDelay + time.Duration(rand.Int64N(int64(mysqlRetryDelay)))
		logr.Warnf("[sql.%s] Retrying in %s after a transient MySQL error (attempt %d of %d): %s", operation, delay, attempts+1, mysqlMaximumAttempts, err.Error())
		time.Sleep(delay)
		err = attempt()
	}
	return err
}

// isTransientMySQLError returns whether err is caused by a deadlock or a lock wait timeout, including when it was
// returned by the commit of a transaction aborted by such an error
func isTransientMySQLError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == mysqlErrorDeadlock || mysqlErr.Number == mysqlErrorLockWaitTimeout)
}
