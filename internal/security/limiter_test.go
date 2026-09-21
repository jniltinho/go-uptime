// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package security

import (
	"net/netip"
	"testing"
	"time"
)

func TestFailureLimiter(t *testing.T) {
	limiter := newFailureLimiter(10, 100)
	client := netip.MustParseAddr("203.0.113.10")
	start := time.Date(2026, 9, 15, 12, 0, 30, 0, time.UTC)
	for i := 0; i < 9; i++ {
		limiter.Failure(client, start.Add(time.Duration(i)*time.Second))
	}
	if blocked, _ := limiter.Blocked(client, start.Add(10*time.Second)); blocked {
		t.Fatal("expected the client not to be blocked after 9 failures")
	}
	limiter.Failure(client, start.Add(10*time.Second))
	blocked, retryAfter := limiter.Blocked(client, start.Add(20*time.Second))
	if !blocked || retryAfter != 40*time.Second {
		t.Fatalf("expected the client to be blocked for 40s after 10 failures, got blocked=%v retryAfter=%s", blocked, retryAfter)
	}
	// Blocked only checks: it does not extend the block
	if blocked, _ := limiter.Blocked(client, start.Add(30*time.Second)); !blocked {
		t.Error("expected the client to still be blocked")
	}
	if blocked, _ := limiter.Blocked(netip.MustParseAddr("203.0.113.11"), start.Add(20*time.Second)); blocked {
		t.Error("expected another client not to be blocked")
	}
	if blocked, retryAfter := limiter.Blocked(client, start.Add(time.Minute-time.Millisecond)); !blocked || retryAfter != time.Second {
		t.Errorf("expected a retry after of at least one second at the end of the window, got blocked=%v retryAfter=%s", blocked, retryAfter)
	}
	// The window restarts one minute after its first failure
	if blocked, _ := limiter.Blocked(client, start.Add(time.Minute)); blocked {
		t.Error("expected the block to end with the window")
	}
	limiter.Failure(client, start.Add(time.Minute))
	for i := 0; i < 8; i++ {
		limiter.Failure(client, start.Add(time.Minute+time.Second))
	}
	if blocked, _ := limiter.Blocked(client, start.Add(time.Minute+2*time.Second)); blocked {
		t.Error("expected the failures of the previous window to be forgotten")
	}
}

func TestFailureLimiter_Keys(t *testing.T) {
	limiter := newFailureLimiter(1, 2)
	now := time.Now()
	limiter.Failure(netip.MustParseAddr("2001:db8::1"), now)
	if blocked, _ := limiter.Blocked(netip.MustParseAddr("2001:db8::ffff"), now); !blocked {
		t.Error("expected the IPv6 clients of the same /64 to share the failures")
	}
	if blocked, _ := limiter.Blocked(netip.MustParseAddr("2001:db8:0:1::1"), now); blocked {
		t.Error("expected the IPv6 clients of another /64 not to be blocked")
	}
	limiter.Failure(netip.MustParseAddr("198.51.100.1"), now)
	if blocked, _ := limiter.Blocked(netip.MustParseAddr("::ffff:198.51.100.1"), now); !blocked {
		t.Error("expected an IPv4-mapped IPv6 address to be identified as its IPv4 address")
	}
	// A third client makes the limiter forget the client with the oldest failure
	limiter.Failure(netip.MustParseAddr("198.51.100.2"), now)
	if len(limiter.entries) != 2 || limiter.recency.Len() != 2 {
		t.Errorf("expected at most 2 tracked clients, got %d", len(limiter.entries))
	}
	if blocked, _ := limiter.Blocked(netip.MustParseAddr("2001:db8::1"), now); blocked {
		t.Error("expected the client with the oldest failure to be forgotten")
	}
}

func TestNewFailureLimiter_OwnWindow(t *testing.T) {
	restore := NewFailureLimiter(15*time.Minute, 2, 100)
	login := newFailureLimiter(2, 100)
	clientIP := netip.MustParseAddr("203.0.113.9")
	now := time.Now()
	restore.Failure(clientIP, now)
	restore.Failure(clientIP, now)
	if blocked, retryAfter := restore.Blocked(clientIP, now.Add(10*time.Minute)); !blocked || retryAfter != 5*time.Minute {
		t.Errorf("expected the restore limiter to block for the rest of its 15 minutes, got %v %s", blocked, retryAfter)
	}
	if blocked, _ := login.Blocked(clientIP, now); blocked {
		t.Error("the failures of the restore limiter must not block the login")
	}
	login.Failure(clientIP, now)
	login.Failure(clientIP, now)
	if blocked, _ := login.Blocked(clientIP, now.Add(2*time.Minute)); blocked {
		t.Error("expected the login limiter to keep its window of one minute")
	}
}
