package sql

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// createResponseTimeBucketsSchema creates the table of the minute and hour aggregates of the results of the endpoints,
// read by the response time chart (fork). It is a table of its own, so that the upstream endpoint_uptimes is not altered,
// and its rows are deleted in cascade with their endpoint. The primary key is declared in CREATE TABLE, because MySQL
// has no CREATE INDEX IF NOT EXISTS.
func (s *Store) createResponseTimeBucketsSchema() error {
	switch s.driver {
	case "sqlite":
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_response_time_buckets (
				endpoint_id            INTEGER NOT NULL REFERENCES endpoints(endpoint_id) ON DELETE CASCADE,
				bucket_seconds         INTEGER NOT NULL,
				bucket_unix_timestamp  INTEGER NOT NULL,
				up_count               INTEGER NOT NULL,
				down_count             INTEGER NOT NULL,
				pending_count          INTEGER NOT NULL,
				timed_up_count         INTEGER NOT NULL,
				up_response_time_total INTEGER NOT NULL,
				up_response_time_min   INTEGER,
				up_response_time_max   INTEGER,
				PRIMARY KEY (endpoint_id, bucket_seconds, bucket_unix_timestamp)
			)
		`)
		return err
	case driverMySQL:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_response_time_buckets (
				endpoint_id            BIGINT NOT NULL,
				bucket_seconds         INT    NOT NULL,
				bucket_unix_timestamp  BIGINT NOT NULL,
				up_count               BIGINT NOT NULL,
				down_count             BIGINT NOT NULL,
				pending_count          BIGINT NOT NULL,
				timed_up_count         BIGINT NOT NULL,
				up_response_time_total BIGINT NOT NULL,
				up_response_time_min   BIGINT,
				up_response_time_max   BIGINT,
				PRIMARY KEY (endpoint_id, bucket_seconds, bucket_unix_timestamp),
				CONSTRAINT fk_endpoint_response_time_buckets_endpoint_id FOREIGN KEY (endpoint_id) REFERENCES endpoints (endpoint_id) ON DELETE CASCADE
			) ` + mysqlTableOptions)
		return err
	default:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_response_time_buckets (
				endpoint_id            BIGINT  NOT NULL REFERENCES endpoints(endpoint_id) ON DELETE CASCADE,
				bucket_seconds         INTEGER NOT NULL,
				bucket_unix_timestamp  BIGINT  NOT NULL,
				up_count               BIGINT  NOT NULL,
				down_count             BIGINT  NOT NULL,
				pending_count          BIGINT  NOT NULL,
				timed_up_count         BIGINT  NOT NULL,
				up_response_time_total BIGINT  NOT NULL,
				up_response_time_min   BIGINT,
				up_response_time_max   BIGINT,
				PRIMARY KEY (endpoint_id, bucket_seconds, bucket_unix_timestamp)
			)
		`)
		return err
	}
}

// upsertResponseTimeBucketQuery returns the query that adds a single result to its bucket. The minimum and the maximum
// are combined as LEAST(COALESCE(current, new), COALESCE(new, current)), because LEAST and GREATEST of MySQL and the
// scalar MIN and MAX of SQLite return NULL when any argument is NULL: a result without duration keeps them unchanged.
func (s *Store) upsertResponseTimeBucketQuery() string {
	const columns = `endpoint_id, bucket_seconds, bucket_unix_timestamp, up_count, down_count, pending_count, timed_up_count, up_response_time_total, up_response_time_min, up_response_time_max`
	const values = `VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	if s.driver == driverMySQL {
		return `INSERT INTO endpoint_response_time_buckets (` + columns + `) ` + values + `
			ON DUPLICATE KEY UPDATE
				up_count = up_count + VALUES(up_count),
				down_count = down_count + VALUES(down_count),
				pending_count = pending_count + VALUES(pending_count),
				timed_up_count = timed_up_count + VALUES(timed_up_count),
				up_response_time_total = up_response_time_total + VALUES(up_response_time_total),
				up_response_time_min = LEAST(COALESCE(up_response_time_min, VALUES(up_response_time_min)), COALESCE(VALUES(up_response_time_min), up_response_time_min)),
				up_response_time_max = GREATEST(COALESCE(up_response_time_max, VALUES(up_response_time_max)), COALESCE(VALUES(up_response_time_max), up_response_time_max))`
	}
	minimum, maximum := "LEAST", "GREATEST"
	if s.driver == "sqlite" {
		minimum, maximum = "MIN", "MAX"
	}
	return fmt.Sprintf(`INSERT INTO endpoint_response_time_buckets (%s) %s
		ON CONFLICT (endpoint_id, bucket_seconds, bucket_unix_timestamp) DO UPDATE SET
			up_count = endpoint_response_time_buckets.up_count + excluded.up_count,
			down_count = endpoint_response_time_buckets.down_count + excluded.down_count,
			pending_count = endpoint_response_time_buckets.pending_count + excluded.pending_count,
			timed_up_count = endpoint_response_time_buckets.timed_up_count + excluded.timed_up_count,
			up_response_time_total = endpoint_response_time_buckets.up_response_time_total + excluded.up_response_time_total,
			up_response_time_min = %s(COALESCE(endpoint_response_time_buckets.up_response_time_min, excluded.up_response_time_min), COALESCE(excluded.up_response_time_min, endpoint_response_time_buckets.up_response_time_min)),
			up_response_time_max = %s(COALESCE(endpoint_response_time_buckets.up_response_time_max, excluded.up_response_time_max), COALESCE(excluded.up_response_time_max, endpoint_response_time_buckets.up_response_time_max))`,
		columns, values, minimum, maximum)
}

// insertResponseTimeBuckets adds a result that was already committed to its minute and hour buckets, in a short
// transaction of its own: a failure only loses the buckets of that result, is logged, and never undoes the result
func (s *Store) insertResponseTimeBuckets(ep *endpoint.Endpoint, result *endpoint.Result) {
	key := ep.Key()
	now := s.currentTime()
	cleanUp := s.responseTimeBucketsCleanUpDue(key, now)
	err := s.retryOnTransientMySQLError("insertResponseTimeBuckets", func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if err = s.upsertResponseTimeBuckets(tx, ep, result, now, cleanUp); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		logr.Errorf("[sql.insertResponseTimeBuckets] Failed to update the response time buckets of endpoint with key=%s: %s", key, err.Error())
		return
	}
	if cleanUp {
		s.markResponseTimeBucketsCleanedUp(key, now)
	}
}

func (s *Store) upsertResponseTimeBuckets(tx *sql.Tx, ep *endpoint.Endpoint, result *endpoint.Result, now time.Time, cleanUp bool) error {
	endpointID, err := s.getEndpointID(tx, ep)
	if err != nil {
		return err
	}
	query := s.upsertResponseTimeBucketQuery()
	for _, bucketSeconds := range []int{common.MinuteBucketSeconds, common.HourBucketSeconds} {
		bucket := common.NewResponseTimeBucket(result, bucketSeconds)
		if _, err = tx.Exec(query, endpointID, bucketSeconds, bucket.Timestamp.Unix(), bucket.Up, bucket.Down, bucket.Pending, bucket.TimedUp, bucket.TotalMs, nullableInt64(bucket.MinMs), nullableInt64(bucket.MaxMs)); err != nil {
			return err
		}
	}
	if !cleanUp {
		return nil
	}
	for bucketSeconds, retention := range map[int]time.Duration{common.MinuteBucketSeconds: common.MinuteBucketRetention, common.HourBucketSeconds: common.HourBucketRetention} {
		if _, err = tx.Exec("DELETE FROM endpoint_response_time_buckets WHERE endpoint_id = $1 AND bucket_seconds = $2 AND bucket_unix_timestamp < $3", endpointID, bucketSeconds, now.Add(-(retention + time.Hour)).Unix()); err != nil {
			return err
		}
	}
	return nil
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// currentTime returns the time used by the response time buckets, which the tests can replace
func (s *Store) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// responseTimeBucketsCleanUpDue returns whether the buckets of the endpoint were not cleaned up during the last
// ResponseTimeBucketCleanUpInterval by this store
func (s *Store) responseTimeBucketsCleanUpDue(key string, now time.Time) bool {
	s.responseTimeBucketsMutex.Lock()
	defer s.responseTimeBucketsMutex.Unlock()
	lastCleanUp, exists := s.responseTimeBucketsCleanUps[key]
	return !exists || now.Sub(lastCleanUp) >= common.ResponseTimeBucketCleanUpInterval || now.Before(lastCleanUp)
}

// markResponseTimeBucketsCleanedUp records a clean up of the buckets of the endpoint, only after its commit
func (s *Store) markResponseTimeBucketsCleanedUp(key string, now time.Time) {
	s.responseTimeBucketsMutex.Lock()
	defer s.responseTimeBucketsMutex.Unlock()
	if s.responseTimeBucketsCleanUps == nil {
		s.responseTimeBucketsCleanUps = make(map[string]time.Time)
	}
	s.responseTimeBucketsCleanUps[key] = now
}

// forgetResponseTimeBucketsCleanUps forgets the clean ups of the endpoints whose key is not in keys, or of every endpoint
// when keys is nil
func (s *Store) forgetResponseTimeBucketsCleanUps(keys []string) {
	s.responseTimeBucketsMutex.Lock()
	defer s.responseTimeBucketsMutex.Unlock()
	for key := range s.responseTimeBucketsCleanUps {
		if keys == nil || !slices.Contains(keys, key) {
			delete(s.responseTimeBucketsCleanUps, key)
		}
	}
}

// GetResponseTimeBuckets returns the buckets of bucketSeconds of the endpoint with the given key whose start is between
// from and to, from the oldest to the most recent
func (s *Store) GetResponseTimeBuckets(key string, bucketSeconds int, from, to time.Time) ([]common.ResponseTimeBucket, error) {
	rows, err := s.db.Query(`
		SELECT b.bucket_unix_timestamp, b.up_count, b.down_count, b.pending_count, b.timed_up_count, b.up_response_time_total, b.up_response_time_min, b.up_response_time_max
		FROM endpoint_response_time_buckets b
		JOIN endpoints e ON e.endpoint_id = b.endpoint_id
		WHERE e.endpoint_key = $1 AND b.bucket_seconds = $2 AND b.bucket_unix_timestamp >= $3 AND b.bucket_unix_timestamp <= $4
		ORDER BY b.bucket_unix_timestamp
	`, key, bucketSeconds, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	buckets := make([]common.ResponseTimeBucket, 0)
	for rows.Next() {
		var timestamp int64
		var bucket common.ResponseTimeBucket
		var minimum, maximum sql.NullInt64
		if err := rows.Scan(&timestamp, &bucket.Up, &bucket.Down, &bucket.Pending, &bucket.TimedUp, &bucket.TotalMs, &minimum, &maximum); err != nil {
			return nil, err
		}
		bucket.Timestamp = time.Unix(timestamp, 0).UTC()
		if minimum.Valid {
			bucket.MinMs = &minimum.Int64
		}
		if maximum.Valid {
			bucket.MaxMs = &maximum.Int64
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// GetRecentResponseTimeResults returns the latest limit results of the endpoint with the given key, without the results
// of suites, from the oldest to the most recent timestamp. Only the columns shown by the chart are read, and the
// write-through cache is neither read nor refreshed.
func (s *Store) GetRecentResponseTimeResults(key string, limit int) ([]common.RecentResponseTimeResult, error) {
	results := make([]common.RecentResponseTimeResult, 0)
	if limit <= 0 {
		return results, nil
	}
	rows, err := s.db.Query(`
		SELECT recent_results.timestamp, recent_results.success, recent_results.duration, m.pending
		FROM (
			SELECT r.endpoint_result_id, r.timestamp, r.success, r.duration
			FROM endpoint_results r
			JOIN endpoints e ON e.endpoint_id = r.endpoint_id
			WHERE e.endpoint_key = $1 AND r.suite_result_id IS NULL
			ORDER BY r.endpoint_result_id DESC
			LIMIT $2
		) recent_results
		LEFT JOIN endpoint_result_messages m ON m.endpoint_result_id = recent_results.endpoint_result_id
	`, key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var result common.RecentResponseTimeResult
		var pending sql.NullBool
		if err := rows.Scan(&result.Timestamp, &result.Success, &result.Duration, &pending); err != nil {
			return nil, err
		}
		result.Pending = pending.Bool
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(results, func(a, b common.RecentResponseTimeResult) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	return results, nil
}
