// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"fmt"
	"strings"
	"testing"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

func TestSelect(t *testing.T) {
	refs := []EndpointRef{
		{Key: "core_web", Name: "web", Group: "core"},
		{Key: "core_api", Name: "API", Group: "core"},
		{Key: "database_postgres", Name: "postgres", Group: "database"},
		{Key: "zeta_worker", Name: "worker", Group: "zeta"},
		{Key: "alpha_cron", Name: "cron", Group: "Alpha"},
		{Key: "_standalone", Name: "standalone", Group: ""},
		{Key: "internal_secret", Name: "secret", Group: "internal"},
		{Key: "spaced_one", Name: "one", Group: " spaced "},
	}
	page := &pageconfig.Page{
		Slug:      "infra",
		Title:     "Infra",
		Groups:    []string{"database", "core", "spaced"},
		Endpoints: []string{"zeta_worker", "_standalone", "alpha_cron", "core_api"},
	}
	selection := Select(page, refs, pageconfig.DefaultMaximumEndpointsPerPage)
	var actual []string
	for _, section := range selection.Sections {
		var names []string
		for _, ref := range section.Endpoints {
			names = append(names, ref.Name)
		}
		actual = append(actual, section.Group+"="+strings.Join(names, ","))
	}
	expected := []string{"database=postgres", "core=API,web", "spaced=one", "Alpha=cron", "zeta=worker", "=standalone"}
	if strings.Join(actual, " | ") != strings.Join(expected, " | ") {
		t.Errorf("expected sections %q, got %q", expected, actual)
	}
	if selection.Truncated {
		t.Error("expected the selection not to be truncated")
	}
	if keys := selection.Keys(); len(keys) != 7 || keys[0] != "database_postgres" || keys[6] != "_standalone" {
		t.Errorf("expected the keys in display order, got %v", keys)
	}
}

func TestSelect_Truncated(t *testing.T) {
	var refs []EndpointRef
	for i := 0; i < pageconfig.DefaultMaximumEndpointsPerPage+5; i++ {
		refs = append(refs, EndpointRef{Key: fmt.Sprintf("core_ep-%03d", i), Name: fmt.Sprintf("ep-%03d", i), Group: "core"})
	}
	refs = append(refs, EndpointRef{Key: "database_postgres", Name: "postgres", Group: "database"})
	selection := Select(&pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core", "database"}}, refs, pageconfig.DefaultMaximumEndpointsPerPage)
	if !selection.Truncated || len(selection.Sections) != 1 || len(selection.Sections[0].Endpoints) != pageconfig.DefaultMaximumEndpointsPerPage {
		t.Errorf("expected only the first %d endpoints of core, got %d sections (truncated=%v)", pageconfig.DefaultMaximumEndpointsPerPage, len(selection.Sections), selection.Truncated)
	}
	if last := selection.Sections[0].Endpoints[pageconfig.DefaultMaximumEndpointsPerPage-1]; last.Name != "ep-399" {
		t.Errorf("expected the endpoints to be truncated in display order, got last=%s", last.Name)
	}
}

func TestSelect_NothingSelected(t *testing.T) {
	selection := Select(&pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"missing"}}, []EndpointRef{{Key: "core_api", Name: "api", Group: "core"}}, pageconfig.DefaultMaximumEndpointsPerPage)
	if len(selection.Sections) != 0 || selection.Truncated {
		t.Errorf("expected an empty selection, got %+v", selection)
	}
}
