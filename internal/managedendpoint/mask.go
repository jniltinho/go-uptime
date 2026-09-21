// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"net/url"
	"strings"
)

// Mask replaces the value of secrets in the definitions returned by the administration API
const Mask = "********"

var sensitiveNameParts = []string{"authorization", "cookie", "token", "secret", "password", "key"}

func isSensitiveName(name string) bool {
	lowerName := strings.ToLower(name)
	for _, part := range sensitiveNameParts {
		if strings.Contains(lowerName, part) {
			return true
		}
	}
	return false
}

// MaskSecrets replaces, in place, the secrets of a definition document by Mask: values of sensitive headers, the
// password and sensitive query parameters of the URL, client.oauth2.client-secret, ssh.password, ssh.private-key and
// every value of alerts[].provider-override.
func MaskSecrets(document map[string]any) {
	if headers, ok := document["headers"].(map[string]any); ok {
		for name, value := range headers {
			if isSensitiveName(name) && value != nil {
				headers[name] = Mask
			}
		}
	}
	if rawURL, ok := document["url"].(string); ok {
		document["url"] = maskURL(rawURL)
	}
	maskValue(nestedMap(document, "client", "oauth2"), "client-secret")
	maskValue(nestedMap(document, "ssh"), "password")
	maskValue(nestedMap(document, "ssh"), "private-key")
	// Fork: push tokens, returned apart by the administration API (see Detail.PushToken)
	maskValue(document, "token")
	maskValue(nestedMap(document, pushField), "token")
	if alerts, ok := document["alerts"].([]any); ok {
		for _, item := range alerts {
			if alert, ok := item.(map[string]any); ok {
				if override, exists := alert["provider-override"]; exists {
					alert["provider-override"] = maskLeaves(override)
				}
			}
		}
	}
}

// RestoreMaskedSecrets replaces, in the submitted document, every secret equal to Mask by the value found at the
// same place in the stored document, so that a client can send back a masked definition without losing secrets.
func RestoreMaskedSecrets(submitted, stored map[string]any) {
	if headers, ok := submitted["headers"].(map[string]any); ok {
		storedHeaders, _ := stored["headers"].(map[string]any)
		for name, value := range headers {
			if value == Mask {
				if storedValue, exists := storedHeaders[name]; exists {
					headers[name] = storedValue
				}
			}
		}
	}
	if rawURL, ok := submitted["url"].(string); ok {
		if storedURL, ok := stored["url"].(string); ok {
			submitted["url"] = restoreURL(rawURL, storedURL)
		}
	}
	restoreValue(nestedMap(submitted, "client", "oauth2"), nestedMap(stored, "client", "oauth2"), "client-secret")
	restoreValue(nestedMap(submitted, "ssh"), nestedMap(stored, "ssh"), "password")
	restoreValue(nestedMap(submitted, "ssh"), nestedMap(stored, "ssh"), "private-key")
	restoreValue(submitted, stored, "token")
	restoreValue(nestedMap(submitted, pushField), nestedMap(stored, pushField), "token")
	submittedAlerts, _ := submitted["alerts"].([]any)
	storedAlerts, _ := stored["alerts"].([]any)
	for i, item := range submittedAlerts {
		if i >= len(storedAlerts) {
			break
		}
		alert, ok := item.(map[string]any)
		storedAlert, storedOK := storedAlerts[i].(map[string]any)
		if !ok || !storedOK || alert["type"] != storedAlert["type"] {
			continue
		}
		if override, exists := alert["provider-override"]; exists {
			alert["provider-override"] = restoreLeaves(override, storedAlert["provider-override"])
		}
	}
}

func nestedMap(document map[string]any, path ...string) map[string]any {
	current := document
	for _, part := range path {
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func maskValue(m map[string]any, key string) {
	if value, exists := m[key]; exists && value != nil && value != "" {
		m[key] = Mask
	}
}

func restoreValue(submitted, stored map[string]any, key string) {
	if submitted == nil || submitted[key] != Mask || stored == nil {
		return
	}
	if storedValue, exists := stored[key]; exists {
		submitted[key] = storedValue
	}
}

func maskLeaves(value any) any {
	switch typedValue := value.(type) {
	case map[string]any:
		for k, v := range typedValue {
			typedValue[k] = maskLeaves(v)
		}
		return typedValue
	case []any:
		for i, v := range typedValue {
			typedValue[i] = maskLeaves(v)
		}
		return typedValue
	case nil:
		return nil
	default:
		return Mask
	}
}

func restoreLeaves(submitted, stored any) any {
	switch typedValue := submitted.(type) {
	case map[string]any:
		storedMap, _ := stored.(map[string]any)
		for k, v := range typedValue {
			typedValue[k] = restoreLeaves(v, storedMap[k])
		}
		return typedValue
	case []any:
		storedSlice, _ := stored.([]any)
		for i, v := range typedValue {
			var storedValue any
			if i < len(storedSlice) {
				storedValue = storedSlice[i]
			}
			typedValue[i] = restoreLeaves(v, storedValue)
		}
		return typedValue
	default:
		if submitted == Mask && stored != nil {
			return stored
		}
		return submitted
	}
}

// maskURL masks the password and the sensitive query parameters of a URL, keeping the order of the parameters
func maskURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	changed := false
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), Mask)
			changed = true
		}
	}
	if len(parsed.RawQuery) > 0 {
		params := strings.Split(parsed.RawQuery, "&")
		for i, param := range params {
			name, _, _ := strings.Cut(param, "=")
			if unescapedName, err := url.QueryUnescape(name); err == nil && isSensitiveName(unescapedName) {
				params[i] = name + "=" + Mask
				changed = true
			}
		}
		parsed.RawQuery = strings.Join(params, "&")
	}
	if !changed {
		return rawURL
	}
	return strings.Replace(parsed.String(), url.QueryEscape(Mask), Mask, -1)
}

// restoreURL restores the password and the query parameters of submittedURL that are equal to Mask from storedURL
func restoreURL(submittedURL, storedURL string) string {
	if !strings.Contains(submittedURL, Mask) {
		return submittedURL
	}
	submitted, err := url.Parse(submittedURL)
	if err != nil {
		return submittedURL
	}
	stored, err := url.Parse(storedURL)
	if err != nil {
		return submittedURL
	}
	if submitted.User != nil && stored.User != nil {
		if password, _ := submitted.User.Password(); password == Mask {
			if storedPassword, hasPassword := stored.User.Password(); hasPassword {
				submitted.User = url.UserPassword(submitted.User.Username(), storedPassword)
			}
		}
	}
	if len(submitted.RawQuery) > 0 {
		storedValues := make(map[string]string)
		for _, param := range strings.Split(stored.RawQuery, "&") {
			name, value, _ := strings.Cut(param, "=")
			if _, exists := storedValues[name]; !exists {
				storedValues[name] = value
			}
		}
		params := strings.Split(submitted.RawQuery, "&")
		for i, param := range params {
			name, value, _ := strings.Cut(param, "=")
			if value == Mask {
				if storedValue, exists := storedValues[name]; exists {
					params[i] = name + "=" + storedValue
				}
			}
		}
		submitted.RawQuery = strings.Join(params, "&")
	}
	return submitted.String()
}
