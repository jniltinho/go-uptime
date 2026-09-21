// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Fork: login of a status page, on the side of the administration. The definition is persisted with yaml.Marshal of
// pageconfig.Page, so a plaintext password in that struct would reach the database, the YAML of the detail and the
// backup. The password therefore lives only in the document submitted by the administration: it is turned into a
// bcrypt hash before anything is parsed or persisted, and every read of the administration answers with the hash
// masked, exactly as the secrets of the endpoints already are.
const (
	// authPasswordFieldName is the field of the submitted document holding the plaintext password
	authPasswordFieldName = "password"

	// authPasswordHashFieldName is the field of the definition holding the bcrypt hash in base64
	authPasswordHashFieldName = "password-bcrypt-base64"

	// authFieldName is the section of the definition holding the credential of the page
	authFieldName = "auth"

	// MinimumAuthPasswordLength is the minimum length of the password of the login of a page
	MinimumAuthPasswordLength = 8

	// MaximumAuthPasswordBytes is the maximum length in bytes of the password, the limit of bcrypt, which silently
	// ignores everything past it
	MaximumAuthPasswordBytes = 72

	// maskedHash is what every read of the administration answers in place of the hash of the credential of a page
	maskedHash = managedendpoint.Mask
)

var (
	// ErrAuthPasswordTooShort is returned when the submitted password is too short
	ErrAuthPasswordTooShort = fmt.Errorf("the password of the login of a status page must have at least %d characters", MinimumAuthPasswordLength)

	// ErrAuthPasswordTooLong is returned when the submitted password is longer than bcrypt accepts
	ErrAuthPasswordTooLong = fmt.Errorf("the password of the login of a status page must have at most %d bytes", MaximumAuthPasswordBytes)

	// ErrAuthPasswordRequired is returned when a page starts requiring a login without a password
	ErrAuthPasswordRequired = errors.New("a password is required to require a login on this status page")
)

// resolveSubmittedCredential turns the document submitted by the administration into a definition that can be parsed
// and persisted: the plaintext password becomes a bcrypt hash, and a password that was not sent, or a hash that comes
// back masked from a read, means "keep the stored hash".
func resolveSubmittedCredential(raw []byte, storedPasswordHash string) ([]byte, error) {
	document, ok := decodeDocument(raw)
	if !ok {
		// Not a document with a mapping: Parse answers with the error of the definition
		return raw, nil
	}
	auth := mappingValue(document.Content[0], authFieldName)
	if auth == nil || auth.Kind != yaml.MappingNode {
		return raw, nil
	}
	password := strings.TrimSpace(valueOf(removeMappingKey(auth, authPasswordFieldName)))
	if len(password) > 0 {
		if utf8.RuneCountInString(password) < MinimumAuthPasswordLength {
			return nil, ErrAuthPasswordTooShort
		}
		if len(password) > MaximumAuthPasswordBytes {
			return nil, ErrAuthPasswordTooLong
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		setMappingValue(auth, authPasswordHashFieldName, base64.URLEncoding.EncodeToString(hash), yaml.SingleQuotedStyle)
		return yaml.Marshal(document)
	}
	submittedHash := strings.TrimSpace(valueOf(mappingValue(auth, authPasswordHashFieldName)))
	if len(submittedHash) > 0 && submittedHash != maskedHash {
		return yaml.Marshal(document)
	}
	if len(storedPasswordHash) == 0 {
		return nil, ErrAuthPasswordRequired
	}
	setMappingValue(auth, authPasswordHashFieldName, storedPasswordHash, yaml.SingleQuotedStyle)
	return yaml.Marshal(document)
}

// mergeRestoredCredential replaces the masked hash of a restored definition by the hash stored at the destination, and
// returns managedendpoint.ErrMaskedSecret when the page does not exist there, so the restore skips it with the reason
// the masked secrets of the endpoints already use
func mergeRestoredCredential(raw []byte) ([]byte, error) {
	document, ok := decodeDocument(raw)
	if !ok {
		return raw, nil
	}
	mapping := document.Content[0]
	auth := mappingValue(mapping, authFieldName)
	if auth == nil || auth.Kind != yaml.MappingNode {
		return raw, nil
	}
	if strings.TrimSpace(valueOf(mappingValue(auth, authPasswordHashFieldName))) != maskedHash {
		return raw, nil
	}
	stored := storedPasswordHash(strings.TrimSpace(valueOf(mappingValue(mapping, "slug"))))
	if len(stored) == 0 {
		return nil, managedendpoint.ErrMaskedSecret
	}
	setMappingValue(auth, authPasswordHashFieldName, stored, yaml.SingleQuotedStyle)
	return yaml.Marshal(document)
}

// storedPasswordHash returns the hash guarded by the managed status page with the given slug, and an empty string when
// the page does not exist, is not managed or does not require a login
func storedPasswordHash(slug string) string {
	if len(slug) == 0 {
		return ""
	}
	snap := current.Load()
	if snap == nil {
		return ""
	}
	state := snap.managedStates[slug]
	if state == nil {
		return ""
	}
	page := state.Page
	if page == nil && state.Stored != nil {
		// The stored definition is invalid: only a credential that parses can be kept
		page, _ = Parse([]byte(state.Stored.Definition))
	}
	if !page.RequiresLogin() {
		return ""
	}
	return page.Auth.PasswordBcryptHashBase64Encoded
}

// maskCredential returns a copy of the page whose hash is masked. The page of the registry, which serves the public
// routes, keeps the real hash.
func maskCredential(page *pageconfig.Page) *pageconfig.Page {
	if !page.RequiresLogin() {
		return page
	}
	masked := *page
	auth := *page.Auth
	auth.PasswordBcryptHashBase64Encoded = maskedHash
	masked.Auth = &auth
	return &masked
}

// maskCredentialInDefinition returns the definition with the hash masked, keeping everything else as the administrator
// wrote it
func maskCredentialInDefinition(raw string) string {
	document, ok := decodeDocument([]byte(raw))
	if !ok {
		return raw
	}
	auth := mappingValue(document.Content[0], authFieldName)
	if auth == nil || auth.Kind != yaml.MappingNode {
		return raw
	}
	if mappingValue(auth, authPasswordHashFieldName) == nil {
		return raw
	}
	setMappingValue(auth, authPasswordHashFieldName, maskedHash, yaml.SingleQuotedStyle)
	masked, err := yaml.Marshal(document)
	if err != nil {
		// Never answer with a definition that could still hold the hash
		return ""
	}
	return string(masked)
}

func decodeDocument(raw []byte) (*yaml.Node, bool) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, false
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, false
	}
	return &document, true
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func removeMappingKey(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return value
		}
	}
	return nil
}

func setMappingValue(mapping *yaml.Node, key, value string, style yaml.Style) {
	if existing := mappingValue(mapping, key); existing != nil {
		existing.Kind, existing.Tag, existing.Style, existing.Value = yaml.ScalarNode, "!!str", style, value
		existing.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: style, Value: value},
	)
}

func valueOf(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return node.Value
}
