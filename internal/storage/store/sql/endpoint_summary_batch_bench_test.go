// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// BenchmarkStore_GetEndpointSummaries assembles a status page of 200 endpoints with 100 results each in SQLite, while
// the watchdog keeps inserting results, and reports the slowest concurrent insertion
func BenchmarkStore_GetEndpointSummaries(b *testing.B) {
	store, err := NewStore("sqlite", filepath.Join(b.TempDir(), "bench.db"), false, 100, 50)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	keys := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		ep := &endpoint.Endpoint{Name: fmt.Sprintf("ep-%03d", i), Group: "bench"}
		keys = append(keys, ep.Key())
		for j := 0; j < 100; j++ {
			result := &endpoint.Result{Success: j%10 != 0, Timestamp: now.Add(-time.Duration(100-j) * time.Minute), Duration: time.Duration(j) * time.Millisecond}
			if err := store.InsertEndpointResult(ep, result); err != nil {
				b.Fatal(err)
			}
		}
	}
	stop := make(chan struct{})
	var waitGroup sync.WaitGroup
	var slowestInsertion time.Duration
	var insertions int
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		ep := &endpoint.Endpoint{Name: "ep-000", Group: "bench"}
		for {
			select {
			case <-stop:
				return
			default:
			}
			start := time.Now()
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: time.Millisecond}); err != nil {
				b.Error(err)
				return
			}
			slowestInsertion = max(slowestInsertion, time.Since(start))
			insertions++
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		summaries, err := store.GetEndpointSummaries(keys, 50, now)
		if err != nil {
			b.Fatal(err)
		}
		if len(summaries) != 200 {
			b.Fatalf("expected 200 summaries, got %d", len(summaries))
		}
	}
	b.StopTimer()
	close(stop)
	waitGroup.Wait()
	b.ReportMetric(float64(slowestInsertion.Microseconds()), "slowest-insertion-µs")
	b.ReportMetric(float64(insertions), "concurrent-insertions")
}
