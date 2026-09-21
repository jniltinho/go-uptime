// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

func TestValidateSlug(t *testing.T) {
	scenarios := []struct {
		slug        string
		expectedErr error
	}{
		{slug: "infra"},
		{slug: "a"},
		{slug: "a1-b2-c3"},
		{slug: strings.Repeat("a", 64)},
		{slug: "", expectedErr: ErrInvalidSlug},
		{slug: "-infra", expectedErr: ErrInvalidSlug},
		{slug: "infra-", expectedErr: ErrInvalidSlug},
		{slug: "Infra", expectedErr: ErrInvalidSlug},
		{slug: "in_fra", expectedErr: ErrInvalidSlug},
		{slug: "a/b", expectedErr: ErrInvalidSlug},
		{slug: strings.Repeat("a", 65), expectedErr: ErrInvalidSlug},
		{slug: "options", expectedErr: ErrReservedSlug},
		{slug: "validate", expectedErr: ErrReservedSlug},
		{slug: "new", expectedErr: ErrReservedSlug},
		{slug: "preview", expectedErr: ErrReservedSlug},
		{slug: "exposure", expectedErr: ErrReservedSlug},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.slug, func(t *testing.T) {
			if err := ValidateSlug(scenario.slug); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected error %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}

func TestPage_ValidateAndSetDefaults(t *testing.T) {
	page := &Page{
		Slug:        " infra ",
		Title:       "  Infraestrutura ",
		Description: " Serviços públicos ",
		Groups:      []string{" core ", "database"},
		Endpoints:   []string{" Core_API "},
	}
	if err := page.ValidateAndSetDefaults(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Slug != "infra" || page.Title != "Infraestrutura" || page.Description != "Serviços públicos" {
		t.Errorf("expected trimmed values, got %+v", page)
	}
	if strings.Join(page.Groups, ",") != "core,database" || strings.Join(page.Endpoints, ",") != "core_api" {
		t.Errorf("expected normalized groups and endpoints, got groups=%q endpoints=%q", page.Groups, page.Endpoints)
	}
	if !page.IsEnabled() {
		t.Error("expected a page without enabled to be enabled")
	}
	disabled := false
	if (&Page{Enabled: &disabled}).IsEnabled() {
		t.Error("expected a page with enabled: false to be disabled")
	}
}

func TestPage_ValidateAndSetDefaultsErrors(t *testing.T) {
	manyEndpoints := make([]string, MaximumEndpointKeys+1)
	for i := range manyEndpoints {
		manyEndpoints[i] = fmt.Sprintf("core_endpoint-%d", i)
	}
	scenarios := []struct {
		name        string
		page        Page
		expectedErr error
	}{
		{name: "invalid-slug", page: Page{Slug: "Infra", Title: "t", Groups: []string{"core"}}, expectedErr: ErrInvalidSlug},
		{name: "reserved-slug", page: Page{Slug: "options", Title: "t", Groups: []string{"core"}}, expectedErr: ErrReservedSlug},
		{name: "empty-title", page: Page{Slug: "infra", Title: "   ", Groups: []string{"core"}}, expectedErr: ErrInvalidTitle},
		{name: "long-title", page: Page{Slug: "infra", Title: strings.Repeat("é", MaximumTitleLength+1), Groups: []string{"core"}}, expectedErr: ErrInvalidTitle},
		{name: "long-description", page: Page{Slug: "infra", Title: "t", Description: strings.Repeat("é", MaximumDescriptionLength+1), Groups: []string{"core"}}, expectedErr: ErrDescriptionTooLong},
		{name: "empty-selection", page: Page{Slug: "infra", Title: "t"}, expectedErr: ErrEmptySelection},
		{name: "duplicate-group", page: Page{Slug: "infra", Title: "t", Groups: []string{"core", " core"}}, expectedErr: ErrInvalidGroups},
		{name: "empty-group", page: Page{Slug: "infra", Title: "t", Groups: []string{" "}}, expectedErr: ErrInvalidGroups},
		{name: "duplicate-endpoint", page: Page{Slug: "infra", Title: "t", Endpoints: []string{"core_api", "CORE_API"}}, expectedErr: ErrInvalidEndpoints},
		{name: "too-many-endpoints", page: Page{Slug: "infra", Title: "t", Endpoints: manyEndpoints}, expectedErr: ErrInvalidEndpoints},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if err := scenario.page.ValidateAndSetDefaults(); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected error %v, got %v", scenario.expectedErr, err)
			}
		})
	}
	title := strings.Repeat("é", MaximumTitleLength)
	if err := (&Page{Slug: "infra", Title: title, Groups: []string{"core"}}).ValidateAndSetDefaults(); err != nil {
		t.Errorf("expected a title with %d multi-byte characters to be valid, got %v", MaximumTitleLength, err)
	}
}

func TestConfig_Defaults(t *testing.T) {
	var nilConfig *Config
	if !nilConfig.IsEnabled() || nilConfig.GetRateLimit() != DefaultRateLimit || nilConfig.TrustedProxyPrefixes() != nil {
		t.Error("expected a nil config to be enabled, with the default rate limit and no trusted proxy")
	}
	disabled, zero := false, 0
	config := &Config{Enabled: &disabled, RateLimit: &zero}
	if config.IsEnabled() || config.GetRateLimit() != 0 {
		t.Errorf("expected enabled=false and rate-limit=0 to be kept, got enabled=%v rate-limit=%d", config.IsEnabled(), config.GetRateLimit())
	}
	negative := -1
	if err := (&Config{RateLimit: &negative}).ValidateAndSetDefaults(); !errors.Is(err, ErrInvalidRateLimit) {
		t.Errorf("expected ErrInvalidRateLimit, got %v", err)
	}
}

func TestConfig_TrustedProxies(t *testing.T) {
	config := &Config{TrustedProxies: []string{"127.0.0.1", "::1", " 172.30.0.1/24 ", "::ffff:10.0.0.1", "::ffff:10.0.0.0/104", "2001:db8::/32"}}
	if err := config.ValidateAndSetDefaults(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"127.0.0.1/32", "::1/128", "172.30.0.0/24", "10.0.0.1/32", "10.0.0.0/8", "2001:db8::/32"}
	prefixes := config.TrustedProxyPrefixes()
	if len(prefixes) != len(expected) {
		t.Fatalf("expected %d prefixes, got %v", len(expected), prefixes)
	}
	for i, prefix := range prefixes {
		if prefix.String() != expected[i] {
			t.Errorf("entry %d: expected %s, got %s", i, expected[i], prefix)
		}
	}
	if !prefixes[2].Contains(netip.MustParseAddr("172.30.0.1")) {
		t.Error("expected 172.30.0.0/24 to contain 172.30.0.1")
	}
	for _, invalid := range []string{"proxy.local", "10.0.0.1/33", "fe80::1%eth0", "::ffff:10.0.0.0/64", ""} {
		if err := (&Config{TrustedProxies: []string{invalid}}).ValidateAndSetDefaults(); !errors.Is(err, ErrInvalidTrustedProxy) {
			t.Errorf("expected ErrInvalidTrustedProxy for %q, got %v", invalid, err)
		}
	}
}

func TestConfig_DuplicateSlug(t *testing.T) {
	config := &Config{Pages: []*Page{
		{Slug: "infra", Title: "Infra", Groups: []string{"core"}},
		{Slug: " infra", Title: "Infra 2", Groups: []string{"database"}},
	}}
	if err := config.ValidateAndSetDefaults(); !errors.Is(err, ErrDuplicateSlug) {
		t.Errorf("expected ErrDuplicateSlug, got %v", err)
	}
	if err := (&Config{Pages: []*Page{nil}}).ValidateAndSetDefaults(); err == nil {
		t.Error("expected an error for an empty page")
	}
}

// Fork: login of a status page

func TestPageAuth_ValidateAndSetDefaults(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("page-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	scenarios := []struct {
		name          string
		auth          *PageAuth
		expectedError error
	}{
		{name: "without a login", auth: nil},
		{
			name: "with a username and a hash",
			auth: &PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)},
		},
		{
			name: "with spaces around the username and the hash",
			auth: &PageAuth{Username: "  client  ", PasswordBcryptHashBase64Encoded: "  " + base64.URLEncoding.EncodeToString(hash) + "  "},
		},
		{
			name:          "without a username",
			auth:          &PageAuth{PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)},
			expectedError: ErrInvalidAuthUsername,
		},
		{
			name:          "with a username that is too long",
			auth:          &PageAuth{Username: strings.Repeat("a", MaximumAuthUsernameLength+1), PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)},
			expectedError: ErrInvalidAuthUsername,
		},
		{name: "without a hash", auth: &PageAuth{Username: "client"}, expectedError: ErrInvalidAuthPasswordHash},
		{
			name:          "with a hash that is not base64",
			auth:          &PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: "not base64!"},
			expectedError: ErrInvalidAuthPasswordHash,
		},
		{
			name:          "with a base64 that is not a bcrypt hash",
			auth:          &PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString([]byte("not-a-hash"))},
			expectedError: ErrInvalidAuthPasswordHash,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			page := &Page{Slug: "clients", Title: "Clients", Groups: []string{"core"}, Auth: scenario.auth}
			err := page.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.expectedError) {
				t.Fatalf("expected %v, got %v", scenario.expectedError, err)
			}
			if scenario.expectedError != nil {
				return
			}
			if page.RequiresLogin() != (scenario.auth != nil) {
				t.Errorf("expected RequiresLogin to be %v", scenario.auth != nil)
			}
			if scenario.auth != nil && page.Auth.Username != "client" {
				t.Errorf("expected the username to be trimmed, got %q", page.Auth.Username)
			}
		})
	}
}

// TestPageAuth_HashWithTheURLAlphabet makes sure the alphabet of security.basic works: the documented generator of the
// fork produces base64 with - and _
func TestPageAuth_HashWithTheURLAlphabet(t *testing.T) {
	for i := 0; i < 50; i++ {
		hash, err := bcrypt.GenerateFromPassword([]byte("page-secret"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		encoded := base64.URLEncoding.EncodeToString(hash)
		if !strings.ContainsAny(encoded, "-_") {
			continue
		}
		page := &Page{Slug: "clients", Title: "Clients", Groups: []string{"core"}, Auth: &PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: encoded}}
		if err = page.ValidateAndSetDefaults(); err != nil {
			t.Fatalf("expected a hash with the URL alphabet (%s) to be accepted, got %v", encoded, err)
		}
		return
	}
	t.Skip("no hash with - or _ was generated")
}

func TestConfig_MaximumEndpointsPerPage(t *testing.T) {
	if limit := (*Config)(nil).GetMaximumEndpointsPerPage(); limit != DefaultMaximumEndpointsPerPage {
		t.Errorf("expected a nil config to answer the default of %d, got %d", DefaultMaximumEndpointsPerPage, limit)
	}
	if limit := (&Config{}).GetMaximumEndpointsPerPage(); limit != 400 {
		t.Errorf("expected the default to be 400, got %d", limit)
	}
	scenarios := []struct {
		yaml     string
		expected int
		invalid  bool
	}{
		{yaml: "enabled: true", expected: 400},
		{yaml: "maximum-endpoints-per-page: 1", expected: 1},
		{yaml: "maximum-endpoints-per-page: 200", expected: 200},
		{yaml: "maximum-endpoints-per-page: 1000", expected: 1000},
		{yaml: "maximum-endpoints-per-page: 0", invalid: true},
		{yaml: "maximum-endpoints-per-page: -5", invalid: true},
		{yaml: "maximum-endpoints-per-page: 1001", invalid: true},
		{yaml: "maximum-endpoints-per-page: 2.5", invalid: true},
		{yaml: "maximum-endpoints-per-page: many", invalid: true},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.yaml, func(t *testing.T) {
			cfg := &Config{}
			err := yaml.Unmarshal([]byte(scenario.yaml), cfg)
			if err == nil {
				err = cfg.ValidateAndSetDefaults()
			}
			if scenario.invalid {
				if err == nil {
					t.Fatal("expected the configuration to be refused")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected the configuration to be valid, got %v", err)
			}
			if limit := cfg.GetMaximumEndpointsPerPage(); limit != scenario.expected {
				t.Errorf("expected %d, got %d", scenario.expected, limit)
			}
		})
	}
}

// The number of keys of a definition is bounded by a fixed ceiling, not by maximum-endpoints-per-page: a definition
// stays valid whatever the limit in force, and is shown truncated.
func TestPage_MaximumEndpointKeys(t *testing.T) {
	for _, count := range []int{200, 201, 1000, 1001} {
		keys := make([]string, count)
		for i := range keys {
			keys[i] = fmt.Sprintf("core_endpoint-%d", i)
		}
		err := (&Page{Slug: "infra", Title: "t", Endpoints: keys}).ValidateAndSetDefaults()
		if count <= MaximumEndpointKeys && err != nil {
			t.Errorf("expected %d keys to be valid, got %v", count, err)
		}
		if count > MaximumEndpointKeys && !errors.Is(err, ErrInvalidEndpoints) {
			t.Errorf("expected %d keys to be refused with ErrInvalidEndpoints, got %v", count, err)
		}
	}
}
