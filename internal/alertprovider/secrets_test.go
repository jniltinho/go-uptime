// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"slices"
	"strings"
	"testing"
)

// TestEveryProviderFieldIsClassified fails when a field of a provider is neither secret nor public, when it is both,
// and when a classified path no longer exists, so that a new field can never be shown by accident
func TestEveryProviderFieldIsClassified(t *testing.T) {
	t.Parallel()
	for _, alertType := range Types() {
		providerStruct, ok := providerType(alertType)
		if !ok {
			t.Fatalf("no provider type for %s", alertType)
		}
		if _, ok := secretPaths[alertType]; !ok {
			t.Errorf("%s has no entry in secretPaths", alertType)
		}
		if _, ok := publicPaths[alertType]; !ok {
			t.Errorf("%s has no entry in publicPaths", alertType)
		}
		secret, public := classifiedPaths(alertType, true), classifiedPaths(alertType, false)
		seen := make(map[string]bool)
		for _, path := range fieldPaths(providerStruct) {
			if strings.HasPrefix(path, defaultAlertField+".") || path == overridesPrefix+"group" {
				continue
			}
			path = strings.TrimPrefix(path, overridesPrefix)
			seen[path] = true
			inSecret, inPublic := slices.Contains(secret, path), slices.Contains(public, path)
			if !inSecret && !inPublic {
				t.Errorf("field %s of %s is not classified: add it to secretPaths or publicPaths", path, alertType)
			}
			if inSecret && inPublic {
				t.Errorf("field %s of %s is both secret and public", path, alertType)
			}
		}
		for _, path := range append(append([]string{}, secret...), public...) {
			if !seen[path] {
				t.Errorf("classified field %s of %s does not exist", path, alertType)
			}
		}
	}
}

func TestIsSecretPath(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		alertType string
		path      string
		secret    bool
	}{
		{"slack", "webhook-url", true},
		{"slack", "title", false},
		{"slack", "overrides[].webhook-url", true},
		{"slack", "overrides[].group", false},
		{"slack", "default-alert.failure-threshold", false},
		{"slack", "default-alert.provider-override.webhook-url", true},
		{"custom", "headers.*", true},
		{"telegram", "client.oauth2.client-secret", true},
		{"telegram", "client.timeout", false},
		{"slack", "unknown", true},
	}
	for _, scenario := range scenarios {
		if got := isSecretPath(alertTypeOf(scenario.alertType), scenario.path); got != scenario.secret {
			t.Errorf("isSecretPath(%s, %s) = %v, expected %v", scenario.alertType, scenario.path, got, scenario.secret)
		}
	}
}
