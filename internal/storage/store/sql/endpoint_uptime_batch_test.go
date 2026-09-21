package sql

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

func TestStore_GetUptimesByKeys(t *testing.T) {
	now := time.Now()
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			withResults := &endpoint.Endpoint{Name: "with-results", Group: managedEndpointTestGroup}
			onlyOld := &endpoint.Endpoint{Name: "only-old", Group: managedEndpointTestGroup}
			for _, insert := range []struct {
				ep      *endpoint.Endpoint
				age     time.Duration
				success bool
			}{
				{withResults, 10 * 24 * time.Hour, true},
				{withResults, 3 * 24 * time.Hour, false},
				{withResults, time.Hour, true},
				{onlyOld, 20 * 24 * time.Hour, false},
			} {
				result := &endpoint.Result{Success: insert.success, Timestamp: now.Add(-insert.age), Duration: 50 * time.Millisecond}
				if err := store.InsertEndpointResult(insert.ep, result); err != nil {
					t.Fatalf("failed to insert result: %v", err)
				}
			}
			keys := []string{withResults.Key(), onlyOld.Key(), withResults.Key(), "managed-test_missing"}
			uptimes, err := store.GetUptimesByKeys(keys, now)
			if err != nil {
				t.Fatalf("failed to get uptimes: %v", err)
			}
			if len(uptimes) != 2 {
				t.Fatalf("expected uptimes for 2 keys, got %d: %+v", len(uptimes), uptimes)
			}
			expectUptime(t, "with-results 24h", uptimes[withResults.Key()].Last24Hours, 1)
			expectUptime(t, "with-results 7d", uptimes[withResults.Key()].Last7Days, 0.5)
			expectUptime(t, "with-results 30d", uptimes[withResults.Key()].Last30Days, 2.0/3)
			expectNoUptime(t, "only-old 24h", uptimes[onlyOld.Key()].Last24Hours)
			expectNoUptime(t, "only-old 7d", uptimes[onlyOld.Key()].Last7Days)
			expectUptime(t, "only-old 30d", uptimes[onlyOld.Key()].Last30Days, 0)
			// The batch must match GetUptimeByKey for the periods that have executions
			for period, from := range map[string]time.Time{"24h": now.Add(-24 * time.Hour), "7d": now.Add(-7 * 24 * time.Hour), "30d": now.Add(-30 * 24 * time.Hour)} {
				single, err := store.GetUptimeByKey(withResults.Key(), from, now)
				if err != nil {
					t.Fatalf("failed to get uptime for %s: %v", period, err)
				}
				batch := map[string]*float64{"24h": uptimes[withResults.Key()].Last24Hours, "7d": uptimes[withResults.Key()].Last7Days, "30d": uptimes[withResults.Key()].Last30Days}[period]
				expectUptime(t, "GetUptimeByKey "+period, batch, single)
			}
			if empty, err := store.GetUptimesByKeys(nil, now); err != nil || len(empty) != 0 {
				t.Errorf("expected no uptime without keys, got %+v (err=%v)", empty, err)
			}
		})
	}
}

func expectUptime(t *testing.T, name string, actual *float64, expected float64) {
	t.Helper()
	if actual == nil {
		t.Errorf("%s: expected uptime %f, got nil", name, expected)
	} else if diff := *actual - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("%s: expected uptime %f, got %f", name, expected, *actual)
	}
}

func expectNoUptime(t *testing.T, name string, actual *float64) {
	t.Helper()
	if actual != nil {
		t.Errorf("%s: expected no uptime, got %f", name, *actual)
	}
}
