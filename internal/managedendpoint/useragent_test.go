package managedendpoint

import (
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

func TestPrepare_UserAgentPreset(t *testing.T) {
	prepared, err := Prepare([]byte("name: site\ngroup: web\nurl: https://example.org\nuser-agent: firefox\nconditions: [\"[STATUS] == 200\"]\n"), newTestContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	definition := string(prepared.Definition)
	if !strings.Contains(definition, "user-agent: firefox") || strings.Contains(definition, "User-Agent") {
		t.Errorf("expected the stored definition to keep the preset without a User-Agent header, got:\n%s", definition)
	}
	effective, err := Effective(prepared.Endpoint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(effective), "user-agent: firefox") || strings.Contains(string(effective), "User-Agent") {
		t.Errorf("expected the effective definition to keep the preset without a User-Agent header, got:\n%s", effective)
	}
	// The effective definition submitted back, and the stored one loaded again, must both validate
	for name, document := range map[string][]byte{"effective": effective, "stored": prepared.Definition} {
		again, err := Prepare(document, newTestContext())
		if err != nil {
			t.Fatalf("%s definition submitted back: unexpected error: %v", name, err)
		}
		if again.Endpoint.UserAgent != endpoint.UserAgentFirefox {
			t.Errorf("%s definition: expected the firefox preset, got %q", name, again.Endpoint.UserAgent)
		}
	}
}

func TestPrepare_UserAgentConflict(t *testing.T) {
	_, err := Prepare([]byte("name: site\ngroup: web\nurl: https://example.org\nuser-agent: edge\nheaders:\n  User-Agent: Mine/1.0\nconditions: [\"[STATUS] == 200\"]\n"), newTestContext())
	if err == nil || !strings.Contains(err.Error(), "User-Agent header") {
		t.Fatalf("expected the conflict error, got %v", err)
	}
}

func TestPrepare_PushDefinitionRejectsUserAgent(t *testing.T) {
	_, err := Prepare([]byte("name: beat\ngroup: web\ntype: push\nuser-agent: chrome\n"), newTestContext())
	if err == nil || !strings.Contains(err.Error(), "user-agent") {
		t.Fatalf("expected a push definition with user-agent to be rejected for that field, got %v", err)
	}
}
