// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package memory

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

func TestStore_GetUptimesByKeys_AverageResponseTimes(t *testing.T) {
	store, _ := NewStore(storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
	defer store.Clear()
	now := time.Now()
	ep := &endpoint.Endpoint{Name: "response-time", Group: "core"}
	for _, insert := range []struct {
		age      time.Duration
		duration time.Duration
	}{
		{3 * 24 * time.Hour, time.Second},
		{90 * time.Minute, 100 * time.Millisecond},
		{30 * time.Minute, 300 * time.Millisecond},
	} {
		if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: now.Add(-insert.age), Duration: insert.duration}); err != nil {
			t.Fatalf("failed to insert result: %v", err)
		}
	}
	uptimes, _ := store.GetUptimesByKeys([]string{ep.Key()}, now)
	averages := uptimes[ep.Key()]
	if averages == nil || averages.AverageResponseTime24Hours == nil || *averages.AverageResponseTime24Hours != 200 {
		t.Fatalf("expected a 24h average of 200 ms, got %+v", averages)
	}
	if *averages.AverageResponseTime7Days != 466 || *averages.AverageResponseTime30Days != 466 {
		t.Errorf("expected 7d and 30d averages of 466 ms, got %d and %d", *averages.AverageResponseTime7Days, *averages.AverageResponseTime30Days)
	}
	single, err := store.GetAverageResponseTimeByKey(ep.Key(), now.Add(-24*time.Hour), now)
	if err != nil || single != *averages.AverageResponseTime24Hours {
		t.Errorf("expected the batch average to match GetAverageResponseTimeByKey (%d), got %d (err=%v)", single, *averages.AverageResponseTime24Hours, err)
	}
}
