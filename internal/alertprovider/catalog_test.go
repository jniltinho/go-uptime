// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"encoding/json"
	"testing"
)

// TestDocsDescribeEveryProvider fails when a provider has no description or when a documented field does not exist
func TestDocsDescribeEveryProvider(t *testing.T) {
	t.Parallel()
	docs := map[string]providerDoc{}
	if err := json.Unmarshal(docsJSON, &docs); err != nil {
		t.Fatal(err)
	}
	for _, alertType := range Types() {
		doc, ok := docs[string(alertType)]
		if !ok || len(doc.Name) == 0 || len(doc.Category) == 0 {
			t.Errorf("%s has no name or category in docs.json", alertType)
			continue
		}
		structType, _ := providerType(alertType)
		paths := map[string]bool{}
		for _, path := range fieldPaths(structType) {
			paths[path] = true
		}
		for key := range doc.Fields {
			if !paths[key] && !paths[key+".*"] && !paths[key+".*.*"] {
				t.Errorf("docs.json documents %s.%s, which does not exist", alertType, key)
			}
		}
	}
	if len(docs) != len(Types()) {
		t.Errorf("docs.json has %d providers, expected %d", len(docs), len(Types()))
	}
}

func TestCatalog(t *testing.T) {
	t.Parallel()
	info := typeInfo("telegram")
	if info.Name != "Telegram" || !info.SupportsOverrides {
		t.Errorf("unexpected telegram info: %+v", info)
	}
	var token *Field
	for i := range info.Fields {
		if info.Fields[i].Key == "token" {
			token = &info.Fields[i]
		}
	}
	if token == nil || !token.Secret || !token.Required || token.Kind != KindString {
		t.Errorf("expected a required secret string token, got %+v", token)
	}
	for _, field := range info.Fields {
		if field.Key == "client" {
			t.Error("the client section must not be a form field")
		}
	}
}
