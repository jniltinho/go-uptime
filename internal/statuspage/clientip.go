// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/TwiN/logr"
)

const (
	// maximumForwardedForEntries is the maximum number of X-Forwarded-For entries considered; with more, the IP address
	// of the connection is used
	maximumForwardedForEntries = 20

	// maximumForwardedForLineLength is the maximum length of an X-Forwarded-For line; with a longer one, the IP address
	// of the connection is used
	maximumForwardedForLineLength = 1024
)

var (
	// sharedRateLimitWarningGeneration is the generation in which the shared rate limit warning was logged
	sharedRateLimitWarningGeneration atomic.Uint64

	// sharedRateLimitWarningIP is the untrusted proxy IP address of the last shared rate limit warning
	sharedRateLimitWarningIP atomic.Pointer[string]

	carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")
)

// ClientIP returns the IP address that identifies the client of a request. It is the IP address of the connection,
// unless the connection comes from a trusted proxy: then, the X-Forwarded-For lines are read from right to left and the
// first address that is not a trusted proxy is used. Too many entries, a line that is too long or an invalid entry
// make the IP address of the connection be used.
func ClientIP(remoteIP netip.Addr, forwardedFor []string, trustedProxies []netip.Prefix) netip.Addr {
	remoteIP = remoteIP.Unmap()
	if len(forwardedFor) == 0 || !isTrustedProxy(remoteIP, trustedProxies) {
		return remoteIP
	}
	entries := make([]string, 0, 4)
	for _, line := range forwardedFor {
		if len(line) > maximumForwardedForLineLength {
			return remoteIP
		}
		for _, entry := range strings.Split(line, ",") {
			if len(entries) == maximumForwardedForEntries {
				return remoteIP
			}
			entries = append(entries, strings.TrimSpace(entry))
		}
	}
	clientIP := remoteIP
	for i := len(entries) - 1; i >= 0; i-- {
		addr, ok := parseForwardedAddr(entries[i])
		if !ok {
			return remoteIP
		}
		clientIP = addr
		if !isTrustedProxy(addr, trustedProxies) {
			break
		}
	}
	// If every entry is a trusted proxy, the left-most one is used
	return clientIP
}

// parseForwardedAddr parses an X-Forwarded-For entry: IP, IP:port or [IPv6]:port
func parseForwardedAddr(entry string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(entry)
	if err != nil {
		addrPort, portErr := netip.ParseAddrPort(entry)
		if portErr != nil {
			return netip.Addr{}, false
		}
		addr = addrPort.Addr()
	}
	if addr.Zone() != "" {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func isTrustedProxy(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// shouldWarnSharedRateLimit returns whether a connection looks like an untrusted reverse proxy: a private, carrier-grade
// NAT, loopback or link-local IP address that is not in trusted-proxies and sends X-Forwarded-For. All the clients of
// such a proxy share the same rate limit.
func shouldWarnSharedRateLimit(remoteIP netip.Addr, hasForwardedFor bool, trustedProxies []netip.Prefix) bool {
	remoteIP = remoteIP.Unmap()
	if !hasForwardedFor || !remoteIP.IsValid() || isTrustedProxy(remoteIP, trustedProxies) {
		return false
	}
	return remoteIP.IsPrivate() || remoteIP.IsLoopback() || remoteIP.IsLinkLocalUnicast() || carrierGradeNAT.Contains(remoteIP)
}

// warnSharedRateLimit logs the shared rate limit warning at most once per generation, and returns whether it was logged
func warnSharedRateLimit(generation uint64, remoteIP netip.Addr) bool {
	for {
		warnedGeneration := sharedRateLimitWarningGeneration.Load()
		if warnedGeneration == generation {
			return false
		}
		if sharedRateLimitWarningGeneration.CompareAndSwap(warnedGeneration, generation) {
			ip := remoteIP.Unmap().String()
			sharedRateLimitWarningIP.Store(&ip)
			logr.Warnf("[statuspage.ClientIP] Requests come from %s with X-Forwarded-For, but %s is not in status-pages.trusted-proxies: every client behind this proxy shares the same rate limit of the public status pages, the same limit of failed logins and the same limit of real-time connections. Add it to status-pages.trusted-proxies if it is your reverse proxy", ip, ip)
			return true
		}
	}
}

// ObserveConnection logs the shared rate limit warning, once per generation, if the connection looks like an untrusted
// reverse proxy
func ObserveConnection(remoteIP netip.Addr, hasForwardedFor bool, trustedProxies []netip.Prefix) {
	if shouldWarnSharedRateLimit(remoteIP, hasForwardedFor, trustedProxies) {
		warnSharedRateLimit(Generation(), remoteIP)
	}
}

// SharedRateLimitWarning returns the untrusted proxy IP address that triggered the shared rate limit warning in the
// current generation, if any
func SharedRateLimitWarning() (string, bool) {
	generation := Generation()
	if generation == 0 || sharedRateLimitWarningGeneration.Load() != generation {
		return "", false
	}
	if ip := sharedRateLimitWarningIP.Load(); ip != nil {
		return *ip, true
	}
	return "", false
}
