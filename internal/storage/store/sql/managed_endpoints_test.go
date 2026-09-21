// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

const (
	managedEndpointTestGroup      = "managed-test"
	managedEndpointTestStatusPage = "managed-test-page"
)

// managedEndpointTestStores returns a SQLite store and, when GO_UPTIME_TEST_POSTGRES_URL, GO_UPTIME_TEST_MYSQL_URL or
// GO_UPTIME_TEST_MARIADB_URL are set, a PostgreSQL, MySQL or MariaDB store. MySQL and MariaDB stores use a database of their
// own, removed when the test ends.
func managedEndpointTestStores(t *testing.T) map[string]*Store {
	t.Helper()
	stores := make(map[string]*Store)
	sqliteStore, err := NewStore("sqlite", filepath.Join(t.TempDir(), "managed.db"), true, storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	t.Cleanup(sqliteStore.Close)
	stores["sqlite"] = sqliteStore
	if postgresURL := os.Getenv("GO_UPTIME_TEST_POSTGRES_URL"); postgresURL == "" {
		t.Log("GO_UPTIME_TEST_POSTGRES_URL is not set, skipping PostgreSQL")
	} else {
		postgresStore, err := NewStore("postgres", postgresURL, true, storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to create postgres store: %v", err)
		}
		t.Cleanup(postgresStore.Close)
		for _, query := range []string{"DELETE FROM managed_endpoints", "DELETE FROM endpoints WHERE endpoint_group = '" + managedEndpointTestGroup + "'", "DELETE FROM managed_status_pages WHERE slug = '" + managedEndpointTestStatusPage + "'"} {
			if _, err := postgresStore.db.Exec(query); err != nil {
				t.Fatalf("failed to clean postgres database: %v", err)
			}
		}
		stores["postgres"] = postgresStore
	}
	for name, dsn := range mysqlTestServers() {
		mysqlStore, err := NewStore(driverMySQL, newMySQLTestDatabase(t, dsn), true, storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to create %s store: %v", name, err)
		}
		t.Cleanup(mysqlStore.Close)
		stores[name] = mysqlStore
	}
	return stores
}

func newManagedEndpointTestEndpoint() *endpoint.Endpoint {
	return &endpoint.Endpoint{Name: "api", Group: managedEndpointTestGroup, URL: "https://example.org"}
}

func TestStore_ManagedEndpoints(t *testing.T) {
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			key := newManagedEndpointTestEndpoint().Key()
			created := &common.ManagedEndpoint{Key: key, Definition: "name: api\n", UpdatedBy: "ops@example.com"}
			if err := store.CreateManagedEndpoint(created, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			if created.Version != 1 || created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
				t.Errorf("expected version 1 and matching timestamps, got version=%d createdAt=%s updatedAt=%s", created.Version, created.CreatedAt, created.UpdatedAt)
			}
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: "name: api\n"}, nil); !errors.Is(err, common.ErrManagedEndpointAlreadyExists) {
				t.Errorf("expected ErrManagedEndpointAlreadyExists, got %v", err)
			}
			fetched, err := store.GetManagedEndpoint(key)
			if err != nil {
				t.Fatalf("failed to get managed endpoint: %v", err)
			}
			if fetched.Definition != created.Definition || fetched.Version != 1 || fetched.UpdatedBy != "ops@example.com" || !fetched.CreatedAt.Equal(created.CreatedAt) {
				t.Errorf("unexpected managed endpoint: %+v", fetched)
			}
			if list, err := store.ListManagedEndpoints(); err != nil || len(list) != 1 || list[0].Key != key {
				t.Errorf("expected a list with the managed endpoint, got %v (err=%v)", list, err)
			}

			time.Sleep(2 * time.Millisecond)
			updated := &common.ManagedEndpoint{Key: key, Definition: "name: api\ninterval: 5m\n", UpdatedBy: "dev@example.com"}
			if err := store.UpdateManagedEndpoint(updated, 2, nil); !errors.Is(err, common.ErrManagedEndpointVersionMismatch) {
				t.Errorf("expected ErrManagedEndpointVersionMismatch, got %v", err)
			}
			if err := store.UpdateManagedEndpoint(updated, 1, nil); err != nil {
				t.Fatalf("failed to update managed endpoint: %v", err)
			}
			if updated.Version != 2 || !updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.After(created.UpdatedAt) {
				t.Errorf("expected version 2, same creation time and a later update time, got %+v", updated)
			}
			if fetched, err := store.GetManagedEndpoint(key); err != nil || fetched.Definition != updated.Definition || fetched.Version != 2 || fetched.UpdatedBy != "dev@example.com" {
				t.Errorf("expected the updated managed endpoint, got %+v (err=%v)", fetched, err)
			}
			if err := store.UpdateManagedEndpoint(&common.ManagedEndpoint{Key: "unknown", Definition: "name: x\n"}, 1, nil); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Errorf("expected ErrManagedEndpointNotFound, got %v", err)
			}

			if err := store.DeleteManagedEndpoint(key, 1, false, nil); !errors.Is(err, common.ErrManagedEndpointVersionMismatch) {
				t.Errorf("expected ErrManagedEndpointVersionMismatch, got %v", err)
			}
			if err := store.DeleteManagedEndpoint(key, 2, false, nil); err != nil {
				t.Fatalf("failed to delete managed endpoint: %v", err)
			}
			if _, err := store.GetManagedEndpoint(key); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Errorf("expected ErrManagedEndpointNotFound after deletion, got %v", err)
			}
			if err := store.DeleteManagedEndpoint(key, 2, false, nil); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Errorf("expected ErrManagedEndpointNotFound when deleting twice, got %v", err)
			}
		})
	}
}

func TestStore_ManagedEndpoints_ApplyErrorRollsBack(t *testing.T) {
	errApply := errors.New("failed to apply")
	failingApply := func() error { return errApply }
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			key := newManagedEndpointTestEndpoint().Key()
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: "name: api\n"}, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if _, err := store.GetManagedEndpoint(key); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Fatalf("expected the creation to be rolled back, got %v", err)
			}
			applied := false
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: "name: api\n"}, func() error { applied = true; return nil }); err != nil || !applied {
				t.Fatalf("expected the creation to be applied, got err=%v applied=%v", err, applied)
			}
			if err := store.UpdateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: "name: changed\n"}, 1, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if fetched, err := store.GetManagedEndpoint(key); err != nil || fetched.Definition != "name: api\n" || fetched.Version != 1 {
				t.Errorf("expected the update to be rolled back, got %+v (err=%v)", fetched, err)
			}
			if err := store.DeleteManagedEndpoint(key, 1, true, failingApply); !errors.Is(err, errApply) {
				t.Fatalf("expected the apply error, got %v", err)
			}
			if _, err := store.GetManagedEndpoint(key); err != nil {
				t.Errorf("expected the deletion to be rolled back, got %v", err)
			}
		})
	}
}

func TestStore_DeleteManagedEndpoint_EndpointData(t *testing.T) {
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			ep := newManagedEndpointTestEndpoint()
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: time.Millisecond}); err != nil {
				t.Fatalf("failed to insert result: %v", err)
			}
			params := paging.NewEndpointStatusParams().WithResults(1, 20)
			if status, err := store.GetEndpointStatusByKey(ep.Key(), params); err != nil || len(status.Results) != 1 {
				t.Fatalf("expected 1 result before deletion, got status=%v err=%v", status, err)
			}
			// Deleting only the definition, as for a managed endpoint in conflict with the configuration file
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: ep.Key(), Definition: "name: api\n"}, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			if err := store.DeleteManagedEndpoint(ep.Key(), 1, false, nil); err != nil {
				t.Fatalf("failed to delete managed endpoint: %v", err)
			}
			if status, err := store.GetEndpointStatusByKey(ep.Key(), params); err != nil || len(status.Results) != 1 {
				t.Errorf("expected the endpoint data to be kept, got status=%v err=%v", status, err)
			}
			// Deleting the definition and the endpoint data
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: ep.Key(), Definition: "name: api\n"}, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			if err := store.DeleteManagedEndpoint(ep.Key(), 1, true, nil); err != nil {
				t.Fatalf("failed to delete managed endpoint: %v", err)
			}
			if _, err := store.GetEndpointStatusByKey(ep.Key(), params); !errors.Is(err, common.ErrEndpointNotFound) {
				t.Errorf("expected ErrEndpointNotFound once the endpoint data is deleted, got %v", err)
			}
		})
	}
}

func TestStore_RenameManagedEndpoint(t *testing.T) {
	errApply := errors.New("failed to apply")
	enabled := true
	triggeredAlert := &alert.Alert{Type: alert.TypeCustom, Enabled: &enabled, FailureThreshold: 1, SuccessThreshold: 1, Triggered: true, ResolveKey: "incident-1"}
	params := paging.NewEndpointStatusParams().WithResults(1, 20).WithEvents(1, 20)
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			original := newManagedEndpointTestEndpoint()
			renamed := &endpoint.Endpoint{Name: "renamed", Group: managedEndpointTestGroup, URL: original.URL}
			for i, success := range []bool{true, false} {
				result := &endpoint.Result{Success: success, Timestamp: time.Now().Add(time.Duration(i-2) * time.Minute), Duration: time.Millisecond}
				if err := store.InsertEndpointResult(original, result); err != nil {
					t.Fatalf("failed to insert result: %v", err)
				}
			}
			if err := store.UpsertTriggeredEndpointAlert(original, triggeredAlert); err != nil {
				t.Fatalf("failed to upsert triggered alert: %v", err)
			}
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: original.Key(), Definition: "name: api\n"}, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			before, err := store.GetEndpointStatusByKey(original.Key(), params)
			if err != nil || len(before.Results) != 2 {
				t.Fatalf("expected 2 results before the rename, got status=%v err=%v", before, err)
			}

			managed := &common.ManagedEndpoint{Key: renamed.Key(), Definition: "name: renamed\n", UpdatedBy: "ops@example.com"}
			if err := store.RenameManagedEndpoint(managed, 1, &common.ManagedEndpointRename{OldKey: original.Key(), Name: renamed.Name, Group: renamed.Group, MoveHistory: true}, nil); err != nil {
				t.Fatalf("failed to rename managed endpoint: %v", err)
			}
			if managed.Version != 2 || managed.CreatedAt.IsZero() {
				t.Errorf("expected version 2 with the creation time, got %+v", managed)
			}
			if _, err := store.GetManagedEndpoint(original.Key()); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Errorf("expected no managed endpoint under the old key, got %v", err)
			}
			if fetched, err := store.GetManagedEndpoint(renamed.Key()); err != nil || fetched.Definition != managed.Definition || fetched.Version != 2 || fetched.UpdatedBy != "ops@example.com" {
				t.Errorf("expected the managed endpoint under the new key, got %+v (err=%v)", fetched, err)
			}
			after, err := store.GetEndpointStatusByKey(renamed.Key(), params)
			if err != nil || after.Name != "renamed" || len(after.Results) != len(before.Results) || len(after.Events) != len(before.Events) {
				t.Errorf("expected the history under the new key, got status=%+v err=%v (before: %d results, %d events)", after, err, len(before.Results), len(before.Events))
			}
			if _, err := store.GetEndpointStatusByKey(original.Key(), params); !errors.Is(err, common.ErrEndpointNotFound) {
				t.Errorf("expected no status under the old key, got %v", err)
			}
			if _, err := store.GetUptimeByKey(renamed.Key(), time.Now().Add(-time.Hour), time.Now()); err != nil {
				t.Errorf("expected the uptime under the new key, got %v", err)
			}
			if exists, resolveKey, _, err := store.GetTriggeredEndpointAlert(renamed, triggeredAlert); err != nil || !exists || resolveKey != "incident-1" {
				t.Errorf("expected the triggered alert under the new key, got exists=%v resolveKey=%s err=%v", exists, resolveKey, err)
			}

			// The new key is used by another managed endpoint
			other := &endpoint.Endpoint{Name: "other", Group: managedEndpointTestGroup}
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: other.Key(), Definition: "name: other\n"}, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			err = store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: other.Key(), Definition: "name: other\n"}, 2, &common.ManagedEndpointRename{OldKey: renamed.Key(), Name: other.Name, Group: other.Group, MoveHistory: true}, nil)
			if !errors.Is(err, common.ErrEndpointKeyInUse) {
				t.Errorf("expected ErrEndpointKeyInUse for the key of another managed endpoint, got %v", err)
			}
			// The new key has endpoint data without a definition
			orphan := &endpoint.Endpoint{Name: "orphan", Group: managedEndpointTestGroup}
			if err := store.InsertEndpointResult(orphan, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: time.Millisecond}); err != nil {
				t.Fatalf("failed to insert result: %v", err)
			}
			err = store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: orphan.Key(), Definition: "name: orphan\n"}, 2, &common.ManagedEndpointRename{OldKey: renamed.Key(), Name: orphan.Name, Group: orphan.Group, MoveHistory: true}, nil)
			if !errors.Is(err, common.ErrEndpointKeyInUse) {
				t.Errorf("expected ErrEndpointKeyInUse for a key with stored data, got %v", err)
			}
			if status, err := store.GetEndpointStatusByKey(orphan.Key(), params); err != nil || len(status.Results) != 1 {
				t.Errorf("expected the data of the used key to be kept, got status=%v err=%v", status, err)
			}
			// Outdated version, and an apply error rolls the rename back
			third := &endpoint.Endpoint{Name: "third", Group: managedEndpointTestGroup}
			thirdRename := &common.ManagedEndpointRename{OldKey: renamed.Key(), Name: third.Name, Group: third.Group, MoveHistory: true}
			if err := store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: third.Key(), Definition: "name: third\n"}, 1, thirdRename, nil); !errors.Is(err, common.ErrManagedEndpointVersionMismatch) {
				t.Errorf("expected ErrManagedEndpointVersionMismatch, got %v", err)
			}
			if err := store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: third.Key(), Definition: "name: third\n"}, 2, thirdRename, func() error { return errApply }); !errors.Is(err, errApply) {
				t.Errorf("expected the apply error, got %v", err)
			}
			if _, err := store.GetManagedEndpoint(third.Key()); !errors.Is(err, common.ErrManagedEndpointNotFound) {
				t.Errorf("expected the failed renames to be rolled back, got %v", err)
			}
			if status, err := store.GetEndpointStatusByKey(renamed.Key(), params); err != nil || len(status.Results) != 2 {
				t.Errorf("expected the history to stay under the current key, got status=%v err=%v", status, err)
			}
			// Same key with a different name text
			sameKey := &common.ManagedEndpoint{Key: renamed.Key(), Definition: "name: Renamed\n"}
			if err := store.RenameManagedEndpoint(sameKey, 2, &common.ManagedEndpointRename{OldKey: renamed.Key(), Name: "Renamed", Group: renamed.Group, MoveHistory: true}, nil); err != nil {
				t.Fatalf("failed to change the name text: %v", err)
			}
			if status, err := store.GetEndpointStatusByKey(renamed.Key(), params); err != nil || status.Name != "Renamed" || len(status.Results) != 2 || sameKey.Version != 3 {
				t.Errorf("expected the new name with the same history at version 3, got status=%+v err=%v version=%d", status, err, sameKey.Version)
			}
		})
	}
}

func TestStore_RenameManagedEndpoint_StatusPagesAndDefinitionOnly(t *testing.T) {
	params := paging.NewEndpointStatusParams().WithResults(1, 20)
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			ep := newManagedEndpointTestEndpoint()
			moved := &endpoint.Endpoint{Name: "moved", Group: managedEndpointTestGroup}
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: time.Millisecond}); err != nil {
				t.Fatalf("failed to insert result: %v", err)
			}
			if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: ep.Key(), Definition: "name: api\n"}, nil); err != nil {
				t.Fatalf("failed to create managed endpoint: %v", err)
			}
			page := &common.ManagedStatusPage{Slug: managedEndpointTestStatusPage, Definition: "slug: managed-test-page\nendpoints: [managed-test_api]\n", UpdatedBy: "ops@example.com"}
			if err := store.CreateManagedStatusPage(page, nil); err != nil {
				t.Fatalf("failed to create managed status page: %v", err)
			}
			movedDefinition := "slug: managed-test-page\nendpoints: [managed-test_moved]\n"

			// A status page changed since it was read makes the whole rename fail
			stale := &common.ManagedStatusPageUpdate{StatusPage: &common.ManagedStatusPage{Slug: page.Slug, Definition: movedDefinition, UpdatedBy: "dev@example.com"}, ExpectedVersion: 2}
			err := store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: moved.Key(), Definition: "name: moved\n"}, 1, &common.ManagedEndpointRename{OldKey: ep.Key(), Name: moved.Name, Group: moved.Group, StatusPages: []*common.ManagedStatusPageUpdate{stale}}, nil)
			if !errors.Is(err, common.ErrManagedStatusPageVersionMismatch) {
				t.Fatalf("expected ErrManagedStatusPageVersionMismatch, got %v", err)
			}
			if fetched, err := store.GetManagedEndpoint(ep.Key()); err != nil || fetched.Version != 1 {
				t.Errorf("expected the rename to be rolled back, got %+v (err=%v)", fetched, err)
			}

			// Definition only, as for a managed endpoint in conflict with the configuration file, with a status page
			update := &common.ManagedStatusPageUpdate{StatusPage: &common.ManagedStatusPage{Slug: page.Slug, Definition: movedDefinition, UpdatedBy: "dev@example.com"}, ExpectedVersion: 1}
			if err := store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: moved.Key(), Definition: "name: moved\n"}, 1, &common.ManagedEndpointRename{OldKey: ep.Key(), Name: moved.Name, Group: moved.Group, StatusPages: []*common.ManagedStatusPageUpdate{update}}, nil); err != nil {
				t.Fatalf("failed to rename managed endpoint: %v", err)
			}
			if update.StatusPage.Version != 2 || update.StatusPage.UpdatedAt.IsZero() {
				t.Errorf("expected the status page update to be at version 2, got %+v", update.StatusPage)
			}
			if fetched, err := store.GetManagedStatusPage(page.Slug); err != nil || fetched.Definition != movedDefinition || fetched.Version != 2 || fetched.UpdatedBy != "dev@example.com" {
				t.Errorf("expected the status page to be updated, got %+v (err=%v)", fetched, err)
			}
			if status, err := store.GetEndpointStatusByKey(ep.Key(), params); err != nil || len(status.Results) != 1 {
				t.Errorf("expected the data of the old key to be kept, got status=%v err=%v", status, err)
			}
			if _, err := store.GetEndpointStatusByKey(moved.Key(), params); !errors.Is(err, common.ErrEndpointNotFound) {
				t.Errorf("expected no data under the new key, got %v", err)
			}
		})
	}
}

func TestNewStore_ManagedEndpointsSchemaIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idempotent.db")
	for i := 0; i < 2; i++ {
		store, err := NewStore("sqlite", path, false, storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
		if err != nil {
			t.Fatalf("failed to open the store (attempt %d): %v", i+1, err)
		}
		store.Close()
	}
}
