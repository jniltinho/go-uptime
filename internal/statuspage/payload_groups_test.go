package statuspage

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// TestBuildPayload_GroupSummary counts, in each group, only the endpoints listed in it: a featured endpoint is listed in
// no group, so it is counted by the page and by no group, even when it is the one that is down.
func TestBuildPayload_GroupSummary(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"apis", "sites"}, Featured: []string{"sites_website"}}
	selection := Selection{
		Featured: []EndpointRef{{Key: "sites_website", Name: "website", Group: "sites"}},
		Sections: []Section{
			{Group: "apis", Endpoints: []EndpointRef{
				{Key: "apis_gateway", Name: "gateway"},
				{Key: "apis_search", Name: "search"},
				{Key: "apis_legacy", Name: "legacy"},
				{Key: "apis_job", Name: "job"},
				{Key: "apis_new", Name: "new"},
			}},
			{Group: "sites", Endpoints: []EndpointRef{
				{Key: "sites_docs", Name: "docs"},
				{Key: "sites_blog", Name: "blog"},
			}},
		},
	}
	up := &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: true}}}
	down := &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: false}}}
	summaries := map[string]*common.EndpointSummary{
		"sites_website": down,
		"apis_gateway":  up,
		"apis_search":   up,
		"apis_legacy":   down,
		"apis_job":      {Results: []common.ResultSummary{{Timestamp: now, Pending: true}}},
		// apis_new has no result yet: it counts as unknown
		"sites_docs": up,
		"sites_blog": up,
	}
	payload := BuildPayload(page, selection, summaries, now)
	if expected := (SummaryPayload{Total: 5, Up: 2, Down: 1, Pending: 1, Unknown: 1}); payload.Groups[0].Summary != expected {
		t.Errorf("apis: expected %+v, got %+v", expected, payload.Groups[0].Summary)
	}
	// The featured endpoint that is down belongs to sites, and sites lists only the other two
	if expected := (SummaryPayload{Total: 2, Up: 2}); payload.Groups[1].Summary != expected {
		t.Errorf("sites: expected %+v, got %+v", expected, payload.Groups[1].Summary)
	}
	if payload.Groups[1].Status != StatusOperational {
		t.Errorf("expected sites to be operational, since its featured endpoint is not listed in it, got %s", payload.Groups[1].Status)
	}
	if expected := (SummaryPayload{Total: 8, Up: 4, Down: 2, Pending: 1, Unknown: 1}); payload.Summary != expected {
		t.Errorf("page: expected %+v, got %+v", expected, payload.Summary)
	}
	for _, group := range payload.Groups {
		summary := group.Summary
		if summary.Total != summary.Up+summary.Down+summary.Pending+summary.Unknown || summary.Total != len(group.Endpoints) {
			t.Errorf("%s: expected the total to be the sum of the four statuses and the number of listed endpoints, got %+v for %d endpoints", group.Name, summary, len(group.Endpoints))
		}
	}
}

// TestBuildPayload_GroupSummaryWithoutData keeps a group whose endpoints have no result as unknown, which the public
// page shows expanded.
func TestBuildPayload_GroupSummaryWithoutData(t *testing.T) {
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"new"}}
	selection := Selection{Sections: []Section{{Group: "new", Endpoints: []EndpointRef{{Key: "new_a", Name: "a"}, {Key: "new_b", Name: "b"}}}}}
	payload := BuildPayload(page, selection, nil, time.Now())
	if payload.Groups[0].Status != StatusUnknown {
		t.Errorf("expected the status unknown, got %s", payload.Groups[0].Status)
	}
	if expected := (SummaryPayload{Total: 2, Unknown: 2}); payload.Groups[0].Summary != expected {
		t.Errorf("expected %+v, got %+v", expected, payload.Groups[0].Summary)
	}
}

// TestBuildPayload_GroupSummaryOfATruncatedPage counts the endpoints that are published, like the summary of the page
func TestBuildPayload_GroupSummaryOfATruncatedPage(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}}
	refs := make([]EndpointRef, 0, pageconfig.DefaultMaximumEndpointsPerPage+50)
	summaries := make(map[string]*common.EndpointSummary, cap(refs))
	for i := 0; i < cap(refs); i++ {
		key := fmt.Sprintf("core_e%03d", i)
		refs = append(refs, EndpointRef{Key: key, Name: key[5:], Group: "core"})
		summaries[key] = &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: true}}}
	}
	payload := BuildPayload(page, Select(page, refs, pageconfig.DefaultMaximumEndpointsPerPage), summaries, now)
	if !payload.Truncated {
		t.Fatal("expected the page to be truncated")
	}
	if expected := (SummaryPayload{Total: pageconfig.DefaultMaximumEndpointsPerPage, Up: pageconfig.DefaultMaximumEndpointsPerPage}); payload.Groups[0].Summary != expected {
		t.Errorf("expected %+v, got %+v", expected, payload.Groups[0].Summary)
	}
}

// TestBuildPayload_GroupsCollapsed publishes the option of the page, false when it is not set
func TestBuildPayload_GroupsCollapsed(t *testing.T) {
	selection := Selection{Sections: []Section{{Group: "core", Endpoints: []EndpointRef{{Key: "core_api", Name: "api"}}}}}
	for _, collapsed := range []bool{false, true} {
		page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, GroupsCollapsed: collapsed}
		body, err := json.Marshal(BuildPayload(page, selection, nil, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		expected := `"groupsCollapsed":false`
		if collapsed {
			expected = `"groupsCollapsed":true`
		}
		if !strings.Contains(string(body), expected) {
			t.Errorf("expected %s in %s", expected, body)
		}
	}
}
