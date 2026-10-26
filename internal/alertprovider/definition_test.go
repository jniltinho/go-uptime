// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/ntfy"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/telegram"
)

func TestPrepare(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name        string
		raw         string
		expectedErr error
		contains    string
	}{
		{name: "minimal-json", raw: `{"type": "ntfy", "topic": "ops"}`},
		{name: "minimal-yaml", raw: "type: slack\nwebhook-url: https://hooks.example.com/x\n"},
		{name: "empty", raw: "  ", expectedErr: ErrEmptyDefinition},
		{name: "no-type", raw: `{"topic": "ops"}`, expectedErr: ErrInvalidDefinition, contains: "type is required"},
		{name: "unknown-type", raw: `{"type": "carrier-pigeon"}`, expectedErr: ErrUnknownType},
		{name: "unknown-key", raw: `{"type": "ntfy", "topic": "ops", "topik": "x"}`, expectedErr: ErrInvalidDefinition, contains: "topik"},
		{name: "provider-validation", raw: `{"type": "slack"}`, expectedErr: ErrInvalidDefinition, contains: "webhook-url"},
		{name: "two-documents", raw: "type: ntfy\ntopic: a\n---\ntype: ntfy\n", expectedErr: ErrInvalidDefinition, contains: "single document"},
		{name: "aws-ses-without-keys", raw: `{"type": "aws-ses", "from": "a@example.com", "to": "b@example.com", "region": "us-east-1"}`, expectedErr: ErrFieldNotAllowed},
		{name: "client-tls-file", raw: `{"type": "telegram", "token": "t", "id": "1", "client": {"tls": {"certificate-file": "/etc/x"}}}`, expectedErr: ErrFieldNotAllowed, contains: "client.tls.certificate-file"},
		{name: "client-iap-in-override", raw: `{"type": "telegram", "token": "t", "id": "1", "overrides": [{"group": "core", "client": {"identity-aware-proxy": {"audience": "a"}}}]}`, expectedErr: ErrFieldNotAllowed, contains: "overrides[].client.identity-aware-proxy"},
		{name: "client-tunnel", raw: `{"type": "telegram", "token": "t", "id": "1", "client": {"tunnel": "bastion"}}`, expectedErr: ErrFieldNotAllowed},
		{name: "invalid-override", raw: `{"type": "ntfy", "topic": "ops", "overrides": [{"group": "core", "priority": 9}]}`, expectedErr: ErrInvalidDefinition},
		{name: "invalid-default-alert", raw: `{"type": "ntfy", "topic": "ops", "default-alert": {"minimum-reminder-interval": "1m"}}`, expectedErr: ErrInvalidDefinition, contains: "default-alert"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			prepared, err := Prepare([]byte(scenario.raw))
			if scenario.expectedErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, scenario.expectedErr) {
				t.Fatalf("expected %v, got %v (prepared=%v)", scenario.expectedErr, err, prepared)
			}
			if !strings.Contains(err.Error(), scenario.contains) {
				t.Errorf("expected the error to contain %q, got %q", scenario.contains, err.Error())
			}
		})
	}
}

func TestPrepareKeepsDefaultsOutOfTheDefinition(t *testing.T) {
	t.Parallel()
	prepared, err := Prepare([]byte(`{"type": "ntfy", "topic": "ops"}`))
	if err != nil {
		t.Fatal(err)
	}
	ntfyProvider := prepared.Provider.(*ntfy.AlertProvider)
	if ntfyProvider.DefaultConfig.URL != "https://ntfy.sh" || ntfyProvider.DefaultConfig.Priority != 3 {
		t.Errorf("expected the defaults of ntfy, got url=%q priority=%d", ntfyProvider.DefaultConfig.URL, ntfyProvider.DefaultConfig.Priority)
	}
	if string(prepared.Definition) != "topic: ops\n" {
		t.Errorf("expected only the submitted keys in the definition, got %q", prepared.Definition)
	}
}

func TestPrepareDoesNotExpandEnvironmentVariables(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "expanded")
	prepared, err := Prepare([]byte(`{"type": "telegram", "token": "${TELEGRAM_TOKEN}", "id": "1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if token := prepared.Provider.(*telegram.AlertProvider).DefaultConfig.Token; token != "${TELEGRAM_TOKEN}" {
		t.Errorf("expected the literal token, got %q", token)
	}
}
