// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/heartbeat"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func TestParseDefinition_Types(t *testing.T) {
	parsed, err := ParseDefinition([]byte("type: push\nname: backup\ngroup: jobs\ntoken: keSDu7G855jvVat1xWiY2Gk4CkL1End5\nheartbeat:\n  interval: 5m\n"))
	if err != nil || !parsed.IsPush() || parsed.Key() != "jobs_backup" || parsed.PushToken() != "keSDu7G855jvVat1xWiY2Gk4CkL1End5" || parsed.Push.Heartbeat.Interval != 5*time.Minute || !parsed.ReceivesPush() {
		t.Fatalf("expected a push endpoint, got %+v (err=%v)", parsed, err)
	}
	parsed, err = ParseDefinition([]byte(`{"name": "site", "url": "https://example.org", "conditions": ["[STATUS] == 200"], "push": {"enabled": true, "token": "erp-site-token"}}`))
	if err != nil || parsed.IsPush() || !parsed.ReceivesPush() || parsed.PushToken() != "erp-site-token" || parsed.Endpoint.URL != "https://example.org" {
		t.Fatalf("expected an active endpoint with the push option, got %+v (err=%v)", parsed, err)
	}
	if parsed, err = ParseDefinition([]byte("name: site\nurl: https://example.org\npush:\n  enabled: false\n  token: kept-token\n")); err != nil || parsed.ReceivesPush() {
		t.Fatalf("expected a disabled push option not to receive push, got %+v (err=%v)", parsed, err)
	}
	scenarios := []struct {
		name       string
		definition string
		expected   error
	}{
		{name: "push-with-url", definition: "type: push\nname: backup\ntoken: abcdefgh\nurl: https://example.org\n", expected: ErrInvalidDefinition},
		{name: "push-with-push-option", definition: "type: push\nname: backup\ntoken: abcdefgh\npush:\n  enabled: true\n", expected: ErrFieldNotAllowed},
		{name: "unknown-type", definition: "type: pull\nname: backup\n", expected: ErrInvalidDefinition},
		{name: "active-with-token", definition: "name: site\nurl: https://example.org\ntoken: abcdefgh\n", expected: ErrInvalidDefinition},
		{name: "active-with-heartbeat", definition: "name: site\nurl: https://example.org\nheartbeat:\n  interval: 1m\n", expected: ErrInvalidDefinition},
		{name: "push-option-with-unknown-field", definition: "name: site\nurl: https://example.org\npush:\n  enable: true\n", expected: ErrInvalidDefinition},
		{name: "two-documents", definition: "type: push\nname: a\n---\nname: b\n", expected: ErrInvalidDefinition},
		{name: "empty", definition: "  \n", expected: ErrEmptyDefinition},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := ParseDefinition([]byte(scenario.definition)); !errors.Is(err, scenario.expected) {
				t.Errorf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
	if _, err := Parse([]byte("type: push\nname: backup\ntoken: abcdefgh\n")); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("expected Parse to reject a push endpoint, got %v", err)
	}
}

func TestPrepare_Push(t *testing.T) {
	ctx := newTestContext()
	ctx.PushTokens = map[string]string{"used-token-1": "another managed endpoint"}
	ctx.IsPushKeyToken = func(token string) bool { return token == "global-key-token-1" }
	prepared, err := Prepare([]byte("type: push\nname: backup\ngroup: jobs\ntoken: keSDu7G855jvVat1xWiY2Gk4CkL1End5\nalerts:\n  - type: custom\n"), ctx)
	if err != nil {
		t.Fatalf("expected a valid push endpoint, got %v", err)
	}
	if prepared.Push.Heartbeat.Interval != DefaultPushHeartbeatInterval || prepared.Push.Alerts[0].FailureThreshold != 5 || strings.Contains(string(prepared.Definition), "heartbeat") {
		t.Errorf("expected the default heartbeat and the provider default alert, without persisting defaults, got %+v and %s", prepared.Push, prepared.Definition)
	}
	const base = "type: push\nname: backup\ngroup: jobs\n"
	const active = "name: site\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n"
	scenarios := []struct {
		name       string
		definition string
		expected   error
	}{
		{name: "without-token", definition: base, expected: ErrInvalidDefinition},
		{name: "short-token", definition: base + "token: short\n", expected: ErrInvalidDefinition},
		{name: "token-with-invalid-character", definition: base + "token: with/slash-token\n", expected: ErrInvalidDefinition},
		{name: "token-used", definition: base + "token: used-token-1\n", expected: ErrPushTokenInUse},
		{name: "token-of-a-global-key", definition: base + "token: global-key-token-1\n", expected: ErrPushTokenInUse},
		{name: "key-of-the-configuration-file", definition: "type: push\nname: heartbeat\ngroup: core\ntoken: keSDu7G855jvVat1xWiY2Gk4CkL1End5\n", expected: ErrKeyConflict},
		{name: "key-of-another-managed-endpoint", definition: "type: push\nname: managed\ngroup: web\ntoken: keSDu7G855jvVat1xWiY2Gk4CkL1End5\n", expected: ErrKeyConflict},
		{name: "heartbeat-too-short", definition: base + "token: keSDu7G855jvVat1xWiY2Gk4CkL1End5\nheartbeat:\n  interval: 5s\n", expected: ErrInvalidDefinition},
		{name: "alert-without-provider", definition: base + "token: keSDu7G855jvVat1xWiY2Gk4CkL1End5\nalerts:\n  - type: slack\n", expected: ErrAlertProviderNotConfigured},
		{name: "active-push-option-with-used-token", definition: active + "push:\n  enabled: true\n  token: used-token-1\n", expected: ErrPushTokenInUse},
		{name: "active-push-option-with-short-token", definition: active + "push:\n  enabled: true\n  token: short\n", expected: ErrInvalidDefinition},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := Prepare([]byte(scenario.definition), ctx); !errors.Is(err, scenario.expected) {
				t.Errorf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
	// A disabled push option keeps its token without checking whether it is used
	if _, err := Prepare([]byte(active+"push:\n  enabled: false\n  token: used-token-1\n"), ctx); err != nil {
		t.Errorf("expected a disabled push option to be accepted, got %v", err)
	}
	// The push option without token accepts push with the global keys only
	if prepared, err := Prepare([]byte(active+"push:\n  enabled: true\n"), ctx); err != nil || !prepared.ReceivesPush() || prepared.PushToken() != "" {
		t.Errorf("expected the push option without token to be accepted, got %+v (err=%v)", prepared, err)
	}
}

func TestWithGeneratedPushToken(t *testing.T) {
	definition, err := withGeneratedPushToken([]byte("type: push\nname: backup\n"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDefinition(definition)
	if err != nil || len(parsed.PushToken()) != pushconfig.GeneratedTokenLength || !pushconfig.ValidToken(parsed.PushToken(), pushconfig.MinimumEndpointTokenLength) {
		t.Fatalf("expected a generated token, got %+v (err=%v)", parsed, err)
	}
	for _, unchanged := range []string{"type: push\nname: backup\ntoken: keep-this-token\n", "name: site\nurl: https://example.org\n"} {
		if definition, err := withGeneratedPushToken([]byte(unchanged)); err != nil || string(definition) != unchanged {
			t.Errorf("expected %q to be kept, got %q (err=%v)", unchanged, definition, err)
		}
	}
}

func TestMaskSecrets_PushTokens(t *testing.T) {
	stored := map[string]any{"type": "push", "token": "secret-token-1", "push": map[string]any{"enabled": true, "token": "erp-site-token"}}
	masked := map[string]any{"type": "push", "token": "secret-token-1", "push": map[string]any{"enabled": true, "token": "erp-site-token"}}
	MaskSecrets(masked)
	if masked["token"] != Mask || masked["push"].(map[string]any)["token"] != Mask {
		t.Fatalf("expected the push tokens to be masked, got %v", masked)
	}
	RestoreMaskedSecrets(masked, stored)
	if masked["token"] != "secret-token-1" || masked["push"].(map[string]any)["token"] != "erp-site-token" {
		t.Errorf("expected the push tokens to be restored, got %v", masked)
	}
}

func TestNewPushIndex(t *testing.T) {
	disabled := false
	stored := func(key string) *common.ManagedEndpoint { return &common.ManagedEndpoint{Key: key} }
	index := newPushIndex(map[string]*State{
		"jobs_backup":  {Stored: stored("jobs_backup"), Push: &endpoint.ExternalEndpoint{Name: "backup", Group: "jobs", Token: "backup-token"}},
		"jobs_off":     {Stored: stored("jobs_off"), Push: &endpoint.ExternalEndpoint{Name: "off", Group: "jobs", Token: "off-token", Enabled: &disabled}},
		"erp_site":     {Stored: stored("erp_site"), Endpoint: &endpoint.Endpoint{Name: "site", Group: "erp"}, PushOption: &PushOption{Enabled: true, Token: "shared-token"}},
		"erp_other":    {Stored: stored("erp_other"), Endpoint: &endpoint.Endpoint{Name: "other", Group: "erp"}, PushOption: &PushOption{Enabled: true, Token: "shared-token"}},
		"erp_plain":    {Stored: stored("erp_plain"), Endpoint: &endpoint.Endpoint{Name: "plain", Group: "erp"}},
		"erp_conflict": {Stored: stored("erp_conflict"), ConflictOrigin: "an endpoint of the configuration file"},
	})
	for _, key := range []string{"jobs_backup", "erp_site", "erp_other"} {
		if _, exists := index.byKey[key]; !exists {
			t.Errorf("expected %s to receive push", key)
		}
	}
	for _, key := range []string{"jobs_off", "erp_plain", "erp_conflict"} {
		if _, exists := index.byKey[key]; exists {
			t.Errorf("expected %s not to receive push", key)
		}
	}
	if index.byToken["backup-token"] != "jobs_backup" || index.byKey["jobs_backup"].Push == nil {
		t.Errorf("expected the token of the push endpoint in the index, got %v", index.byToken)
	}
	if _, exists := index.byToken["shared-token"]; exists || index.tokens["erp_site"] != "shared-token" {
		t.Errorf("expected a shared token to be left out of the tokens but kept by key, got %v and %v", index.byToken, index.tokens)
	}
}

func TestService_ConfigurationFileEndpointsThatReceivePush(t *testing.T) {
	initializeSQLiteStore(t)
	cfg := &config.Config{
		Endpoints:         []*endpoint.Endpoint{{Name: "site", Group: "erp", URL: "https://example.org"}},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{{Name: "backup", Group: "jobs", Token: "backup-token", Heartbeat: heartbeat.Config{Interval: time.Minute}}},
		Push:              &pushconfig.Config{Endpoints: []*pushconfig.Endpoint{{Key: "erp_site"}}},
	}
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	service := NewService(cfg)
	items := make(map[string]*Item)
	for _, item := range service.List() {
		items[item.Key] = item
	}
	if backup := items["jobs_backup"]; backup == nil || backup.Type != ItemTypePush || backup.Source != SourceConfig || !backup.AcceptsPush || backup.Interval != "1m0s" {
		t.Errorf("expected the external endpoint in the list as a push endpoint of the configuration file, got %+v", backup)
	}
	if site := items["erp_site"]; site == nil || !site.AcceptsPush {
		t.Errorf("expected the endpoint of push.endpoints to receive push, got %+v", site)
	}
	detail, err := service.Get("jobs_backup")
	if err != nil || detail.Definition.JSON["token"] != Mask || detail.Source != SourceConfig {
		t.Errorf("expected the external endpoint with its token masked, got %+v (err=%v)", detail, err)
	}
}
