package metrics

import (
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/prometheus/client_golang/prometheus"
)

func metricNames(t *testing.T, reg *prometheus.Registry) []string {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	return names
}

func TestInitializePrometheusMetrics_Namespace(t *testing.T) {
	scenarios := []struct {
		name           string
		cfg            *config.Config
		expectedPrefix string
	}{
		{name: "default", cfg: &config.Config{}, expectedPrefix: "go_uptime_"},
		{name: "the names of v6", cfg: &config.Config{MetricsNamespace: "gatus"}, expectedPrefix: "gatus_"},
		{name: "another prefix", cfg: &config.Config{MetricsNamespace: "monitoring"}, expectedPrefix: "monitoring_"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			InitializePrometheusMetrics(scenario.cfg, reg)
			t.Cleanup(UnregisterPrometheusMetrics)
			ep := &endpoint.Endpoint{Name: "api", Group: "core", URL: "https://example.org"}
			PublishMetricsForEndpoint(ep, &endpoint.Result{Success: true, Connected: true, HTTPStatus: 200}, nil)
			names := metricNames(t, reg)
			if len(names) == 0 {
				t.Fatal("expected metrics to be published")
			}
			for _, name := range names {
				if !strings.HasPrefix(name, scenario.expectedPrefix) {
					t.Errorf("expected every metric to start with %q, got %q", scenario.expectedPrefix, name)
				}
			}
		})
	}
}

// A reload that changes the prefix must leave no metric under the previous one
func TestInitializePrometheusMetrics_NamespaceChangedOnReload(t *testing.T) {
	reg := prometheus.NewRegistry()
	InitializePrometheusMetrics(&config.Config{MetricsNamespace: "gatus"}, reg)
	t.Cleanup(UnregisterPrometheusMetrics)
	ep := &endpoint.Endpoint{Name: "api", Group: "core", URL: "https://example.org"}
	PublishMetricsForEndpoint(ep, &endpoint.Result{Success: true}, nil)
	InitializePrometheusMetrics(&config.Config{}, reg)
	PublishMetricsForEndpoint(ep, &endpoint.Result{Success: true}, nil)
	for _, name := range metricNames(t, reg) {
		if strings.HasPrefix(name, "gatus_") {
			t.Errorf("expected no metric with the previous prefix after the reload, got %q", name)
		}
	}
}
