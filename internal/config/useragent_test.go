// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

func TestParseAndValidateConfigBytesWithUserAgentPreset(t *testing.T) {
	t.Parallel()
	cfg, err := parseAndValidateConfigBytes([]byte(`
endpoints:
  - name: website
    url: https://example.org
    user-agent: firefox
    conditions:
      - "[STATUS] == 200"
external-endpoints:
  - name: heartbeat
    token: a-token-of-a-reasonable-length
    user-agent: chrome
`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Endpoints[0].UserAgent != endpoint.UserAgentFirefox {
		t.Errorf("expected the firefox preset, got %q", cfg.Endpoints[0].UserAgent)
	}
	if _, ok := cfg.Endpoints[0].Headers["User-Agent"]; ok {
		t.Error("expected the preset not to be written into the headers")
	}
}

func TestParseAndValidateConfigBytesWithInvalidUserAgent(t *testing.T) {
	t.Parallel()
	scenarios := map[string]struct {
		yaml     string
		expected error
	}{
		"unknown": {
			yaml:     "endpoints:\n  - name: website\n    url: https://example.org\n    user-agent: opera\n    conditions: [\"[STATUS] == 200\"]\n",
			expected: endpoint.ErrEndpointWithInvalidUserAgent,
		},
		"conflict": {
			yaml:     "endpoints:\n  - name: website\n    url: https://example.org\n    user-agent: chrome\n    headers:\n      User-Agent: Mine/1.0\n    conditions: [\"[STATUS] == 200\"]\n",
			expected: endpoint.ErrEndpointUserAgentConflict,
		},
		"tcp": {
			yaml:     "endpoints:\n  - name: website\n    url: tcp://example.org:443\n    user-agent: edge\n    conditions: [\"[CONNECTED] == true\"]\n",
			expected: endpoint.ErrEndpointUserAgentNotSupported,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseAndValidateConfigBytes([]byte(scenario.yaml)); !errors.Is(err, scenario.expected) {
				t.Fatalf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
}
