// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// A pending last result makes the endpoint pending and its group degraded (fork)
func TestBuildPayload_PendingStatus(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "jobs", Title: "Jobs", Groups: []string{"jobs"}, ShowMessages: true}
	selection := Selection{Sections: []Section{{Group: "jobs", Endpoints: []EndpointRef{{Key: "jobs_backup", Name: "backup"}, {Key: "jobs_report", Name: "report"}}}}}
	summaries := map[string]*common.EndpointSummary{
		"jobs_backup": {Results: []common.ResultSummary{{Timestamp: now, Pending: true, Message: "Aguardando disco", Origin: "push"}}},
		"jobs_report": {Results: []common.ResultSummary{{Timestamp: now, Success: true}}},
	}
	payload := BuildPayload(page, selection, summaries, now)
	backup := payload.Groups[0].Endpoints[0]
	if backup.Status != StatusPending || !backup.Results[0].Pending || payload.Groups[0].Status != StatusDegraded || payload.Status != StatusDegraded {
		t.Errorf("expected a pending endpoint in a degraded group and page, got %+v", payload)
	}
	body, _ := json.Marshal(payload)
	if strings.Contains(string(body), "Aguardando") || strings.Contains(string(body), `"origin"`) || strings.Contains(string(body), `"message"`) {
		t.Errorf("expected no message nor origin in the payload of the page, even with show-messages, got %s", body)
	}
}

func TestAggregateStatus_Pending(t *testing.T) {
	scenarios := map[string]struct {
		statuses []string
		expected string
	}{
		"only-pending":    {statuses: []string{StatusPending}, expected: StatusDegraded},
		"pending-and-up":  {statuses: []string{StatusPending, StatusUp}, expected: StatusDegraded},
		"pending-unknown": {statuses: []string{StatusPending, StatusUnknown}, expected: StatusDegraded},
		"up":              {statuses: []string{StatusUp, StatusUnknown}, expected: StatusOperational},
		"down":            {statuses: []string{StatusDown}, expected: StatusDown},
		"unknown":         {statuses: []string{StatusUnknown}, expected: StatusUnknown},
	}
	for name, scenario := range scenarios {
		if got := aggregateStatus(scenario.statuses); got != scenario.expected {
			t.Errorf("%s: expected %s, got %s", name, scenario.expected, got)
		}
	}
}

// The details page of a page that shows messages publishes the messages of pushes and heartbeats and the HTTP status,
// never the other errors (fork)
func TestBuildEndpointDetailsPayload_Messages(t *testing.T) {
	now := time.Now()
	ref := EndpointRef{Key: "core_api", Name: "api", Group: "core"}
	summary := &common.EndpointSummary{Results: []common.ResultSummary{
		{Timestamp: now.Add(-4 * time.Minute), Success: false, Errors: []string{"dial tcp 10.0.0.5:443: connection refused"}},
		{Timestamp: now.Add(-3 * time.Minute), Success: true, HTTPStatus: 200},
		{Timestamp: now.Add(-2 * time.Minute), Success: false, Errors: []string{"heartbeat: no update received within 1m0s"}},
		{Timestamp: now.Add(-time.Minute), Pending: true, Message: "Aguardando", Origin: "push"},
	}}
	withMessages := &pageconfig.Page{Slug: "infra", Title: "Infra", ShowMessages: true}
	payload := BuildEndpointDetailsPayload(withMessages, ref, summary, nil, now)
	messages := []string{payload.Results[0].Message, payload.Results[1].Message, payload.Results[2].Message, payload.Results[3].Message}
	// Fork: the check that failed without answering publishes the reason of the failure, never the error
	if !payload.Page.ShowMessages || messages[0] != ReasonConnectionFailed || messages[1] != "HTTP 200" || messages[2] != "heartbeat: no update received within 1m0s" || messages[3] != "Aguardando" || payload.Results[3].Origin != "push" {
		t.Errorf("unexpected messages %q in %+v", messages, payload)
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
	if strings.Contains(string(body), "10.0.0.5") || strings.Contains(string(body), "dial tcp") {
		t.Errorf("expected the errors of the checks not to be published, got %s", body)
	}
	withoutMessages := &pageconfig.Page{Slug: "infra", Title: "Infra"}
	body, _ = json.Marshal(BuildEndpointDetailsPayload(withoutMessages, ref, summary, nil, now))
	if strings.Contains(string(body), `"message"`) || strings.Contains(string(body), `"origin"`) || !strings.Contains(string(body), `"showMessages":false`) {
		t.Errorf("expected no message nor origin without show-messages, got %s", body)
	}
}
