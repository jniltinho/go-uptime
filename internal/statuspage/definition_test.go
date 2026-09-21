// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"errors"
	"strings"
	"testing"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

func TestParse(t *testing.T) {
	page, err := Parse([]byte("slug: infra\ntitle: ' Infra '\ngroups: [core]\nendpoints: [Core_API]\nenabled: true\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Slug != "infra" || page.Title != "Infra" || page.Endpoints[0] != "core_api" || page.Enabled == nil || !*page.Enabled {
		t.Errorf("expected a normalized page, got %+v", page)
	}
	if page, err := Parse([]byte(`{"slug": "apps", "title": "Apps", "groups": ["apps"]}`)); err != nil || page.Slug != "apps" || page.Enabled != nil {
		t.Errorf("expected JSON to be accepted without enabled, got page=%+v err=%v", page, err)
	}
}

func TestParseErrors(t *testing.T) {
	scenarios := []struct {
		name        string
		definition  string
		expectedErr error
	}{
		{name: "empty", definition: "  \n", expectedErr: ErrEmptyDefinition},
		{name: "comment-only", definition: "# nothing\n", expectedErr: ErrEmptyDefinition},
		{name: "unknown-field", definition: "slug: infra\ntitle: Infra\ngroups: [core]\nindexable: true\n", expectedErr: ErrInvalidDefinition},
		{name: "multiple-documents", definition: "slug: infra\ntitle: Infra\ngroups: [core]\n---\nslug: apps\n", expectedErr: ErrInvalidDefinition},
		{name: "wrong-type", definition: "slug: infra\ntitle: Infra\ngroups: core\n", expectedErr: ErrInvalidDefinition},
		{name: "invalid-slug", definition: "slug: Infra\ntitle: Infra\ngroups: [core]\n", expectedErr: pageconfig.ErrInvalidSlug},
		{name: "empty-selection", definition: "slug: infra\ntitle: Infra\n", expectedErr: pageconfig.ErrEmptySelection},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := Parse([]byte(scenario.definition)); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected error %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}

func TestParse_ShowCertificateExpiration(t *testing.T) {
	if page, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\nshow-certificate-expiration: true\n")); err != nil || !page.ShowCertificateExpiration {
		t.Errorf("expected show-certificate-expiration to be accepted in YAML, got page=%+v err=%v", page, err)
	}
	if page, err := Parse([]byte(`{"slug": "apps", "title": "Apps", "groups": ["apps"], "show-certificate-expiration": true}`)); err != nil || !page.ShowCertificateExpiration {
		t.Errorf("expected show-certificate-expiration to be accepted in JSON, got page=%+v err=%v", page, err)
	}
	if page, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\n")); err != nil || page.ShowCertificateExpiration {
		t.Errorf("expected the certificate expiration to be hidden by default, got page=%+v err=%v", page, err)
	}
}

// TestParse_GroupsCollapsed accepts groups-collapsed in YAML and in JSON, defaults it to false, refuses a value that is
// not a boolean and keeps it through the normalization of a managed page, which is what is stored and backed up.
func TestParse_GroupsCollapsed(t *testing.T) {
	if page, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\ngroups-collapsed: true\n")); err != nil || !page.GroupsCollapsed {
		t.Errorf("expected groups-collapsed to be accepted in YAML, got page=%+v err=%v", page, err)
	}
	if page, err := Parse([]byte(`{"slug": "apps", "title": "Apps", "groups": ["apps"], "groups-collapsed": true}`)); err != nil || !page.GroupsCollapsed {
		t.Errorf("expected groups-collapsed to be accepted in JSON, got page=%+v err=%v", page, err)
	}
	if page, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\n")); err != nil || page.GroupsCollapsed {
		t.Errorf("expected the groups to start expanded by default, got page=%+v err=%v", page, err)
	}
	// "sim" is the value of the scenario of the specification. The YAML 1.1 booleans (yes, on, ...) are accepted, quoted
	// or not, like for every other boolean of a definition: that is how yaml.v3 decodes into a bool.
	if page, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\ngroups-collapsed: yes\n")); err != nil || !page.GroupsCollapsed {
		t.Errorf("expected the YAML 1.1 boolean yes to be accepted, got page=%+v err=%v", page, err)
	}
	for _, value := range []string{`"sim"`, `1`, `[true]`} {
		if _, err := Parse([]byte("slug: infra\ntitle: Infra\ngroups: [core]\ngroups-collapsed: " + value + "\n")); err == nil {
			t.Errorf("expected groups-collapsed: %s to be refused", value)
		}
	}
	page, definition, err := NormalizeDefinition([]byte("slug: infra\ntitle: Infra\ngroups: [core]\ngroups-collapsed: true\n"))
	if err != nil || !page.GroupsCollapsed {
		t.Fatalf("expected the normalization to keep the option, got page=%+v err=%v", page, err)
	}
	again, err := Parse(definition)
	if err != nil || !again.GroupsCollapsed {
		t.Errorf("expected the normalized definition to carry groups-collapsed, got %s, err=%v", definition, err)
	}
	if _, definition, _ = NormalizeDefinition([]byte("slug: infra\ntitle: Infra\ngroups: [core]\n")); strings.Contains(string(definition), "groups-collapsed") {
		t.Errorf("expected a page without the option to be stored without it, got %s", definition)
	}
}
