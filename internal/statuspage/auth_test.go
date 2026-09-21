// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"bytes"
	"encoding/base64"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"golang.org/x/crypto/bcrypt"
)

// Fork: login of a status page

func pageWithLogin(t *testing.T, slug, username, password string) *pageconfig.Page {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &pageconfig.Page{
		Slug:  slug,
		Title: slug,
		Auth:  &pageconfig.PageAuth{Username: username, PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)},
	}
}

func TestVerifyPageCredential(t *testing.T) {
	defer resetAuthState()
	resetAuthState()
	page := pageWithLogin(t, "clients", "client", "page-secret")
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	scenarios := []struct {
		name          string
		page          *pageconfig.Page
		username      string
		password      string
		hasCredential bool
		expected      AuthResult
	}{
		{name: "a page without a login lets anyone in", page: &pageconfig.Page{Slug: "infra"}, expected: AuthAllowed},
		{name: "without a credential", page: page, expected: AuthUnauthorized},
		{name: "with a wrong password", page: page, username: "client", password: "wrong", hasCredential: true, expected: AuthUnauthorized},
		{name: "with a wrong username", page: page, username: "other", password: "page-secret", hasCredential: true, expected: AuthUnauthorized},
		{name: "with the credential of the page", page: page, username: "client", password: "page-secret", hasCredential: true, expected: AuthAllowed},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			resetAuthState()
			result, _ := VerifyPageCredential(scenario.page, scenario.page.Slug, scenario.username, scenario.password, scenario.hasCredential, client, now)
			if result != scenario.expected {
				t.Errorf("expected %d, got %d", scenario.expected, result)
			}
		})
	}
}

func TestVerifyPageCredential_FailuresOfOnePageDoNotLockAnother(t *testing.T) {
	defer resetAuthState()
	resetAuthState()
	clients := pageWithLogin(t, "clients", "client", "page-secret")
	partners := pageWithLogin(t, "partners", "partner", "other-secret")
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	for i := 0; i < authMaximumFailures; i++ {
		if result, _ := VerifyPageCredential(clients, clients.Slug, "client", "wrong", true, client, now); result != AuthUnauthorized {
			t.Fatalf("expected 401 on the failure %d, got %d", i+1, result)
		}
	}
	result, retryAfter := VerifyPageCredential(clients, clients.Slug, "client", "page-secret", true, client, now)
	if result != AuthTooManyFailures {
		t.Fatalf("expected the client to be blocked on the page, got %d", result)
	}
	if retryAfter <= 0 || retryAfter > authFailureWindow {
		t.Errorf("expected a wait inside the window, got %s", retryAfter)
	}
	// The same client on another page, and another client on the same page, are not affected
	if result, _ = VerifyPageCredential(partners, partners.Slug, "partner", "other-secret", true, client, now); result != AuthAllowed {
		t.Errorf("expected the other page to let the client in, got %d", result)
	}
	other := netip.MustParseAddr("203.0.113.20")
	if result, _ = VerifyPageCredential(clients, clients.Slug, "client", "page-secret", true, other, now); result != AuthAllowed {
		t.Errorf("expected another client to be let in, got %d", result)
	}
	// After the window the client may try again
	if result, _ = VerifyPageCredential(clients, clients.Slug, "client", "page-secret", true, client, now.Add(authFailureWindow+time.Second)); result != AuthAllowed {
		t.Errorf("expected the client to be let in after the window, got %d", result)
	}
}

func TestVerifyPageCredential_RemembersASuccessfulCheck(t *testing.T) {
	defer func() {
		checkCredentials = originalCheckCredentials
		resetAuthState()
	}()
	resetAuthState()
	var mutex sync.Mutex
	var checks int
	checkCredentials = func(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password string) bool {
		mutex.Lock()
		checks++
		mutex.Unlock()
		return originalCheckCredentials(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password)
	}
	page := pageWithLogin(t, "clients", "client", "page-secret")
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	for i := 0; i < 5; i++ {
		if result, _ := VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now); result != AuthAllowed {
			t.Fatalf("expected the credential to be accepted, got %d", result)
		}
	}
	if checks != 1 {
		t.Errorf("expected a single check of the password, got %d", checks)
	}
	// Past the lifetime of the memory the password is checked again
	if result, _ := VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now.Add(authVerificationTTL+time.Second)); result != AuthAllowed {
		t.Fatalf("expected the credential to keep being accepted, got %d", result)
	}
	if checks != 2 {
		t.Errorf("expected the password to be checked again after the lifetime of the memory, got %d checks", checks)
	}
	// A wrong password is never remembered
	before := checks
	for i := 0; i < 3; i++ {
		VerifyPageCredential(page, page.Slug, "client", "wrong", true, client, now)
	}
	if checks != before+3 {
		t.Errorf("expected every wrong password to be checked, got %d checks", checks-before)
	}
}

var originalCheckCredentials = checkCredentials

func TestVerificationKey_SeparatesTheParts(t *testing.T) {
	// "a" / "b:c" and "a:b" / "c" must never land on the same entry of the memory
	first := verificationKey("clients", "a", "b:c", "fingerprint")
	second := verificationKey("clients", "a:b", "c", "fingerprint")
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("expected the key of the process to be available")
	}
	if first == second {
		t.Error("expected credentials cut differently to have different keys")
	}
	// The page, the credential and the fingerprint all take part in the key
	if first == verificationKey("partners", "a", "b:c", "fingerprint") {
		t.Error("expected the page to take part in the key")
	}
	if first == verificationKey("clients", "a", "b:c", "other") {
		t.Error("expected the fingerprint of the credential of the page to take part in the key")
	}
}

func TestResetAuthState(t *testing.T) {
	defer resetAuthState()
	resetAuthState()
	page := pageWithLogin(t, "clients", "client", "page-secret")
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	for i := 0; i < authMaximumFailures; i++ {
		VerifyPageCredential(page, page.Slug, "client", "wrong", true, client, now)
	}
	if result, _ := VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now); result != AuthTooManyFailures {
		t.Fatalf("expected the client to be blocked, got %d", result)
	}
	// Loading the status pages forgets the failures and the remembered verifications
	resetAuthState()
	if result, _ := VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now); result != AuthAllowed {
		t.Errorf("expected the client to be let in after the state was reset, got %d", result)
	}
}

func TestVerifyPageCredential_BlockedClientDoesNotReachTheComparison(t *testing.T) {
	defer func() {
		checkCredentials = originalCheckCredentials
		resetAuthState()
	}()
	resetAuthState()
	var checks int
	checkCredentials = func(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password string) bool {
		checks++
		return originalCheckCredentials(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password)
	}
	page := pageWithLogin(t, "clients", "client", "page-secret")
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	for i := 0; i < authMaximumFailures; i++ {
		VerifyPageCredential(page, page.Slug, "client", "wrong", true, client, now)
	}
	blockedAfter := checks
	for i := 0; i < 5; i++ {
		if result, _ := VerifyPageCredential(page, page.Slug, "client", "wrong", true, client, now); result != AuthTooManyFailures {
			t.Fatalf("expected the client to be blocked, got %d", result)
		}
	}
	if checks != blockedAfter {
		t.Errorf("expected no comparison of the password while the client is blocked, got %d", checks-blockedAfter)
	}
}

func TestVerifyPageCredential_ChangingThePasswordOfThePageForgetsTheMemory(t *testing.T) {
	defer func() {
		checkCredentials = originalCheckCredentials
		resetAuthState()
	}()
	resetAuthState()
	var checks int
	checkCredentials = func(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password string) bool {
		checks++
		return originalCheckCredentials(expectedUsername, expectedPasswordBcryptHashBase64Encoded, username, password)
	}
	client := netip.MustParseAddr("203.0.113.10")
	now := time.Now()
	page := pageWithLogin(t, "clients", "client", "page-secret")
	VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now)
	VerifyPageCredential(page, page.Slug, "client", "page-secret", true, client, now)
	if checks != 1 {
		t.Fatalf("expected the memory to answer the second request, got %d comparisons", checks)
	}
	// The same credential on a page whose stored hash changed is compared again, and refused when it no longer matches
	changed := pageWithLogin(t, "clients", "client", "another-secret")
	if result, _ := VerifyPageCredential(changed, changed.Slug, "client", "page-secret", true, client, now); result != AuthUnauthorized {
		t.Errorf("expected the old password to be refused, got %d", result)
	}
	if checks != 2 {
		t.Errorf("expected the password to be compared again after the page changed, got %d comparisons", checks)
	}
}

func TestWarnAboutLoginWithoutSecurity(t *testing.T) {
	defer logr.SetOutput(os.Stdout)
	page := pageWithLogin(t, "clients", "client", "page-secret")
	published := &snapshot{enabled: true, configStates: map[string]*State{"clients": {Origin: OriginConfig, Slug: "clients", Page: page}}, managedStates: map[string]*State{}}
	scenarios := []struct {
		name     string
		cfg      *config.Config
		snap     *snapshot
		expected bool
	}{
		{name: "without security", cfg: &config.Config{}, snap: published, expected: true},
		{name: "with security", cfg: &config.Config{Security: &security.Config{}}, snap: published, expected: false},
		{
			name:     "without a page that requires a login",
			cfg:      &config.Config{},
			snap:     &snapshot{enabled: true, configStates: map[string]*State{"infra": {Origin: OriginConfig, Slug: "infra", Page: &pageconfig.Page{Slug: "infra"}}}, managedStates: map[string]*State{}},
			expected: false,
		},
		{
			name:     "with the status pages disabled",
			cfg:      &config.Config{},
			snap:     &snapshot{enabled: false, configStates: published.configStates, managedStates: map[string]*State{}},
			expected: false,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var logs bytes.Buffer
			logr.SetOutput(&logs)
			warnAboutLoginWithoutSecurity(scenario.cfg, scenario.snap)
			if warned := strings.Contains(logs.String(), "login of their own"); warned != scenario.expected {
				t.Errorf("expected the warning to be %v, got %q", scenario.expected, logs.String())
			}
		})
	}
}
