package config

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

func TestParseAndValidateConfigBytes_StatusPages(t *testing.T) {
	cfg, err := parseAndValidateConfigBytes([]byte(`
status-pages:
  trusted-proxies: ["172.30.0.1"]
  rate-limit: 60
  pages:
    - slug: infra
      title: " Infraestrutura "
      groups: [core]
      endpoints: [Core_API]
    - slug: apps
      title: Aplicações
      groups: [apps]
      enabled: false
endpoints:
  - name: api
    group: core
    url: https://example.org
    conditions:
      - "[STATUS] == 200"
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	statusPages := cfg.StatusPages
	if !statusPages.IsEnabled() || statusPages.GetRateLimit() != 60 || len(statusPages.TrustedProxyPrefixes()) != 1 {
		t.Errorf("unexpected status-pages configuration: enabled=%v rate-limit=%d trusted-proxies=%v", statusPages.IsEnabled(), statusPages.GetRateLimit(), statusPages.TrustedProxyPrefixes())
	}
	if len(statusPages.Pages) != 2 || statusPages.Pages[0].Title != "Infraestrutura" || statusPages.Pages[0].Endpoints[0] != "core_api" {
		t.Errorf("expected normalized pages, got %+v", statusPages.Pages)
	}
	if statusPages.Pages[1].IsEnabled() {
		t.Error("expected the apps page to be disabled")
	}
}

func TestParseAndValidateConfigBytes_WithoutStatusPages(t *testing.T) {
	cfg, err := parseAndValidateConfigBytes([]byte(`
endpoints:
  - name: api
    url: https://example.org
    conditions:
      - "[STATUS] == 200"
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StatusPages != nil || !cfg.StatusPages.IsEnabled() || cfg.StatusPages.GetRateLimit() != statuspage.DefaultRateLimit {
		t.Error("expected status pages to be enabled with the defaults when the section is absent")
	}
}

func TestParseAndValidateConfigBytes_InvalidStatusPages(t *testing.T) {
	scenarios := []struct {
		name        string
		section     string
		expectedErr error
	}{
		{name: "invalid-slug", section: "pages: [{slug: Infra, title: t, groups: [core]}]", expectedErr: statuspage.ErrInvalidSlug},
		{name: "duplicate-slug", section: "pages: [{slug: infra, title: t, groups: [core]}, {slug: infra, title: u, groups: [db]}]", expectedErr: statuspage.ErrDuplicateSlug},
		{name: "empty-selection", section: "pages: [{slug: infra, title: t}]", expectedErr: statuspage.ErrEmptySelection},
		{name: "invalid-trusted-proxy", section: "trusted-proxies: [nginx]", expectedErr: statuspage.ErrInvalidTrustedProxy},
		{name: "negative-rate-limit", section: "rate-limit: -1", expectedErr: statuspage.ErrInvalidRateLimit},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := parseAndValidateConfigBytes([]byte(`
status-pages:
  ` + scenario.section + `
endpoints:
  - name: api
    url: https://example.org
    conditions:
      - "[STATUS] == 200"
`))
			if !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected error %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}
