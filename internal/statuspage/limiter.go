// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"container/list"
	"net/netip"
	"sync"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

const (
	limiterWindow = time.Minute

	// maximumLimiterKeys is the maximum number of clients tracked by the limiter of the public status pages
	maximumLimiterKeys = 50000
)

// publicLimiter limits the costly responses of the public status pages. It is reused across reloads: only its limit is
// reconfigured, so that no state or goroutine is left behind.
var publicLimiter = NewLimiter(pageconfig.DefaultRateLimit, maximumLimiterKeys)

// ConfigureLimiter sets the number of costly responses per minute accepted from each client of the public status pages
func ConfigureLimiter(limit int) {
	publicLimiter.SetLimit(limit)
}

// HitLimiter counts a costly response for the client of the public status pages, see Limiter.Hit
func HitLimiter(clientIP netip.Addr, now time.Time) (bool, time.Duration) {
	return publicLimiter.Hit(clientIP, now)
}

// Limiter limits the number of costly responses per client with a sliding window, approximated by the counters of the
// current and the previous minute. IPv4 clients are identified by address and IPv6 clients by /64 prefix. At most
// maximumKeys clients are tracked, the least recently seen ones being forgotten first, without background goroutine.
type Limiter struct {
	mutex       sync.Mutex
	limit       int
	maximumKeys int
	entries     map[netip.Prefix]*list.Element
	recency     *list.List // front: most recently seen
}

type limiterEntry struct {
	key         netip.Prefix
	windowStart int64 // start of the current window, in nanoseconds since the epoch
	current     int
	previous    int
}

// NewLimiter creates a Limiter accepting limit costly responses per minute per client, 0 meaning unlimited
func NewLimiter(limit, maximumKeys int) *Limiter {
	return &Limiter{
		limit:       limit,
		maximumKeys: maximumKeys,
		entries:     make(map[netip.Prefix]*list.Element),
		recency:     list.New(),
	}
}

// SetLimit changes the number of costly responses per minute accepted from each client, 0 meaning unlimited. When the
// limit changes, the counters of every client are reset.
func (limiter *Limiter) SetLimit(limit int) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.limit != limit {
		limiter.entries = make(map[netip.Prefix]*list.Element)
		limiter.recency.Init()
	}
	limiter.limit = limit
}

// Hit counts a costly response for the client and returns whether it is allowed. When it is not, the response is not
// counted and the returned duration is how long the client should wait before retrying.
func (limiter *Limiter) Hit(clientIP netip.Addr, now time.Time) (bool, time.Duration) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.limit <= 0 {
		return true, 0
	}
	key := limiterKey(clientIP)
	nowNano := now.UnixNano()
	windowStart := nowNano - nowNano%int64(limiterWindow)
	var entry *limiterEntry
	if element, exists := limiter.entries[key]; exists {
		limiter.recency.MoveToFront(element)
		entry = element.Value.(*limiterEntry)
	} else {
		entry = &limiterEntry{key: key, windowStart: windowStart}
		limiter.entries[key] = limiter.recency.PushFront(entry)
		for len(limiter.entries) > limiter.maximumKeys {
			oldest := limiter.recency.Back()
			delete(limiter.entries, oldest.Value.(*limiterEntry).key)
			limiter.recency.Remove(oldest)
		}
	}
	switch elapsedWindows := (windowStart - entry.windowStart) / int64(limiterWindow); {
	case elapsedWindows == 1:
		entry.previous, entry.current = entry.current, 0
		entry.windowStart = windowStart
	case elapsedWindows > 1:
		entry.previous, entry.current = 0, 0
		entry.windowStart = windowStart
	}
	elapsedInWindow := float64(nowNano-windowStart) / float64(limiterWindow)
	estimated := float64(entry.previous)*(1-elapsedInWindow) + float64(entry.current)
	if estimated+1 > float64(limiter.limit) {
		retryAfter := time.Duration(windowStart + int64(limiterWindow) - nowNano)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}
	entry.current++
	return true, 0
}

// limiterKey identifies an IPv4 client by address and an IPv6 client by /64 prefix
func limiterKey(clientIP netip.Addr) netip.Prefix {
	clientIP = clientIP.Unmap()
	if !clientIP.IsValid() {
		return netip.Prefix{}
	}
	if clientIP.Is4() {
		return netip.PrefixFrom(clientIP, 32)
	}
	prefix, _ := clientIP.WithZone("").Prefix(64)
	return prefix
}
