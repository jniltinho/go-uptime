package suite

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

func TestSuite_ExecuteSendsUserAgentPreset(t *testing.T) {
	t.Parallel()
	var (
		mutex     sync.Mutex
		userAgent string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		userAgent = r.UserAgent()
		mutex.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	s := &Suite{
		Name: "flow",
		Endpoints: []*endpoint.Endpoint{{
			Name:       "step",
			URL:        server.URL,
			Conditions: []endpoint.Condition{"[STATUS] == 200"},
			UserAgent:  endpoint.UserAgentEdge,
		}},
	}
	if err := s.ValidateAndSetDefaults(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result := s.Execute(); !result.Success {
		t.Fatalf("expected the suite to succeed, got errors %v", result.Errors)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if !strings.Contains(userAgent, " Edg/") {
		t.Errorf("expected the Edge agent, got %q", userAgent)
	}
}

func TestSuite_ValidateAndSetDefaultsWithInvalidUserAgent(t *testing.T) {
	t.Parallel()
	s := &Suite{
		Name: "flow",
		Endpoints: []*endpoint.Endpoint{{
			Name:       "step",
			URL:        "https://example.org",
			Conditions: []endpoint.Condition{"[STATUS] == 200"},
			UserAgent:  "opera",
		}},
	}
	err := s.ValidateAndSetDefaults()
	if !errors.Is(err, endpoint.ErrEndpointWithInvalidUserAgent) {
		t.Fatalf("expected ErrEndpointWithInvalidUserAgent, got %v", err)
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("expected the error to name the step, got %q", err.Error())
	}
}
