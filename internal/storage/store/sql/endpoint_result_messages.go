package sql

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/go-sql-driver/mysql"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// createEndpointResultMessagesSchema creates the table of the messages, origins and pending marks of the endpoint
// results (fork). It is a table of its own instead of columns of endpoint_results, so that the upstream schema is not
// altered, and its rows are deleted in cascade with their results.
func (s *Store) createEndpointResultMessagesSchema() error {
	if err := s.createEndpointResultMessagesTable(); err != nil {
		return err
	}
	if err := s.addEndpointResultMessagesPendingColumn(); err != nil {
		return err
	}
	// The events of an endpoint are compared with its last healthy or unhealthy event when a result is inserted. MySQL
	// and MariaDB already index endpoint_id with the foreign key.
	if s.driver != driverMySQL {
		_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS endpoint_events_endpoint_id_event_id_idx ON endpoint_events (endpoint_id, endpoint_event_id)`)
		return err
	}
	return nil
}

func (s *Store) createEndpointResultMessagesTable() error {
	switch s.driver {
	case "sqlite":
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_result_messages (
				endpoint_result_id INTEGER PRIMARY KEY REFERENCES endpoint_results(endpoint_result_id) ON DELETE CASCADE,
				message            TEXT    NOT NULL,
				origin             TEXT    NOT NULL,
				pending            INTEGER NOT NULL DEFAULT 0
			)
		`)
		return err
	case driverMySQL:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_result_messages (
				endpoint_result_id BIGINT      NOT NULL PRIMARY KEY,
				message            TEXT        NOT NULL,
				origin             VARCHAR(16) NOT NULL,
				pending            BOOLEAN     NOT NULL DEFAULT FALSE,
				FOREIGN KEY (endpoint_result_id) REFERENCES endpoint_results(endpoint_result_id) ON DELETE CASCADE
			) ` + mysqlTableOptions)
		return err
	default:
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS endpoint_result_messages (
				endpoint_result_id BIGINT PRIMARY KEY REFERENCES endpoint_results(endpoint_result_id) ON DELETE CASCADE,
				message            TEXT    NOT NULL,
				origin             TEXT    NOT NULL,
				pending            BOOLEAN NOT NULL DEFAULT FALSE
			)
		`)
		return err
	}
}

// addEndpointResultMessagesPendingColumn adds the pending column to a table created by a previous version of the fork.
// It is idempotent, including when two instances start at the same time.
func (s *Store) addEndpointResultMessagesPendingColumn() error {
	switch s.driver {
	case "sqlite":
		rows, err := s.db.Query("SELECT name FROM pragma_table_info('endpoint_result_messages')")
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return err
			}
			found = found || name == "pending"
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if found {
			return nil
		}
		_, err = s.db.Exec("ALTER TABLE endpoint_result_messages ADD COLUMN pending INTEGER NOT NULL DEFAULT 0")
		return err
	case driverMySQL:
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'endpoint_result_messages' AND column_name = 'pending'").Scan(&count)
		if err != nil || count > 0 {
			return err
		}
		_, err = s.db.Exec("ALTER TABLE endpoint_result_messages ADD COLUMN pending BOOLEAN NOT NULL DEFAULT FALSE")
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrorDuplicateColumn {
			return nil
		}
		return err
	default:
		_, err := s.db.Exec("ALTER TABLE endpoint_result_messages ADD COLUMN IF NOT EXISTS pending BOOLEAN NOT NULL DEFAULT FALSE")
		return err
	}
}

// insertEndpointResultMessage stores the message, the origin and the pending mark of a result, if it has any
func (s *Store) insertEndpointResultMessage(tx *sql.Tx, endpointResultID int64, result *endpoint.Result) error {
	if len(result.Message) == 0 && len(result.Origin) == 0 && !result.Pending {
		return nil
	}
	_, err := tx.Exec(
		"INSERT INTO endpoint_result_messages (endpoint_result_id, message, origin, pending) VALUES ($1, $2, $3, $4)",
		endpointResultID,
		endpoint.TruncateResultMessage(result.Message),
		result.Origin,
		result.Pending,
	)
	return err
}

// loadEndpointResultMessages sets the message, the origin and the pending mark of the results in idResultMap that have
// them
func (s *Store) loadEndpointResultMessages(tx *sql.Tx, idResultMap map[int64]*endpoint.Result) error {
	if len(idResultMap) == 0 {
		return nil
	}
	args := make([]any, 0, len(idResultMap))
	query := "SELECT endpoint_result_id, message, origin, pending FROM endpoint_result_messages WHERE endpoint_result_id IN ("
	index := 1
	for endpointResultID := range idResultMap {
		query += "$" + strconv.Itoa(index) + ","
		args = append(args, endpointResultID)
		index++
	}
	query = query[:len(query)-1] + ")"
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var endpointResultID int64
		var message, origin string
		var pending bool
		if err := rows.Scan(&endpointResultID, &message, &origin, &pending); err != nil {
			return err
		}
		if result := idResultMap[endpointResultID]; result != nil {
			result.Message, result.Origin, result.Pending = message, origin, pending
		}
	}
	return rows.Err()
}
