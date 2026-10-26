// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package resend

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/test"
)

func validConfig() Config {
	return Config{APIKey: "re_123", From: "Go Uptime <alerts@example.com>", To: "ops@example.com"}
}

func TestConfig_Validate(t *testing.T) {
	scenarios := []struct {
		name     string
		mutate   func(*Config)
		expected error
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "no-api-key", mutate: func(cfg *Config) { cfg.APIKey = "" }, expected: ErrAPIKeyNotSet},
		{name: "no-from", mutate: func(cfg *Config) { cfg.From = "" }, expected: ErrFromNotSet},
		{name: "no-to", mutate: func(cfg *Config) { cfg.To = "" }, expected: ErrToNotSet},
		{name: "only-commas", mutate: func(cfg *Config) { cfg.To = " , ," }, expected: ErrToNotSet},
		{name: "fifty-recipients", mutate: func(cfg *Config) { cfg.To = strings.Repeat("a@example.com,", 50) }},
		{name: "fifty-one-recipients", mutate: func(cfg *Config) { cfg.To = strings.Repeat("a@example.com,", 51) }, expected: ErrTooManyRecipients},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := validConfig()
			scenario.mutate(&cfg)
			if err := cfg.Validate(); !errors.Is(err, scenario.expected) {
				t.Errorf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
}

func TestAlertProvider_Validate(t *testing.T) {
	provider := AlertProvider{DefaultConfig: validConfig(), Overrides: []Override{{Group: "core", Config: Config{To: "core@example.com"}}}}
	if err := provider.Validate(); err != nil {
		t.Errorf("expected a valid provider, got %v", err)
	}
	provider.Overrides = append(provider.Overrides, Override{Group: "core"})
	if err := provider.Validate(); !errors.Is(err, ErrDuplicateGroupOverride) {
		t.Errorf("expected ErrDuplicateGroupOverride for two overrides of a group, got %v", err)
	}
	provider.Overrides = []Override{{Group: ""}}
	if err := provider.Validate(); !errors.Is(err, ErrDuplicateGroupOverride) {
		t.Errorf("expected ErrDuplicateGroupOverride for an override without group, got %v", err)
	}
}

func TestAlertProvider_GetConfig(t *testing.T) {
	provider := AlertProvider{DefaultConfig: validConfig(), Overrides: []Override{{Group: "core", Config: Config{To: "core@example.com"}}}}
	cfg, err := provider.GetConfig("core", &alert.Alert{})
	if err != nil || cfg.To != "core@example.com" || cfg.APIKey != "re_123" {
		t.Errorf("expected the override of the group over the default configuration, got %+v (%v)", cfg, err)
	}
	cfg, err = provider.GetConfig("other", &alert.Alert{ProviderOverride: map[string]any{"from": "Other <other@example.com>"}})
	if err != nil || cfg.To != "ops@example.com" || cfg.From != "Other <other@example.com>" {
		t.Errorf("expected the provider-override of the alert, got %+v (%v)", cfg, err)
	}
	if err := provider.ValidateOverrides("other", &alert.Alert{ProviderOverride: map[string]any{"to": strings.Repeat("a@example.com,", 51)}}); !errors.Is(err, ErrTooManyRecipients) {
		t.Errorf("expected an invalid provider-override to be refused, got %v", err)
	}
	if provider.GetDefaultAlert() != nil {
		t.Error("expected no default alert")
	}
}

func TestAlertProvider_Send(t *testing.T) {
	defer client.InjectHTTPClient(nil)
	description := "checkout is down"
	ep := &endpoint.Endpoint{Name: "api", Group: "core"}
	testAlert := &alert.Alert{Description: &description, FailureThreshold: 3, SuccessThreshold: 2}
	result := &endpoint.Result{ConditionResults: []*endpoint.ConditionResult{{Condition: "[STATUS] == 200", Success: false}}}
	provider := AlertProvider{DefaultConfig: Config{APIKey: "re_123", From: "Go Uptime <alerts@example.com>", To: "ops@example.com, dev@example.com"}}

	var request *http.Request
	var payload Payload
	client.InjectHTTPClient(&http.Client{Transport: test.MockRoundTripper(func(r *http.Request) *http.Response {
		request = r
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"49a3999c"}`))}
	})})
	if err := provider.Send(ep, testAlert, result, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Method != http.MethodPost || request.URL.String() != APIURL || request.Header.Get("Authorization") != "Bearer re_123" || request.Header.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected request: %s %s, Authorization=%q", request.Method, request.URL, request.Header.Get("Authorization"))
	}
	if payload.From != "Go Uptime <alerts@example.com>" || !reflect.DeepEqual(payload.To, []string{"ops@example.com", "dev@example.com"}) || payload.Subject != "[core/api] Alert triggered" {
		t.Errorf("unexpected payload: %+v", payload)
	}
	for _, expected := range []string{"failed 3 time(s)", "Alert description: checkout is down", "❌ [STATUS] == 200"} {
		if !strings.Contains(payload.Text, expected) {
			t.Errorf("expected the text to contain %q, got %q", expected, payload.Text)
		}
	}
	if !strings.Contains(payload.HTML, "<br>") || strings.Contains(payload.HTML, "\n") {
		t.Errorf("expected the line breaks of the HTML as <br>, got %q", payload.HTML)
	}

	if err := provider.Send(ep, testAlert, &endpoint.Result{}, true); err != nil || payload.Subject != "[core/api] Alert resolved" || !strings.Contains(payload.Text, "passing successfully 2 time(s)") {
		t.Errorf("unexpected resolution: subject=%q err=%v", payload.Subject, err)
	}

	client.InjectHTTPClient(&http.Client{Transport: test.MockRoundTripper(func(r *http.Request) *http.Response {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"message":"The example.com domain is not verified."}`))}
	})})
	err := provider.Send(ep, testAlert, result, false)
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "not verified") {
		t.Errorf("expected the status and the message of Resend, got %v", err)
	}
}
