// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestStore_RetryOnTransientMySQLError(t *testing.T) {
	deadlock := fmt.Errorf("%w: %w", errTransactionAborted, &mysql.MySQLError{Number: mysqlErrorDeadlock, Message: "Deadlock found"})
	lockWaitTimeout := &mysql.MySQLError{Number: mysqlErrorLockWaitTimeout}
	scenarios := []struct {
		name             string
		driver           string
		errors           []error
		expectedAttempts int
		expectedErr      error
	}{
		{name: "mysql-success", driver: driverMySQL, errors: []error{nil}, expectedAttempts: 1},
		{name: "mysql-deadlock-then-success", driver: driverMySQL, errors: []error{deadlock, nil}, expectedAttempts: 2},
		{name: "mysql-deadlock-twice-then-success", driver: driverMySQL, errors: []error{deadlock, lockWaitTimeout, nil}, expectedAttempts: 3},
		{name: "mysql-lock-wait-timeout-on-every-attempt", driver: driverMySQL, errors: []error{lockWaitTimeout, lockWaitTimeout, lockWaitTimeout}, expectedAttempts: mysqlMaximumAttempts, expectedErr: lockWaitTimeout},
		{name: "mysql-duplicate-entry", driver: driverMySQL, errors: []error{&mysql.MySQLError{Number: mysqlErrorDuplicateEntry}}, expectedAttempts: 1, expectedErr: &mysql.MySQLError{Number: mysqlErrorDuplicateEntry}},
		{name: "mysql-other-error", driver: driverMySQL, errors: []error{errNoRowsReturned}, expectedAttempts: 1, expectedErr: errNoRowsReturned},
		{name: "postgres-deadlock-is-not-retried", driver: "postgres", errors: []error{deadlock}, expectedAttempts: 1, expectedErr: deadlock},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			store := &Store{driver: scenario.driver}
			attempts := 0
			err := store.retryOnTransientMySQLError("Test", func() error {
				err := scenario.errors[attempts]
				attempts++
				return err
			})
			if attempts != scenario.expectedAttempts {
				t.Errorf("expected %d attempts, got %d", scenario.expectedAttempts, attempts)
			}
			var expectedMySQLErr, actualMySQLErr *mysql.MySQLError
			switch {
			case scenario.expectedErr == nil && err != nil:
				t.Errorf("expected no error, got %v", err)
			case errors.As(scenario.expectedErr, &expectedMySQLErr):
				if !errors.As(err, &actualMySQLErr) || actualMySQLErr.Number != expectedMySQLErr.Number {
					t.Errorf("expected MySQL error %d, got %v", expectedMySQLErr.Number, err)
				}
			case scenario.expectedErr != nil && !errors.Is(err, scenario.expectedErr):
				t.Errorf("expected %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}
