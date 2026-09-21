package statuspage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func uptimePointer(value float64) *float64 {
	return &value
}

func TestBuildPayload(t *testing.T) {
	now := time.Date(2026, 9, 14, 15, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	page := &pageconfig.Page{Slug: "infra", Title: "Infraestrutura", Description: "Serviços", Groups: []string{"core", "database", "jobs"}}
	selection := Selection{Sections: []Section{
		{Group: "core", Endpoints: []EndpointRef{{Key: "core_api", Name: "api"}, {Key: "core_web", Name: "web"}, {Key: "core_new", Name: "new"}}},
		{Group: "database", Endpoints: []EndpointRef{{Key: "database_pg", Name: "pg"}}},
		{Group: "jobs", Endpoints: []EndpointRef{{Key: "jobs_batch", Name: "batch"}}},
		{Group: "", Endpoints: []EndpointRef{{Key: "_cdn", Name: "cdn"}}},
	}}
	summaries := map[string]*common.EndpointSummary{
		"core_api": {
			Results: []common.ResultSummary{{Timestamp: now.Add(-2 * time.Minute), Success: false, Duration: 1500 * time.Microsecond}, {Timestamp: now.Add(-time.Minute), Success: true, Duration: 123 * time.Millisecond}},
			Uptimes: common.EndpointUptimes{Last24Hours: uptimePointer(0.5), Last7Days: uptimePointer(0.75)},
		},
		"core_web":    {Results: []common.ResultSummary{{Timestamp: now.Add(-time.Minute), Success: false, Duration: time.Second}}},
		"database_pg": {Results: []common.ResultSummary{{Timestamp: now.Add(-time.Minute), Success: false}}},
		"jobs_batch":  {Results: []common.ResultSummary{}},
	}
	payload := BuildPayload(page, selection, summaries, now)
	if payload.Slug != "infra" || payload.Title != "Infraestrutura" || payload.Description != "Serviços" || !payload.UpdatedAt.Equal(now) || payload.UpdatedAt.Location() != time.UTC {
		t.Errorf("unexpected page fields: %+v", payload)
	}
	if payload.Status != StatusDegraded {
		t.Errorf("expected the page to be degraded, got %s", payload.Status)
	}
	expectedGroupStatuses := []string{StatusDegraded, StatusDown, StatusUnknown, StatusUnknown}
	for i, group := range payload.Groups {
		if group.Status != expectedGroupStatuses[i] {
			t.Errorf("group %q: expected %s, got %s", group.Name, expectedGroupStatuses[i], group.Status)
		}
	}
	api := payload.Groups[0].Endpoints[0]
	if api.Status != StatusUp || len(api.Results) != 2 || api.Results[0].DurationMs != 1 || api.Results[1].DurationMs != 123 || *api.Uptime.Last24Hours != 0.5 || api.Uptime.Last30Days != nil {
		t.Errorf("unexpected api endpoint: %+v", api)
	}
	if newEndpoint := payload.Groups[0].Endpoints[2]; newEndpoint.Status != StatusUnknown || newEndpoint.Results == nil || len(newEndpoint.Results) != 0 {
		t.Errorf("expected an endpoint without summary to be unknown with empty results, got %+v", newEndpoint)
	}
	if payload.Groups[3].Name != "" || payload.Groups[3].Endpoints[0].Name != "cdn" {
		t.Errorf("expected the section without group to keep an empty name, got %+v", payload.Groups[3])
	}
	if BuildPayload(page, Selection{}, nil, now).Status != StatusUnknown {
		t.Error("expected a page without endpoint to be unknown")
	}
}

// Mirror types of the public JSON: decoding with DisallowUnknownFields fails if any other field is published
type allowedPayload struct {
	Slug            string            `json:"slug"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Status          string            `json:"status"`
	UpdatedAt       string            `json:"updatedAt"`
	Truncated       bool              `json:"truncated"`
	GroupsCollapsed bool              `json:"groupsCollapsed"`
	Summary         allowedSummary    `json:"summary"`
	Featured        []allowedFeatured `json:"featured"`
	Groups          []allowedGroup    `json:"groups"`
}

// allowedSummary is the count of the endpoints of the page by status (fork)
type allowedSummary struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Pending int `json:"pending"`
	Unknown int `json:"unknown"`
}

type allowedFeatured struct {
	allowedEndpoint
	Group string `json:"group"`
}

type allowedGroup struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Summary   allowedSummary    `json:"summary"`
	Endpoints []allowedEndpoint `json:"endpoints"`
}

type allowedEndpoint struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Uptime struct {
		Last24Hours *float64 `json:"24h"`
		Last7Days   *float64 `json:"7d"`
		Last30Days  *float64 `json:"30d"`
	} `json:"uptime"`
	ResponseTime struct {
		Last24Hours *int `json:"24h"`
		Last7Days   *int `json:"7d"`
		Last30Days  *int `json:"30d"`
	} `json:"responseTime"`
	Results []struct {
		Timestamp  string `json:"timestamp"`
		Success    bool   `json:"success"`
		DurationMs int64  `json:"durationMs"`
		// Only published for pending results (fork)
		Pending bool `json:"pending"`
	} `json:"results"`
	// Only published when the page shows the certificate expiration (fork)
	CertificateExpiresInDays *int    `json:"certificateExpiresInDays"`
	CertificateExpiresAt     *string `json:"certificateExpiresAt"`
}

func TestBuildPayload_Allowlist(t *testing.T) {
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}}
	selection := Selection{Sections: []Section{{Group: "core", Endpoints: []EndpointRef{{Key: "core_db-master-10-0-0-5", Name: "db"}, {Key: "core_api", Name: "api"}}}}}
	// Fork: a failed result with errors, so that a regression that published messages or a reason on the page is caught
	summaries := map[string]*common.EndpointSummary{
		"core_db-master-10-0-0-5": {Results: []common.ResultSummary{{Timestamp: time.Now(), Success: true, Duration: time.Millisecond}}},
		"core_api":                {Results: []common.ResultSummary{{Timestamp: time.Now(), Success: false, Errors: []string{"dial tcp 10.0.0.5:443: connect: connection refused"}}}},
	}
	body, err := json.Marshal(BuildPayload(page, selection, summaries, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decoded allowedPayload
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("expected only allowed fields, got %v in %s", err, body)
	}
	if strings.Contains(string(body), "10-0-0-5") {
		t.Errorf("expected the endpoint key not to be published, got %s", body)
	}
	if strings.Contains(string(body), "10.0.0.5") || strings.Contains(string(body), "dial tcp") || strings.Contains(string(body), ReasonConnectionFailed) {
		t.Errorf("expected the page not to publish errors nor a reason, got %s", body)
	}
}

type allowedEndpointDetails struct {
	Page struct {
		Slug         string `json:"slug"`
		Title        string `json:"title"`
		ShowMessages bool   `json:"showMessages"`
	} `json:"page"`
	allowedEndpoint
	// Only published when the page shows messages (fork)
	Results []struct {
		Timestamp  string `json:"timestamp"`
		Success    bool   `json:"success"`
		DurationMs int64  `json:"durationMs"`
		Pending    bool   `json:"pending"`
		Message    string `json:"message"`
		Origin     string `json:"origin"`
	} `json:"results"`
	Group     string `json:"group"`
	UpdatedAt string `json:"updatedAt"`
	Events    []struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
	} `json:"events"`
}

func TestBuildEndpointDetailsPayload_Allowlist(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}}
	ref := EndpointRef{Key: "core_db-master-10-0-0-5", Name: "db", Group: "core"}
	summary := &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: true, Duration: time.Millisecond}}}
	events := []*endpoint.Event{
		{Type: endpoint.EventStart, Timestamp: now.Add(-time.Hour)},
		{Type: "CUSTOM", Timestamp: now.Add(-time.Minute)},
		{Type: endpoint.EventHealthy, Timestamp: now},
	}
	payload := BuildEndpointDetailsPayload(page, ref, summary, events, now)
	if payload.Status != StatusUp || len(payload.Events) != 2 || payload.Events[0].Type != "START" || payload.Events[1].Type != "HEALTHY" {
		t.Errorf("expected the known events only, got %+v", payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decoded allowedEndpointDetails
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("expected only allowed fields, got %v in %s", err, body)
	}
	if strings.Contains(string(body), "10-0-0-5") || strings.Contains(string(body), "CUSTOM") {
		t.Errorf("expected neither the endpoint key nor unknown events to be published, got %s", body)
	}
}
