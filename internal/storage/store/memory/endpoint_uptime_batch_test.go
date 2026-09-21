package memory

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

func TestStore_GetUptimesByKeys(t *testing.T) {
	store, _ := NewStore(storage.DefaultMaximumNumberOfResults, storage.DefaultMaximumNumberOfEvents)
	defer store.Clear()
	now := time.Now()
	withResults := &endpoint.Endpoint{Name: "with-results", Group: "core"}
	onlyOld := &endpoint.Endpoint{Name: "only-old", Group: "core"}
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
	uptimes, err := store.GetUptimesByKeys([]string{withResults.Key(), onlyOld.Key(), withResults.Key(), "core_missing"}, now)
	if err != nil {
		t.Fatalf("failed to get uptimes: %v", err)
	}
	if len(uptimes) != 2 {
		t.Fatalf("expected uptimes for 2 keys, got %d: %+v", len(uptimes), uptimes)
	}
	expected := map[string][3]*float64{
		withResults.Key(): {floatPointer(1), floatPointer(0.5), floatPointer(2.0 / 3)},
		onlyOld.Key():     {nil, nil, floatPointer(0)},
	}
	for key, windows := range expected {
		actual := [3]*float64{uptimes[key].Last24Hours, uptimes[key].Last7Days, uptimes[key].Last30Days}
		for i := range windows {
			if (windows[i] == nil) != (actual[i] == nil) || (windows[i] != nil && *windows[i]-*actual[i] > 1e-9) || (windows[i] != nil && *actual[i]-*windows[i] > 1e-9) {
				t.Errorf("key=%s window=%d: expected %v, got %v", key, i, describe(windows[i]), describe(actual[i]))
			}
		}
	}
	// The batch must match GetUptimeByKey for the periods that have executions
	for i, from := range []time.Time{now.Add(-24 * time.Hour), now.Add(-7 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour)} {
		single, err := store.GetUptimeByKey(withResults.Key(), from, now)
		if err != nil {
			t.Fatalf("failed to get uptime: %v", err)
		}
		batch := [3]*float64{uptimes[withResults.Key()].Last24Hours, uptimes[withResults.Key()].Last7Days, uptimes[withResults.Key()].Last30Days}[i]
		if diff := *batch - single; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("window=%d: batch uptime %f does not match GetUptimeByKey %f", i, *batch, single)
		}
	}
}

func floatPointer(value float64) *float64 {
	return &value
}

func describe(value *float64) any {
	if value == nil {
		return "nil"
	}
	return *value
}
