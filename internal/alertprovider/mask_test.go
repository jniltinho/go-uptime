// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func decodeYAML(t *testing.T, text string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestMaskSecrets(t *testing.T) {
	t.Parallel()
	document := decodeYAML(t, `
webhook-url: https://hooks.example.com/secret
title: Ops
overrides:
  - group: core
    webhook-url: https://hooks.example.com/core
default-alert:
  failure-threshold: 5
  provider-override:
    webhook-url: https://hooks.example.com/default
`)
	masked := MaskSecrets("slack", document)
	expected := decodeYAML(t, `
webhook-url: "********"
title: Ops
overrides:
  - group: core
    webhook-url: "********"
default-alert:
  failure-threshold: 5
  provider-override:
    webhook-url: "********"
`)
	if !reflect.DeepEqual(masked, expected) {
		t.Errorf("expected %v, got %v", expected, masked)
	}
	if document["webhook-url"] != "https://hooks.example.com/secret" {
		t.Error("MaskSecrets changed its input")
	}
	custom := MaskSecrets("custom", decodeYAML(t, "url: https://x\nheaders:\n  Authorization: Bearer a\nmethod: POST\n"))
	if custom["url"] != MaskedValue || custom["headers"].(map[string]any)["Authorization"] != MaskedValue || custom["method"] != "POST" {
		t.Errorf("expected url and headers masked and method kept, got %v", custom)
	}
	empty := MaskSecrets("telegram", decodeYAML(t, "token: \"\"\nid: \"1\"\n"))
	if empty["token"] != "" {
		t.Errorf("expected an empty secret to stay empty, got %v", empty["token"])
	}
}

func TestRestoreSecrets(t *testing.T) {
	t.Parallel()
	stored := decodeYAML(t, `
webhook-url: https://hooks.example.com/secret
overrides:
  - group: apps
    webhook-url: https://hooks.example.com/apps
  - group: core
    webhook-url: https://hooks.example.com/core
`)
	submitted := decodeYAML(t, `
webhook-url: "********"
title: "********"
overrides:
  - group: core
    webhook-url: "********"
`)
	if err := RestoreSecrets("slack", submitted, stored); err != nil {
		t.Fatal(err)
	}
	if submitted["webhook-url"] != "https://hooks.example.com/secret" {
		t.Errorf("expected the stored webhook, got %v", submitted["webhook-url"])
	}
	if submitted["title"] != MaskedValue {
		t.Errorf("expected a public field to keep the literal mask, got %v", submitted["title"])
	}
	if got := submitted["overrides"].([]any)[0].(map[string]any)["webhook-url"]; got != "https://hooks.example.com/core" {
		t.Errorf("expected the webhook of the override of the same group, got %v", got)
	}
	err := RestoreSecrets("slack", decodeYAML(t, "overrides:\n  - group: new\n    webhook-url: \"********\"\n"), stored)
	if !errors.Is(err, ErrMaskedSecretWithoutValue) {
		t.Errorf("expected ErrMaskedSecretWithoutValue for a new group, got %v", err)
	}
}
