// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"errors"
	"testing"
)

func TestValidateMetricsConfig(t *testing.T) {
	scenarios := []struct {
		value    string
		expected string
		invalid  bool
	}{
		{value: "", expected: DefaultMetricsNamespace},
		{value: "gatus", expected: LegacyMetricsNamespace},
		{value: "go_uptime", expected: "go_uptime"},
		{value: "_private1", expected: "_private1"},
		{value: "go-uptime", invalid: true},
		{value: "1monitor", invalid: true},
		{value: "with space", invalid: true},
		{value: "rule:name", invalid: true},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.value, func(t *testing.T) {
			cfg := &Config{MetricsNamespace: scenario.value}
			err := ValidateMetricsConfig(cfg)
			if scenario.invalid {
				if !errors.Is(err, ErrInvalidMetricsNamespace) {
					t.Fatalf("expected ErrInvalidMetricsNamespace, got %v", err)
				}
				return
			}
			if err != nil || cfg.MetricsNamespace != scenario.expected || cfg.GetMetricsNamespace() != scenario.expected {
				t.Errorf("expected %q, got %q (err=%v)", scenario.expected, cfg.MetricsNamespace, err)
			}
		})
	}
	if namespace := (*Config)(nil).GetMetricsNamespace(); namespace != DefaultMetricsNamespace {
		t.Errorf("expected a nil configuration to answer the default, got %q", namespace)
	}
}

func TestParseAndValidateConfigBytes_MetricsNamespace(t *testing.T) {
	base := "endpoints:\n  - name: api\n    url: https://example.org\n    conditions: [\"[STATUS] == 200\"]\n"
	cfg, err := parseAndValidateConfigBytes([]byte(base + "metrics: true\nmetrics-namespace: gatus\n"))
	if err != nil || cfg.MetricsNamespace != "gatus" {
		t.Fatalf("expected the names of v6 to be accepted, got %v", err)
	}
	if cfg, err = parseAndValidateConfigBytes([]byte(base)); err != nil || cfg.MetricsNamespace != DefaultMetricsNamespace {
		t.Fatalf("expected the default, got %+v (err=%v)", cfg, err)
	}
	if _, err = parseAndValidateConfigBytes([]byte(base + "metrics-namespace: go-uptime\n")); !errors.Is(err, ErrInvalidMetricsNamespace) {
		t.Errorf("expected a hyphen to invalidate the configuration, got %v", err)
	}
}
