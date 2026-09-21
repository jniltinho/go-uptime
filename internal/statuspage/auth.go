package statuspage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"strconv"
	"sync"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/security"
)

// Fork: login of a status page. A page with auth answers 401 on every route of the page without the credential of that
// page. Everything here is about making that check cheap and safe:
//
//   - failures are counted per page and client IP, so that one page never locks another;
//   - a successful check is remembered for a few minutes, so that a page being refreshed by many visitors does not pay
//     bcrypt on every request;
//   - the key of that memory is an HMAC with a key drawn at startup, so that it is not a digest of the password that
//     could be attacked outside of the process, and it has the length of each part as a prefix, so that credentials
//     like "a" / "b:c" and "a:b" / "c" never land on the same entry;
//   - the number of bcrypt checks running at the same time is capped, so that a flood of wrong passwords cannot take
//     the CPU of the whole installation.
const (
	// authFailureWindow is how long the failures of a client on a page are counted
	authFailureWindow = 5 * time.Minute

	// authMaximumFailures is how many failures a client may have on a page during the window
	authMaximumFailures = 10

	// authMaximumFailureKeys is the maximum number of page and client pairs kept by the limiter
	authMaximumFailureKeys = 10000

	// authVerificationTTL is how long a successful verification is remembered
	authVerificationTTL = 5 * time.Minute

	// authMaximumVerifications is the maximum number of remembered verifications
	authMaximumVerifications = 10000

	// authMaximumConcurrentChecks is the maximum number of bcrypt checks running at the same time
	authMaximumConcurrentChecks = 4
)

var (
	// authMemoryKey keys the HMAC of the remembered verifications. It is drawn once per process and never leaves it.
	authMemoryKey = newAuthMemoryKey()

	authChecks = make(chan struct{}, authMaximumConcurrentChecks)

	authMemory = &verificationMemory{entries: make(map[string]time.Time)}

	authFailures = &pageFailureLimiter{failures: make(map[string]*failureCount)}
)

func newAuthMemoryKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// A process without a source of randomness cannot remember verifications safely: the memory is then disabled by
		// an empty key, and every request pays bcrypt.
		return nil
	}
	return key
}

// AuthResult is the outcome of the verification of the credential of a page
type AuthResult int

const (
	// AuthAllowed means the request may go on: either the page has no login, or the credential is right
	AuthAllowed AuthResult = iota

	// AuthUnauthorized means the page requires a login and the credential is missing or wrong
	AuthUnauthorized

	// AuthTooManyFailures means the client failed too many times on this page during the window
	AuthTooManyFailures
)

// VerifyPageCredential returns whether the request may see the page, and for how long the client has to wait when it
// failed too many times. The order is fixed: limiter, memory, bcrypt, and only then the failure is recorded.
func VerifyPageCredential(page *pageconfig.Page, slug, username, password string, hasCredential bool, clientIP netip.Addr, now time.Time) (AuthResult, time.Duration) {
	if !page.RequiresLogin() {
		return AuthAllowed, 0
	}
	key := failureKey(slug, clientIP)
	if blocked, retryAfter := authFailures.blocked(key, now); blocked {
		return AuthTooManyFailures, retryAfter
	}
	if !hasCredential {
		authFailures.failure(key, now)
		return AuthUnauthorized, 0
	}
	fingerprint := security.CredentialFingerprint(page.Auth.Username, page.Auth.PasswordBcryptHashBase64Encoded)
	memoryKey := verificationKey(slug, username, password, fingerprint)
	if authMemory.contains(memoryKey, now) {
		return AuthAllowed, 0
	}
	if !checkWithLimitedConcurrency(page, username, password) {
		authFailures.failure(key, now)
		return AuthUnauthorized, 0
	}
	authMemory.remember(memoryKey, now.Add(authVerificationTTL))
	return AuthAllowed, 0
}

// checkCredentials is security.CheckCredentials, replaced by the tests to count how many checks are really made
var checkCredentials = security.CheckCredentials

func checkWithLimitedConcurrency(page *pageconfig.Page, username, password string) bool {
	authChecks <- struct{}{}
	defer func() { <-authChecks }()
	return checkCredentials(page.Auth.Username, page.Auth.PasswordBcryptHashBase64Encoded, username, password)
}

// failureKey identifies a client on a page: the prefix of the IP address, as the limiter of the public routes does
func failureKey(slug string, clientIP netip.Addr) string {
	prefixLength := 32
	if clientIP.Is6() && !clientIP.Is4In6() {
		prefixLength = 64
	}
	prefix, err := clientIP.Prefix(prefixLength)
	if err != nil {
		return slug + "\x00" + clientIP.String()
	}
	return slug + "\x00" + prefix.String()
}

// verificationKey is the HMAC of the slug, the credential presented and the fingerprint of the credential of the page,
// each part with its length as a prefix. Without the key of the process it returns an empty string, which disables the
// memory.
func verificationKey(slug, username, password, fingerprint string) string {
	if len(authMemoryKey) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, authMemoryKey)
	for _, part := range []string{slug, username, password, fingerprint} {
		mac.Write([]byte(strconv.Itoa(len(part))))
		mac.Write([]byte(":"))
		mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// resetAuthState forgets every remembered verification and every counted failure. It is called when the status pages are
// loaded, like the cache of the public payloads.
func resetAuthState() {
	authMemory.reset()
	authFailures.reset()
}

// verificationMemory remembers successful verifications until their expiration
type verificationMemory struct {
	mutex   sync.Mutex
	entries map[string]time.Time
}

func (m *verificationMemory) contains(key string, now time.Time) bool {
	if len(key) == 0 {
		return false
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	expiresAt, exists := m.entries[key]
	if !exists {
		return false
	}
	if !now.Before(expiresAt) {
		delete(m.entries, key)
		return false
	}
	return true
}

func (m *verificationMemory) remember(key string, expiresAt time.Time) {
	if len(key) == 0 {
		return
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(m.entries) >= authMaximumVerifications {
		m.evictExpired(expiresAt.Add(-authVerificationTTL))
		if len(m.entries) >= authMaximumVerifications {
			// Still full of valid entries: the oldest one gives way, so that the memory never grows without a bound
			oldestKey, oldest := "", time.Time{}
			for entryKey, entryExpiresAt := range m.entries {
				if oldest.IsZero() || entryExpiresAt.Before(oldest) {
					oldestKey, oldest = entryKey, entryExpiresAt
				}
			}
			delete(m.entries, oldestKey)
		}
	}
	m.entries[key] = expiresAt
}

// evictExpired removes the entries expired at the given time. The caller holds the lock.
func (m *verificationMemory) evictExpired(now time.Time) {
	for key, expiresAt := range m.entries {
		if !now.Before(expiresAt) {
			delete(m.entries, key)
		}
	}
}

func (m *verificationMemory) reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entries = make(map[string]time.Time)
}

// pageFailureLimiter counts the failed credentials of each client on each page
type pageFailureLimiter struct {
	mutex    sync.Mutex
	failures map[string]*failureCount
}

type failureCount struct {
	count     int
	expiresAt time.Time
}

func (l *pageFailureLimiter) blocked(key string, now time.Time) (bool, time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	entry, exists := l.failures[key]
	if !exists {
		return false, 0
	}
	if !now.Before(entry.expiresAt) {
		delete(l.failures, key)
		return false, 0
	}
	if entry.count < authMaximumFailures {
		return false, 0
	}
	return true, entry.expiresAt.Sub(now)
}

func (l *pageFailureLimiter) failure(key string, now time.Time) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	entry, exists := l.failures[key]
	if !exists || !now.Before(entry.expiresAt) {
		if len(l.failures) >= authMaximumFailureKeys {
			l.evictExpired(now)
			if len(l.failures) >= authMaximumFailureKeys {
				return
			}
		}
		l.failures[key] = &failureCount{count: 1, expiresAt: now.Add(authFailureWindow)}
		return
	}
	entry.count++
}

// evictExpired removes the entries expired at the given time. The caller holds the lock.
func (l *pageFailureLimiter) evictExpired(now time.Time) {
	for key, entry := range l.failures {
		if !now.Before(entry.expiresAt) {
			delete(l.failures, key)
		}
	}
}

func (l *pageFailureLimiter) reset() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.failures = make(map[string]*failureCount)
}
