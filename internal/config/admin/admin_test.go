// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package admin

import (
	"errors"
	"testing"
)

func TestConfig_IsEnabled(t *testing.T) {
	var nilConfig *Config
	if nilConfig.IsEnabled() {
		t.Error("expected a nil config not to be enabled")
	}
	if (&Config{}).IsEnabled() {
		t.Error("expected the administration to be disabled by default")
	}
	if !(&Config{Enabled: true}).IsEnabled() {
		t.Error("expected the administration to be enabled")
	}
}

func TestConfig_ValidateAndSetDefaults(t *testing.T) {
	scenarios := []struct {
		name            string
		config          Config
		expectedErr     error
		expectedOrigins []string
		expectedSubject []string
	}{
		{
			name:            "normalizes-origins-and-subjects",
			config:          Config{AllowedOrigins: []string{" HTTPS://Status.Example.com:8443 ", "http://localhost:8080/"}, AllowedSubjects: []string{" ops@example.com ", ""}},
			expectedOrigins: []string{"https://status.example.com:8443", "http://localhost:8080"},
			expectedSubject: []string{"ops@example.com"},
		},
		{
			name:        "origin-with-path",
			config:      Config{AllowedOrigins: []string{"https://status.example.com/admin"}},
			expectedErr: ErrInvalidAllowedOrigin,
		},
		{
			name:        "origin-without-scheme",
			config:      Config{AllowedOrigins: []string{"status.example.com"}},
			expectedErr: ErrInvalidAllowedOrigin,
		},
		{
			name:        "origin-with-unsupported-scheme",
			config:      Config{AllowedOrigins: []string{"ftp://status.example.com"}},
			expectedErr: ErrInvalidAllowedOrigin,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			err := scenario.config.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.expectedErr) {
				t.Fatalf("expected error %v, got %v", scenario.expectedErr, err)
			}
			if scenario.expectedErr != nil {
				return
			}
			if len(scenario.config.AllowedOrigins) != len(scenario.expectedOrigins) {
				t.Fatalf("expected origins %v, got %v", scenario.expectedOrigins, scenario.config.AllowedOrigins)
			}
			for i, origin := range scenario.expectedOrigins {
				if scenario.config.AllowedOrigins[i] != origin {
					t.Errorf("expected origin %q, got %q", origin, scenario.config.AllowedOrigins[i])
				}
			}
			if len(scenario.config.AllowedSubjects) != len(scenario.expectedSubject) || (len(scenario.expectedSubject) > 0 && scenario.config.AllowedSubjects[0] != scenario.expectedSubject[0]) {
				t.Errorf("expected subjects %v, got %v", scenario.expectedSubject, scenario.config.AllowedSubjects)
			}
		})
	}
}

func TestConfig_IsSubjectAllowed(t *testing.T) {
	config := &Config{AllowedSubjects: []string{"ops@example.com"}}
	if !config.IsSubjectAllowed("OPS@example.com") {
		t.Error("expected subjects to be compared ignoring case")
	}
	if config.IsSubjectAllowed("dev@example.com") {
		t.Error("expected a subject outside the list not to be allowed")
	}
}
