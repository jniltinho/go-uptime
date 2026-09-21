package statuspage

import (
	"fmt"
	"time"

	"github.com/TwiN/gocache/v2"
	"github.com/jniltinho/go-uptime/v7/internal/liveupdates"
	"golang.org/x/sync/singleflight"
)

const (
	// maximumChartCacheEntries and maximumChartCacheMemory limit the cache of the response time charts, separate from the
	// cache of the pages: a page with 200 endpoints and 5 periods alone would fill the cache of the pages (fork)
	maximumChartCacheEntries = 2000
	maximumChartCacheMemory  = 32 * 1024 * 1024
)

var (
	// chartCache holds the JSON payloads of the response time charts of the endpoints of the published pages
	chartCache = gocache.NewCache().WithMaxSize(maximumChartCacheEntries).WithMaxMemoryUsage(maximumChartCacheMemory).WithEvictionPolicy(gocache.LeastRecentlyUsed)

	// chartAssemblies deduplicates the concurrent assemblies of the same chart payload
	chartAssemblies singleflight.Group
)

// PublicResponseTimeChart returns the JSON payload of the response time chart of the endpoint with the given key on the
// published status page that the caller looked up and authenticated, for the given period, which the caller has already
// validated. The page, its limits and its revision all come from that one capture, so a reload between the
// authentication and the answer cannot serve an endpoint under a limit or a login that were not the ones checked. The
// payload is assembled by assembleFunc with the maximum number of recent results of the page, at most once every 30
// seconds: for the recent period, per result sequence of the endpoint, so that a new result renews it, and for the
// other periods, per minute.
//
// It returns ErrPageNotFound, without reading the storage, when the page does not show the endpoint, and
// ErrPageUnavailable when the payload could not be assembled or waited too long for a slot.
func PublicResponseTimeChart(published Published, key, period string, now time.Time, assembleFunc func(maximumResults int) ([]byte, error)) ([]byte, error) {
	if !IsEndpointShownOf(published, key) {
		return nil, ErrPageNotFound
	}
	slug := published.Page.Slug
	version := uint64(now.Unix() / 60)
	if period == "recent" {
		version = liveupdates.Sequence(key)
	}
	cacheKey := fmt.Sprintf("%s|%d|%d|chart|%s|%s|%d", slug, published.Revision, published.Generation, key, period, version)
	return cachedAssemblyIn(chartCache, &chartAssemblies, cacheKey, publicCacheTTL, slug, func() ([]byte, error) {
		return assembleFunc(published.MaximumResults)
	})
}
