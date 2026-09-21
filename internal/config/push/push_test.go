// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package push

import (
	"errors"
	"strings"
	"testing"
)

func TestConfig_ValidateAndSetDefaults(t *testing.T) {
	validKeyToken := strings.Repeat("a", MinimumKeyTokenLength)
	scenarios := []struct {
		name     string
		config   *Config
		expected error
	}{
		{name: "empty", config: &Config{}},
		{name: "valid", config: &Config{
			Keys:      []*Key{{Name: " akamai ", Token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5"}},
			Endpoints: []*Endpoint{{Key: " Core_API "}, {Key: "core_site", Token: "site-token_1"}},
		}},
		{name: "key-without-name", config: &Config{Keys: []*Key{{Name: " ", Token: validKeyToken}}}, expected: ErrInvalidKeyName},
		{name: "key-name-too-long", config: &Config{Keys: []*Key{{Name: strings.Repeat("n", MaximumKeyNameLength+1), Token: validKeyToken}}}, expected: ErrInvalidKeyName},
		{name: "duplicate-key-name", config: &Config{Keys: []*Key{{Name: "a", Token: validKeyToken}, {Name: "a", Token: validKeyToken + "b"}}}, expected: ErrDuplicateKeyName},
		{name: "key-token-too-short", config: &Config{Keys: []*Key{{Name: "a", Token: strings.Repeat("a", MinimumKeyTokenLength-1)}}}, expected: ErrInvalidKeyToken},
		{name: "key-token-too-long", config: &Config{Keys: []*Key{{Name: "a", Token: strings.Repeat("a", MaximumTokenLength+1)}}}, expected: ErrInvalidKeyToken},
		{name: "key-token-with-invalid-character", config: &Config{Keys: []*Key{{Name: "a", Token: validKeyToken + "/"}}}, expected: ErrInvalidKeyToken},
		{name: "duplicate-key-token", config: &Config{Keys: []*Key{{Name: "a", Token: validKeyToken}, {Name: "b", Token: validKeyToken}}}, expected: ErrDuplicateToken},
		{name: "endpoint-without-key", config: &Config{Endpoints: []*Endpoint{{Key: " "}}}, expected: ErrEndpointKeyRequired},
		{name: "duplicate-endpoint", config: &Config{Endpoints: []*Endpoint{{Key: "core_api"}, {Key: "CORE_API"}}}, expected: ErrDuplicateEndpoint},
		{name: "endpoint-token-too-short", config: &Config{Endpoints: []*Endpoint{{Key: "core_api", Token: "short"}}}, expected: ErrInvalidEndpointToken},
		{name: "endpoint-token-equal-to-key-token", config: &Config{Keys: []*Key{{Name: "a", Token: validKeyToken}}, Endpoints: []*Endpoint{{Key: "core_api", Token: validKeyToken}}}, expected: ErrDuplicateToken},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			err := scenario.config.ValidateAndSetDefaults()
			if scenario.expected == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if scenario.expected != nil && !errors.Is(err, scenario.expected) {
				t.Fatalf("expected %v, got %v", scenario.expected, err)
			}
		})
	}
}

func TestConfig_ValidateAndSetDefaults_Normalizes(t *testing.T) {
	config := &Config{
		Keys:      []*Key{{Name: " akamai ", Token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5"}},
		Endpoints: []*Endpoint{{Key: " Core_API ", Token: "site-token_1"}},
	}
	if err := config.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	if config.Keys[0].Name != "akamai" || config.Endpoints[0].Key != "core_api" {
		t.Errorf("expected the name to be trimmed and the endpoint key in lowercase, got %q and %q", config.Keys[0].Name, config.Endpoints[0].Key)
	}
	if config.Keys[0].Hash() != HashToken("keSDu7G855jvVat1xWiY2Gk4CkL1End5") || config.Keys[0].Hint() != "End5" {
		t.Errorf("expected the hash and the hint of the token, got hint %q", config.Keys[0].Hint())
	}
	if tokens := config.Tokens(); len(tokens) != 2 {
		t.Errorf("expected the 2 tokens of the configuration, got %v", tokens)
	}
}

func TestValidToken(t *testing.T) {
	if !ValidToken("keSDu7G855jvVat1xWiY2Gk4CkL1End5", MinimumEndpointTokenLength) {
		t.Error("expected a token of the Uptime Kuma to be valid")
	}
	for _, token := range []string{"", "short", "with space1", "with/slash", strings.Repeat("a", MaximumTokenLength+1)} {
		if ValidToken(token, MinimumEndpointTokenLength) {
			t.Errorf("expected %q to be invalid", token)
		}
	}
	if TokenHint("abc") != "" || TokenHint("abcdef") != "cdef" {
		t.Error("unexpected token hints")
	}
}
