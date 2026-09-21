// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/custom"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
)

func newTestContext() Context {
	return Context{
		Config: &config.Config{
			Endpoints:         []*endpoint.Endpoint{{Name: "api", Group: "core"}},
			ExternalEndpoints: []*endpoint.ExternalEndpoint{{Name: "heartbeat", Group: "core"}},
			Suites:            []*suite.Suite{{Name: "flow", Group: "suites", Endpoints: []*endpoint.Endpoint{{Name: "step"}}}},
			Alerting: &alerting.Config{
				Custom: &custom.AlertProvider{
					DefaultConfig: custom.Config{URL: "https://example.org/alert"},
					DefaultAlert:  &alert.Alert{FailureThreshold: 5},
				},
			},
		},
		ManagedKeys:        []string{"web_managed"},
		AllowedExtraLabels: []string{"environment"},
	}
}

func TestParse(t *testing.T) {
	ep, err := Parse([]byte(`{"name": "site", "url": "https://example.org", "interval": "90s", "conditions": ["[STATUS] == 200"]}`))
	if err != nil {
		t.Fatalf("expected JSON to be accepted, got %v", err)
	}
	if ep.Name != "site" || ep.Interval != 90*time.Second {
		t.Errorf("unexpected endpoint: name=%s interval=%s", ep.Name, ep.Interval)
	}
	ep, err = Parse([]byte("name: site\nurl: https://example.org\nheaders:\n  Authorization: Bearer ${API_TOKEN}\nconditions: [\"[STATUS] == 200\"]\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Headers["Authorization"] != "Bearer ${API_TOKEN}" {
		t.Errorf("expected environment variables not to be expanded, got %q", ep.Headers["Authorization"])
	}
	scenarios := []struct {
		name        string
		definition  string
		expectedErr error
	}{
		{name: "empty", definition: "  \n", expectedErr: ErrEmptyDefinition},
		{name: "unknown-field", definition: "name: site\nintervall: 1m\n", expectedErr: ErrInvalidDefinition},
		{name: "unknown-nested-field", definition: "name: site\nclient:\n  timeoutt: 1s\n", expectedErr: ErrInvalidDefinition},
		{name: "multiple-documents", definition: "name: a\n---\nname: b\n", expectedErr: ErrInvalidDefinition},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := Parse([]byte(scenario.definition)); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}

func TestPrepare(t *testing.T) {
	prepared, err := Prepare([]byte("name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"), newTestContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prepared.Endpoint.Interval != time.Minute || prepared.Endpoint.Method != "GET" {
		t.Errorf("expected default values on the endpoint, got interval=%s method=%s", prepared.Endpoint.Interval, prepared.Endpoint.Method)
	}
	definition := string(prepared.Definition)
	if strings.Contains(definition, "interval") || strings.Contains(definition, "User-Agent") || strings.Contains(definition, "method") {
		t.Errorf("expected the definition to be persisted without default values, got:\n%s", definition)
	}
	effective, err := Effective(prepared.Endpoint)
	if err != nil || !strings.Contains(string(effective), "interval: 1m0s") {
		t.Errorf("expected the effective definition to include default values, got:\n%s (err=%v)", effective, err)
	}
}

func TestPrepare_Alerts(t *testing.T) {
	prepared, err := Prepare([]byte("name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\nalerts:\n  - type: custom\n"), newTestContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prepared.Endpoint.Alerts[0].FailureThreshold != 5 {
		t.Errorf("expected the provider default alert to be merged, got failure-threshold=%d", prepared.Endpoint.Alerts[0].FailureThreshold)
	}
	if strings.Contains(string(prepared.Definition), "failure-threshold") {
		t.Errorf("expected the provider default alert not to be persisted, got:\n%s", prepared.Definition)
	}
	if _, err := Prepare([]byte("name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\nalerts:\n  - type: slack\n"), newTestContext()); !errors.Is(err, ErrAlertProviderNotConfigured) {
		t.Errorf("expected ErrAlertProviderNotConfigured, got %v", err)
	}
}

func TestPrepare_Rejections(t *testing.T) {
	const base = "url: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"
	scenarios := []struct {
		name        string
		definition  string
		expectedErr error
	}{
		{name: "identity-aware-proxy", definition: "name: a\ngroup: web\n" + base + "client:\n  identity-aware-proxy:\n    audience: x\n", expectedErr: ErrFieldNotAllowed},
		{name: "tls-certificate-file", definition: "name: a\ngroup: web\n" + base + "client:\n  tls:\n    certificate-file: /etc/ssl/cert.pem\n", expectedErr: ErrFieldNotAllowed},
		{name: "tls-private-key-file", definition: "name: a\ngroup: web\n" + base + "client:\n  tls:\n    private-key-file: /etc/ssl/key.pem\n", expectedErr: ErrFieldNotAllowed},
		{name: "store", definition: "name: a\ngroup: web\n" + base + "store:\n  id: \"[BODY].id\"\n", expectedErr: ErrFieldNotAllowed},
		{name: "always-run", definition: "name: a\ngroup: web\n" + base + "always-run: true\n", expectedErr: ErrFieldNotAllowed},
		{name: "unknown-tunnel", definition: "name: a\ngroup: web\n" + base + "client:\n  tunnel: bastion\n", expectedErr: ErrInvalidDefinition},
		{name: "missing-conditions", definition: "name: a\ngroup: web\nurl: https://example.org\n", expectedErr: ErrInvalidDefinition},
		{name: "key-of-config-endpoint", definition: "name: API\ngroup: core\n" + base, expectedErr: ErrKeyConflict},
		{name: "key-of-external-endpoint", definition: "name: heartbeat\ngroup: core\n" + base, expectedErr: ErrKeyConflict},
		{name: "key-of-suite-endpoint", definition: "name: step\ngroup: suites\n" + base, expectedErr: ErrKeyConflict},
		{name: "key-of-other-managed-endpoint", definition: "name: managed\ngroup: web\n" + base, expectedErr: ErrKeyConflict},
		{name: "extra-label-not-registered", definition: "name: a\ngroup: web\n" + base + "extra-labels:\n  team: core\n", expectedErr: ErrExtraLabelNotAllowed},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := Prepare([]byte(scenario.definition), newTestContext()); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected %v, got %v", scenario.expectedErr, err)
			}
		})
	}
	if _, err := Prepare([]byte("name: a\ngroup: web\n"+base+"extra-labels:\n  environment: prod\n"), newTestContext()); err != nil {
		t.Errorf("expected a registered extra label to be accepted, got %v", err)
	}
}

func TestConfigKeyOrigin(t *testing.T) {
	cfg := newTestContext().Config
	if origin, used := ConfigKeyOrigin(cfg, "core_api"); !used || !strings.Contains(origin, "endpoint of the configuration file") {
		t.Errorf("expected core_api to be used by an endpoint, got used=%v origin=%q", used, origin)
	}
	if _, used := ConfigKeyOrigin(cfg, "web_free"); used {
		t.Error("expected web_free not to be used")
	}
}
