package sql

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// querier is implemented by *sql.DB and *sql.Tx
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// GetUptimesByKeys returns the uptimes over the last 24 hours, 7 days and 30 days before now of the endpoints with the
// given keys, in a single query. Like getEndpointUptime, an uptime entry counts if its hour starts within the period.
func (s *Store) GetUptimesByKeys(keys []string, now time.Time) (map[string]*common.EndpointUptimes, error) {
	return queryUptimesByKeys(s.db, keys, now)
}

// queryUptimesByKeys returns the uptimes of the endpoints with the given keys. Keys without any execution in the last
// 30 days are absent from the map.
func queryUptimesByKeys(q querier, keys []string, now time.Time) (map[string]*common.EndpointUptimes, error) {
	uptimes := make(map[string]*common.EndpointUptimes, len(keys))
	uniqueKeys := uniqueStrings(keys)
	if len(uniqueKeys) == 0 {
		return uptimes, nil
	}
	args := []any{
		now.Add(-24 * time.Hour).Unix(),
		now.Add(-7 * 24 * time.Hour).Unix(),
		now.Add(-30 * 24 * time.Hour).Unix(),
		now.Unix(),
	}
	args, placeholders := appendPlaceholders(args, uniqueKeys)
	rows, err := q.Query(`
		SELECT e.endpoint_key,
			SUM(CASE WHEN u.hour_unix_timestamp >= $1 THEN u.total_executions ELSE 0 END),
			SUM(CASE WHEN u.hour_unix_timestamp >= $1 THEN u.successful_executions ELSE 0 END),
			SUM(CASE WHEN u.hour_unix_timestamp >= $1 THEN u.total_response_time ELSE 0 END),
			SUM(CASE WHEN u.hour_unix_timestamp >= $2 THEN u.total_executions ELSE 0 END),
			SUM(CASE WHEN u.hour_unix_timestamp >= $2 THEN u.successful_executions ELSE 0 END),
			SUM(CASE WHEN u.hour_unix_timestamp >= $2 THEN u.total_response_time ELSE 0 END),
			SUM(u.total_executions),
			SUM(u.successful_executions),
			SUM(u.total_response_time)
		FROM endpoint_uptimes u
		JOIN endpoints e ON e.endpoint_id = u.endpoint_id
		WHERE u.hour_unix_timestamp >= $3
			AND u.hour_unix_timestamp <= $4
			AND e.endpoint_key IN (`+placeholders+`)
		GROUP BY e.endpoint_key
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var total24h, successful24h, responseTime24h, total7d, successful7d, responseTime7d, total30d, successful30d, responseTime30d int64
		if err := rows.Scan(&key, &total24h, &successful24h, &responseTime24h, &total7d, &successful7d, &responseTime7d, &total30d, &successful30d, &responseTime30d); err != nil {
			return nil, err
		}
		uptimes[key] = &common.EndpointUptimes{
			Last24Hours:                uptimeRatio(successful24h, total24h),
			Last7Days:                  uptimeRatio(successful7d, total7d),
			Last30Days:                 uptimeRatio(successful30d, total30d),
			AverageResponseTime24Hours: averageResponseTime(responseTime24h, total24h),
			AverageResponseTime7Days:   averageResponseTime(responseTime7d, total7d),
			AverageResponseTime30Days:  averageResponseTime(responseTime30d, total30d),
		}
	}
	return uptimes, rows.Err()
}

// appendPlaceholders appends values to args and returns the comma-separated placeholders of the appended values
func appendPlaceholders[T any](args []any, values []T) ([]any, string) {
	placeholders := make([]string, 0, len(values))
	for _, value := range values {
		args = append(args, value)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	return args, strings.Join(placeholders, ", ")
}

func uptimeRatio(successfulExecutions, totalExecutions int64) *float64 {
	if totalExecutions <= 0 {
		return nil
	}
	ratio := float64(successfulExecutions) / float64(totalExecutions)
	return &ratio
}

// averageResponseTime returns the average response time in milliseconds, rounded down like
// GetAverageResponseTimeByKey, or nil without execution
func averageResponseTime(totalResponseTime, totalExecutions int64) *int {
	if totalExecutions <= 0 {
		return nil
	}
	average := int(totalResponseTime / totalExecutions)
	return &average
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			unique = append(unique, value)
		}
	}
	return unique
}
