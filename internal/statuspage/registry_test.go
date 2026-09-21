package statuspage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const testConfig = `
endpoints:
  - name: api
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: off
    group: core
    enabled: false
    url: https://example.org
    conditions: ["[STATUS] == 200"]
external-endpoints:
  - name: batch
    group: jobs
    token: secret
status-pages:
  pages:
    - slug: infra
      title: Infra
      groups: [core]
    - slug: hidden
      title: Hidden
      groups: [core]
      enabled: false
`

func loadTestConfig(t *testing.T, yaml string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfiguration(path)
	if err != nil {
		t.Fatalf("failed to load configuration: %v", err)
	}
	return cfg
}

func initializeSQLiteStore(t *testing.T) store.ManagedStatusPageStore {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "gatus.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() { store.Get().Close() })
	managedStatusPageStore, ok := store.GetManagedStatusPageStore()
	if !ok {
		t.Fatal("expected the sqlite store to support managed status pages")
	}
	return managedStatusPageStore
}

func createStoredPage(t *testing.T, managedStatusPageStore store.ManagedStatusPageStore, slug, definition string) {
	t.Helper()
	if err := managedStatusPageStore.CreateManagedStatusPage(&common.ManagedStatusPage{Slug: slug, Definition: definition}, nil); err != nil {
		t.Fatalf("failed to create managed status page %s: %v", slug, err)
	}
}

func TestLoad_MemoryStorage(t *testing.T) {
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, testConfig)
	Load(cfg)
	if !IsEnabled() || IsManagedUnavailable() {
		t.Errorf("expected status pages to be enabled and available, got enabled=%v managedUnavailable=%v", IsEnabled(), IsManagedUnavailable())
	}
	published, ok := Lookup("infra")
	if !ok || published.Page.Title != "Infra" || published.Revision == 0 {
		t.Errorf("expected the infra page to be published, got %+v (ok=%v)", published, ok)
	}
	if _, ok := Lookup("hidden"); ok {
		t.Error("expected the disabled page of the configuration file not to be published")
	}
	if _, ok := Lookup("missing"); ok {
		t.Error("expected an unknown slug not to be published")
	}
	if list := List(); len(list) != 2 || list[0].Slug != "hidden" || list[1].Slug != "infra" {
		t.Errorf("expected the two pages of the configuration file ordered by slug, got %+v", list)
	}
	refs := Endpoints()
	if len(refs) != 2 || refs[0].Key != "core_api" || refs[1].Key != "jobs_batch" {
		t.Errorf("expected the enabled endpoint and the external endpoint, got %+v", refs)
	}
	previousRevision := published.Revision
	Load(cfg)
	if published, _ := Lookup("infra"); published.Revision <= previousRevision {
		t.Errorf("expected a new revision after Load, got %d then %d", previousRevision, published.Revision)
	}
}

func TestLoad_ManagedStatusPages(t *testing.T) {
	managedStatusPageStore := initializeSQLiteStore(t)
	createStoredPage(t, managedStatusPageStore, "apps", "slug: apps\ntitle: Apps\ngroups: [web]\nenabled: true\n")
	createStoredPage(t, managedStatusPageStore, "draft", "slug: draft\ntitle: Draft\ngroups: [web]\n")
	createStoredPage(t, managedStatusPageStore, "infra", "slug: infra\ntitle: Managed infra\ngroups: [web]\nenabled: true\n")
	createStoredPage(t, managedStatusPageStore, "broken", "slug: broken\ntitle: Broken\n")
	createStoredPage(t, managedStatusPageStore, "renamed", "slug: other\ntitle: Renamed\ngroups: [web]\nenabled: true\n")
	managedEndpointStore, _ := store.GetManagedEndpointStore()
	if err := managedEndpointStore.CreateManagedEndpoint(&common.ManagedEndpoint{Key: "web_site", Definition: "name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"}, nil); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, testConfig)
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	if published, ok := Lookup("apps"); !ok || published.Page.Title != "Apps" {
		t.Errorf("expected the enabled managed page to be published, got %+v (ok=%v)", published, ok)
	}
	if _, ok := Lookup("draft"); ok {
		t.Error("expected a managed page without enabled not to be published")
	}
	if published, ok := Lookup("infra"); !ok || published.Page.Title != "Infra" {
		t.Errorf("expected the page of the configuration file to win the slug conflict, got %+v (ok=%v)", published, ok)
	}
	states := make(map[string]*State)
	for _, state := range List() {
		if state.Origin == OriginAdmin {
			states[state.Slug] = state
		}
	}
	if state := states["infra"]; state == nil || !state.InConflict() || state.IsPublished() {
		t.Errorf("expected the managed infra page to be in conflict, got %+v", state)
	}
	if state := states["broken"]; state == nil || state.Page != nil || !errors.Is(state.Err, pageconfig.ErrEmptySelection) {
		t.Errorf("expected the broken page to be invalid, got %+v", state)
	}
	if state := states["renamed"]; state == nil || state.Page != nil || !errors.Is(state.Err, ErrInvalidDefinition) {
		t.Errorf("expected a definition with another slug to be invalid, got %+v", state)
	}
	found := false
	for _, ref := range Endpoints() {
		found = found || (ref.Key == "web_site" && ref.Group == "web")
	}
	if !found {
		t.Error("expected the managed endpoint to be selectable")
	}
}

func TestLoad_Disabled(t *testing.T) {
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	Load(loadTestConfig(t, testConfig+"  enabled: false\n"))
	if IsEnabled() {
		t.Error("expected status pages to be disabled")
	}
	if _, ok := Lookup("infra"); ok {
		t.Error("expected no page to be published when status pages are disabled")
	}
	if len(List()) != 2 {
		t.Error("expected the pages to still be listed for the administration")
	}
}

func TestLoad_ManagedStatusPagesUnavailable(t *testing.T) {
	initializeSQLiteStore(t)
	store.Get().Close()
	Load(loadTestConfig(t, testConfig))
	if !IsManagedUnavailable() {
		t.Error("expected the managed status pages to be unavailable")
	}
	if _, ok := Lookup("infra"); !ok {
		t.Error("expected the pages of the configuration file to be published")
	}
}
