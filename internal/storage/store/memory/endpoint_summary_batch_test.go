// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package memory

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

func TestStore_GetEndpointSummaries(t *testing.T) {
	store, _ := NewStore(storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
	defer store.Clear()
	now := time.Now()
	recent := &endpoint.Endpoint{Name: "recent", Group: "core"}
	old := &endpoint.Endpoint{Name: "old", Group: "core"}
	for i := 5; i >= 1; i-- {
		result := &endpoint.Result{Success: i%2 == 1, Timestamp: now.Add(-time.Duration(i) * time.Minute), Duration: time.Duration(i) * 10 * time.Millisecond, CertificateExpiration: time.Duration(i) * 24 * time.Hour, Hostname: "10.0.0.5"}
		if err := store.InsertEndpointResult(recent, result); err != nil {
			t.Fatalf("failed to insert result: %v", err)
		}
	}
	if err := store.InsertEndpointResult(old, &endpoint.Result{Success: true, Timestamp: now.Add(-40 * 24 * time.Hour), Duration: time.Millisecond}); err != nil {
		t.Fatalf("failed to insert old result: %v", err)
	}
	summaries, err := store.GetEndpointSummaries([]string{recent.Key(), old.Key(), recent.Key(), "core_missing"}, 3, now)
	if err != nil {
		t.Fatalf("failed to get summaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected summaries for the 2 existing endpoints, got %d: %+v", len(summaries), summaries)
	}
	expected, err := store.GetEndpointStatusByKey(recent.Key(), paging.NewEndpointStatusParams().WithResults(1, 3))
	if err != nil {
		t.Fatalf("failed to get endpoint status: %v", err)
	}
	summary := summaries[recent.Key()]
	if len(summary.Results) != 3 || len(expected.Results) != 3 {
		t.Fatalf("expected the 3 latest results, got %d (status has %d)", len(summary.Results), len(expected.Results))
	}
	for i, result := range summary.Results {
		want := expected.Results[i]
		if !result.Timestamp.Equal(want.Timestamp) || result.Success != want.Success || result.Duration != want.Duration || result.CertificateExpiration != want.CertificateExpiration {
			t.Errorf("result %d: expected %s/%v/%s/%s, got %s/%v/%s/%s", i, want.Timestamp, want.Success, want.Duration, want.CertificateExpiration, result.Timestamp, result.Success, result.Duration, result.CertificateExpiration)
		}
	}
	if summary.Results[2].CertificateExpiration != 24*time.Hour {
		t.Errorf("expected the certificate expiration of the most recent result, got %s", summary.Results[2].CertificateExpiration)
	}
	if uptime := summary.Uptimes.Last24Hours; uptime == nil || *uptime-0.6 > 1e-9 || 0.6-*uptime > 1e-9 {
		t.Errorf("expected a 24h uptime of 0.6, got %v", describe(uptime))
	}
	oldSummary := summaries[old.Key()]
	if len(oldSummary.Results) != 1 || oldSummary.Uptimes.Last24Hours != nil || oldSummary.Uptimes.Last7Days != nil || oldSummary.Uptimes.Last30Days != nil {
		t.Errorf("expected the old endpoint with its result and without uptimes, got %+v", oldSummary)
	}
	withoutResults, _ := store.GetEndpointSummaries([]string{recent.Key()}, 0, now)
	if withoutResults[recent.Key()] == nil || len(withoutResults[recent.Key()].Results) != 0 || withoutResults[recent.Key()].Uptimes.Last24Hours == nil {
		t.Errorf("expected the endpoint with uptimes and without results, got %+v", withoutResults)
	}
}
