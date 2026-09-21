package sql

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// bucketsFingerprint describes the minute and hour buckets and the recent results of an endpoint
func bucketsFingerprint(t *testing.T, store *Store, key string, from, to time.Time) string {
	t.Helper()
	var lines []string
	for _, bucketSeconds := range []int{common.MinuteBucketSeconds, common.HourBucketSeconds} {
		buckets, err := store.GetResponseTimeBuckets(key, bucketSeconds, from, to)
		if err != nil {
			t.Fatalf("failed to get the buckets of %s: %v", key, err)
		}
		for _, bucket := range buckets {
			lines = append(lines, fmt.Sprintf("bucket %d %s %s", bucketSeconds, bucket.Timestamp.Format(time.RFC3339), describeBucket(bucket)))
		}
	}
	recent, err := store.GetRecentResponseTimeResults(key, 100)
	if err != nil {
		t.Fatalf("failed to get the recent results of %s: %v", key, err)
	}
	for _, result := range recent {
		lines = append(lines, fmt.Sprintf("recent %s success=%v pending=%v duration=%s", result.Timestamp.UTC().Format(time.RFC3339Nano), result.Success, result.Pending, result.Duration))
	}
	return strings.Join(lines, "\n")
}

func describeBucket(bucket common.ResponseTimeBucket) string {
	describe := func(value *int64) string {
		if value == nil {
			return "nil"
		}
		return fmt.Sprint(*value)
	}
	return fmt.Sprintf("up=%d down=%d pending=%d timed=%d total=%d min=%s max=%s", bucket.Up, bucket.Down, bucket.Pending, bucket.TimedUp, bucket.TotalMs, describe(bucket.MinMs), describe(bucket.MaxMs))
}

func TestConformance_ResponseTimeBuckets(t *testing.T) {
	stores := newConformanceStores(t, 100, 50)
	ep := &endpoint.Endpoint{Name: "backup", Group: "jobs"}
	minute := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour).Add(5 * time.Minute)
	results := []*endpoint.Result{
		{Success: true, Timestamp: minute.Add(1 * time.Second), Duration: 10 * time.Millisecond},
		{Success: true, Timestamp: minute.Add(2 * time.Second), Duration: 30 * time.Millisecond},
		{Success: true, Timestamp: minute.Add(3 * time.Second)},
		{Success: false, Pending: true, Timestamp: minute.Add(4 * time.Second), Message: "running"},
		{Success: false, Timestamp: minute.Add(5 * time.Second), Errors: []string{"failed"}},
		// The next minute: a down result after an up one keeps the minimum and the maximum, and a result below 1 ms is
		// not timed
		{Success: true, Timestamp: minute.Add(61 * time.Second), Duration: 10 * time.Millisecond},
		{Success: false, Timestamp: minute.Add(62 * time.Second)},
		{Success: true, Timestamp: minute.Add(63 * time.Second), Duration: 500 * time.Microsecond},
		{Success: true, Timestamp: minute.Add(64 * time.Second), Duration: time.Millisecond},
	}
	for _, conformance := range stores {
		for i, result := range results {
			if err := conformance.store.InsertEndpointResult(ep, result); err != nil {
				t.Fatalf("%s: failed to insert result %d: %v", conformance.name, i, err)
			}
		}
	}
	from, to := minute.Add(-24*time.Hour), minute.Add(24*time.Hour)
	compareWithSQLite(t, stores, func(t *testing.T, store *Store) string {
		return bucketsFingerprint(t, store, ep.Key(), from, to)
	})
	minuteBuckets, err := stores[0].store.GetResponseTimeBuckets(ep.Key(), common.MinuteBucketSeconds, from, to)
	if err != nil || len(minuteBuckets) != 2 {
		t.Fatalf("expected 2 minute buckets, got %d (err=%v)", len(minuteBuckets), err)
	}
	if actual := describeBucket(minuteBuckets[0]); actual != "up=3 down=1 pending=1 timed=2 total=40 min=10 max=30" {
		t.Errorf("unexpected first minute bucket: %s", actual)
	}
	if actual := describeBucket(minuteBuckets[1]); actual != "up=3 down=1 pending=0 timed=2 total=11 min=1 max=10" {
		t.Errorf("unexpected second minute bucket: %s", actual)
	}
	hourBuckets, err := stores[0].store.GetResponseTimeBuckets(ep.Key(), common.HourBucketSeconds, from, to)
	if err != nil || len(hourBuckets) != 1 || describeBucket(hourBuckets[0]) != "up=6 down=2 pending=1 timed=4 total=51 min=1 max=30" {
		t.Errorf("unexpected hour buckets: %v (err=%v)", hourBuckets, err)
	}
	if buckets, err := stores[0].store.GetResponseTimeBuckets("jobs_unknown", common.MinuteBucketSeconds, from, to); err != nil || len(buckets) != 0 {
		t.Errorf("expected no bucket for an unknown key, got %v (err=%v)", buckets, err)
	}
	if recent, err := stores[0].store.GetRecentResponseTimeResults("jobs_unknown", 100); err != nil || len(recent) != 0 {
		t.Errorf("expected no recent result for an unknown key, got %v (err=%v)", recent, err)
	}
	if recent, err := stores[0].store.GetRecentResponseTimeResults(ep.Key(), 3); err != nil || len(recent) != 3 || !recent[0].Timestamp.Equal(results[6].Timestamp) || !recent[2].Timestamp.Equal(results[8].Timestamp) {
		t.Errorf("expected the 3 latest results from the oldest, got %v (err=%v)", recent, err)
	}
}

func TestConformance_ResponseTimeBucketsCleanUp(t *testing.T) {
	for _, conformance := range newConformanceStores(t, 100, 50) {
		t.Run(conformance.name, func(t *testing.T) {
			store := conformance.store
			ep := &endpoint.Endpoint{Name: "cleanup", Group: "jobs"}
			now := time.Now().UTC().Truncate(time.Minute)
			store.now = func() time.Time { return now }
			defer func() { store.now = nil }()
			count := func(bucketSeconds int) int {
				buckets, err := store.GetResponseTimeBuckets(ep.Key(), bucketSeconds, now.Add(-30*24*time.Hour), now.Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				return len(buckets)
			}
			old := now.Add(-26 * time.Hour)
			// The first result cleans up right away: its own minute bucket is older than the retention
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: old, Duration: time.Millisecond}); err != nil {
				t.Fatal(err)
			}
			if minutes, hours := count(common.MinuteBucketSeconds), count(common.HourBucketSeconds); minutes != 0 || hours != 1 {
				t.Fatalf("expected the old minute bucket to be deleted and the hour bucket to be kept, got %d and %d", minutes, hours)
			}
			// Within the hour, there is no other clean up
			now = now.Add(30 * time.Minute)
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: old, Duration: time.Millisecond}); err != nil {
				t.Fatal(err)
			}
			if minutes := count(common.MinuteBucketSeconds); minutes != 1 {
				t.Fatalf("expected no clean up within the hour, got %d minute buckets", minutes)
			}
			// After the hour, the old bucket is deleted and the recent ones are kept
			now = now.Add(31 * time.Minute)
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: now, Duration: time.Millisecond}); err != nil {
				t.Fatal(err)
			}
			if minutes, hours := count(common.MinuteBucketSeconds), count(common.HourBucketSeconds); minutes != 1 || hours != 2 {
				t.Fatalf("expected only the recent minute bucket and both hour buckets, got %d and %d", minutes, hours)
			}
			// A removed endpoint loses its buckets in cascade
			store.DeleteAllEndpointStatusesNotInKeys([]string{"jobs_other"})
			if minutes, hours := count(common.MinuteBucketSeconds), count(common.HourBucketSeconds); minutes != 0 || hours != 0 {
				t.Fatalf("expected the buckets to be deleted with the endpoint, got %d and %d", minutes, hours)
			}
		})
	}
}

func TestConformance_ResponseTimeBucketsFailureKeepsTheResult(t *testing.T) {
	for _, conformance := range newConformanceStores(t, 100, 50) {
		t.Run(conformance.name, func(t *testing.T) {
			store := conformance.store
			ep := &endpoint.Endpoint{Name: "failure", Group: "jobs"}
			if _, err := store.db.Exec("ALTER TABLE endpoint_response_time_buckets RENAME TO endpoint_response_time_buckets_moved"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := store.db.Exec("ALTER TABLE endpoint_response_time_buckets_moved RENAME TO endpoint_response_time_buckets"); err != nil {
					t.Fatal(err)
				}
			}()
			timestamp := time.Now().UTC().Truncate(time.Second)
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: timestamp, Duration: 5 * time.Millisecond}); err != nil {
				t.Fatalf("expected the result to be inserted despite the failure of the buckets, got %v", err)
			}
			if recent, err := store.GetRecentResponseTimeResults(ep.Key(), 10); err != nil || len(recent) != 1 {
				t.Errorf("expected the result to be kept, got %v (err=%v)", recent, err)
			}
			var executions int
			if err := store.db.QueryRow("SELECT SUM(total_executions) FROM endpoint_uptimes WHERE endpoint_id = (SELECT endpoint_id FROM endpoints WHERE endpoint_key = $1)", ep.Key()).Scan(&executions); err != nil || executions != 1 {
				t.Errorf("expected the uptime to be kept, got %d (err=%v)", executions, err)
			}
		})
	}
}

func TestConformance_ResponseTimeBucketsIgnoreSuites(t *testing.T) {
	for _, conformance := range newConformanceStores(t, 100, 50) {
		t.Run(conformance.name, func(t *testing.T) {
			store := conformance.store
			su := &suite.Suite{Name: "buckets", Group: "flows"}
			timestamp := time.Now().UTC().Truncate(time.Second)
			result := &suite.Result{Name: su.Name, Group: su.Group, Success: true, Timestamp: timestamp, Duration: time.Second,
				EndpointResults: []*endpoint.Result{{Name: "login", Success: true, Timestamp: timestamp, Duration: 10 * time.Millisecond}}}
			if err := store.InsertSuiteResult(su, result); err != nil {
				t.Fatal(err)
			}
			var buckets int
			if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_response_time_buckets").Scan(&buckets); err != nil || buckets != 0 {
				t.Errorf("expected no bucket for the results of a suite, got %d (err=%v)", buckets, err)
			}
			var key string
			if err := store.db.QueryRow("SELECT endpoint_key FROM endpoints e JOIN endpoint_results r ON r.endpoint_id = e.endpoint_id WHERE r.suite_result_id IS NOT NULL").Scan(&key); err != nil {
				t.Fatal(err)
			}
			if recent, err := store.GetRecentResponseTimeResults(key, 10); err != nil || len(recent) != 0 {
				t.Errorf("expected no recent result from a suite, got %v (err=%v)", recent, err)
			}
			store.DeleteAllSuiteStatusesNotInKeys(nil)
		})
	}
}
