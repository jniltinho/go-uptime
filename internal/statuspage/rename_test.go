package statuspage

import (
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
)

// Renames of a managed endpoint and changes of the status pages take the locks in the same order, so they never
// deadlock, and the rename keeps the pages pointing to the current key
func TestKeyRename_ConcurrentWithStatusPageChanges(t *testing.T) {
	managedStatusPageStore := initializeSQLiteStore(t)
	cfg := loadTestConfig(t, testConfig)
	createStoredPage(t, managedStatusPageStore, "team", "slug: team\ntitle: Team\nendpoints: [web_site]\nenabled: true\n")
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	t.Cleanup(publicCache.Clear)
	endpoints, pages := managedendpoint.NewService(cfg), NewService()
	definition := func(group string) []byte {
		return []byte("name: site\ngroup: " + group + "\nenabled: false\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n")
	}
	if _, err := endpoints.Create(definition("web"), "ops@example.com"); err != nil {
		t.Fatalf("failed to create the managed endpoint: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			groups := []string{"web", "clientes"}
			for i := 0; i < 20; i++ {
				from, to := groups[i%2], groups[(i+1)%2]
				state := managedendpoint.Get(from + "_site")
				if state == nil {
					t.Errorf("expected %s_site to exist before rename %d", from, i)
					return
				}
				if _, err := endpoints.Update(from+"_site", definition(to), state.Stored.Version, "ops@example.com"); err != nil {
					t.Errorf("rename %d failed: %v", i, err)
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				// A stale version is expected now and then; only deadlocks and lost references matter here
				_, _ = pages.SetEnabled("team", i%2 == 0, findState("team").Stored.Version, "dev@example.com")
			}
		}()
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("timed out: renames and status page changes deadlocked")
	}
	if page := findState("team").Page; page == nil || !reflect.DeepEqual(page.Endpoints, []string{"web_site"}) {
		t.Errorf("expected the team page to select web_site after an even number of renames, got %+v", page)
	}
}

func TestReplaceEndpointKey(t *testing.T) {
	scenarios := []struct {
		name            string
		keys            []string
		expected        []string
		expectedChanged bool
	}{
		{name: "absent", keys: []string{"core_api"}, expected: []string{"core_api"}, expectedChanged: false},
		{name: "replaced-in-place", keys: []string{"core_api", "web_site", "jobs_batch"}, expected: []string{"core_api", "web_new", "jobs_batch"}, expectedChanged: true},
		{name: "new-key-already-present", keys: []string{"web_new", "web_site"}, expected: []string{"web_new"}, expectedChanged: true},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			actual, changed := replaceEndpointKey(scenario.keys, "web_site", "web_new")
			if changed != scenario.expectedChanged || !reflect.DeepEqual(actual, scenario.expected) {
				t.Errorf("expected %v (changed=%v), got %v (changed=%v)", scenario.expected, scenario.expectedChanged, actual, changed)
			}
		})
	}
}

func TestKeyRenameParticipant(t *testing.T) {
	managedStatusPageStore := initializeSQLiteStore(t)
	cfg := loadTestConfig(t, testConfig+"    - slug: keyed\n      title: Keyed\n      endpoints: [web_site]\n")
	createStoredPage(t, managedStatusPageStore, "team", "slug: team\ntitle: Team\nendpoints: [web_site, web_new]\nfeatured: [web_site]\nenabled: true\n")
	createStoredPage(t, managedStatusPageStore, "apps", "slug: apps\ntitle: Apps\ngroups: [core]\nenabled: true\n")
	createStoredPage(t, managedStatusPageStore, "spotlight", "slug: spotlight\ntitle: Spotlight\nfeatured: [core_api]\nenabled: true\n")
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	t.Cleanup(publicCache.Clear)

	plan, err := keyRenameParticipant{}.PrepareKeyRename("web_site", "web_new", "ops@example.com")
	if err != nil {
		t.Fatalf("failed to prepare the rename: %v", err)
	}
	if !reflect.DeepEqual(plan.AffectedConfigStatusPages, []managedendpoint.AffectedStatusPage{{Slug: "keyed", Title: "Keyed"}}) {
		t.Errorf("expected the keyed page of the configuration file to be affected, got %v", plan.AffectedConfigStatusPages)
	}
	if len(plan.StatusPages) != 1 || plan.StatusPages[0].StatusPage.Slug != "team" || plan.StatusPages[0].ExpectedVersion != 1 || plan.StatusPages[0].StatusPage.UpdatedBy != "ops@example.com" {
		t.Fatalf("expected only the team page to be updated at version 1, got %+v", plan.StatusPages)
	}
	page, err := Parse([]byte(plan.StatusPages[0].StatusPage.Definition))
	if err != nil {
		t.Fatalf("expected a valid updated definition, got %v", err)
	}
	if !reflect.DeepEqual(page.Endpoints, []string{"web_new"}) || !reflect.DeepEqual(page.Featured, []string{"web_new"}) || page.Enabled == nil || !*page.Enabled {
		t.Errorf("expected the new key once in endpoints and featured, keeping the page enabled, got %+v", page)
	}
	if mutex.TryLock() {
		mutex.Unlock()
		t.Fatal("expected the mutex to be held until the plan is committed or discarded")
	}
	plan.Discard()
	if !mutex.TryLock() {
		t.Fatal("expected the mutex to be released when the plan is discarded")
	}
	mutex.Unlock()
	if state := findState("team"); !slices.Contains(state.Page.Featured, "web_site") {
		t.Errorf("expected a discarded plan not to change the published page, got %+v", state.Page)
	}

	// Committed plan, with the version set by the storage
	plan, err = keyRenameParticipant{}.PrepareKeyRename("web_site", "web_new", "ops@example.com")
	if err != nil {
		t.Fatalf("failed to prepare the rename: %v", err)
	}
	revision := current.Load().revision
	plan.StatusPages[0].StatusPage.Version = 2
	plan.Commit()
	if !mutex.TryLock() {
		t.Fatal("expected the mutex to be released when the plan is committed")
	}
	mutex.Unlock()
	state := findState("team")
	if state.Stored.Version != 2 || !reflect.DeepEqual(state.Page.Featured, []string{"web_new"}) || current.Load().revision <= revision {
		t.Errorf("expected the updated page to be published with a new revision, got %+v (stored %+v)", state.Page, state.Stored)
	}

	// The exposure also considers the featured endpoints
	exposure, err := NewService().Exposure("", "core_api")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range exposure.StatusPages {
		if item.Slug == "spotlight" && item.Reason == "key" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the spotlight page to expose core_api as featured, got %+v", exposure.StatusPages)
	}
}
