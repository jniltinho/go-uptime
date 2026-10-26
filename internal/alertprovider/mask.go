// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

// MaskedValue replaces every secret in what the administration answers. Receiving it back keeps the stored value.
const MaskedValue = "********"

// ErrMaskedSecretWithoutValue is returned when a submitted definition has the mask in a field that has no stored value
var ErrMaskedSecretWithoutValue = errors.New("a secret field has the mask but no stored value: type the secret")

// MaskSecrets returns a deep copy of document, the definition of a provider of alertType without its type key, in which
// every non-empty value of a secret field is MaskedValue. An empty value stays empty, so that the administration can
// tell a secret that is not set from one that is.
func MaskSecrets(alertType alert.Type, document map[string]any) map[string]any {
	masked, _ := maskValue(alertType, document, "").(map[string]any)
	return masked
}

func maskValue(alertType alert.Type, value any, path string) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, child := range typed {
			copied[key] = maskValue(alertType, child, joinPath(path, key))
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, child := range typed {
			copied[i] = maskValue(alertType, child, path+"[]")
		}
		return copied
	case nil:
		return nil
	case string:
		if len(typed) > 0 && isSecretPath(alertType, path) {
			return MaskedValue
		}
		return typed
	default:
		if len(path) > 0 && isSecretPath(alertType, path) {
			return MaskedValue
		}
		return typed
	}
}

// RestoreSecrets replaces, in submitted, every MaskedValue of a secret field by the value stored at the same path of
// stored. The overrides are matched by group, the other lists by position. It returns ErrMaskedSecretWithoutValue when
// the mask has no stored value to stand for.
func RestoreSecrets(alertType alert.Type, submitted, stored map[string]any) error {
	_, err := restoreValue(alertType, submitted, stored, "")
	return err
}

func restoreValue(alertType alert.Type, submitted, stored any, path string) (any, error) {
	switch typed := submitted.(type) {
	case map[string]any:
		storedMap, _ := stored.(map[string]any)
		for key, child := range typed {
			var storedChild any
			if storedMap != nil {
				storedChild = storedMap[key]
			}
			restored, err := restoreValue(alertType, child, storedChild, joinPath(path, key))
			if err != nil {
				return nil, err
			}
			typed[key] = restored
		}
		return typed, nil
	case []any:
		storedList, _ := stored.([]any)
		for i, child := range typed {
			restored, err := restoreValue(alertType, child, matchingElement(path, child, storedList, i), path+"[]")
			if err != nil {
				return nil, err
			}
			typed[i] = restored
		}
		return typed, nil
	case string:
		if typed != MaskedValue || !isSecretPath(alertType, path) {
			return typed, nil
		}
		if stored == nil {
			return nil, fmt.Errorf("%w: %s", ErrMaskedSecretWithoutValue, path)
		}
		return stored, nil
	default:
		return typed, nil
	}
}

// matchingElement returns the stored element that a submitted element of a list stands for: the override of the same
// group, or the element at the same position of any other list
func matchingElement(path string, submitted any, stored []any, index int) any {
	if path == "overrides" {
		group := groupOf(submitted)
		for _, element := range stored {
			if groupOf(element) == group {
				return element
			}
		}
		return nil
	}
	if index < len(stored) {
		return stored[index]
	}
	return nil
}

func groupOf(element any) string {
	override, _ := element.(map[string]any)
	switch group := override["group"].(type) {
	case string:
		return group
	case int:
		return strconv.Itoa(group)
	}
	return ""
}
