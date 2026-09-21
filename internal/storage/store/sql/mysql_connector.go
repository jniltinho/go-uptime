// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
)

var (
	// errTransactionAborted is returned by every statement and by Commit after a statement of the transaction failed,
	// as PostgreSQL does. It wraps the error of the failed statement.
	errTransactionAborted = errors.New("transaction aborted by a previous error")

	errMySQLConnUnsupported = errors.New("the mysql driver returned a connection without the expected interfaces")
)

// mysqlInnerConn is the set of interfaces implemented by the connections of the MySQL driver and forwarded by
// mysqlConn. SessionResetter and Validator are what keeps dead connections out of the pool.
type mysqlInnerConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.QueryerContext
	driver.ExecerContext
	driver.Pinger
	driver.NamedValueChecker
	driver.SessionResetter
	driver.Validator
}

var (
	_ driver.Connector         = (*mysqlConnector)(nil)
	_ mysqlInnerConn           = (*mysqlConn)(nil)
	_ driver.Tx                = (*mysqlTx)(nil)
	_ driver.StmtExecContext   = (*mysqlStmt)(nil)
	_ driver.StmtQueryContext  = (*mysqlStmt)(nil)
	_ driver.NamedValueChecker = (*mysqlStmt)(nil)
	_ driver.RowsNextResultSet = (*rowsClosingStmt)(nil)
)

// mysqlConnector wraps the connector of the MySQL driver, so that the queries of the store, written with PostgreSQL
// placeholders ($1, $2, ...), run on MySQL and MariaDB (see translatePlaceholders), and so that a transaction behaves
// like in PostgreSQL after an error (see mysqlTx)
type mysqlConnector struct {
	connector driver.Connector
}

// Connect returns a new connection of the MySQL driver wrapped in a mysqlConn
func (connector *mysqlConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := connector.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	inner, ok := conn.(mysqlInnerConn)
	if !ok {
		_ = conn.Close()
		return nil, errMySQLConnUnsupported
	}
	// READ COMMITTED, the default of PostgreSQL: with the REPEATABLE READ default of InnoDB, the gap locks taken by the
	// concurrent insertions of results of different endpoints deadlock each other
	if _, err := inner.ExecContext(ctx, "SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED", nil); err != nil {
		_ = inner.Close()
		return nil, err
	}
	return &mysqlConn{inner: inner}, nil
}

// Driver returns the driver of the wrapped connector
func (connector *mysqlConnector) Driver() driver.Driver {
	return connector.connector.Driver()
}

// mysqlConn is a connection of the MySQL driver whose queries are translated. database/sql never uses a connection
// from two goroutines at the same time, so it needs no lock.
type mysqlConn struct {
	inner mysqlInnerConn

	// tx is the active transaction of the connection, if any
	tx *mysqlTx
}

func (conn *mysqlConn) Prepare(query string) (driver.Stmt, error) {
	return conn.PrepareContext(context.Background(), query)
}

func (conn *mysqlConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := conn.abortedError(); err != nil {
		return nil, err
	}
	translated, err := translatePlaceholders(query)
	if err != nil {
		return nil, conn.observe(err)
	}
	stmt, err := conn.inner.PrepareContext(ctx, translated.query)
	if err != nil {
		return nil, conn.observe(err)
	}
	return &mysqlStmt{conn: conn, inner: stmt, translated: translated}, nil
}

func (conn *mysqlConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := conn.abortedError(); err != nil {
		return nil, err
	}
	translated, arguments, err := translateQuery(query, args)
	if err != nil {
		return nil, conn.observe(err)
	}
	result, err := conn.inner.ExecContext(ctx, translated.query, arguments)
	if errors.Is(err, driver.ErrSkip) {
		// The driver asks for a prepared statement: it must be prepared with the translated query here, because
		// database/sql would prepare the original query and check its arguments against the translated one
		result, err = conn.execPrepared(ctx, translated.query, arguments)
	}
	return result, conn.observe(err)
}

func (conn *mysqlConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := conn.abortedError(); err != nil {
		return nil, err
	}
	translated, arguments, err := translateQuery(query, args)
	if err != nil {
		return nil, conn.observe(err)
	}
	if len(translated.returningColumn) > 0 {
		rows, err := conn.queryInsertReturning(ctx, translated, arguments)
		return rows, conn.observe(err)
	}
	rows, err := conn.inner.QueryContext(ctx, translated.query, arguments)
	if errors.Is(err, driver.ErrSkip) {
		rows, err = conn.queryPrepared(ctx, translated.query, arguments)
	}
	return rows, conn.observe(err)
}

// queryInsertReturning runs an INSERT ... RETURNING <column>, which MySQL does not support, as the INSERT alone and
// returns the generated id as the single row of the query. It keeps the upstream queries unchanged: they insert one row
// and return its AUTO_INCREMENT primary key, and LastInsertId is reliable because the connection is not shared.
func (conn *mysqlConn) queryInsertReturning(ctx context.Context, translated *translatedQuery, arguments []driver.NamedValue) (driver.Rows, error) {
	result, err := conn.inner.ExecContext(ctx, translated.insertWithoutReturning, arguments)
	if errors.Is(err, driver.ErrSkip) {
		result, err = conn.execPrepared(ctx, translated.insertWithoutReturning, arguments)
	}
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &lastInsertIDRows{column: translated.returningColumn, id: id}, nil
}

// lastInsertIDRows is the single row of an emulated INSERT ... RETURNING <column>
type lastInsertIDRows struct {
	column string
	id     int64
	read   bool
}

func (rows *lastInsertIDRows) Columns() []string {
	return []string{rows.column}
}

func (rows *lastInsertIDRows) Close() error {
	return nil
}

func (rows *lastInsertIDRows) Next(dest []driver.Value) error {
	if rows.read {
		return io.EOF
	}
	rows.read = true
	dest[0] = rows.id
	return nil
}

func (conn *mysqlConn) execPrepared(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Result, error) {
	stmt, err := conn.inner.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	execer, ok := stmt.(driver.StmtExecContext)
	if !ok {
		return nil, errMySQLConnUnsupported
	}
	return execer.ExecContext(ctx, arguments)
}

func (conn *mysqlConn) queryPrepared(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	stmt, err := conn.inner.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	queryer, ok := stmt.(driver.StmtQueryContext)
	if !ok {
		_ = stmt.Close()
		return nil, errMySQLConnUnsupported
	}
	rows, err := queryer.QueryContext(ctx, arguments)
	if err != nil {
		_ = stmt.Close()
		return nil, err
	}
	return &rowsClosingStmt{Rows: rows, stmt: stmt}, nil
}

func (conn *mysqlConn) Begin() (driver.Tx, error) {
	return conn.BeginTx(context.Background(), driver.TxOptions{})
}

func (conn *mysqlConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := conn.inner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	conn.tx = &mysqlTx{conn: conn, inner: tx}
	return conn.tx, nil
}

func (conn *mysqlConn) Close() error {
	return conn.inner.Close()
}

func (conn *mysqlConn) Ping(ctx context.Context) error {
	return conn.inner.Ping(ctx)
}

func (conn *mysqlConn) CheckNamedValue(value *driver.NamedValue) error {
	return conn.inner.CheckNamedValue(value)
}

func (conn *mysqlConn) ResetSession(ctx context.Context) error {
	return conn.inner.ResetSession(ctx)
}

func (conn *mysqlConn) IsValid() bool {
	return conn.inner.IsValid()
}

// observe records err as the error that aborts the active transaction, if any, and returns it
func (conn *mysqlConn) observe(err error) error {
	if err != nil && conn.tx != nil && conn.tx.err == nil {
		conn.tx.err = err
	}
	return err
}

// abortedError returns errTransactionAborted when a statement of the active transaction failed
func (conn *mysqlConn) abortedError() error {
	if conn.tx != nil && conn.tx.err != nil {
		return fmt.Errorf("%w: %w", errTransactionAborted, conn.tx.err)
	}
	return nil
}

// mysqlTx is a transaction that, like in PostgreSQL, is aborted by the first failed statement: the following statements
// fail without running and Commit rolls back and returns the error. Without it, InnoDB would commit the statements
// that followed a deadlock (which rolls back the whole transaction) or a lock wait timeout (which only rolls back the
// statement), and the caller would not see the error.
type mysqlTx struct {
	conn  *mysqlConn
	inner driver.Tx

	// err is the error of the first failed statement of the transaction
	err error
}

func (tx *mysqlTx) Commit() error {
	tx.detach()
	if tx.err != nil {
		_ = tx.inner.Rollback()
		return fmt.Errorf("%w: %w", errTransactionAborted, tx.err)
	}
	return tx.inner.Commit()
}

func (tx *mysqlTx) Rollback() error {
	tx.detach()
	return tx.inner.Rollback()
}

func (tx *mysqlTx) detach() {
	if tx.conn.tx == tx {
		tx.conn.tx = nil
	}
}

// mysqlStmt is a prepared statement of a translated query. NumInput returns -1, so that database/sql does not compare
// the arguments, written for the original query, with the placeholders of the translated one.
type mysqlStmt struct {
	conn       *mysqlConn
	inner      driver.Stmt
	translated *translatedQuery
}

func (stmt *mysqlStmt) Close() error {
	return stmt.inner.Close()
}

func (stmt *mysqlStmt) NumInput() int {
	return -1
}

func (stmt *mysqlStmt) Exec(args []driver.Value) (driver.Result, error) {
	return stmt.ExecContext(context.Background(), valuesToNamedValues(args))
}

func (stmt *mysqlStmt) Query(args []driver.Value) (driver.Rows, error) {
	return stmt.QueryContext(context.Background(), valuesToNamedValues(args))
}

func (stmt *mysqlStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if err := stmt.conn.abortedError(); err != nil {
		return nil, err
	}
	arguments, err := stmt.translated.arguments(args)
	if err != nil {
		return nil, stmt.conn.observe(err)
	}
	execer, ok := stmt.inner.(driver.StmtExecContext)
	if !ok {
		return nil, errMySQLConnUnsupported
	}
	result, err := execer.ExecContext(ctx, arguments)
	return result, stmt.conn.observe(err)
}

func (stmt *mysqlStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if err := stmt.conn.abortedError(); err != nil {
		return nil, err
	}
	arguments, err := stmt.translated.arguments(args)
	if err != nil {
		return nil, stmt.conn.observe(err)
	}
	queryer, ok := stmt.inner.(driver.StmtQueryContext)
	if !ok {
		return nil, errMySQLConnUnsupported
	}
	rows, err := queryer.QueryContext(ctx, arguments)
	return rows, stmt.conn.observe(err)
}

func (stmt *mysqlStmt) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := stmt.inner.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return driver.ErrSkip
}

// rowsClosingStmt closes the statement prepared for a query when its rows are closed
type rowsClosingStmt struct {
	driver.Rows
	stmt driver.Stmt
}

func (rows *rowsClosingStmt) Close() error {
	err := rows.Rows.Close()
	if stmtErr := rows.stmt.Close(); err == nil {
		err = stmtErr
	}
	return err
}

func (rows *rowsClosingStmt) HasNextResultSet() bool {
	if next, ok := rows.Rows.(driver.RowsNextResultSet); ok {
		return next.HasNextResultSet()
	}
	return false
}

func (rows *rowsClosingStmt) NextResultSet() error {
	if next, ok := rows.Rows.(driver.RowsNextResultSet); ok {
		return next.NextResultSet()
	}
	return io.EOF
}

func (rows *rowsClosingStmt) ColumnTypeScanType(index int) reflect.Type {
	if scanType, ok := rows.Rows.(driver.RowsColumnTypeScanType); ok {
		return scanType.ColumnTypeScanType(index)
	}
	return reflect.TypeFor[any]()
}

func (rows *rowsClosingStmt) ColumnTypeDatabaseTypeName(index int) string {
	if typeName, ok := rows.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return typeName.ColumnTypeDatabaseTypeName(index)
	}
	return ""
}

// translateQuery translates the placeholders of query and reorders its arguments
func translateQuery(query string, args []driver.NamedValue) (*translatedQuery, []driver.NamedValue, error) {
	translated, err := translatePlaceholders(query)
	if err != nil {
		return nil, nil, err
	}
	arguments, err := translated.arguments(args)
	if err != nil {
		return nil, nil, err
	}
	return translated, arguments, nil
}

func valuesToNamedValues(values []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(values))
	for i, value := range values {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return named
}
