package managedendpoint

import (
	"reflect"
	"testing"
)

const secretsDefinition = `
name: api
group: web
url: https://user:p4ss@api.example.com/health?region=br&api_key=abc123&token=t0k
headers:
  Authorization: Bearer abc
  X-Api-Key: k3y
  Accept: application/json
client:
  oauth2:
    token-url: https://sso.example.com/token
    client-id: go-uptime
    client-secret: s3cret
ssh:
  username: monitor
  password: sshp4ss
  private-key: PRIVATE
alerts:
  - type: custom
    provider-override:
      url: https://hooks.example.com/secret-path
      headers:
        X-Token: abc
conditions: ["[STATUS] == 200"]
`

func TestMaskSecrets(t *testing.T) {
	document, err := ToDocument([]byte(secretsDefinition))
	if err != nil {
		t.Fatal(err)
	}
	MaskSecrets(document)
	if url := document["url"]; url != "https://user:********@api.example.com/health?region=br&api_key=********&token=********" {
		t.Errorf("unexpected masked url: %v", url)
	}
	headers := document["headers"].(map[string]any)
	if headers["Authorization"] != Mask || headers["X-Api-Key"] != Mask || headers["Accept"] != "application/json" {
		t.Errorf("unexpected masked headers: %v", headers)
	}
	if nestedMap(document, "client", "oauth2")["client-secret"] != Mask || nestedMap(document, "client", "oauth2")["client-id"] != "go-uptime" {
		t.Errorf("unexpected masked oauth2: %v", nestedMap(document, "client", "oauth2"))
	}
	ssh := nestedMap(document, "ssh")
	if ssh["password"] != Mask || ssh["private-key"] != Mask || ssh["username"] != "monitor" {
		t.Errorf("unexpected masked ssh: %v", ssh)
	}
	override := document["alerts"].([]any)[0].(map[string]any)["provider-override"].(map[string]any)
	if override["url"] != Mask || override["headers"].(map[string]any)["X-Token"] != Mask {
		t.Errorf("unexpected masked provider-override: %v", override)
	}
}

func TestRestoreMaskedSecrets(t *testing.T) {
	stored, err := ToDocument([]byte(secretsDefinition))
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := ToDocument([]byte(secretsDefinition))
	if err != nil {
		t.Fatal(err)
	}
	MaskSecrets(submitted)
	// The client changes a non-secret value and keeps the masked secrets
	submitted["headers"].(map[string]any)["Accept"] = "text/plain"
	RestoreMaskedSecrets(submitted, stored)
	if submitted["url"] != "https://user:p4ss@api.example.com/health?region=br&api_key=abc123&token=t0k" {
		t.Errorf("unexpected restored url: %v", submitted["url"])
	}
	expected, _ := ToDocument([]byte(secretsDefinition))
	expected["headers"].(map[string]any)["Accept"] = "text/plain"
	if !reflect.DeepEqual(submitted, expected) {
		t.Errorf("expected every secret to be restored\nexpected: %v\ngot:      %v", expected, submitted)
	}
}

func TestRestoreMaskedSecrets_ChangedSecretIsKept(t *testing.T) {
	stored, _ := ToDocument([]byte(secretsDefinition))
	submitted, _ := ToDocument([]byte(secretsDefinition))
	MaskSecrets(submitted)
	submitted["headers"].(map[string]any)["Authorization"] = "Bearer new"
	RestoreMaskedSecrets(submitted, stored)
	if submitted["headers"].(map[string]any)["Authorization"] != "Bearer new" {
		t.Errorf("expected a new secret to be kept, got %v", submitted["headers"])
	}
}

func TestHasMaskedSecret(t *testing.T) {
	scenarios := map[string]struct {
		definition string
		expected   bool
	}{
		"clean":              {"name: a\nurl: https://user:pass@example.org?token=abc\nheaders:\n  Authorization: Bearer x\n", false},
		"header":             {"name: a\nheaders:\n  Authorization: '********'\n", true},
		"url-password":       {"name: a\nurl: https://user:********@example.org\n", true},
		"url-query":          {"name: a\nurl: https://example.org?token=********\n", true},
		"oauth2":             {"name: a\nclient:\n  oauth2:\n    client-secret: '********'\n", true},
		"ssh":                {"name: a\nssh:\n  private-key: '********'\n", true},
		"push-token":         {"type: push\nname: a\ntoken: '********'\n", true},
		"push-option-token":  {"name: a\npush:\n  token: '********'\n", true},
		"provider-override":  {"name: a\nalerts:\n  - type: slack\n    provider-override:\n      webhook-url: '********'\n", true},
		"nested-override":    {"name: a\nalerts:\n  - type: custom\n    provider-override:\n      headers:\n        X-Key: '********'\n", true},
		"insensitive-header": {"name: a\nheaders:\n  Accept: '********'\n", false},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			document, err := ToDocument([]byte(scenario.definition))
			if err != nil {
				t.Fatal(err)
			}
			if actual := HasMaskedSecret(document); actual != scenario.expected {
				t.Errorf("expected %v, got %v", scenario.expected, actual)
			}
		})
	}
}
