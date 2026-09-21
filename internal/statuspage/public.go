// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TwiN/gocache/v2"
	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/liveupdates"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
	"golang.org/x/sync/singleflight"
)

const (
	publicCacheTTL      = 30 * time.Second
	unavailableCacheTTL = 5 * time.Second

	maximumConcurrentAssemblies = 4

	// maximumPublicCacheEntries and maximumPublicCacheMemory bound the cache of the payloads of the pages and of the
	// details of their endpoints
	maximumPublicCacheEntries = 1000
	maximumPublicCacheMemory  = 128 * 1024 * 1024

	// maximumPublicEvents is the maximum number of latest events shown on the details page of an endpoint
	maximumPublicEvents = 50
)

var (
	// ErrPageNotFound is returned when the status page does not exist or is not published
	ErrPageNotFound = errors.New("status page not found")

	// ErrPageUnavailable is returned when the status page could not be assembled
	ErrPageUnavailable = errors.New("status page temporarily unavailable")

	errStorageNotSupported = errors.New("the storage does not support public status pages")
)

// summaryReader is the part of the storage used to assemble the public status pages
type summaryReader interface {
	GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error)
}

// eventReader is the part of the storage used to read the events of an endpoint for its details page
type eventReader interface {
	GetEndpointStatusByKey(key string, params *paging.EndpointStatusParams) (*endpoint.Status, error)
}

var (
	// getSummaryReader resolves the reader once per assembly: the storage is closed and replaced on reload, so no reader is
	// kept between assemblies. It is replaced in tests.
	getSummaryReader = func() (summaryReader, bool) {
		return store.GetEndpointSummaryBatchReader()
	}

	// getEventReader resolves the reader of the events once per assembly. It is replaced in tests.
	getEventReader = func() (eventReader, bool) {
		return store.Get(), true
	}

	// publicCache holds the JSON payloads, and the unavailability of the payloads that failed to be assembled, by
	// slug|revision|generation (and the endpoint key for the details pages). It is bounded in entries and in bytes: a
	// page of 1000 endpoints is a payload of about 4 MiB (TestMeasureEndpointLimit), so the number of entries alone
	// would let a few dozen large pages hold gigabytes. Above the budget the least recently used payloads are
	// discarded and assembled again when asked, which costs milliseconds.
	publicCache = gocache.NewCache().WithMaxSize(maximumPublicCacheEntries).WithMaxMemoryUsage(maximumPublicCacheMemory).WithEvictionPolicy(gocache.LeastRecentlyUsed)

	// assemblies deduplicates the concurrent assemblies of the same payload
	assemblies singleflight.Group

	// assemblySemaphore limits the concurrent assemblies of the public status pages and of their endpoint details
	assemblySemaphore = make(chan struct{}, maximumConcurrentAssemblies)

	// semaphoreTimeout is how long an assembly waits for a slot of its semaphore
	semaphoreTimeout = 5 * time.Second
)

// unavailableMarker is cached when a payload could not be assembled, so that a failing storage is not queried by every
// request
type unavailableMarker struct{}

// PublicPage returns the JSON payload of the published status page with the given slug. The payload is assembled at most
// once per page revision every 30 seconds, whatever the number of concurrent requests.
//
// It returns ErrPageNotFound, without reading the storage, when the page is not published, and ErrPageUnavailable when
// the storage could not be read or the assembly waited too long for a slot.
func PublicPage(slug string) ([]byte, error) {
	published, ok := Lookup(slug)
	if !ok {
		return nil, ErrPageNotFound
	}
	return PublicPageOf(slug, published)
}

// PublicPageOf returns the payload of a page already looked up by the caller, so that a request that authorises and
// then assembles resolves the slug only once (fork)
func PublicPageOf(slug string, published Published) ([]byte, error) {
	cacheKey := fmt.Sprintf("%s|%d|%d", slug, published.Revision, published.Generation)
	return cachedAssembly(cacheKey, publicCacheTTL, slug, func() ([]byte, error) {
		return assemble(published.Page, published.MaximumResults, published.MaximumEndpoints, time.Now())
	})
}

// PublicEndpointDetails returns the JSON payload of the details page of the endpoint with the given key on the published
// status page with the given slug. Like the page, it is assembled at most once per page revision and endpoint every 30
// seconds.
//
// It returns ErrPageNotFound, without reading the storage, when the page is not published or does not show the
// endpoint, and ErrPageUnavailable when the storage could not be read or the assembly waited too long for a slot.
func PublicEndpointDetails(slug, key string) ([]byte, error) {
	published, ok := Lookup(slug)
	if !ok {
		return nil, ErrPageNotFound
	}
	return PublicEndpointDetailsOf(slug, published, key)
}

// PublicEndpointDetailsOf returns the details of an endpoint of a page already looked up by the caller (fork)
func PublicEndpointDetailsOf(slug string, published Published, key string) ([]byte, error) {
	if len(key) == 0 || len(key) > pageconfig.MaximumEndpointKeyLength {
		return nil, ErrPageNotFound
	}
	ref, shown := findShownEndpoint(published, key)
	if !shown {
		return nil, ErrPageNotFound
	}
	// Fork: the sequence of the last result renews the cached details as soon as a new result is stored
	cacheKey := fmt.Sprintf("%s|%d|%d|endpoint|%s|%d", slug, published.Revision, published.Generation, key, liveupdates.Sequence(key))
	return cachedAssembly(cacheKey, publicCacheTTL, slug, func() ([]byte, error) {
		return assembleEndpointDetails(published.Page, ref, published.MaximumResults, time.Now())
	})
}

// IsEndpointShownOf returns whether a published status page already looked up by the caller shows the endpoint with
// the given key, without reading the storage (fork)
func IsEndpointShownOf(published Published, key string) bool {
	if len(key) == 0 || len(key) > pageconfig.MaximumEndpointKeyLength {
		return false
	}
	_, shown := findShownEndpoint(published, key)
	return shown
}

// findShownEndpoint returns the endpoint with the given key among the endpoints shown on the page, with the limit
// captured with it: an endpoint beyond the cut is not shown, and is therefore not served by any route of the page
func findShownEndpoint(published Published, key string) (EndpointRef, bool) {
	for _, ref := range Select(published.Page, Endpoints(), published.MaximumEndpoints).Refs() {
		if ref.Key == key {
			return ref, true
		}
	}
	return EndpointRef{}, false
}

// cachedAssembly returns the payload cached under cacheKey or assembles it once for all concurrent requests, in a slot of
// assemblySemaphore. A failed assembly is cached for unavailableCacheTTL; a timeout waiting for a slot is not cached.
func cachedAssembly(cacheKey string, ttl time.Duration, slug string, assembleFunc func() ([]byte, error)) ([]byte, error) {
	return cachedAssemblyIn(publicCache, &assemblies, cacheKey, ttl, slug, assembleFunc)
}

// cachedAssemblyIn is cachedAssembly with the cache and the deduplication group given, so that the payloads of the
// response time chart do not evict the payloads of the pages (fork)
func cachedAssemblyIn(cache *gocache.Cache, group *singleflight.Group, cacheKey string, ttl time.Duration, slug string, assembleFunc func() ([]byte, error)) ([]byte, error) {
	if body, cached, err := cachedPayload(cache, cacheKey); cached {
		return body, err
	}
	value, err, _ := group.Do(cacheKey, func() (any, error) {
		if body, cached, err := cachedPayload(cache, cacheKey); cached {
			return body, err
		}
		release, acquired := acquireSlot(assemblySemaphore)
		if !acquired {
			logr.Warnf("[statuspage.cachedAssembly] Timed out waiting to assemble status page with slug=%s", slug)
			return nil, ErrPageUnavailable
		}
		defer release()
		body, err := assembleFunc()
		if err != nil {
			logr.Errorf("[statuspage.cachedAssembly] Failed to assemble status page with slug=%s: %s", slug, err.Error())
			cache.SetWithTTL(cacheKey, unavailableMarker{}, unavailableCacheTTL)
			return nil, ErrPageUnavailable
		}
		cache.SetWithTTL(cacheKey, body, ttl)
		return body, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]byte), nil
}

func cachedPayload(cache *gocache.Cache, cacheKey string) ([]byte, bool, error) {
	value, exists := cache.Get(cacheKey)
	if !exists {
		return nil, false, nil
	}
	switch cached := value.(type) {
	case []byte:
		return cached, true, nil
	case unavailableMarker:
		return nil, true, ErrPageUnavailable
	}
	return nil, false, nil
}

// assemble reads the summaries of the endpoints selected by the page and encodes its public payload
func assemble(page *pageconfig.Page, maximumResults, maximumEndpoints int, now time.Time) ([]byte, error) {
	reader, ok := getSummaryReader()
	if !ok {
		return nil, errStorageNotSupported
	}
	selection := Select(page, Endpoints(), maximumEndpoints)
	if selection.Truncated {
		logr.Warnf("[statuspage.assemble] Status page with slug=%s selects more than %d endpoints (status-pages.maximum-endpoints-per-page), only the first %d are shown", page.Slug, maximumEndpoints, maximumEndpoints)
	}
	summaries, err := reader.GetEndpointSummaries(selection.Keys(), maximumResults, now)
	if err != nil {
		return nil, err
	}
	return json.Marshal(BuildPayload(page, selection, summaries, now))
}

// assembleEndpointDetails reads the summary and the latest events of an endpoint of the page and encodes the payload of
// its details page. An endpoint that is not in the store yet is unknown, without events.
func assembleEndpointDetails(page *pageconfig.Page, ref EndpointRef, maximumResults int, now time.Time) ([]byte, error) {
	summaries, summariesSupported := getSummaryReader()
	events, eventsSupported := getEventReader()
	if !summariesSupported || !eventsSupported {
		return nil, errStorageNotSupported
	}
	summaryByKey, err := summaries.GetEndpointSummaries([]string{ref.Key}, maximumResults, now)
	if err != nil {
		return nil, err
	}
	var endpointEvents []*endpoint.Event
	status, err := events.GetEndpointStatusByKey(ref.Key, paging.NewEndpointStatusParams().WithResults(1, 1).WithEvents(1, maximumPublicEvents))
	switch {
	case err == nil:
		endpointEvents = status.Events
	case !errors.Is(err, common.ErrEndpointNotFound):
		return nil, err
	}
	return json.Marshal(BuildEndpointDetailsPayload(page, ref, summaryByKey[ref.Key], endpointEvents, now))
}

// acquireSlot waits at most semaphoreTimeout for a slot of the semaphore, and returns the function releasing it
func acquireSlot(semaphore chan struct{}) (func(), bool) {
	timer := time.NewTimer(semaphoreTimeout)
	defer timer.Stop()
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, true
	case <-timer.C:
		return nil, false
	}
}
