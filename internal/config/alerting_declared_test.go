// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"slices"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

func TestConfig_DeclaredAlertTypesIncludesInvalidProviders(t *testing.T) {
	cfg, err := parseAndValidateConfigBytes([]byte(`
alerting:
  slack:
    webhook-url: https://hooks.example.com/x
  ntfy:
    priority: 3
endpoints:
  - name: website
    url: https://example.org
    conditions:
      - "[STATUS] == 200"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alerting.Ntfy != nil {
		t.Fatal("expected the invalid ntfy provider to be discarded by the validation")
	}
	declared := cfg.DeclaredAlertTypes()
	if !slices.Contains(declared, alert.TypeSlack) || !slices.Contains(declared, alert.TypeNtfy) || len(declared) != 2 {
		t.Errorf("expected slack and ntfy to be declared, got %v", declared)
	}
}
