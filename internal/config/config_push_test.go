package config

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/push"
)

func TestParseAndValidateConfigBytes_Push(t *testing.T) {
	config, err := parseAndValidateConfigBytes([]byte(`
endpoints:
  - name: site
    group: erp
    url: https://example.org
    conditions: ["[STATUS] == 200"]
external-endpoints:
  - name: backup
    group: jobs
    token: backup-token
push:
  keys:
    - name: akamai
      token: keSDu7G855jvVat1xWiY2Gk4CkL1End5
  endpoints:
    - key: ERP_site
      token: erp-site-token
`))
	if err != nil {
		t.Fatalf("expected a valid configuration, got %v", err)
	}
	if config.Push == nil || len(config.Push.Keys) != 1 || config.Push.Keys[0].Hash() != push.HashToken("keSDu7G855jvVat1xWiY2Gk4CkL1End5") {
		t.Fatalf("expected the global key with its hash, got %+v", config.Push)
	}
	if len(config.Push.Endpoints) != 1 || config.Push.Endpoints[0].Key != "erp_site" {
		t.Errorf("expected the endpoint key in lowercase, got %+v", config.Push.Endpoints)
	}
}

func TestValidatePushConfig(t *testing.T) {
	newConfig := func(pushConfig *push.Config) *Config {
		return &Config{
			Endpoints:         []*endpoint.Endpoint{{Name: "site", Group: "erp"}},
			ExternalEndpoints: []*endpoint.ExternalEndpoint{{Name: "backup", Group: "jobs", Token: "backup-token"}},
			Push:              pushConfig,
		}
	}
	scenarios := []struct {
		name     string
		config   *Config
		expected error
	}{
		{name: "without-push", config: newConfig(nil)},
		{name: "endpoint-of-the-configuration-file", config: newConfig(&push.Config{Endpoints: []*push.Endpoint{{Key: "erp_site"}}})},
		{name: "unknown-endpoint", config: newConfig(&push.Config{Endpoints: []*push.Endpoint{{Key: "erp_other"}}}), expected: ErrPushEndpointNotFound},
		{name: "external-endpoint-is-not-an-active-endpoint", config: newConfig(&push.Config{Endpoints: []*push.Endpoint{{Key: "jobs_backup"}}}), expected: ErrPushEndpointNotFound},
		{name: "endpoint-token-of-an-external-endpoint", config: newConfig(&push.Config{Endpoints: []*push.Endpoint{{Key: "erp_site", Token: "backup-token"}}}), expected: push.ErrDuplicateToken},
		{name: "invalid-key", config: newConfig(&push.Config{Keys: []*push.Key{{Name: "akamai", Token: "short"}}}), expected: push.ErrInvalidKeyToken},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			err := ValidatePushConfig(scenario.config)
			if scenario.expected == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if scenario.expected != nil && !errors.Is(err, scenario.expected) {
				t.Fatalf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
}
