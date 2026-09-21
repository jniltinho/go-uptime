package statuspage

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// Fork: the reason of a failed check on a page that shows messages

// detailsMessageOf returns the message published for a single result, and the body of the payload
func detailsMessageOf(t *testing.T, result common.ResultSummary) (string, string) {
	t.Helper()
	now := time.Now()
	result.Timestamp = now.Add(-time.Minute)
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", ShowMessages: true}
	ref := EndpointRef{Key: "core_api", Name: "api", Group: "core"}
	payload := BuildEndpointDetailsPayload(page, ref, &common.EndpointSummary{Results: []common.ResultSummary{result}}, nil, now)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return payload.Results[0].Message, string(body)
}

func TestPublicMessage_FailureReason(t *testing.T) {
	scenarios := []struct {
		name     string
		result   common.ResultSummary
		expected string
		// forbidden are texts of the error that must never reach the payload
		forbidden []string
	}{
		{
			name:      "certificate of another host",
			result:    common.ResultSummary{Errors: []string{`Get "https://tag.example.org": tls: failed to verify certificate: x509: certificate is valid for *.example.com, not tag.example.org`}},
			expected:  ReasonCertificateError,
			forbidden: []string{"x509", "*.example.com", "tag.example.org"},
		},
		{
			name:      "certificate that expired",
			result:    common.ResultSummary{Errors: []string{`x509: certificate has expired or is not yet valid`}},
			expected:  ReasonCertificateError,
			forbidden: []string{"x509"},
		},
		{
			name:      "name that does not resolve",
			result:    common.ResultSummary{Errors: []string{`Get "https://sso.example.org": dial tcp: lookup sso.example.org on 127.0.0.11:53: no such host`}},
			expected:  ReasonDNSError,
			forbidden: []string{"127.0.0.11", "lookup", "no such host"},
		},
		{
			name:      "time that ran out",
			result:    common.ResultSummary{Errors: []string{`Get "https://slow.example.org": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`}},
			expected:  ReasonTimeout,
			forbidden: []string{"deadline", "slow.example.org"},
		},
		{
			name:      "connection refused",
			result:    common.ResultSummary{Errors: []string{`Get "https://api.example.org/certificate-status": dial tcp 10.0.0.5:443: connect: connection refused`}},
			expected:  ReasonConnectionFailed,
			forbidden: []string{"10.0.0.5", "dial tcp", "certificate-status"},
		},
		{
			name:      "connection closed by the other side",
			result:    common.ResultSummary{Errors: []string{"unexpected EOF"}},
			expected:  ReasonConnectionFailed,
			forbidden: []string{"EOF"},
		},
		{
			name:      "error that is not recognized",
			result:    common.ResultSummary{Errors: []string{"invalid condition: [BODY].name is not a string"}},
			expected:  ReasonCheckFailed,
			forbidden: []string{"invalid condition", "BODY"},
		},
		{
			name:     "failure without any error and without connection",
			result:   common.ResultSummary{},
			expected: ReasonConnectionFailed,
		},
		{
			name:     "condition that failed with the service answering",
			result:   common.ResultSummary{Connected: true},
			expected: ReasonCheckFailed,
		},
		{
			name:      "ssh banner, whose status is not from HTTP",
			result:    common.ResultSummary{HTTPStatus: 1, Errors: []string{"dial tcp 10.0.0.9:22: connect: connection refused"}},
			expected:  ReasonConnectionFailed,
			forbidden: []string{"HTTP 1", "10.0.0.9"},
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			message, body := detailsMessageOf(t, scenario.result)
			if message != scenario.expected {
				t.Errorf("expected %q, got %q", scenario.expected, message)
			}
			for _, forbidden := range scenario.forbidden {
				if strings.Contains(body, forbidden) {
					t.Errorf("expected %q not to be published, got %s", forbidden, body)
				}
			}
		})
	}
}

// TestPublicMessage_FailureReasonIsNotChosenByTheAddress makes sure the markers do not match the URL of the endpoint:
// the category would then be chosen by the address the page hides
func TestPublicMessage_FailureReasonIsNotChosenByTheAddress(t *testing.T) {
	addresses := []string{
		`Get "https://dns.corp.example.org/health": dial tcp 10.0.0.5:443: connect: connection refused`,
		`Get "https://timeout.example.org/health": dial tcp 10.0.0.5:443: connect: connection refused`,
		`Get "https://api.example.org/certificate-renewal": dial tcp 10.0.0.5:443: connect: connection refused`,
	}
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			if message, _ := detailsMessageOf(t, common.ResultSummary{Errors: []string{address}}); message != ReasonConnectionFailed {
				t.Errorf("expected %q, got %q", ReasonConnectionFailed, message)
			}
		})
	}
}

// TestPublicMessage_FailureReasonScansByCategory makes sure the order is the order of the categories, and not the order
// of the errors of the result
func TestPublicMessage_FailureReasonScansByCategory(t *testing.T) {
	result := common.ResultSummary{Errors: []string{
		"invalid condition: [BODY].timeout is not a number",
		`Get "https://api.example.org": tls: failed to verify certificate: x509: certificate signed by unknown authority`,
	}}
	if message, _ := detailsMessageOf(t, result); message != ReasonCertificateError {
		t.Errorf("expected the category of the certificate, got %q", message)
	}
}

func TestPublicMessage_WithoutReason(t *testing.T) {
	scenarios := []struct {
		name     string
		result   common.ResultSummary
		expected string
	}{
		{name: "successful result without HTTP status", result: common.ResultSummary{Success: true, Connected: true}, expected: ""},
		{name: "successful result with HTTP status", result: common.ResultSummary{Success: true, HTTPStatus: 200}, expected: "HTTP 200"},
		{name: "failure with HTTP status", result: common.ResultSummary{HTTPStatus: 500}, expected: "HTTP 500"},
		{name: "pending result without message", result: common.ResultSummary{Pending: true, Origin: "push"}, expected: ""},
		{name: "pending result of a check", result: common.ResultSummary{Pending: true}, expected: ""},
		{name: "push with a message", result: common.ResultSummary{Message: "Disco cheio", Origin: "push"}, expected: "Disco cheio"},
		{
			name:     "result of the external API, whose error is the text of the operator",
			result:   common.ResultSummary{Origin: "push", Errors: []string{"timeout na fila de pagamento"}},
			expected: "",
		},
		{
			name:     "heartbeat of a result stored before results had a message",
			result:   common.ResultSummary{Errors: []string{"heartbeat: no update received within 1m0s"}},
			expected: "heartbeat: no update received within 1m0s",
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			message, body := detailsMessageOf(t, scenario.result)
			if message != scenario.expected {
				t.Errorf("expected %q, got %q", scenario.expected, message)
			}
			if scenario.name == "result of the external API, whose error is the text of the operator" && strings.Contains(body, "fila de pagamento") {
				t.Errorf("expected the text of the operator not to be published, got %s", body)
			}
		})
	}
}

// TestPublicMessage_PageWithoutMessages makes sure a page without show-messages never gets a reason
func TestPublicMessage_PageWithoutMessages(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra"}
	ref := EndpointRef{Key: "core_api", Name: "api", Group: "core"}
	summary := &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now.Add(-time.Minute), Errors: []string{"dial tcp 10.0.0.5:443: connect: connection refused"}}}}
	body, err := json.Marshal(BuildEndpointDetailsPayload(page, ref, summary, nil, now))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"message"`) || strings.Contains(string(body), ReasonConnectionFailed) {
		t.Errorf("expected no message without show-messages, got %s", body)
	}
}

// TestBuildPayload_Summary covers the count of the endpoints of the page shown in the status banner (fork)
func TestBuildPayload_Summary(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}, Featured: []string{"core_site"}}
	selection := Selection{
		Featured: []EndpointRef{{Key: "core_site", Name: "site", Group: "core"}},
		Sections: []Section{{Group: "core", Endpoints: []EndpointRef{
			{Key: "core_api", Name: "api"},
			{Key: "core_db", Name: "db"},
			{Key: "core_job", Name: "job"},
			{Key: "core_new", Name: "new"},
		}}},
	}
	up := &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: true}}}
	summaries := map[string]*common.EndpointSummary{
		"core_site": up,
		"core_api":  up,
		"core_db":   {Results: []common.ResultSummary{{Timestamp: now, Success: false}}},
		"core_job":  {Results: []common.ResultSummary{{Timestamp: now, Pending: true}}},
		// core_new has no result yet: it counts as unknown
	}
	payload := BuildPayload(page, selection, summaries, now)
	expected := SummaryPayload{Total: 5, Up: 2, Down: 1, Pending: 1, Unknown: 1}
	if payload.Summary != expected {
		t.Errorf("expected %+v, got %+v", expected, payload.Summary)
	}
	// The details payload of an endpoint has no summary: it describes an endpoint, not the page
	body, err := json.Marshal(BuildEndpointDetailsPayload(page, EndpointRef{Key: "core_api", Name: "api", Group: "core"}, up, nil, now))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"summary"`) {
		t.Errorf("expected no summary on the details payload, got %s", body)
	}
}

// TestBuildPayload_SummaryOfATruncatedPage counts the endpoints that are published, which are the ones the page shows
func TestBuildPayload_SummaryOfATruncatedPage(t *testing.T) {
	now := time.Now()
	page := &pageconfig.Page{Slug: "infra", Title: "Infra", Groups: []string{"core"}}
	refs := make([]EndpointRef, 0, pageconfig.DefaultMaximumEndpointsPerPage)
	summaries := make(map[string]*common.EndpointSummary, pageconfig.DefaultMaximumEndpointsPerPage)
	for i := 0; i < pageconfig.DefaultMaximumEndpointsPerPage; i++ {
		key := "core_" + strconv.Itoa(i)
		refs = append(refs, EndpointRef{Key: key, Name: key})
		summaries[key] = &common.EndpointSummary{Results: []common.ResultSummary{{Timestamp: now, Success: true}}}
	}
	payload := BuildPayload(page, Selection{Sections: []Section{{Group: "core", Endpoints: refs}}, Truncated: true}, summaries, now)
	if !payload.Truncated || payload.Summary.Total != pageconfig.DefaultMaximumEndpointsPerPage || payload.Summary.Up != pageconfig.DefaultMaximumEndpointsPerPage {
		t.Errorf("expected the count of the published endpoints, got %+v", payload.Summary)
	}
}
