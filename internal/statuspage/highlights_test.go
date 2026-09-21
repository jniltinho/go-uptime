package statuspage

import (
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

func TestSelect_Featured(t *testing.T) {
	refs := []EndpointRef{
		{Key: "core_web", Name: "web", Group: "core"},
		{Key: "core_api", Name: "api", Group: "core"},
		{Key: "database_pg", Name: "pg", Group: "database"},
	}
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, Featured: []string{"core_api", "missing_endpoint", "database_pg"}}
	selection := Select(page, refs, pageconfig.DefaultMaximumEndpointsPerPage)
	if len(selection.Featured) != 2 || selection.Featured[0].Name != "api" || selection.Featured[1].Name != "pg" {
		t.Fatalf("expected api and pg featured in the order of the page, got %+v", selection.Featured)
	}
	if len(selection.Sections) != 1 || selection.Sections[0].Group != "core" || len(selection.Sections[0].Endpoints) != 1 || selection.Sections[0].Endpoints[0].Name != "web" {
		t.Errorf("expected the featured endpoints to be left out of their section, got %+v", selection.Sections)
	}
	if strings.Join(selection.Keys(), ",") != "core_api,database_pg,core_web" {
		t.Errorf("expected the featured keys first, got %v", selection.Keys())
	}
	if refs := selection.Refs(); len(refs) != 3 || refs[0].Key != "core_api" || refs[2].Key != "core_web" {
		t.Errorf("expected the refs in display order, got %+v", refs)
	}
}

func TestBuildPayload_FeaturedAndResponseTimes(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, Featured: []string{"core_api"}, Charts: []string{"core_api"}}
	selection := Selection{
		Featured: []EndpointRef{{Key: "core_api", Name: "api", Group: "core"}},
		Sections: []Section{{Group: "core", Endpoints: []EndpointRef{{Key: "core_web", Name: "web", Group: "core"}}}},
	}
	average := 120
	summaries := map[string]*common.EndpointSummary{
		"core_api": {Results: []common.ResultSummary{{Timestamp: now, Success: false, Duration: 120 * time.Millisecond}}, Uptimes: common.EndpointUptimes{AverageResponseTime24Hours: &average}},
		"core_web": {Results: []common.ResultSummary{{Timestamp: now, Success: true}}},
	}
	payload := BuildPayload(page, selection, summaries, now)
	if len(payload.Featured) != 1 || payload.Featured[0].Name != "api" || payload.Featured[0].Group != "core" {
		t.Fatalf("expected api featured with its group, got %+v", payload.Featured)
	}
	if *payload.Featured[0].ResponseTime.Last24Hours != 120 || payload.Featured[0].ResponseTime.Last7Days != nil {
		t.Errorf("expected the average response times of the summary, got %+v", payload.Featured[0].ResponseTime)
	}
	if payload.Status != StatusDegraded {
		t.Errorf("expected the page status to include the featured endpoints, got %s", payload.Status)
	}
	body, _ := json.Marshal(payload)
	if !strings.Contains(string(body), `"featured":[{"name":"api"`) || !strings.Contains(string(body), `"group":"core"`) || strings.Contains(string(body), "core_api") || strings.Contains(string(body), "chart") {
		t.Errorf("unexpected JSON of the featured endpoint: %s", body)
	}
}

func TestSelectionWarnings_FeaturedAndDeprecatedCharts(t *testing.T) {
	refs := []EndpointRef{{Key: "core_web", Name: "web", Group: "core"}, {Key: "database_pg", Name: "pg", Group: "database"}}
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, Featured: []string{"missing_endpoint"}, Charts: []string{"core_web", "database_pg"}}
	var found []string
	for _, warning := range selectionWarnings(page, refs) {
		found = append(found, warning.Type+"="+warning.Value)
	}
	if strings.Join(found, ",") != "featured=missing_endpoint,charts=core_web, database_pg" {
		t.Errorf("expected the missing featured endpoint and the deprecated charts, got %v", found)
	}
	page.Charts = nil
	if warnings := selectionWarnings(page, refs); len(warnings) != 1 {
		t.Errorf("expected no charts warning without charts, got %+v", warnings)
	}
}

// countingEventReader counts the reads of the events and can fail
type countingEventReader struct {
	calls atomic.Int32
	err   error
}

func (reader *countingEventReader) GetEndpointStatusByKey(key string, params *paging.EndpointStatusParams) (*endpoint.Status, error) {
	reader.calls.Add(1)
	if reader.err != nil {
		return nil, reader.err
	}
	return store.Get().GetEndpointStatusByKey(key, params)
}

const endpointDetailsTestConfig = `
endpoints:
  - name: api
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: web
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: new
    group: core
    url: https://example.org
    conditions: ["[STATUS] == 200"]
  - name: pg
    group: database
    url: https://example.org
    conditions: ["[STATUS] == 200"]
status-pages:
  pages:
    - slug: infra
      title: Infra
      groups: [core]
      featured: [core_api]
      charts: [core_api]
`

func setupEndpointDetailsTest(t *testing.T, reader *countingEventReader) {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, endpointDetailsTestConfig)
	now := time.Now()
	for _, insert := range []struct {
		ep      *endpoint.Endpoint
		age     time.Duration
		success bool
	}{
		{cfg.Endpoints[0], 2 * time.Hour, true},
		{cfg.Endpoints[0], 10 * time.Minute, false},
		{cfg.Endpoints[1], 5 * time.Minute, true},
		{cfg.Endpoints[3], 5 * time.Minute, true},
	} {
		result := &endpoint.Result{Success: insert.success, Timestamp: now.Add(-insert.age), Duration: 100 * time.Millisecond, Hostname: "10.0.0.5", Errors: []string{"secret-error"}}
		if err := store.Get().InsertEndpointResult(insert.ep, result); err != nil {
			t.Fatal(err)
		}
	}
	Load(cfg)
	previousReader := getEventReader
	getEventReader = func() (eventReader, bool) { return reader, true }
	t.Cleanup(func() {
		getEventReader = previousReader
		publicCache.Clear()
	})
}

func TestPublicEndpointDetails(t *testing.T) {
	reader := &countingEventReader{}
	setupEndpointDetailsTest(t, reader)
	body, err := PublicEndpointDetails("infra", "core_api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload EndpointDetailsPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Page.Slug != "infra" || payload.Page.Title != "Infra" || payload.Name != "api" || payload.Group != "core" || payload.Status != StatusDown || len(payload.Results) != 2 {
		t.Fatalf("unexpected details of api: %s", body)
	}
	// The memory store dates the START event with the time of the insertion, so only the order of the types is checked
	if events := payload.Events; len(events) < 2 || events[0].Type != string(endpoint.EventStart) || events[len(events)-1].Type != string(endpoint.EventUnhealthy) {
		t.Errorf("expected the events from START to UNHEALTHY, got %+v", payload.Events)
	}
	for _, sensitive := range []string{"core_api", "10.0.0.5", "secret-error", "example.org"} {
		if strings.Contains(string(body), sensitive) {
			t.Errorf("expected %q not to be published, got %s", sensitive, body)
		}
	}
	calls := reader.calls.Load()
	if _, err := PublicEndpointDetails("infra", "core_api"); err != nil || reader.calls.Load() != calls {
		t.Errorf("expected the second request to be served from the cache, got %d reads (err=%v)", reader.calls.Load(), err)
	}
	if body, err := PublicEndpointDetails("infra", "core_web"); err != nil || !strings.Contains(string(body), `"name":"web"`) || !strings.Contains(string(body), `"status":"up"`) {
		t.Errorf("expected the details of web, got %s (err=%v)", body, err)
	}
	if body, err := PublicEndpointDetails("infra", "core_new"); err != nil || !strings.Contains(string(body), `"status":"unknown"`) || !strings.Contains(string(body), `"events":[]`) {
		t.Errorf("expected an endpoint without results to be unknown without events, got %s (err=%v)", body, err)
	}
	calls = reader.calls.Load()
	for _, scenario := range []struct{ slug, key string }{{"infra", "database_pg"}, {"infra", "missing_endpoint"}, {"infra", ""}, {"infra", strings.Repeat("a", 401)}, {"missing", "core_api"}} {
		if _, err := PublicEndpointDetails(scenario.slug, scenario.key); !errors.Is(err, ErrPageNotFound) {
			t.Errorf("expected ErrPageNotFound for %+v, got %v", scenario, err)
		}
	}
	if reader.calls.Load() != calls {
		t.Errorf("expected no read for an endpoint that is not on a published page, got %d reads", reader.calls.Load()-calls)
	}
}

func TestPublicEndpointDetails_Unavailable(t *testing.T) {
	reader := &countingEventReader{err: errors.New("database is locked")}
	setupEndpointDetailsTest(t, reader)
	for i := 0; i < 10; i++ {
		if _, err := PublicEndpointDetails("infra", "core_api"); !errors.Is(err, ErrPageUnavailable) {
			t.Fatalf("expected ErrPageUnavailable, got %v", err)
		}
	}
	if reader.calls.Load() != 1 {
		t.Errorf("expected a failing storage to be read once, got %d reads", reader.calls.Load())
	}
}
