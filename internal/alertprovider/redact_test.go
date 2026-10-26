// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	t.Parallel()
	document := decodeYAML(t, "webhook-url: https://hooks.example.com/T0/B0/x y\ntitle: Ops\n")
	scenarios := []string{
		`Post "https://hooks.example.com/T0/B0/x y": dial tcp: no such host`,
		`Post "https%3A%2F%2Fhooks.example.com%2FT0%2FB0%2Fx+y": failed`,
		`{"url":"https:\/\/hooks.example.com\/T0\/B0\/x y"}`,
	}
	for _, text := range scenarios[:2] {
		if redacted := redact("slack", document, text); strings.Contains(redacted, "hooks.example.com") {
			t.Errorf("expected the webhook to be redacted from %q, got %q", text, redacted)
		}
	}
	if redacted := redact("slack", document, "title Ops"); redacted != "title Ops" {
		t.Errorf("expected public values to stay, got %q", redacted)
	}
	if redacted := redact("slack", document, strings.Repeat("a", 600)); len(redacted) > maximumDiagnosticLength+len("…") {
		t.Errorf("expected the text to be truncated, got %d characters", len(redacted))
	}
	cause := errors.New("token 123456789:ABC failed")
	err := redactError("telegram", decodeYAML(t, "token: \"123456789:ABC\"\nid: \"1\"\n"), cause)
	if strings.Contains(err.Error(), "ABC") || !errors.Is(err, cause) {
		t.Errorf("expected a redacted error wrapping its cause, got %q", err.Error())
	}
}
