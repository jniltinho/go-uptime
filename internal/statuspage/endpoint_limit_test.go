package statuspage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

// endpointLimitTestConfig has four endpoints in one group, in the display order alpha, bravo, charlie, delta, and a
// page that selects them all. The limit is appended by limitTestConfig.
const endpointLimitTestConfig = `
endpoints:
  - name: alpha
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: bravo
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: charlie
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: delta
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
status-pages:
  pages:
    - slug: infra
      title: Infra
      groups: [core]
`

func limitTestConfig(limit int) string {
	if limit == 0 {
		return endpointLimitTestConfig
	}
	return strings.Replace(endpointLimitTestConfig, "status-pages:\n", fmt.Sprintf("status-pages:\n  maximum-endpoints-per-page: %d\n", limit), 1)
}

func loadWithLimit(t *testing.T, limit int) {
	t.Helper()
	cfg := loadTestConfig(t, limitTestConfig(limit))
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
}

func setupEndpointLimitTest(t *testing.T, limit int) {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	loadWithLimit(t, limit)
	t.Cleanup(func() {
		publicCache.Clear()
		chartCache.Clear()
	})
}

func publicPayloadOf(t *testing.T, slug string) Payload {
	t.Helper()
	body, err := PublicPage(slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func shownNames(payload Payload) string {
	var names []string
	for _, featured := range payload.Featured {
		names = append(names, "*"+featured.Name)
	}
	for _, group := range payload.Groups {
		for _, shown := range group.Endpoints {
			names = append(names, shown.Name)
		}
	}
	return strings.Join(names, ",")
}

func manyRefs(count int, group func(i int) string) []EndpointRef {
	refs := make([]EndpointRef, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("ep-%04d", i)
		refs = append(refs, EndpointRef{Key: group(i) + "_" + name, Name: name, Group: group(i)})
	}
	return refs
}

func TestSelect_Limit(t *testing.T) {
	oneGroup := func(int) string { return "core" }
	t.Run("default", func(t *testing.T) {
		selection := Select(&pageconfig.Page{Groups: []string{"core"}}, manyRefs(450, oneGroup), pageconfig.DefaultMaximumEndpointsPerPage)
		if keys := selection.Keys(); len(keys) != 400 || !selection.Truncated || keys[399] != "core_ep-0399" {
			t.Errorf("expected the first 400 endpoints, got %d (truncated=%v)", len(keys), selection.Truncated)
		}
	})
	t.Run("limit-above-the-selection", func(t *testing.T) {
		selection := Select(&pageconfig.Page{Groups: []string{"core"}}, manyRefs(650, oneGroup), 1000)
		if keys := selection.Keys(); len(keys) != 650 || selection.Truncated {
			t.Errorf("expected the 650 endpoints without truncation, got %d (truncated=%v)", len(keys), selection.Truncated)
		}
	})
	t.Run("limit-equal-to-the-selection", func(t *testing.T) {
		selection := Select(&pageconfig.Page{Groups: []string{"core"}}, manyRefs(200, oneGroup), 200)
		if keys := selection.Keys(); len(keys) != 200 || selection.Truncated {
			t.Errorf("expected the 200 endpoints without truncation, got %d (truncated=%v)", len(keys), selection.Truncated)
		}
	})
	t.Run("limit-of-one-with-several-featured", func(t *testing.T) {
		page := &pageconfig.Page{Groups: []string{"core"}, Featured: []string{"core_ep-0002", "core_ep-0001"}}
		selection := Select(page, manyRefs(5, oneGroup), 1)
		if keys := selection.Keys(); len(keys) != 1 || keys[0] != "core_ep-0002" || !selection.Truncated || len(selection.Sections) != 0 {
			t.Errorf("expected only the first featured endpoint, got %v (truncated=%v, sections=%d)", keys, selection.Truncated, len(selection.Sections))
		}
	})
	t.Run("featured-first-then-the-sections", func(t *testing.T) {
		page := &pageconfig.Page{Groups: []string{"core"}, Featured: []string{"core_ep-0004", "core_ep-0003"}}
		selection := Select(page, manyRefs(5, oneGroup), 3)
		if keys := strings.Join(selection.Keys(), ","); keys != "core_ep-0004,core_ep-0003,core_ep-0000" || !selection.Truncated {
			t.Errorf("expected the featured endpoints in their order and then the first of the group, got %s", keys)
		}
	})
	t.Run("more-than-50-sections-reached-by-keys", func(t *testing.T) {
		refs := manyRefs(120, func(i int) string { return fmt.Sprintf("group-%03d", i) })
		page := &pageconfig.Page{}
		for _, ref := range refs {
			page.Endpoints = append(page.Endpoints, ref.Key)
		}
		selection := Select(page, refs, 1000)
		if len(selection.Sections) != 120 || selection.Truncated {
			t.Errorf("expected 120 sections without truncation, got %d (truncated=%v)", len(selection.Sections), selection.Truncated)
		}
		selection = Select(page, refs, 60)
		if len(selection.Sections) != 60 || !selection.Truncated || selection.Sections[59].Group != "group-059" {
			t.Errorf("expected the first 60 sections in display order, got %d (truncated=%v)", len(selection.Sections), selection.Truncated)
		}
	})
	t.Run("limit-below-one-shows-nothing", func(t *testing.T) {
		if selection := Select(&pageconfig.Page{Groups: []string{"core"}}, manyRefs(3, oneGroup), -1); len(selection.Keys()) != 0 {
			t.Errorf("expected nothing to be shown, got %v", selection.Keys())
		}
	})
}

// The limit is part of the snapshot: the payload, the per-endpoint authorization and the counts of the administration
// all read it from there, also for the pages that are created, edited or renamed after the load.
func TestEndpointLimit_SnapshotAndAdministration(t *testing.T) {
	initializeSQLiteStore(t)
	loadWithLimit(t, 2)
	t.Cleanup(publicCache.Clear)
	published, ok := Lookup("infra")
	if !ok || published.MaximumEndpoints != 2 {
		t.Fatalf("expected the page to be captured with the limit of 2, got %+v (found=%v)", published.MaximumEndpoints, ok)
	}
	if payload := publicPayloadOf(t, "infra"); shownNames(payload) != "alpha,bravo" || !payload.Truncated || payload.Summary.Total != 2 {
		t.Errorf("expected alpha and bravo, truncated, with a total of 2, got %s (truncated=%v, total=%d)", shownNames(payload), payload.Truncated, payload.Summary.Total)
	}
	service := NewService()
	for _, item := range service.List().StatusPages {
		if item.Slug == "infra" && (item.Endpoints != 2 || !item.Truncated) {
			t.Errorf("expected the listing to count 2 endpoints, truncated, got %+v", item)
		}
	}
	// Created after the load: the copy of the snapshot keeps the limit
	detail, err := service.Create([]byte("slug: apps\ntitle: Apps\ngroups: [core]\nenabled: true\n"), "ops@example.com")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	if detail.Endpoints != 2 || !detail.Truncated {
		t.Errorf("expected the created page to count 2 endpoints, truncated, got %+v", detail.Item)
	}
	if published, _ = Lookup("apps"); published.MaximumEndpoints != 2 {
		t.Errorf("expected the created page to be captured with the limit of 2, got %d", published.MaximumEndpoints)
	}
	if IsEndpointShownOf(published, "core_charlie") || !IsEndpointShownOf(published, "core_bravo") {
		t.Error("expected the created page to show bravo and not charlie")
	}
	// Edited: a selection within the limit is not truncated
	if detail, err = service.Update("apps", []byte("slug: apps\ntitle: Apps\nendpoints: [core_delta, core_charlie]\nenabled: true\n"), detail.Version, "ops@example.com"); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	if detail.Endpoints != 2 || detail.Truncated {
		t.Errorf("expected the edited page to count 2 endpoints without truncation, got %+v", detail.Item)
	}
	if published, _ = Lookup("apps"); published.MaximumEndpoints != 2 || !IsEndpointShownOf(published, "core_delta") {
		t.Errorf("expected the edited page to keep the limit and show delta, got %+v", published.MaximumEndpoints)
	}
	// Disabled and enabled again, and the key of a managed endpoint renamed: two more copies of the snapshot
	if detail, err = service.SetEnabled("apps", false, detail.Version, "ops@example.com"); err != nil {
		t.Fatalf("failed to disable: %v", err)
	}
	if detail, err = service.SetEnabled("apps", true, detail.Version, "ops@example.com"); err != nil {
		t.Fatalf("failed to enable: %v", err)
	}
	cfg := loadTestConfig(t, limitTestConfig(2))
	endpoints := managedendpoint.NewService(cfg)
	definition := func(group string) []byte {
		return []byte("name: site\ngroup: " + group + "\nenabled: false\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n")
	}
	created, err := endpoints.Create(definition("web"), "ops@example.com")
	if err != nil {
		t.Fatalf("failed to create the managed endpoint: %v", err)
	}
	if detail, err = service.Update("apps", []byte("slug: apps\ntitle: Apps\ngroups: [core]\nendpoints: [web_site]\nenabled: true\n"), detail.Version, "ops@example.com"); err != nil {
		t.Fatalf("failed to select the managed endpoint: %v", err)
	}
	if _, err = endpoints.Update("web_site", definition("clients"), created.Version, "ops@example.com"); err != nil {
		t.Fatalf("failed to rename the managed endpoint: %v", err)
	}
	if page := findState("apps").Page; page == nil || len(page.Endpoints) != 1 || page.Endpoints[0] != "clients_site" {
		t.Fatalf("expected the page to follow the renamed key, got %+v", page)
	}
	if published, _ = Lookup("apps"); published.MaximumEndpoints != 2 || IsEndpointShownOf(published, "core_charlie") {
		t.Errorf("expected the limit of 2 after the rename, got %d", published.MaximumEndpoints)
	}
}

func TestEndpointLimit_ValidationWarning(t *testing.T) {
	initializeSQLiteStore(t)
	loadWithLimit(t, 3)
	t.Cleanup(publicCache.Clear)
	service := NewService()
	validation, err := service.Validate([]byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if validation.Endpoints != 3 || len(validation.Warnings) != 1 || validation.Warnings[0] != (Warning{Type: WarningTypeTruncated, Value: "3"}) {
		t.Errorf("expected 3 endpoints and the warning of the truncation with the limit, got %d and %+v", validation.Endpoints, validation.Warnings)
	}
	if validation, err = service.Validate([]byte("slug: apps\ntitle: Apps\nendpoints: [core_alpha, core_bravo, core_charlie]\n"), ""); err != nil || len(validation.Warnings) != 0 {
		t.Errorf("expected no warning for a selection equal to the limit, got %+v (err=%v)", validation, err)
	}
	// The restore computes its warnings with the endpoints of its plan and the limit in force
	_, _, warnings, err := service.ValidateRestore([]byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), Endpoints())
	if err != nil || len(warnings) != 1 || warnings[0].Type != WarningTypeTruncated || warnings[0].Value != "3" {
		t.Errorf("expected the restore to warn about the truncation, got %+v (err=%v)", warnings, err)
	}
	if warnings = SelectionWarnings(&pageconfig.Page{Groups: []string{"core", "missing"}}); len(warnings) != 2 || warnings[0].Type != "group" || warnings[1].Type != WarningTypeTruncated {
		t.Errorf("expected the group without match and then the truncation, got %+v", warnings)
	}
}

// The cut is an access rule: an endpoint beyond it is not served by the routes of the page, and nothing is read from
// the storage for it
func TestEndpointLimit_AccessBeyondTheCut(t *testing.T) {
	for _, scenario := range []struct {
		limit  int
		shown  []string
		hidden []string
	}{
		{limit: 0, shown: []string{"core_alpha", "core_delta"}},
		{limit: 1000, shown: []string{"core_alpha", "core_delta"}},
		{limit: 2, shown: []string{"core_alpha", "core_bravo"}, hidden: []string{"core_charlie", "core_delta"}},
		{limit: 1, shown: []string{"core_alpha"}, hidden: []string{"core_bravo", "core_delta"}},
	} {
		t.Run(fmt.Sprintf("limit-%d", scenario.limit), func(t *testing.T) {
			setupEndpointLimitTest(t, scenario.limit)
			published, ok := Lookup("infra")
			if !ok {
				t.Fatal("expected the page to be published")
			}
			assemblies := 0
			chart := func(key string) error {
				_, err := PublicResponseTimeChart(published, key, "24h", time.Now(), func(int) ([]byte, error) {
					assemblies++
					return []byte("{}"), nil
				})
				return err
			}
			for _, key := range scenario.shown {
				if !IsEndpointShownOf(published, key) {
					t.Errorf("expected %s to be shown", key)
				}
				if _, err := PublicEndpointDetailsOf("infra", published, key); err != nil {
					t.Errorf("expected the details of %s, got %v", key, err)
				}
				if err := chart(key); err != nil {
					t.Errorf("expected the chart of %s, got %v", key, err)
				}
			}
			assembliesOfShown := assemblies
			for _, key := range scenario.hidden {
				if IsEndpointShownOf(published, key) {
					t.Errorf("expected %s not to be shown", key)
				}
				if _, err := PublicEndpointDetailsOf("infra", published, key); !errors.Is(err, ErrPageNotFound) {
					t.Errorf("expected ErrPageNotFound for the details of %s, got %v", key, err)
				}
				if err := chart(key); !errors.Is(err, ErrPageNotFound) {
					t.Errorf("expected ErrPageNotFound for the chart of %s, got %v", key, err)
				}
			}
			if assemblies != assembliesOfShown {
				t.Errorf("expected no chart to be assembled beyond the cut, got %d assemblies", assemblies-assembliesOfShown)
			}
		})
	}
}

// A reload that changes the limit changes the payload, the authorization and the counts together, and what was cached
// under the previous limit is not served for an endpoint that left the cut
func TestEndpointLimit_Reload(t *testing.T) {
	setupEndpointLimitTest(t, 0)
	before, _ := Lookup("infra")
	if payload := publicPayloadOf(t, "infra"); shownNames(payload) != "alpha,bravo,charlie,delta" || payload.Truncated {
		t.Fatalf("expected the four endpoints, got %s", shownNames(payload))
	}
	// Cached under the previous limit, without any new result to renew them
	if _, err := PublicEndpointDetailsOf("infra", before, "core_delta"); err != nil {
		t.Fatal(err)
	}
	if _, err := PublicResponseTimeChart(before, "core_delta", "24h", time.Now(), func(int) ([]byte, error) { return []byte("{}"), nil }); err != nil {
		t.Fatal(err)
	}
	loadWithLimit(t, 2)
	after, _ := Lookup("infra")
	if after.MaximumEndpoints != 2 || after.Generation == before.Generation {
		t.Fatalf("expected a new generation with the limit of 2, got %+v", after)
	}
	if payload := publicPayloadOf(t, "infra"); shownNames(payload) != "alpha,bravo" || !payload.Truncated || payload.Summary.Total != 2 {
		t.Errorf("expected the payload to follow the new limit, got %s (truncated=%v)", shownNames(payload), payload.Truncated)
	}
	if _, err := PublicEndpointDetails("infra", "core_delta"); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("expected the cached details of delta not to be served, got %v", err)
	}
	if _, err := PublicResponseTimeChart(after, "core_delta", "24h", time.Now(), func(int) ([]byte, error) {
		t.Error("expected the chart of delta not to be assembled")
		return nil, nil
	}); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("expected the cached chart of delta not to be served, got %v", err)
	}
	if item := NewService().List().StatusPages[0]; item.Endpoints != 2 || !item.Truncated {
		t.Errorf("expected the listing to follow the new limit, got %+v", item)
	}
	// Raised again: everything comes back
	loadWithLimit(t, 1000)
	if payload := publicPayloadOf(t, "infra"); shownNames(payload) != "alpha,bravo,charlie,delta" || payload.Truncated {
		t.Errorf("expected the four endpoints again, got %s", shownNames(payload))
	}
	if _, err := PublicEndpointDetails("infra", "core_delta"); err != nil {
		t.Errorf("expected the details of delta again, got %v", err)
	}
}

// Reads of the chart during reloads that change the limit: each read answers according to the one capture it holds,
// never with the authorization of one limit and the payload of another
func TestEndpointLimit_ConcurrentChartReadsAndReloads(t *testing.T) {
	setupEndpointLimitTest(t, 0)
	configs := map[int]string{1: limitTestConfig(1), 4: limitTestConfig(4)}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				published, ok := Lookup("infra")
				if !ok {
					continue
				}
				_, err := PublicResponseTimeChart(published, "core_delta", "24h", time.Now(), func(int) ([]byte, error) { return []byte("{}"), nil })
				switch {
				case published.MaximumEndpoints == 1 && !errors.Is(err, ErrPageNotFound):
					t.Errorf("expected delta to be refused under the limit of 1, got %v", err)
				case published.MaximumEndpoints != 1 && err != nil:
					t.Errorf("expected delta to be served under the limit of %d, got %v", published.MaximumEndpoints, err)
				}
			}
		}()
	}
	for i := 0; i < 20; i++ {
		limit := 1
		if i%2 == 1 {
			limit = 4
		}
		Load(loadTestConfig(t, configs[limit]))
	}
	close(stop)
	readers.Wait()
}

// The preview of the administration is truncated like the public page, whatever the page: from the configuration file,
// disabled, or with a login of its own
func TestEndpointLimit_Preview(t *testing.T) {
	initializeSQLiteStore(t)
	loadWithLimit(t, 2)
	t.Cleanup(publicCache.Clear)
	service := NewService()
	if _, err := service.Create([]byte("slug: disabled\ntitle: Disabled\ngroups: [core]\nenabled: false\n"), "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	login := pageWithLogin(t, "clients", "client", "page-secret")
	definition := fmt.Sprintf("slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password-bcrypt-base64: %s\n", login.Auth.PasswordBcryptHashBase64Encoded)
	if _, err := service.Create([]byte(definition), "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"infra", "disabled", "clients"} {
		body, err := service.Preview(slug)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", slug, err)
		}
		var payload Payload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if shownNames(payload) != "alpha,bravo" || !payload.Truncated || payload.Summary.Total != 2 {
			t.Errorf("%s: expected the preview truncated at 2, got %s (truncated=%v, total=%d)", slug, shownNames(payload), payload.Truncated, payload.Summary.Total)
		}
	}
}

// The cache of the payloads has a budget in bytes: payloads above it push the least recently used ones out, instead of
// piling up to the maximum number of entries
func TestPublicCache_MemoryBudget(t *testing.T) {
	if publicCache.MaxMemoryUsage() != maximumPublicCacheMemory || publicCache.MaxSize() != maximumPublicCacheEntries {
		t.Fatalf("expected the cache to be bounded to %d entries and %d bytes, got %d and %d", maximumPublicCacheEntries, maximumPublicCacheMemory, publicCache.MaxSize(), publicCache.MaxMemoryUsage())
	}
	t.Cleanup(publicCache.Clear)
	publicCache.Clear()
	payload := make([]byte, 8*1024*1024)
	for i := 0; i < 40; i++ {
		publicCache.SetWithTTL(fmt.Sprintf("budget|%d", i), payload, publicCacheTTL)
	}
	if usage := publicCache.MemoryUsage(); usage > maximumPublicCacheMemory {
		t.Errorf("expected at most %d bytes in the cache, got %d", maximumPublicCacheMemory, usage)
	}
	if count := publicCache.Count(); count >= 40 || count < 10 {
		t.Errorf("expected the oldest payloads to be discarded, got %d entries", count)
	}
	if _, exists := publicCache.Get("budget|39"); !exists {
		t.Error("expected the most recent payload to stay in the cache")
	}
}
