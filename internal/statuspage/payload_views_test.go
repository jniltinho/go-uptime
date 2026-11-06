// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const classifiedPage = `slug: acme
title: Acme
groups: [Client A]
featured: [client-b_portal]
views:
  enabled: [grouped, matrix]
  group-label: Client
  environments:
    - {id: prod, name: Production}
    - {id: qa}
  applications: [Billing]
  groups:
    Client A: {description: Acme customer, tags: [BR]}
    Client B: {description: Featured only}
  endpoints:
    client-a_billing-secret-host: {application: Billing, environment: prod}
    client-a_billing-qa: {application: Billing, environment: qa}
`

func TestBuildPayload_Views(t *testing.T) {
	page, err := Parse([]byte(classifiedPage))
	if err != nil {
		t.Fatal(err)
	}
	selection := Selection{
		Featured: []EndpointRef{{Key: "client-a_billing-qa", Name: "billing-qa", Group: "Client A"}, {Key: "client-b_portal", Name: "Portal", Group: "Client B"}},
		Sections: []Section{{Group: "Client A", Endpoints: []EndpointRef{{Key: "client-a_billing-secret-host", Name: "billing"}, {Key: "client-a_portal", Name: "Portal"}}}},
	}
	summaries := map[string]*common.EndpointSummary{"client-a_billing-secret-host": {Results: []common.ResultSummary{{Timestamp: time.Now(), Success: true}}}}
	payload := BuildPayload(page, selection, summaries, time.Now())
	expectedViews := ViewsPayload{
		Enabled:      []string{"grouped", "matrix"},
		Default:      "grouped",
		GroupLabel:   "Client",
		Environments: []EnvironmentPayload{{ID: "prod", Name: "Production", Short: "PROD"}, {ID: "qa", Name: "qa", Short: "QA"}},
		Applications: []string{"Billing"},
		Groups:       []GroupDetailsPayload{{Name: "Client B", Description: "Featured only"}},
	}
	if !reflect.DeepEqual(payload.Views, expectedViews) {
		t.Errorf("unexpected views %+v", payload.Views)
	}
	group := payload.Groups[0]
	if group.Description != "Acme customer" || !reflect.DeepEqual(group.Tags, []string{"BR"}) {
		t.Errorf("expected the details of the group, got %+v", group)
	}
	if e := group.Endpoints[0]; e.Application != "Billing" || e.Environment != "prod" {
		t.Errorf("expected the classification, got %+v", e)
	}
	if e := group.Endpoints[1]; e.Application != "Portal" || e.Environment != "" {
		t.Errorf("expected the name as application and no environment, got %+v", e)
	}
	if f := payload.Featured[0]; f.Application != "Billing" || f.Environment != "qa" {
		t.Errorf("expected the classification on a featured endpoint, got %+v", f)
	}
	details := BuildEndpointDetailsPayload(page, EndpointRef{Key: "client-a_billing-qa", Name: "billing-qa", Group: "Client A"}, nil, nil, time.Now())
	if details.Application != "Billing" || details.Environment != "qa" {
		t.Errorf("expected the classification on the details payload, got %+v", details.EndpointPayload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret-host") || strings.Contains(string(body), "client-a_") {
		t.Errorf("expected no endpoint key in the payload, got %s", body)
	}
}

func TestBuildPayload_ViewsDefaults(t *testing.T) {
	page := &pageconfig.Page{Slug: "infra", Title: "Infra"}
	payload := BuildPayload(page, Selection{}, nil, time.Now())
	body, err := json.Marshal(payload.Views)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"enabled":["list"],"default":"list","groupLabel":"Group","environments":[],"applications":[],"groups":[]}` {
		t.Errorf("unexpected default views %s", body)
	}
}

func TestParse_ViewsUnknownNestedField(t *testing.T) {
	for _, definition := range []string{
		"slug: a\ntitle: A\ngroups: [g]\nviews:\n  environments: [{id: prod, colour: red}]\n",
		`{"slug":"a","title":"A","groups":["g"],"views":{"environments":[{"id":"prod","colour":"red"}]}}`,
	} {
		if _, err := Parse([]byte(definition)); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("expected the unknown field to be refused, got %v", err)
		}
	}
}
