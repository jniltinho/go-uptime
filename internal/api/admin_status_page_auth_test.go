// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Fork: login of a status page, on the side of the administration

const statusPageWithLogin = "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: page-secret\n"

// expectNoSecret fails when the answer of the administration carries the hash of the credential or the password
func expectNoSecret(t *testing.T, what string, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"page-secret", "$2a$", "$2b$", "$2y$"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("%s carries %q: %s", what, secret, encoded)
		}
	}
}

func maskedDefinitionOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	definition, ok := body["definition"].(map[string]any)
	if !ok {
		t.Fatalf("expected a definition in the answer, got %v", body)
	}
	auth, ok := definition["auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected the credential in the definition, got %v", definition)
	}
	if auth["username"] != "client" {
		t.Errorf("expected the username to be shown, got %v", auth)
	}
	if auth["password-bcrypt-base64"] != "********" {
		t.Errorf("expected the hash to be masked, got %v", auth)
	}
	return definition
}

func TestAdminStatusPagesAPI_Login(t *testing.T) {
	env := newAdminStatusPageEnvironment(t, true)
	publicStatus := func(t *testing.T, authorization string) int {
		t.Helper()
		status, _, _ := env.do(t, http.MethodGet, "/api/v1/status-pages/clients", "", map[string]string{"Authorization": authorization})
		return status
	}
	var yamlOfTheDetail string

	t.Run("create-with-a-password", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", statusPageWithLogin, nil, http.StatusCreated)
		if body["requiresLogin"] != true {
			t.Errorf("expected the page to be listed as requiring a login, got %v", body["requiresLogin"])
		}
		maskedDefinitionOf(t, body)
		expectNoSecret(t, "the answer of the creation", body)
		if status := publicStatus(t, basicAuthorization("client", "page-secret")); status != http.StatusOK {
			t.Errorf("expected the password to open the page, got %d", status)
		}
		if status := publicStatus(t, ""); status != http.StatusUnauthorized {
			t.Errorf("expected the page to require the credential, got %d", status)
		}
	})
	t.Run("reads-do-not-carry-the-hash", func(t *testing.T) {
		_, detail := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/clients", "", nil, http.StatusOK)
		maskedDefinitionOf(t, detail)
		expectNoSecret(t, "the detail", detail)
		yamlOfTheDetail, _ = detail["yaml"].(string)
		if !strings.Contains(yamlOfTheDetail, "password-bcrypt-base64: '********'") {
			t.Errorf("expected the hash to be masked in the YAML, got %q", yamlOfTheDetail)
		}
		_, validation := env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages/validate?slug=clients", yamlOfTheDetail, nil, http.StatusOK)
		maskedDefinitionOf(t, validation)
		expectNoSecret(t, "the validation", validation)
		_, preview := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/clients/preview", "", nil, http.StatusOK)
		expectNoSecret(t, "the preview", preview)
		_, listing := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages", "", nil, http.StatusOK)
		expectNoSecret(t, "the list", listing)
	})
	t.Run("saving-the-document-that-was-read-keeps-the-password", func(t *testing.T) {
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", yamlOfTheDetail, map[string]string{"If-Match": `"1"`}, http.StatusOK)
		if status := publicStatus(t, basicAuthorization("client", "page-secret")); status != http.StatusOK {
			t.Errorf("expected the stored password to be kept, got %d", status)
		}
	})
	t.Run("a-submission-without-a-password-keeps-the-password", func(t *testing.T) {
		definition := "slug: clients\ntitle: Clients of the fork\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: \"\"\n"
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", definition, map[string]string{"If-Match": `"2"`}, http.StatusOK)
		if status := publicStatus(t, basicAuthorization("client", "page-secret")); status != http.StatusOK {
			t.Errorf("expected the stored password to be kept, got %d", status)
		}
	})
	t.Run("a-new-password-replaces-the-stored-one", func(t *testing.T) {
		definition := "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: another-secret\n"
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", definition, map[string]string{"If-Match": `"3"`}, http.StatusOK)
		if status := publicStatus(t, basicAuthorization("client", "another-secret")); status != http.StatusOK {
			t.Errorf("expected the new password to open the page, got %d", status)
		}
		if status := publicStatus(t, basicAuthorization("client", "page-secret")); status != http.StatusUnauthorized {
			t.Errorf("expected the old password to stop working, got %d", status)
		}
	})
	t.Run("a-password-that-does-not-fit-is-refused", func(t *testing.T) {
		short := "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: short\n"
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", short, map[string]string{"If-Match": `"4"`}, http.StatusBadRequest)
		long := "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: client\n  password: " + strings.Repeat("a", 73) + "\n"
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", long, map[string]string{"If-Match": `"4"`}, http.StatusBadRequest)
		withoutUsername := "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\nauth:\n  username: \"\"\n  password: page-secret\n"
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", withoutUsername, map[string]string{"If-Match": `"4"`}, http.StatusBadRequest)
		if status := publicStatus(t, basicAuthorization("client", "another-secret")); status != http.StatusOK {
			t.Errorf("expected the page to be untouched by the refused writes, got %d", status)
		}
	})
	t.Run("a-new-page-with-a-login-needs-a-password", func(t *testing.T) {
		withoutPassword := "slug: partners\ntitle: Partners\ngroups: [core]\nenabled: true\nauth:\n  username: partner\n"
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", withoutPassword, nil, http.StatusBadRequest)
		masked := "slug: partners\ntitle: Partners\ngroups: [core]\nenabled: true\nauth:\n  username: partner\n  password-bcrypt-base64: '********'\n"
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", masked, nil, http.StatusBadRequest)
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/partners", "", nil, http.StatusNotFound)
	})
	t.Run("removing-the-credential-opens-the-page", func(t *testing.T) {
		definition := "slug: clients\ntitle: Clients\ngroups: [core]\nenabled: true\n"
		_, body := env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/clients", definition, map[string]string{"If-Match": `"4"`}, http.StatusOK)
		if body["requiresLogin"] != false {
			t.Errorf("expected the page to stop requiring a login, got %v", body["requiresLogin"])
		}
		if status := publicStatus(t, ""); status != http.StatusOK {
			t.Errorf("expected the page to be public again, got %d", status)
		}
	})
}
