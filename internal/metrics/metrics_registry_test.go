package metrics

import (
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newConfigWithExtraLabels(labels map[string]string) *config.Config {
	return &config.Config{Endpoints: []*endpoint.Endpoint{{Name: "labeled", URL: "https://example.org", ExtraLabels: labels}}}
}

func TestRegisteredExtraLabels(t *testing.T) {
	InitializePrometheusMetrics(newConfigWithExtraLabels(map[string]string{"team": "core", "environment": "prod"}), prometheus.NewRegistry())
	labels := RegisteredExtraLabels()
	if len(labels) != 2 || labels[0] != "environment" || labels[1] != "team" {
		t.Fatalf("expected [environment team], got %v", labels)
	}
	labels[0] = "changed"
	if RegisteredExtraLabels()[0] != "environment" {
		t.Error("expected RegisteredExtraLabels to return a copy")
	}
	UnregisterPrometheusMetrics()
	if labels := RegisteredExtraLabels(); labels != nil {
		t.Errorf("expected no labels after unregistering the metrics, got %v", labels)
	}
}

func TestPublishMetricsForEndpoint_UsesRegisteredExtraLabels(t *testing.T) {
	InitializePrometheusMetrics(newConfigWithExtraLabels(map[string]string{"environment": "prod"}), prometheus.NewRegistry())
	defer UnregisterPrometheusMetrics()
	ep := &endpoint.Endpoint{Name: "new", Group: "web", URL: "https://example.org", ExtraLabels: map[string]string{"team": "core"}}
	result := &endpoint.Result{HTTPStatus: 200, Success: true, Duration: time.Millisecond}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected no panic when publishing with a different list of extra labels, got %v", r)
		}
	}()
	PublishMetricsForEndpoint(ep, result, []string{"environment", "team"})
	if count := testutil.CollectAndCount(resultTotal); count != 1 {
		t.Errorf("expected 1 series in results_total, got %d", count)
	}
}

func TestDeleteMetricsForEndpointKey(t *testing.T) {
	InitializePrometheusMetrics(&config.Config{}, prometheus.NewRegistry())
	defer UnregisterPrometheusMetrics()
	removed := &endpoint.Endpoint{Name: "removed", Group: "core", URL: "https://example.org"}
	kept := &endpoint.Endpoint{Name: "kept", Group: "core", URL: "https://example.org"}
	result := &endpoint.Result{HTTPStatus: 200, Connected: true, Success: true, Duration: time.Millisecond}
	PublishMetricsForEndpoint(removed, result, nil)
	PublishMetricsForEndpoint(kept, result, nil)
	if deleted := DeleteMetricsForEndpointKey(removed.Key()); deleted == 0 {
		t.Fatal("expected series to be deleted")
	}
	for name, vec := range map[string]prometheus.Collector{"results_total": resultTotal, "results_endpoint_success": resultEndpointSuccess, "results_code_total": resultCodeTotal} {
		if count := testutil.CollectAndCount(vec); count != 1 {
			t.Errorf("expected only the kept endpoint in %s, got %d series", name, count)
		}
	}
	if deleted := DeleteMetricsForEndpointKey(removed.Key()); deleted != 0 {
		t.Errorf("expected nothing left to delete, got %d", deleted)
	}
}

func TestDeleteMetricsForEndpointKey_NotInitialized(t *testing.T) {
	UnregisterPrometheusMetrics()
	if deleted := DeleteMetricsForEndpointKey("core_api"); deleted != 0 {
		t.Errorf("expected 0 when metrics are not initialized, got %d", deleted)
	}
}
