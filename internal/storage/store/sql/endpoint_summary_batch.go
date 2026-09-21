package sql

import (
	"database/sql"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// GetEndpointSummaries returns the latest maximumResults results and the uptimes of the endpoints with the given keys,
// reading only the columns that can be published, in a single read transaction with three queries
func (s *Store) GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error) {
	summaries := make(map[string]*common.EndpointSummary, len(keys))
	uniqueKeys := uniqueStrings(keys)
	if len(uniqueKeys) == 0 {
		return summaries, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	// Nothing is written, so the transaction is always rolled back
	defer func() { _ = tx.Rollback() }()
	args, placeholders := appendPlaceholders(nil, uniqueKeys)
	rows, err := tx.Query("SELECT endpoint_id, endpoint_key FROM endpoints WHERE endpoint_key IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	keysByID := make(map[int64]string, len(uniqueKeys))
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keysByID[id] = key
		summaries[key] = &common.EndpointSummary{Results: []common.ResultSummary{}}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(keysByID) == 0 {
		return summaries, nil
	}
	if maximumResults > 0 {
		ids := make([]int64, 0, len(keysByID))
		for id := range keysByID {
			ids = append(ids, id)
		}
		args, placeholders := appendPlaceholders([]any{maximumResults}, ids)
		// Fork: the pending mark, the message and the origin are in endpoint_result_messages
		rows, err := tx.Query(`
			SELECT recent_results.endpoint_id, recent_results.success, recent_results.duration,
				recent_results.certificate_expiration, recent_results.timestamp, recent_results.status, recent_results.errors,
				recent_results.connected, m.message, m.origin, m.pending
			FROM (
				SELECT endpoint_id, endpoint_result_id, success, duration, certificate_expiration, timestamp, status, errors, connected,
					ROW_NUMBER() OVER (PARTITION BY endpoint_id ORDER BY endpoint_result_id DESC) AS rn
				FROM endpoint_results
				WHERE endpoint_id IN (`+placeholders+`)
			) recent_results
			LEFT JOIN endpoint_result_messages m ON m.endpoint_result_id = recent_results.endpoint_result_id
			WHERE recent_results.rn <= $1
			ORDER BY recent_results.endpoint_id, recent_results.endpoint_result_id
		`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var result common.ResultSummary
			var joinedErrors string
			var message, origin sql.NullString
			var pending sql.NullBool
			if err := rows.Scan(&id, &result.Success, &result.Duration, &result.CertificateExpiration, &result.Timestamp, &result.HTTPStatus, &joinedErrors, &result.Connected, &message, &origin, &pending); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if len(joinedErrors) > 0 {
				result.Errors = strings.Split(joinedErrors, arraySeparator)
			}
			result.Message, result.Origin, result.Pending = message.String, origin.String, pending.Bool
			summary := summaries[keysByID[id]]
			summary.Results = append(summary.Results, result)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	existingKeys := make([]string, 0, len(keysByID))
	for _, key := range keysByID {
		existingKeys = append(existingKeys, key)
	}
	uptimes, err := queryUptimesByKeys(tx, existingKeys, now)
	if err != nil {
		return nil, err
	}
	for key, endpointUptimes := range uptimes {
		summaries[key].Uptimes = *endpointUptimes
	}
	return summaries, nil
}
