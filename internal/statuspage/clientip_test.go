// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"net/netip"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	proxy := netip.MustParseAddr("172.30.0.1")
	scenarios := []struct {
		name         string
		remoteIP     netip.Addr
		forwardedFor []string
		expected     string
	}{
		{name: "untrusted-connection-ignores-header", remoteIP: netip.MustParseAddr("198.51.100.1"), forwardedFor: []string{"203.0.113.7"}, expected: "198.51.100.1"},
		{name: "trusted-proxy-without-header", remoteIP: proxy, expected: "172.30.0.1"},
		{name: "trusted-proxy", remoteIP: proxy, forwardedFor: []string{"203.0.113.7"}, expected: "203.0.113.7"},
		{name: "forged-left-most-entry", remoteIP: proxy, forwardedFor: []string{"1.1.1.1, 203.0.113.7"}, expected: "203.0.113.7"},
		{name: "chain-of-trusted-proxies", remoteIP: proxy, forwardedFor: []string{"203.0.113.7, 10.0.0.2"}, expected: "203.0.113.7"},
		{name: "two-lines", remoteIP: proxy, forwardedFor: []string{"1.1.1.1", "203.0.113.7"}, expected: "203.0.113.7"},
		{name: "ipv4-mapped-connection", remoteIP: netip.MustParseAddr("::ffff:172.30.0.1"), forwardedFor: []string{"203.0.113.7"}, expected: "203.0.113.7"},
		{name: "ipv4-mapped-entry", remoteIP: proxy, forwardedFor: []string{"::ffff:203.0.113.7"}, expected: "203.0.113.7"},
		{name: "ipv4-with-port", remoteIP: proxy, forwardedFor: []string{"203.0.113.7:8080"}, expected: "203.0.113.7"},
		{name: "ipv6", remoteIP: proxy, forwardedFor: []string{"2001:db8::2"}, expected: "2001:db8::2"},
		{name: "ipv6-with-port", remoteIP: proxy, forwardedFor: []string{"[2001:db8::1]:443"}, expected: "2001:db8::1"},
		{name: "every-entry-trusted", remoteIP: proxy, forwardedFor: []string{"10.0.0.3, 10.0.0.2"}, expected: "10.0.0.3"},
		{name: "invalid-entry", remoteIP: proxy, forwardedFor: []string{"unknown, 203.0.113.7, nonsense"}, expected: "172.30.0.1"},
		{name: "empty-entry", remoteIP: proxy, forwardedFor: []string{"1.1.1.1,,203.0.113.7"}, expected: "203.0.113.7"},
		{name: "empty-right-most-entry", remoteIP: proxy, forwardedFor: []string{"203.0.113.7,"}, expected: "172.30.0.1"},
		{name: "zone", remoteIP: proxy, forwardedFor: []string{"fe80::1%eth0"}, expected: "172.30.0.1"},
		{name: "too-many-entries", remoteIP: proxy, forwardedFor: []string{strings.Repeat("203.0.113.7, ", 20) + "203.0.113.8"}, expected: "172.30.0.1"},
		{name: "ten-thousand-entries", remoteIP: proxy, forwardedFor: []string{strings.TrimSuffix(strings.Repeat("1.1.1.1,", 10000), ",")}, expected: "172.30.0.1"},
		{name: "too-many-entries-across-lines", remoteIP: proxy, forwardedFor: append(make([]string, 0, 21), strings.Split(strings.TrimSuffix(strings.Repeat("1.1.1.1\n", 21), "\n"), "\n")...), expected: "172.30.0.1"},
		{name: "line-too-long", remoteIP: proxy, forwardedFor: []string{strings.Repeat(" ", 1100) + "203.0.113.7"}, expected: "172.30.0.1"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if actual := ClientIP(scenario.remoteIP, scenario.forwardedFor, trusted); actual.String() != scenario.expected {
				t.Errorf("expected %s, got %s", scenario.expected, actual)
			}
		})
	}
}

func TestShouldWarnSharedRateLimit(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.1/32")}
	scenarios := []struct {
		remoteIP        string
		hasForwardedFor bool
		expected        bool
	}{
		{remoteIP: "172.18.0.1", hasForwardedFor: true, expected: true},
		{remoteIP: "192.168.1.10", hasForwardedFor: true, expected: true},
		{remoteIP: "100.64.1.1", hasForwardedFor: true, expected: true},
		{remoteIP: "127.0.0.1", hasForwardedFor: true, expected: true},
		{remoteIP: "169.254.1.1", hasForwardedFor: true, expected: true},
		{remoteIP: "fc00::1", hasForwardedFor: true, expected: true},
		{remoteIP: "::ffff:10.1.2.3", hasForwardedFor: true, expected: true},
		{remoteIP: "172.30.0.1", hasForwardedFor: true, expected: false},
		{remoteIP: "172.18.0.1", hasForwardedFor: false, expected: false},
		{remoteIP: "203.0.113.7", hasForwardedFor: true, expected: false},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.remoteIP, func(t *testing.T) {
			if actual := shouldWarnSharedRateLimit(netip.MustParseAddr(scenario.remoteIP), scenario.hasForwardedFor, trusted); actual != scenario.expected {
				t.Errorf("expected %v, got %v", scenario.expected, actual)
			}
		})
	}
}

func TestWarnSharedRateLimit(t *testing.T) {
	proxy := netip.MustParseAddr("172.18.0.1")
	generation := sharedRateLimitWarningGeneration.Load() + 1000
	if !warnSharedRateLimit(generation, proxy) {
		t.Error("expected the warning to be logged for a new generation")
	}
	if warnSharedRateLimit(generation, proxy) {
		t.Error("expected the warning to be logged only once per generation")
	}
	if !warnSharedRateLimit(generation+1, proxy) {
		t.Error("expected the warning to be logged again after a reload")
	}
}
