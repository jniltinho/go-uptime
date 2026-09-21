// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package push

import (
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
)

func newResolverTestConfig(t *testing.T) *config.Config {
	t.Helper()
	disabled := false
	cfg := &config.Config{
		Endpoints: []*endpoint.Endpoint{
			{Name: "site", Group: "erp"},
			{Name: "other", Group: "erp"},
			{Name: "off", Group: "erp", Enabled: &disabled},
		},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{
			{Name: "backup", Group: "jobs", Token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5"},
			{Name: "first", Group: "dup", Token: "shared-token"},
			{Name: "second", Group: "dup", Token: "shared-token"},
			{Name: "stopped", Group: "jobs", Token: "stopped-token", Enabled: &disabled},
		},
		Push: &pushconfig.Config{
			Keys:      []*pushconfig.Key{{Name: "akamai", Token: "global-key-token-123"}},
			Endpoints: []*pushconfig.Endpoint{{Key: "erp_site", Token: "erp-site-token"}, {Key: "erp_off"}},
		},
	}
	if err := config.ValidatePushConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestResolver_Resolve(t *testing.T) {
	resolver := NewResolver(newResolverTestConfig(t))
	scenarios := []struct {
		name          string
		token         string
		endpointKey   string
		expectedKey   string
		expectedKeyOf string
		external      bool
	}{
		{name: "token-of-an-external-endpoint", token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5", expectedKey: "jobs_backup", external: true},
		{name: "token-of-an-active-endpoint", token: "erp-site-token", expectedKey: "erp_site"},
		{name: "unknown-token", token: "unknown-token"},
		{name: "empty-token", token: ""},
		{name: "ambiguous-token", token: "shared-token"},
		{name: "ambiguous-token-with-its-key", token: "shared-token", endpointKey: "dup_second", expectedKey: "dup_second", external: true},
		{name: "token-of-a-disabled-external-endpoint", token: "stopped-token"},
		{name: "global-key-on-an-external-endpoint", token: "global-key-token-123", endpointKey: "JOBS_backup", expectedKey: "jobs_backup", expectedKeyOf: "akamai", external: true},
		{name: "global-key-on-an-active-endpoint-with-push", token: "global-key-token-123", endpointKey: "erp_site", expectedKey: "erp_site", expectedKeyOf: "akamai"},
		{name: "global-key-on-an-active-endpoint-without-push", token: "global-key-token-123", endpointKey: "erp_other"},
		{name: "global-key-on-a-disabled-endpoint", token: "global-key-token-123", endpointKey: "erp_off"},
		{name: "global-key-without-endpoint-key", token: "global-key-token-123"},
		{name: "token-of-another-endpoint", token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5", endpointKey: "erp_site"},
		{name: "own-token-with-its-key", token: "erp-site-token", endpointKey: "erp_site", expectedKey: "erp_site"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			target, keyName, ok := resolver.Resolve(scenario.token, scenario.endpointKey)
			if ok != (len(scenario.expectedKey) > 0) {
				t.Fatalf("expected ok=%v, got ok=%v with %+v", len(scenario.expectedKey) > 0, ok, target)
			}
			if !ok {
				return
			}
			if target.Key != scenario.expectedKey || (target.External != nil) != scenario.external || keyName != scenario.expectedKeyOf {
				t.Errorf("expected key=%s external=%v global key=%q, got key=%s external=%v global key=%q", scenario.expectedKey, scenario.external, scenario.expectedKeyOf, target.Key, target.External != nil, keyName)
			}
		})
	}
}
