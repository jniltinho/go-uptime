// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseViewsPage(t *testing.T, views string) (*Page, error) {
	t.Helper()
	var page Page
	if err := yaml.Unmarshal([]byte("slug: acme\ntitle: Acme\ngroups: [Client A]\n"+views), &page); err != nil {
		return nil, err
	}
	return &page, page.ValidateAndSetDefaults()
}

func TestViews_Defaults(t *testing.T) {
	t.Parallel()
	for name, views := range map[string]string{"omitted": "", "null": "views: null\n", "empty": "views: {}\n"} {
		page, err := parseViewsPage(t, views)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := page.Views.EffectiveEnabled(); !reflect.DeepEqual(got, []string{ViewList}) {
			t.Errorf("%s: expected only the list, got %v", name, got)
		}
		if page.Views.EffectiveDefault() != ViewList || page.Views.EffectiveGroupLabel() != DefaultGroupLabel {
			t.Errorf("%s: expected the defaults, got %q and %q", name, page.Views.EffectiveDefault(), page.Views.EffectiveGroupLabel())
		}
	}
	page, err := parseViewsPage(t, "views:\n  enabled: [matrix, list]\n")
	if err != nil {
		t.Fatal(err)
	}
	if page.Views.EffectiveDefault() != ViewMatrix {
		t.Errorf("expected the first enabled view to be the default, got %q", page.Views.EffectiveDefault())
	}
}

func TestViews_DefaultsAreNotWrittenBack(t *testing.T) {
	t.Parallel()
	page, err := parseViewsPage(t, "views:\n  enabled: [grouped]\n  environments: [{id: prod}]\n  endpoints:\n    ' Client-A_Billing ': {environment: prod}\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(page.Views)
	if err != nil {
		t.Fatal(err)
	}
	expected := "enabled:\n    - grouped\nenvironments:\n    - id: prod\nendpoints:\n    client-a_billing:\n        environment: prod\n"
	if string(out) != expected {
		t.Errorf("expected the block as written, normalized, got:\n%s", out)
	}
	if page.Views.Environments[0].ShortName() != "PROD" || page.Views.Environments[0].DisplayName() != "prod" {
		t.Errorf("expected the default names of the environment, got %q and %q", page.Views.Environments[0].ShortName(), page.Views.Environments[0].DisplayName())
	}
	long := &Environment{ID: "pre-production-eu"}
	if long.ShortName() != "PRE-PROD" {
		t.Errorf("expected the short name cut to 8 characters, got %q", long.ShortName())
	}
}

func TestViews_Valid(t *testing.T) {
	t.Parallel()
	page, err := parseViewsPage(t, `views:
  enabled: [list, grouped, matrix]
  default: grouped
  group-label: Client
  environments:
    - {id: prod, name: Production, short: PROD}
    - {id: stg, name: Staging}
  applications: [Billing, API]
  groups:
    Client A: {description: Acme customer, tags: [BR, OCI]}
  endpoints:
    client-a_billing: {application: Billing, environment: prod}
    client-a_reports: {application: Reports}
`)
	if err != nil {
		t.Fatal(err)
	}
	if c := page.Views.Classification("client-a_billing"); c == nil || c.Application != "Billing" || c.Environment != "prod" {
		t.Errorf("unexpected classification %+v", c)
	}
	if d := page.Views.GroupDetailsOf("Client A"); d == nil || !reflect.DeepEqual(d.Tags, []string{"BR", "OCI"}) {
		t.Errorf("unexpected group details %+v", d)
	}
}

func TestViews_Invalid(t *testing.T) {
	t.Parallel()
	scenarios := map[string]string{
		"empty enabled":           "views:\n  enabled: []\n",
		"unknown view":            "views:\n  enabled: [cards]\n",
		"duplicate view":          "views:\n  enabled: [list, list]\n",
		"default not enabled":     "views:\n  enabled: [list]\n  default: matrix\n",
		"long group label":        "views:\n  group-label: " + strings.Repeat("a", 31) + "\n",
		"bad environment id":      "views:\n  environments: [{id: Prod}]\n",
		"duplicate environment":   "views:\n  environments: [{id: prod}, {id: prod}]\n",
		"long short name":         "views:\n  environments: [{id: prod, short: PRODUCTION}]\n",
		"too many environments":   "views:\n  environments: [{id: a},{id: b},{id: c},{id: d},{id: e},{id: f},{id: g},{id: h},{id: i},{id: j},{id: k}]\n",
		"duplicate application":   "views:\n  applications: [API, API]\n",
		"undeclared environment":  "views:\n  endpoints:\n    client-a_billing: {environment: uat}\n",
		"empty classification":    "views:\n  endpoints:\n    client-a_billing: {}\n",
		"null classification":     "views:\n  endpoints:\n    client-a_billing: null\n",
		"colliding keys":          "views:\n  endpoints:\n    ' Group_Billing': {application: A}\n    group_billing: {application: B}\n",
		"too many tags":           "views:\n  groups:\n    Client A: {tags: [a, b, c, d, e, f]}\n",
		"long description":        "views:\n  groups:\n    Client A: {description: " + strings.Repeat("a", 101) + "}\n",
		"empty group details":     "views:\n  groups:\n    Client A: {}\n",
		"long application":        "views:\n  endpoints:\n    client-a_billing: {application: " + strings.Repeat("a", 41) + "}\n",
		"duplicate group details": "views:\n  groups:\n    ' Client A': {description: a}\n    Client A: {description: b}\n",
	}
	for name, views := range scenarios {
		_, err := parseViewsPage(t, views)
		if !errors.Is(err, ErrInvalidViews) {
			t.Errorf("%s: expected ErrInvalidViews, got %v", name, err)
		}
	}
	_, err := parseViewsPage(t, "views:\n  endpoints:\n    client-a_billing: {environment: uat}\n")
	if err == nil || !strings.Contains(err.Error(), "client-a_billing") || !strings.Contains(err.Error(), "uat") {
		t.Errorf("expected the error to name the key and the environment, got %v", err)
	}
}

func TestViews_ApplicationNotListedIsValid(t *testing.T) {
	t.Parallel()
	if _, err := parseViewsPage(t, "views:\n  applications: [API]\n  endpoints:\n    client-a_billing: {application: Billing}\n"); err != nil {
		t.Errorf("expected an application that is not listed to be valid, got %v", err)
	}
}

func TestViews_UnknownFieldInTheConfigurationFile(t *testing.T) {
	t.Parallel()
	var config Config
	err := yaml.Unmarshal([]byte("pages:\n  - slug: a\n    title: A\n    groups: [g]\n    views:\n      environments: [{id: prod, colour: red}]\n"), &config)
	if !errors.Is(err, ErrInvalidViews) {
		t.Errorf("expected an unknown field of the views to be refused, got %v", err)
	}
}
