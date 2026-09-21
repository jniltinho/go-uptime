// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestLimiter_SlidingWindow(t *testing.T) {
	limiter := NewLimiter(3, 100)
	client := netip.MustParseAddr("203.0.113.7")
	start := time.Unix(600, 0) // start of a window
	for i := 0; i < 3; i++ {
		if allowed, _ := limiter.Hit(client, start.Add(time.Duration(i)*time.Second)); !allowed {
			t.Fatalf("expected hit %d to be allowed", i+1)
		}
	}
	allowed, retryAfter := limiter.Hit(client, start.Add(10*time.Second))
	if allowed || retryAfter != 50*time.Second {
		t.Errorf("expected the 4th hit to be blocked for 50s, got allowed=%v retryAfter=%s", allowed, retryAfter)
	}
	// At the start of the next window, the previous window still counts fully
	if allowed, _ := limiter.Hit(client, start.Add(time.Minute)); allowed {
		t.Error("expected the hit at the start of the next window to be blocked")
	}
	// Halfway through the next window, the previous window counts for half: 1.5 + 1 <= 3
	if allowed, _ := limiter.Hit(client, start.Add(90*time.Second)); !allowed {
		t.Error("expected a hit halfway through the next window to be allowed")
	}
	if allowed, _ := limiter.Hit(client, start.Add(90*time.Second)); allowed {
		t.Error("expected the next hit to be blocked: 1.5 + 1 + 1 > 3")
	}
	if allowed, _ := limiter.Hit(client, start.Add(5*time.Minute)); !allowed {
		t.Error("expected the counters to be reset after two windows")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.8"), start.Add(10*time.Second)); !allowed {
		t.Error("expected another IPv4 client to have its own limit")
	}
}

func TestLimiter_Unlimited(t *testing.T) {
	limiter := NewLimiter(0, 100)
	for i := 0; i < 1000; i++ {
		if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.7"), time.Unix(600, 0)); !allowed {
			t.Fatal("expected every hit to be allowed with a limit of 0")
		}
	}
	if len(limiter.entries) != 0 {
		t.Errorf("expected no client to be tracked with a limit of 0, got %d", len(limiter.entries))
	}
	limiter.SetLimit(1)
	limiter.Hit(netip.MustParseAddr("203.0.113.7"), time.Unix(600, 0))
	if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.7"), time.Unix(601, 0)); allowed {
		t.Error("expected SetLimit to apply to the next hits")
	}
}

func TestLimiter_IPv6Prefix(t *testing.T) {
	limiter := NewLimiter(1, 100)
	now := time.Unix(600, 0)
	if allowed, _ := limiter.Hit(netip.MustParseAddr("2001:db8::1"), now); !allowed {
		t.Fatal("expected the first hit to be allowed")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("2001:db8::ffff:2"), now); allowed {
		t.Error("expected addresses of the same /64 to share the limit")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("2001:db8:0:1::1"), now); !allowed {
		t.Error("expected another /64 to have its own limit")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("::ffff:203.0.113.7"), now); !allowed {
		t.Fatal("expected the first hit of an IPv4-mapped address to be allowed")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.7"), now); allowed {
		t.Error("expected an IPv4-mapped address to share the limit of the IPv4 address")
	}
}

func TestLimiter_MaximumKeys(t *testing.T) {
	limiter := NewLimiter(1, 3)
	now := time.Unix(600, 0)
	for i := 1; i <= 4; i++ {
		limiter.Hit(netip.MustParseAddr(fmt.Sprintf("203.0.113.%d", i)), now)
	}
	if len(limiter.entries) != 3 || limiter.recency.Len() != 3 {
		t.Fatalf("expected at most 3 tracked clients, got %d", len(limiter.entries))
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.1"), now); !allowed {
		t.Error("expected the least recently seen client to have been forgotten")
	}
	if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.4"), now); allowed {
		t.Error("expected a recently seen client to still be limited")
	}
}

func TestLimiter_Concurrency(t *testing.T) {
	limiter := NewLimiter(50, 1000)
	now := time.Unix(600, 0)
	var waitGroup sync.WaitGroup
	var mutex sync.Mutex
	allowedHits := 0
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for j := 0; j < 10; j++ {
				if allowed, _ := limiter.Hit(netip.MustParseAddr("203.0.113.7"), now); allowed {
					mutex.Lock()
					allowedHits++
					mutex.Unlock()
				}
			}
		}()
	}
	waitGroup.Wait()
	if allowedHits != 50 {
		t.Errorf("expected exactly 50 allowed hits, got %d", allowedHits)
	}
}
