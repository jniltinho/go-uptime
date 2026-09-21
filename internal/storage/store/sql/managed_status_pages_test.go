// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// managedStatusPageTestStores returns the stores of managedEndpointTestStores with an empty managed_status_pages table
func managedStatusPageTestStores(t *testing.T) map[string]*Store {
	t.Helper()
	stores := managedEndpointTestStores(t)
	for driver, store := range stores {
		if _, err := store.db.Exec("DELETE FROM managed_status_pages"); err != nil {
			t.Fatalf("failed to clean managed_status_pages with %s: %v", driver, err)
		}
	}
	return stores
}

func TestStore_ManagedStatusPages(t *testing.T) {
	for driver, store := range managedStatusPageTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			created := &common.ManagedStatusPage{Slug: "infra", Definition: "slug: infra\n", UpdatedBy: "ops@example.com"}
			if err := store.CreateManagedStatusPage(created, nil); err != nil {
				t.Fatalf("failed to create managed status page: %v", err)
			}
			if created.Version != 1 || created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
				t.Errorf("expected version 1 and matching timestamps, got version=%d createdAt=%s updatedAt=%s", created.Version, created.CreatedAt, created.UpdatedAt)
			}
			if err := store.CreateManagedStatusPage(&common.ManagedStatusPage{Slug: "infra", Definition: "slug: infra\n"}, nil); !errors.Is(err, common.ErrManagedStatusPageAlreadyExists) {
				t.Errorf("expected ErrManagedStatusPageAlreadyExists, got %v", err)
			}
			if err := store.CreateManagedStatusPage(&common.ManagedStatusPage{Slug: "apps", Definition: "slug: apps\n"}, nil); err != nil {
				t.Fatalf("failed to create second managed status page: %v", err)
			}
			list, err := store.ListManagedStatusPages()
			if err != nil {
				t.Fatalf("failed to list managed status pages: %v", err)
			}
			if len(list) != 2 || list[0].Slug != "apps" || list[1].Slug != "infra" {
				t.Fatalf("expected apps and infra ordered by slug, got %+v", list)
			}
			updated := &common.ManagedStatusPage{Slug: "infra", Definition: "slug: infra\ntitle: Infra\n", UpdatedBy: "admin"}
			if err := store.UpdateManagedStatusPage(updated, 1, nil); err != nil {
				t.Fatalf("failed to update managed status page: %v", err)
			}
			if updated.Version != 2 || !updated.CreatedAt.Equal(created.CreatedAt) {
				t.Errorf("expected version 2 and the original creation time, got version=%d createdAt=%s", updated.Version, updated.CreatedAt)
			}
			if err := store.UpdateManagedStatusPage(updated, 1, nil); !errors.Is(err, common.ErrManagedStatusPageVersionMismatch) {
				t.Errorf("expected ErrManagedStatusPageVersionMismatch, got %v", err)
			}
			if err := store.UpdateManagedStatusPage(&common.ManagedStatusPage{Slug: "missing"}, 1, nil); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
				t.Errorf("expected ErrManagedStatusPageNotFound, got %v", err)
			}
			got, err := store.GetManagedStatusPage("infra")
			if err != nil {
				t.Fatalf("failed to get managed status page: %v", err)
			}
			if got.Definition != updated.Definition || got.Version != 2 || got.UpdatedBy != "admin" {
				t.Errorf("unexpected managed status page: %+v", got)
			}
			if _, err := store.GetManagedStatusPage("missing"); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
				t.Errorf("expected ErrManagedStatusPageNotFound, got %v", err)
			}
			if err := store.DeleteManagedStatusPage("infra", 1, nil); !errors.Is(err, common.ErrManagedStatusPageVersionMismatch) {
				t.Errorf("expected ErrManagedStatusPageVersionMismatch, got %v", err)
			}
			if err := store.DeleteManagedStatusPage("missing", 1, nil); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
				t.Errorf("expected ErrManagedStatusPageNotFound, got %v", err)
			}
			if err := store.DeleteManagedStatusPage("infra", 2, nil); err != nil {
				t.Fatalf("failed to delete managed status page: %v", err)
			}
			if _, err := store.GetManagedStatusPage("infra"); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
				t.Errorf("expected the managed status page to be deleted, got %v", err)
			}
		})
	}
}

func TestStore_ManagedStatusPagesApplyRollsBack(t *testing.T) {
	for driver, store := range managedStatusPageTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			errApply := errors.New("apply failed")
			failingApply := func() error { return errApply }
			if err := store.CreateManagedStatusPage(&common.ManagedStatusPage{Slug: "infra", Definition: "slug: infra\n"}, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if _, err := store.GetManagedStatusPage("infra"); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
				t.Fatalf("expected the creation to be rolled back, got %v", err)
			}
			page := &common.ManagedStatusPage{Slug: "infra", Definition: "slug: infra\n"}
			if err := store.CreateManagedStatusPage(page, nil); err != nil {
				t.Fatalf("failed to create managed status page: %v", err)
			}
			if err := store.UpdateManagedStatusPage(&common.ManagedStatusPage{Slug: "infra", Definition: "changed\n"}, 1, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if err := store.DeleteManagedStatusPage("infra", 1, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			got, err := store.GetManagedStatusPage("infra")
			if err != nil {
				t.Fatalf("expected the update and the deletion to be rolled back, got %v", err)
			}
			if got.Version != 1 || got.Definition != "slug: infra\n" {
				t.Errorf("expected the original definition at version 1, got %+v", got)
			}
		})
	}
}
