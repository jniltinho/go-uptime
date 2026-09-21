package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// Pending results do not create events, and the other results are compared with the last healthy or unhealthy event
// (fork)
func TestAddResult_PendingResults(t *testing.T) {
	scenarios := map[string]struct {
		results  []*endpoint.Result
		expected string
	}{
		"first-pending":   {results: []*endpoint.Result{{Pending: true}, {Success: true}}, expected: "HEALTHY"},
		"up-pending-down": {results: []*endpoint.Result{{Success: true}, {Pending: true}, {Success: false}}, expected: "HEALTHY,UNHEALTHY"},
		"up-pending-up":   {results: []*endpoint.Result{{Success: true}, {Pending: true}, {Success: true}}, expected: "HEALTHY"},
		"only-pending":    {results: []*endpoint.Result{{Pending: true}, {Pending: true}}, expected: ""},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			status := endpoint.NewStatus("group", name)
			for _, result := range scenario.results {
				result.Timestamp = time.Now()
				AddResult(status, result, 3, 50)
			}
			var types []string
			for _, event := range status.Events {
				types = append(types, string(event.Type))
			}
			if got := strings.Join(types, ","); got != scenario.expected {
				t.Errorf("expected events %q, got %q", scenario.expected, got)
			}
		})
	}
	// More pending results than the maximum number of results
	status := endpoint.NewStatus("group", "many-pending")
	AddResult(status, &endpoint.Result{Success: false, Timestamp: time.Now()}, 3, 50)
	for i := 0; i < 10; i++ {
		AddResult(status, &endpoint.Result{Pending: true, Timestamp: time.Now()}, 3, 50)
	}
	AddResult(status, &endpoint.Result{Success: false, Timestamp: time.Now()}, 3, 50)
	if len(status.Events) != 1 || status.Events[0].Type != endpoint.EventUnhealthy {
		t.Errorf("expected a single UNHEALTHY event, got %+v", status.Events)
	}
}

// The summaries carry the pending mark and the message of the results (fork)
func TestStore_GetEndpointSummaries_Pending(t *testing.T) {
	store, _ := NewStore(10, 10)
	ep := &endpoint.Endpoint{Name: "backup", Group: "jobs"}
	_ = store.InsertEndpointResult(ep, &endpoint.Result{Timestamp: time.Now(), Pending: true, Message: "Aguardando", Origin: endpoint.ResultOriginPush, HTTPStatus: 0, Errors: []string{"x"}})
	summaries, _ := store.GetEndpointSummaries([]string{ep.Key()}, 10, time.Now())
	result := summaries[ep.Key()].Results[0]
	if !result.Pending || result.Message != "Aguardando" || result.Origin != endpoint.ResultOriginPush || len(result.Errors) != 1 {
		t.Errorf("expected the pending mark, the message, the origin and the errors, got %+v", result)
	}
}
