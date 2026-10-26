// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"
)

const (
	// maximumDiagnosticLength is the length at which a text built from a definition is truncated
	maximumDiagnosticLength = 512

	// minimumRedactedLength is the length below which a secret is not searched for: replacing every "1" of a message
	// would make it unreadable without protecting anything
	minimumRedactedLength = 4
)

// redact replaces in text every secret value of document, the definition of a provider of alertType, in its raw, URL-
// and JSON-escaped forms, by MaskedValue, and truncates the result. Every text built from a managed definition goes
// through it before reaching a response or a log.
func redact(alertType alert.Type, document map[string]any, text string) string {
	secrets := secretValues(alertType, document, "", nil)
	// The longest first, so that a secret containing another one is replaced whole
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		forms := []string{secret, url.QueryEscape(secret), url.PathEscape(secret)}
		if encoded, err := json.Marshal(secret); err == nil {
			forms = append(forms, strings.Trim(string(encoded), `"`))
		}
		for _, form := range forms {
			if len(form) >= minimumRedactedLength {
				text = strings.ReplaceAll(text, form, MaskedValue)
			}
		}
	}
	if len(text) > maximumDiagnosticLength {
		text = text[:maximumDiagnosticLength] + "…"
	}
	return text
}

func init() {
	watchdog.SetSendErrorRedactor(redactSendError)
}

// redactSendError redacts a delivery error with the secrets of the provider of alertType in use, managed or of the
// configuration file
func redactSendError(alertType alert.Type, err error) string {
	snap := current.Load()
	if snap == nil {
		return redact(alertType, nil, err.Error())
	}
	document, _ := snap.definitionDocument(alertType)
	return redact(alertType, document, err.Error())
}

// redactError returns err with its text redacted, or nil
func redactError(alertType alert.Type, document map[string]any, err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{text: redact(alertType, document, err.Error()), cause: err}
}

// redactedError keeps the cause for errors.Is while only exposing the redacted text
type redactedError struct {
	text  string
	cause error
}

func (err *redactedError) Error() string { return err.text }

func (err *redactedError) Unwrap() error { return err.cause }

func secretValues(alertType alert.Type, value any, path string, values []string) []string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			values = secretValues(alertType, child, joinPath(path, key), values)
		}
	case []any:
		for _, child := range typed {
			values = secretValues(alertType, child, path+"[]", values)
		}
	case nil:
	default:
		if len(path) > 0 && isSecretPath(alertType, path) {
			if text := fmt.Sprint(typed); len(text) >= minimumRedactedLength && text != MaskedValue {
				values = append(values, text)
			}
		}
	}
	return values
}
