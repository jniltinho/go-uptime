// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// newMockStore returns a PostgreSQL store on a mocked database, for the failures a real database hardly reproduces:
// a row changed between two statements of a transaction, a failed listing, an apply that fails before the commit
func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return &Store{driver: "postgres", db: db}, mock
}

func TestManagedAlertProviders_UpdateLosesTheRace(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT version, created_at FROM managed_alert_providers WHERE alert_type = $1")).
		WithArgs("slack").WillReturnRows(sqlmock.NewRows([]string{"version", "created_at"}).AddRow(3, 1000))
	// Another transaction changed the row after the SELECT: the conditional UPDATE matches nothing
	mock.ExpectExec(regexp.QuoteMeta("UPDATE managed_alert_providers SET")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := store.UpdateManagedAlertProvider(&common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: x\n"}, 3, nil)
	if !errors.Is(err, common.ErrManagedAlertProviderVersionMismatch) {
		t.Fatalf("expected a version mismatch, got %v", err)
	}
}

func TestManagedAlertProviders_CreateDuplicate(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO managed_alert_providers")).
		WillReturnError(errors.New(`pq: duplicate key value violates unique constraint "managed_alert_providers_alert_type_key"`))
	mock.ExpectRollback()
	err := store.CreateManagedAlertProvider(&common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: x\n"}, nil)
	if !errors.Is(err, common.ErrManagedAlertProviderAlreadyExists) {
		t.Fatalf("expected ErrManagedAlertProviderAlreadyExists, got %v", err)
	}
}

func TestManagedAlertProviders_ApplyFailureRollsBack(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO managed_alert_providers")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()
	applyErr := errors.New("apply failed")
	provider := &common.ManagedAlertProvider{Type: "ntfy", Definition: "topic: ops\n"}
	if err := store.CreateManagedAlertProvider(provider, func() error { return applyErr }); !errors.Is(err, applyErr) {
		t.Fatalf("expected the error of apply, got %v", err)
	}
	if provider.Version != 0 {
		t.Errorf("expected no version after a rollback, got %d", provider.Version)
	}
}

func TestManagedAlertProviders_DeleteDistinguishesMissingFromStale(t *testing.T) {
	store, mock := newMockStore(t)
	deleteQuery := regexp.QuoteMeta("DELETE FROM managed_alert_providers WHERE alert_type = $1 AND version = $2")
	versionQuery := regexp.QuoteMeta("SELECT version FROM managed_alert_providers WHERE alert_type = $1")
	mock.ExpectBegin()
	mock.ExpectExec(deleteQuery).WithArgs("ntfy", 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(versionQuery).WithArgs("ntfy").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(4))
	mock.ExpectRollback()
	if err := store.DeleteManagedAlertProvider("ntfy", 1, nil); !errors.Is(err, common.ErrManagedAlertProviderVersionMismatch) {
		t.Fatalf("expected a version mismatch for a stale version, got %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(deleteQuery).WithArgs("ntfy", 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(versionQuery).WithArgs("ntfy").WillReturnRows(sqlmock.NewRows([]string{"version"}))
	mock.ExpectRollback()
	if err := store.DeleteManagedAlertProvider("ntfy", 1, nil); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
		t.Fatalf("expected not found for a missing row, got %v", err)
	}
}

func TestManagedAlertProviders_ListFailure(t *testing.T) {
	store, mock := newMockStore(t)
	listErr := errors.New("connection reset")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT alert_type, definition")).WillReturnError(listErr)
	if _, err := store.ListManagedAlertProviders(); !errors.Is(err, listErr) {
		t.Fatalf("expected the error of the listing, got %v", err)
	}
}
