package statuspage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func certificateDays(days int) *int {
	return &days
}

func TestCertificateExpiresInDays(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	checkedAt := now.Add(-10 * time.Minute)
	scenarios := []struct {
		name     string
		results  []common.ResultSummary
		expected *int
	}{
		{name: "without-results"},
		{name: "without-certificate", results: []common.ResultSummary{{Timestamp: checkedAt, Success: true}}},
		{name: "73-days", results: []common.ResultSummary{{Timestamp: checkedAt, CertificateExpiration: 73*24*time.Hour + 70*time.Minute}}, expected: certificateDays(73)},
		{name: "expires-later-today", results: []common.ResultSummary{{Timestamp: checkedAt, CertificateExpiration: 5 * time.Hour}}, expected: certificateDays(0)},
		{name: "expired-36-hours-ago", results: []common.ResultSummary{{Timestamp: checkedAt, CertificateExpiration: -36*time.Hour + 10*time.Minute}}, expected: certificateDays(-2)},
		{
			name: "push-after-the-check",
			results: []common.ResultSummary{
				{Timestamp: checkedAt.Add(-time.Hour), Success: true, CertificateExpiration: 11 * 24 * time.Hour},
				{Timestamp: checkedAt, Success: false},
			},
			expected: certificateDays(10),
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			actual, expiresAt := certificateExpiration(scenario.results, now)
			if (actual == nil) != (scenario.expected == nil) || (actual != nil && *actual != *scenario.expected) {
				t.Errorf("expected %v, got %v", describeDays(scenario.expected), describeDays(actual))
			}
			// Fork: the instant of the expiration comes from the same result as the number of days
			if (expiresAt == nil) != (actual == nil) {
				t.Fatalf("the days and the instant of the expiration must be set together, got %v and %v", describeDays(actual), expiresAt)
			}
			if expiresAt == nil {
				return
			}
			if expiresAt.Location() != time.UTC {
				t.Errorf("the instant of the expiration must be in UTC, got %s", expiresAt.Location())
			}
			var expected time.Time
			for i := len(scenario.results) - 1; i >= 0; i-- {
				if scenario.results[i].CertificateExpiration != 0 {
					expected = scenario.results[i].Timestamp.Add(scenario.results[i].CertificateExpiration).UTC()
					break
				}
			}
			if !expiresAt.Equal(expected) {
				t.Errorf("expected the expiration at %s, got %s", expected, expiresAt)
			}
		})
	}
}

func describeDays(days *int) any {
	if days == nil {
		return "nil"
	}
	return *days
}

func TestBuildPayload_CertificateExpiration(t *testing.T) {
	now := time.Now()
	selection := Selection{
		Featured: []EndpointRef{{Key: "core_site", Name: "site", Group: "core"}},
		Sections: []Section{{Group: "core", Endpoints: []EndpointRef{{Key: "core_api", Name: "api", Group: "core"}, {Key: "core_tcp", Name: "tcp", Group: "core"}}}},
	}
	summaries := map[string]*common.EndpointSummary{
		"core_site": {Results: []common.ResultSummary{{Timestamp: now, Success: true, CertificateExpiration: 73*24*time.Hour + time.Hour}}},
		"core_api":  {Results: []common.ResultSummary{{Timestamp: now, Success: true, CertificateExpiration: 10*24*time.Hour + time.Hour}}},
		"core_tcp":  {Results: []common.ResultSummary{{Timestamp: now, Success: true}}},
	}

	// Without the option of the page, nothing about the certificate is published
	hiddenPage := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}}
	hidden, err := json.Marshal(BuildPayload(hiddenPage, selection, summaries, now))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hidden), "certificate") {
		t.Errorf("expected no certificate expiration without the option of the page, got %s", hidden)
	}
	hiddenDetails, err := json.Marshal(BuildEndpointDetailsPayload(hiddenPage, selection.Featured[0], summaries["core_site"], nil, now))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hiddenDetails), "certificate") {
		t.Errorf("expected no certificate expiration on the details page without the option, got %s", hiddenDetails)
	}

	// With the option, the days are published for the featured endpoints, the groups and the details page
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, ShowCertificateExpiration: true}
	payload := BuildPayload(page, selection, summaries, now)
	if days := payload.Featured[0].CertificateExpiresInDays; days == nil || *days != 73 {
		t.Errorf("expected 73 days for the featured endpoint, got %v", describeDays(days))
	}
	if days := payload.Groups[0].Endpoints[0].CertificateExpiresInDays; days == nil || *days != 10 {
		t.Errorf("expected 10 days for the endpoint of the group, got %v", describeDays(days))
	}
	if days := payload.Groups[0].Endpoints[1].CertificateExpiresInDays; days != nil {
		t.Errorf("expected no days for the endpoint without certificate, got %v", *days)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decoded allowedPayload
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("expected only allowed fields with the certificate expiration, got %v in %s", err, body)
	}
	if strings.Contains(string(body), "certificateExpiration\"") {
		t.Errorf("expected only the number of days and the instant of the expiration to be published, got %s", body)
	}
	// Fork: the instant of the expiration is published next to the days, from the same result
	expectedExpiration := now.Add(73*24*time.Hour + time.Hour).UTC()
	if at := payload.Featured[0].CertificateExpiresAt; at == nil || !at.Equal(expectedExpiration) {
		t.Errorf("expected the featured endpoint to expire at %s, got %v", expectedExpiration, at)
	}
	if at := payload.Groups[0].Endpoints[1].CertificateExpiresAt; at != nil {
		t.Errorf("expected no expiration for the endpoint without certificate, got %s", at)
	}
	details := BuildEndpointDetailsPayload(page, selection.Featured[0], summaries["core_site"], nil, now)
	if details.CertificateExpiresInDays == nil || *details.CertificateExpiresInDays != 73 {
		t.Errorf("expected 73 days on the details page, got %v", describeDays(details.CertificateExpiresInDays))
	}
	if at := details.CertificateExpiresAt; at == nil || !at.Equal(expectedExpiration) {
		t.Errorf("expected the details page to expire at %s, got %v", expectedExpiration, at)
	}
}
