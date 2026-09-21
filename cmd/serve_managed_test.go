// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

// The history of managed endpoints must be preserved on startup and reload, even without admin.enabled
func TestInitializeStorage_PreservesManagedEndpointHistory(t *testing.T) {
	cfg := &config.Config{Storage: &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	managed := &endpoint.Endpoint{Name: "site", Group: "web", URL: "https://example.org"}
	orphan := &endpoint.Endpoint{Name: "orphan", Group: "web", URL: "https://example.org"}
	for _, ep := range []*endpoint.Endpoint{managed, orphan} {
		if err := store.Get().InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now()}); err != nil {
			t.Fatalf("failed to insert result: %v", err)
		}
	}
	managedEndpointStore, _ := store.GetManagedEndpointStore()
	if err := managedEndpointStore.CreateManagedEndpoint(&common.ManagedEndpoint{Key: managed.Key(), Definition: "name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"}, nil); err != nil {
		t.Fatalf("failed to create managed endpoint: %v", err)
	}
	store.Get().Close()

	initializeStorage(cfg)
	defer store.Get().Close()

	params := paging.NewEndpointStatusParams().WithResults(1, 20)
	if status, err := store.Get().GetEndpointStatusByKey(managed.Key(), params); err != nil || len(status.Results) != 1 {
		t.Errorf("expected the history of the managed endpoint to be preserved, got status=%v err=%v", status, err)
	}
	if _, err := store.Get().GetEndpointStatusByKey(orphan.Key(), params); err == nil {
		t.Error("expected the history of an endpoint that is neither configured nor managed to be deleted")
	}
	if managedendpoint.EndpointByKey(managed.Key()) == nil {
		t.Error("expected the managed endpoint to be loaded")
	}
}

// The history of a renamed managed endpoint must be preserved under its new key on startup and reload (fork)
func TestInitializeStorage_PreservesRenamedManagedEndpointHistory(t *testing.T) {
	cfg := &config.Config{Storage: &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	original := &endpoint.Endpoint{Name: "site", Group: "web", URL: "https://example.org"}
	renamed := &endpoint.Endpoint{Name: "site", Group: "clientes", URL: "https://example.org"}
	for i := 0; i < 3; i++ {
		if err := store.Get().InsertEndpointResult(original, &endpoint.Result{Success: true, Timestamp: time.Now().Add(time.Duration(i-3) * time.Minute)}); err != nil {
			t.Fatalf("failed to insert result: %v", err)
		}
	}
	managedEndpointStore, _ := store.GetManagedEndpointStore()
	if err := managedEndpointStore.CreateManagedEndpoint(&common.ManagedEndpoint{Key: original.Key(), Definition: "name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"}, nil); err != nil {
		t.Fatalf("failed to create managed endpoint: %v", err)
	}
	rename := &common.ManagedEndpointRename{OldKey: original.Key(), Name: renamed.Name, Group: renamed.Group, MoveHistory: true}
	if err := managedEndpointStore.RenameManagedEndpoint(&common.ManagedEndpoint{Key: renamed.Key(), Definition: "name: site\ngroup: clientes\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"}, 1, rename, nil); err != nil {
		t.Fatalf("failed to rename managed endpoint: %v", err)
	}
	store.Get().Close()

	initializeStorage(cfg)
	defer store.Get().Close()

	params := paging.NewEndpointStatusParams().WithResults(1, 20)
	if status, err := store.Get().GetEndpointStatusByKey(renamed.Key(), params); err != nil || len(status.Results) != 3 || status.Group != "clientes" {
		t.Errorf("expected the history of the renamed managed endpoint to be preserved, got status=%v err=%v", status, err)
	}
	if _, err := store.Get().GetEndpointStatusByKey(original.Key(), params); err == nil {
		t.Error("expected no history under the old key")
	}
	if managedendpoint.EndpointByKey(renamed.Key()) == nil || managedendpoint.EndpointByKey(original.Key()) != nil {
		t.Error("expected the managed endpoint to be loaded under its new key only")
	}
}
