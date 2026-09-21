package statuspage

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"golang.org/x/crypto/bcrypt"
)

// Fork: login of a status page, on the side of the administration

const definitionWithLogin = "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: page-secret\n"

// TestService_StoredDefinitionNeverCarriesThePassword makes sure the plaintext password never reaches the database:
// the definition is persisted with yaml.Marshal of the page, so a password in that struct would be written
func TestService_StoredDefinitionNeverCarriesThePassword(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	if _, err := service.Create([]byte(definitionWithLogin), "ops@example.com"); err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	stored, err := managedStatusPageStore.GetManagedStatusPage("clients")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Definition, "password: ") || strings.Contains(stored.Definition, "page-secret") {
		t.Errorf("expected the stored definition to carry only the hash, got %q", stored.Definition)
	}
	page, err := Parse([]byte(stored.Definition))
	if err != nil {
		t.Fatalf("expected the stored definition to be valid, got %v", err)
	}
	if !page.RequiresLogin() || page.Auth.Username != "client" {
		t.Fatalf("expected the stored definition to require a login, got %+v", page.Auth)
	}
	if !security.CheckCredentials(page.Auth.Username, page.Auth.PasswordBcryptHashBase64Encoded, "client", "page-secret") {
		t.Error("expected the stored hash to be the hash of the submitted password")
	}
	hash, err := base64.URLEncoding.DecodeString(page.Auth.PasswordBcryptHashBase64Encoded)
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost(hash); err != nil || cost != bcrypt.DefaultCost {
		t.Errorf("expected a hash of cost %d, got %d (%v)", bcrypt.DefaultCost, cost, err)
	}
	// The registry keeps the real hash, which is what the public routes check
	published, ok := Lookup("clients")
	if !ok {
		t.Fatal("expected the page to be published")
	}
	if published.Page.Auth.PasswordBcryptHashBase64Encoded != page.Auth.PasswordBcryptHashBase64Encoded {
		t.Error("expected the published page to keep the real hash")
	}
	// And every read of the administration masks it
	detail, err := service.Get("clients")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Definition.Auth.PasswordBcryptHashBase64Encoded != maskedHash || strings.Contains(detail.YAML, "$2a$") {
		t.Errorf("expected the hash to be masked, got %+v and %q", detail.Definition.Auth, detail.YAML)
	}
	if !detail.RequiresLogin {
		t.Error("expected the detail to say that the page requires a login")
	}
}

func TestResolveSubmittedCredential(t *testing.T) {
	const storedHash = "c3RvcmVkLWhhc2g="
	scenarios := []struct {
		name          string
		raw           string
		storedHash    string
		expectedError error
		// expectedHash is the hash the resolved definition must carry, or empty for a hash generated from the password
		expectedHash string
	}{
		{name: "without a credential", raw: "slug: clients\ntitle: Clients\n"},
		{name: "with a password", raw: definitionWithLogin},
		{
			name:         "with the masked hash of a page that already requires a login",
			raw:          "slug: clients\nauth:\n  username: client\n  password-bcrypt-base64: '********'\n",
			storedHash:   storedHash,
			expectedHash: storedHash,
		},
		{
			name:         "with an empty password on a page that already requires a login",
			raw:          "slug: clients\nauth:\n  username: client\n  password: ''\n",
			storedHash:   storedHash,
			expectedHash: storedHash,
		},
		{
			name:         "with a hash pasted by hand",
			raw:          "slug: clients\nauth:\n  username: client\n  password-bcrypt-base64: cGFzdGVk\n",
			storedHash:   storedHash,
			expectedHash: "cGFzdGVk",
		},
		{
			name:          "with the masked hash of a page that does not exist yet",
			raw:           "slug: clients\nauth:\n  username: client\n  password-bcrypt-base64: '********'\n",
			expectedError: ErrAuthPasswordRequired,
		},
		{
			name:          "without a password on a page that does not exist yet",
			raw:           "slug: clients\nauth:\n  username: client\n",
			expectedError: ErrAuthPasswordRequired,
		},
		{
			name:          "with a password that is too short",
			raw:           "slug: clients\nauth:\n  username: client\n  password: short\n",
			expectedError: ErrAuthPasswordTooShort,
		},
		{
			name:          "with a password longer than bcrypt accepts",
			raw:           "slug: clients\nauth:\n  username: client\n  password: " + strings.Repeat("a", 73) + "\n",
			expectedError: ErrAuthPasswordTooLong,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			resolved, err := resolveSubmittedCredential([]byte(scenario.raw), scenario.storedHash)
			if err != scenario.expectedError {
				t.Fatalf("expected %v, got %v", scenario.expectedError, err)
			}
			if err != nil {
				return
			}
			if strings.Contains(string(resolved), "password: ") {
				t.Errorf("expected the plaintext password to be gone, got %q", resolved)
			}
			if len(scenario.expectedHash) > 0 && !strings.Contains(string(resolved), scenario.expectedHash) {
				t.Errorf("expected the hash %s, got %q", scenario.expectedHash, resolved)
			}
		})
	}
}

// TestResolveSubmittedCredential_JSON makes sure a JSON submission, which the administration also accepts, goes through
// the same path
func TestResolveSubmittedCredential_JSON(t *testing.T) {
	resolved, err := resolveSubmittedCredential([]byte(`{"slug":"clients","title":"Clients","groups":["core"],"auth":{"username":"client","password":"page-secret"}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	page, err := Parse(resolved)
	if err != nil {
		t.Fatalf("expected the resolved definition to be valid, got %v", err)
	}
	if !security.CheckCredentials(page.Auth.Username, page.Auth.PasswordBcryptHashBase64Encoded, "client", "page-secret") {
		t.Error("expected the password of the JSON submission to be hashed")
	}
}

func TestMaskCredentialInDefinition(t *testing.T) {
	scenarios := []struct {
		name     string
		raw      string
		expected string
	}{
		{name: "without a credential", raw: "slug: clients\ntitle: Clients\n", expected: "slug: clients\ntitle: Clients\n"},
		{
			name:     "with a credential",
			raw:      "slug: clients\nauth:\n  username: client\n  password-bcrypt-base64: c2VjcmV0\n",
			expected: "slug: clients\nauth:\n    username: client\n    password-bcrypt-base64: '********'\n",
		},
		{name: "that is not a mapping", raw: "- one\n- two\n", expected: "- one\n- two\n"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if masked := maskCredentialInDefinition(scenario.raw); masked != scenario.expected {
				t.Errorf("expected %q, got %q", scenario.expected, masked)
			}
		})
	}
}

// TestService_ValidateRestore_MaskedCredential covers the file assembled from the reads of the administration, which
// carries the hash masked: the credential of the destination is merged before the validation
func TestService_ValidateRestore_MaskedCredential(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	if _, err := service.Create([]byte(definitionWithLogin), "ops@example.com"); err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	stored, err := managedStatusPageStore.GetManagedStatusPage("clients")
	if err != nil {
		t.Fatal(err)
	}
	storedPage, err := Parse([]byte(stored.Definition))
	if err != nil {
		t.Fatal(err)
	}
	refs := Endpoints()
	masked := "slug: clients\ntitle: Clients of the backup\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password-bcrypt-base64: '********'\n"
	page, definition, _, err := service.ValidateRestore([]byte(masked), refs)
	if err != nil {
		t.Fatalf("expected the page of the destination to keep its credential, got %v", err)
	}
	if page.Auth.PasswordBcryptHashBase64Encoded != storedPage.Auth.PasswordBcryptHashBase64Encoded {
		t.Error("expected the hash of the destination to be kept")
	}
	if strings.Contains(string(definition), maskedHash) {
		t.Errorf("expected the definition to be restored without the mask, got %q", definition)
	}
	// A page that does not exist at the destination has no credential to keep
	newPage := strings.Replace(masked, "slug: clients", "slug: partners", 1)
	if _, _, _, err = service.ValidateRestore([]byte(newPage), refs); !errors.Is(err, managedendpoint.ErrMaskedSecret) {
		t.Errorf("expected a masked secret, got %v", err)
	}
	// A backup with the real hash keeps working, and a plaintext password is refused as an unknown field
	if _, _, _, err = service.ValidateRestore([]byte(stored.Definition), refs); err != nil {
		t.Errorf("expected the definition of the backup to be restored, got %v", err)
	}
	if _, _, _, err = service.ValidateRestore([]byte(definitionWithLogin), refs); err == nil {
		t.Error("expected a plaintext password in a backup to be refused")
	}
}
