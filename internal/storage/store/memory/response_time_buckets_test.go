// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package memory

import (
	"fmt"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func describeBucket(bucket common.ResponseTimeBucket) string {
	describe := func(value *int64) string {
		if value == nil {
			return "nil"
		}
		return fmt.Sprint(*value)
	}
	return fmt.Sprintf("up=%d down=%d pending=%d timed=%d total=%d min=%s max=%s", bucket.Up, bucket.Down, bucket.Pending, bucket.TimedUp, bucket.TotalMs, describe(bucket.MinMs), describe(bucket.MaxMs))
}

// The memory store must aggregate like the SQL stores (see TestConformance_ResponseTimeBuckets of the sql package)
func TestStore_ResponseTimeBuckets(t *testing.T) {
	store, _ := NewStore(100, 50)
	ep := &endpoint.Endpoint{Name: "backup", Group: "jobs"}
	minute := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour).Add(5 * time.Minute)
	results := []*endpoint.Result{
		{Success: true, Timestamp: minute.Add(1 * time.Second), Duration: 10 * time.Millisecond},
		{Success: true, Timestamp: minute.Add(2 * time.Second), Duration: 30 * time.Millisecond},
		{Success: true, Timestamp: minute.Add(3 * time.Second)},
		{Success: false, Pending: true, Timestamp: minute.Add(4 * time.Second)},
		{Success: false, Timestamp: minute.Add(5 * time.Second)},
		{Success: true, Timestamp: minute.Add(61 * time.Second), Duration: 10 * time.Millisecond},
		{Success: false, Timestamp: minute.Add(62 * time.Second)},
		{Success: true, Timestamp: minute.Add(63 * time.Second), Duration: 500 * time.Microsecond},
		{Success: true, Timestamp: minute.Add(64 * time.Second), Duration: time.Millisecond},
	}
	for _, result := range results {
		if err := store.InsertEndpointResult(ep, result); err != nil {
			t.Fatal(err)
		}
	}
	from, to := minute.Add(-24*time.Hour), minute.Add(24*time.Hour)
	minuteBuckets, _ := store.GetResponseTimeBuckets(ep.Key(), common.MinuteBucketSeconds, from, to)
	if len(minuteBuckets) != 2 || describeBucket(minuteBuckets[0]) != "up=3 down=1 pending=1 timed=2 total=40 min=10 max=30" || describeBucket(minuteBuckets[1]) != "up=3 down=1 pending=0 timed=2 total=11 min=1 max=10" {
		t.Errorf("unexpected minute buckets: %v", minuteBuckets)
	}
	hourBuckets, _ := store.GetResponseTimeBuckets(ep.Key(), common.HourBucketSeconds, from, to)
	if len(hourBuckets) != 1 || describeBucket(hourBuckets[0]) != "up=6 down=2 pending=1 timed=4 total=51 min=1 max=30" {
		t.Errorf("unexpected hour buckets: %v", hourBuckets)
	}
	// The buckets read are copies
	*minuteBuckets[0].MinMs = 999
	if again, _ := store.GetResponseTimeBuckets(ep.Key(), common.MinuteBucketSeconds, from, to); *again[0].MinMs != 10 {
		t.Error("expected the buckets read to be copies")
	}
	recent, _ := store.GetRecentResponseTimeResults(ep.Key(), 3)
	if len(recent) != 3 || !recent[0].Timestamp.Equal(results[6].Timestamp) || !recent[2].Timestamp.Equal(results[8].Timestamp) {
		t.Errorf("expected the 3 latest results from the oldest, got %v", recent)
	}
	if recent, _ := store.GetRecentResponseTimeResults("jobs_unknown", 3); len(recent) != 0 {
		t.Errorf("expected no result for an unknown key, got %v", recent)
	}
	store.DeleteAllEndpointStatusesNotInKeys([]string{"jobs_other"})
	if buckets, _ := store.GetResponseTimeBuckets(ep.Key(), common.MinuteBucketSeconds, from, to); len(buckets) != 0 {
		t.Errorf("expected the buckets to be removed with the endpoint, got %v", buckets)
	}
}

func TestStore_ResponseTimeBucketsCleanUp(t *testing.T) {
	store, _ := NewStore(100, 50)
	ep := &endpoint.Endpoint{Name: "cleanup", Group: "jobs"}
	now := time.Now().UTC().Truncate(time.Minute)
	store.now = func() time.Time { return now }
	count := func(bucketSeconds int) int {
		buckets, _ := store.GetResponseTimeBuckets(ep.Key(), bucketSeconds, now.Add(-30*24*time.Hour), now.Add(time.Hour))
		return len(buckets)
	}
	old := now.Add(-26 * time.Hour)
	_ = store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: old, Duration: time.Millisecond})
	if minutes, hours := count(common.MinuteBucketSeconds), count(common.HourBucketSeconds); minutes != 0 || hours != 1 {
		t.Fatalf("expected the old minute bucket to be deleted and the hour bucket to be kept, got %d and %d", minutes, hours)
	}
	now = now.Add(30 * time.Minute)
	_ = store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: old, Duration: time.Millisecond})
	if minutes := count(common.MinuteBucketSeconds); minutes != 1 {
		t.Fatalf("expected no clean up within the hour, got %d minute buckets", minutes)
	}
	now = now.Add(31 * time.Minute)
	_ = store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: now, Duration: time.Millisecond})
	if minutes, hours := count(common.MinuteBucketSeconds), count(common.HourBucketSeconds); minutes != 1 || hours != 2 {
		t.Fatalf("expected only the recent minute bucket and both hour buckets, got %d and %d", minutes, hours)
	}
	store.Clear()
	if minutes := count(common.MinuteBucketSeconds); minutes != 0 {
		t.Errorf("expected Clear to remove the buckets, got %d", minutes)
	}
}
