// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// managedAlertProviderTestStores returns the stores of managedEndpointTestStores with an empty managed_alert_providers table
func managedAlertProviderTestStores(t *testing.T) map[string]*Store {
	t.Helper()
	stores := managedEndpointTestStores(t)
	for driver, store := range stores {
		if _, err := store.db.Exec("DELETE FROM managed_alert_providers"); err != nil {
			t.Fatalf("failed to clean managed_alert_providers with %s: %v", driver, err)
		}
	}
	return stores
}

func TestStore_ManagedAlertProviders(t *testing.T) {
	for driver, store := range managedAlertProviderTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			created := &common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: https://hooks.example.com/a\n", UpdatedBy: "ops@example.com"}
			if err := store.CreateManagedAlertProvider(created, nil); err != nil {
				t.Fatalf("failed to create managed alerting provider: %v", err)
			}
			if created.Version != 1 || created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
				t.Errorf("expected version 1 and matching timestamps, got version=%d createdAt=%s updatedAt=%s", created.Version, created.CreatedAt, created.UpdatedAt)
			}
			if err := store.CreateManagedAlertProvider(&common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: https://hooks.example.com/a\n"}, nil); !errors.Is(err, common.ErrManagedAlertProviderAlreadyExists) {
				t.Errorf("expected ErrManagedAlertProviderAlreadyExists, got %v", err)
			}
			if err := store.CreateManagedAlertProvider(&common.ManagedAlertProvider{Type: "ntfy", Definition: "topic: ops\n"}, nil); err != nil {
				t.Fatalf("failed to create second managed alerting provider: %v", err)
			}
			list, err := store.ListManagedAlertProviders()
			if err != nil {
				t.Fatalf("failed to list managed alerting providers: %v", err)
			}
			if len(list) != 2 || list[0].Type != "ntfy" || list[1].Type != "slack" {
				t.Fatalf("expected ntfy and slack ordered by type, got %+v", list)
			}
			updated := &common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: https://hooks.example.com/b\n", UpdatedBy: "admin"}
			if err := store.UpdateManagedAlertProvider(updated, 1, nil); err != nil {
				t.Fatalf("failed to update managed alerting provider: %v", err)
			}
			if updated.Version != 2 || !updated.CreatedAt.Equal(created.CreatedAt) {
				t.Errorf("expected version 2 and the original creation time, got version=%d createdAt=%s", updated.Version, updated.CreatedAt)
			}
			if err := store.UpdateManagedAlertProvider(updated, 1, nil); !errors.Is(err, common.ErrManagedAlertProviderVersionMismatch) {
				t.Errorf("expected ErrManagedAlertProviderVersionMismatch, got %v", err)
			}
			if err := store.UpdateManagedAlertProvider(&common.ManagedAlertProvider{Type: "missing"}, 1, nil); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
				t.Errorf("expected ErrManagedAlertProviderNotFound, got %v", err)
			}
			got, err := store.GetManagedAlertProvider("slack")
			if err != nil {
				t.Fatalf("failed to get managed alerting provider: %v", err)
			}
			if got.Definition != updated.Definition || got.Version != 2 || got.UpdatedBy != "admin" {
				t.Errorf("unexpected managed alerting provider: %+v", got)
			}
			if _, err := store.GetManagedAlertProvider("missing"); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
				t.Errorf("expected ErrManagedAlertProviderNotFound, got %v", err)
			}
			if err := store.DeleteManagedAlertProvider("slack", 1, nil); !errors.Is(err, common.ErrManagedAlertProviderVersionMismatch) {
				t.Errorf("expected ErrManagedAlertProviderVersionMismatch, got %v", err)
			}
			if err := store.DeleteManagedAlertProvider("missing", 1, nil); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
				t.Errorf("expected ErrManagedAlertProviderNotFound, got %v", err)
			}
			if err := store.DeleteManagedAlertProvider("slack", 2, nil); err != nil {
				t.Fatalf("failed to delete managed alerting provider: %v", err)
			}
			if _, err := store.GetManagedAlertProvider("slack"); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
				t.Errorf("expected the managed alerting provider to be deleted, got %v", err)
			}
		})
	}
}

func TestStore_ManagedAlertProvidersApplyRollsBack(t *testing.T) {
	for driver, store := range managedAlertProviderTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			errApply := errors.New("apply failed")
			failingApply := func() error { return errApply }
			if err := store.CreateManagedAlertProvider(&common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: https://hooks.example.com/a\n"}, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if _, err := store.GetManagedAlertProvider("slack"); !errors.Is(err, common.ErrManagedAlertProviderNotFound) {
				t.Fatalf("expected the creation to be rolled back, got %v", err)
			}
			page := &common.ManagedAlertProvider{Type: "slack", Definition: "webhook-url: https://hooks.example.com/a\n"}
			if err := store.CreateManagedAlertProvider(page, nil); err != nil {
				t.Fatalf("failed to create managed alerting provider: %v", err)
			}
			if err := store.UpdateManagedAlertProvider(&common.ManagedAlertProvider{Type: "slack", Definition: "changed\n"}, 1, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if err := store.DeleteManagedAlertProvider("slack", 1, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			got, err := store.GetManagedAlertProvider("slack")
			if err != nil {
				t.Fatalf("expected the update and the deletion to be rolled back, got %v", err)
			}
			if got.Version != 1 || got.Definition != "webhook-url: https://hooks.example.com/a\n" {
				t.Errorf("expected the original definition at version 1, got %+v", got)
			}
		})
	}
}
