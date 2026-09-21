// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

// TestMeasureEndpointLimit is the reproducible measurement behind the ceiling of maximum-endpoints-per-page and the
// memory budget of the cache: 1000 endpoints with 50 results each, shown by several pages. It only runs with
// GO_UPTIME_MEASURE_ENDPOINT_LIMIT=1, and its numbers are in docs/status-pages.md.
func TestMeasureEndpointLimit(t *testing.T) {
	if os.Getenv("GO_UPTIME_MEASURE_ENDPOINT_LIMIT") != "1" {
		t.Skip("set GO_UPTIME_MEASURE_ENDPOINT_LIMIT=1 to measure")
	}
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	const endpoints, results, pages = 1000, 50, 10
	var builder strings.Builder
	builder.WriteString("endpoints:\n")
	for i := 0; i < endpoints; i++ {
		fmt.Fprintf(&builder, "  - name: service-%04d\n    group: group-%02d\n    url: https://example.org\n    conditions: [\"[STATUS] == 200\"]\n", i, i%20)
	}
	builder.WriteString("status-pages:\n  maximum-endpoints-per-page: 1000\n  pages:\n")
	groups := make([]string, 20)
	for i := range groups {
		groups[i] = fmt.Sprintf("group-%02d", i)
	}
	for i := 0; i < pages; i++ {
		fmt.Fprintf(&builder, "    - slug: page-%d\n      title: Page %d\n      groups: [%s]\n", i, i, strings.Join(groups, ", "))
	}
	cfg := loadTestConfig(t, builder.String())
	now := time.Now()
	for _, ep := range cfg.Endpoints {
		for i := 0; i < results; i++ {
			result := &endpoint.Result{Success: i%10 != 0, Timestamp: now.Add(-time.Duration(results-i) * time.Minute), Duration: time.Duration(40+i) * time.Millisecond, HTTPStatus: 200}
			if !result.Success {
				result.Errors = []string{"Get \"https://example.org\": context deadline exceeded"}
			}
			if err := store.Get().InsertEndpointResult(ep, result); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, limit := range []int{200, 400, 1000} {
		limitValue := limit
		cfg.StatusPages.MaximumEndpointsPerPage = &limitValue
		Load(cfg)
		publicCache.Clear()
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		var first []byte
		start := time.Now()
		for i := 0; i < pages; i++ {
			body, err := PublicPage(fmt.Sprintf("page-%d", i))
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				first = body
				t.Logf("limit=%d: one payload assembled in %s", limit, time.Since(start))
			}
		}
		assembled := time.Since(start)
		published, _ := Lookup("page-0")
		detailsBytes := 0
		for i := 0; i < limit; i++ {
			body, err := PublicEndpointDetailsOf("page-0", published, cfg.Endpoints[i].Key())
			if err == nil {
				detailsBytes += len(body)
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		_, _ = writer.Write(first)
		_ = writer.Close()
		t.Logf("limit=%d: payload=%d KiB, gzip=%d KiB, %d pages in %s, details of one page=%d KiB, cache entries=%d, heap delta=%d KiB",
			limit, len(first)/1024, compressed.Len()/1024, pages, assembled, detailsBytes/1024, publicCache.Count(), (int64(after.HeapAlloc)-int64(before.HeapAlloc))/1024)
	}
	publicCache.Clear()
}
