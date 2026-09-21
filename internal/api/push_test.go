// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	pushTestExternalToken = "keSDu7G855jvVat1xWiY2Gk4CkL1End5"
	pushTestActiveToken   = "erp-site-token"
	pushTestGlobalKey     = "akamai-global-key-123"
)

func newPushTestRouter(t *testing.T) *echo.Echo {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	cfg := &config.Config{
		Security:    &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}},
		Maintenance: &maintenance.Config{Enabled: &disabled},
		Endpoints: []*endpoint.Endpoint{
			{Name: "site", Group: "erp", URL: server.URL, Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}},
			{Name: "other", Group: "erp", URL: server.URL, Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}},
		},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{
			{Name: "backup", Group: "jobs", Token: pushTestExternalToken},
			{Name: "stopped", Group: "jobs", Token: "stopped-token", Enabled: &disabled},
		},
		Push: &pushconfig.Config{
			Keys:      []*pushconfig.Key{{Name: "akamai", Token: pushTestGlobalKey}},
			Endpoints: []*pushconfig.Endpoint{{Key: "erp_site", Token: pushTestActiveToken}},
		},
	}
	for _, ep := range cfg.Endpoints {
		if err := ep.ValidateAndSetDefaults(); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.ValidatePushConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Security.ValidateAndSetDefaults() {
		t.Fatal("invalid security configuration")
	}
	watchdog.Monitor(cfg)
	t.Cleanup(func() { watchdog.Shutdown(cfg) })
	previousLimiter := pushLimiter
	pushLimiter = statuspage.NewLimiter(pushRejectionsPerMinute, maximumPushLimiterKeys)
	t.Cleanup(func() { pushLimiter = previousLimiter })
	return New(cfg).Router()
}

func doPush(t *testing.T, router *echo.Echo, method, path string) (int, http.Header, string) {
	t.Helper()
	response, err := testHTTP(router, httptest.NewRequest(method, path, http.NoBody))
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, string(body)
}

func latestResult(t *testing.T, key string) (*endpoint.Result, int) {
	t.Helper()
	status, err := store.Get().GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 100))
	if err != nil || len(status.Results) == 0 {
		return nil, 0
	}
	return status.Results[len(status.Results)-1], len(status.Results)
}

// latestPushResult returns the most recent pushed result of the endpoint, or nil
func latestPushResult(t *testing.T, key string) *endpoint.Result {
	t.Helper()
	status, err := store.Get().GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(1, 100))
	if err != nil {
		return nil
	}
	for i := len(status.Results) - 1; i >= 0; i-- {
		if status.Results[i].Origin == endpoint.ResultOriginPush {
			return status.Results[i]
		}
	}
	return nil
}

func TestPush_CompatibleWithUptimeKuma(t *testing.T) {
	router := newPushTestRouter(t)
	scenarios := []struct {
		name             string
		method           string
		query            string
		expectedSuccess  bool
		expectedMessage  string
		expectedDuration time.Duration
	}{
		{name: "url-of-the-uptime-kuma", method: http.MethodGet, query: "?status=up&msg=OK&ping=", expectedSuccess: true, expectedMessage: "OK"},
		{name: "failure-with-message-and-ping", method: http.MethodPost, query: "?status=down&msg=Falha%20no%20backup&ping=120", expectedMessage: "Falha no backup", expectedDuration: 120 * time.Millisecond},
		{name: "status-other-than-up", method: http.MethodGet, query: "?status=warning", expectedMessage: "OK"},
		{name: "without-query", method: http.MethodGet, expectedSuccess: true, expectedMessage: "OK"},
		{name: "ping-with-numeric-prefix", method: http.MethodPut, query: "?ping=12.5ms", expectedSuccess: true, expectedMessage: "OK", expectedDuration: 12500 * time.Microsecond},
		{name: "ping-zero-is-ignored", method: http.MethodGet, query: "?ping=0", expectedSuccess: true, expectedMessage: "OK"},
		{name: "empty-status-and-message", method: http.MethodGet, query: "?status=&msg=", expectedSuccess: true, expectedMessage: "OK"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			status, header, body := doPush(t, router, scenario.method, "/api/push/"+pushTestExternalToken+scenario.query)
			if status != http.StatusOK || body != `{"ok":true}` || header.Get("Cache-Control") != "no-store" {
				t.Fatalf("expected 200 with {\"ok\":true} and no-store, got %d %s (Cache-Control=%q)", status, body, header.Get("Cache-Control"))
			}
			result, _ := latestResult(t, "jobs_backup")
			if result == nil || result.Success != scenario.expectedSuccess || result.Message != scenario.expectedMessage || result.Duration != scenario.expectedDuration || result.Origin != endpoint.ResultOriginPush {
				t.Fatalf("expected success=%v message=%q duration=%s origin=push, got %+v", scenario.expectedSuccess, scenario.expectedMessage, scenario.expectedDuration, result)
			}
			if !result.Success && (len(result.Errors) != 1 || result.Errors[0] != scenario.expectedMessage) {
				t.Errorf("expected the message in the errors of a failure, got %v", result.Errors)
			}
		})
	}
	t.Run("head", func(t *testing.T) {
		if status, _, _ := doPush(t, router, http.MethodHead, "/api/push/"+pushTestExternalToken); status != http.StatusOK {
			t.Errorf("expected 200 for HEAD, got %d", status)
		}
	})
}

func TestPush_Rejections(t *testing.T) {
	router := newPushTestRouter(t)
	_, before := latestResult(t, "jobs_backup")
	scenarios := []struct {
		name            string
		method          string
		path            string
		expectedMessage string
	}{
		{name: "unknown-token", method: http.MethodGet, path: "/api/push/unknown-token?status=up", expectedMessage: pushNotFoundMessage},
		{name: "negative-ping", method: http.MethodGet, path: "/api/push/" + pushTestExternalToken + "?ping=-5", expectedMessage: errInvalidPushPing.Error()},
		{name: "ping-above-the-maximum", method: http.MethodGet, path: "/api/push/" + pushTestExternalToken + "?ping=100000000001", expectedMessage: errInvalidPushPing.Error()},
		{name: "disabled-external-endpoint", method: http.MethodGet, path: "/api/push/stopped-token", expectedMessage: pushNotFoundMessage},
		{name: "token-of-another-endpoint", method: http.MethodGet, path: "/api/push/" + pushTestExternalToken + "/erp_site", expectedMessage: pushNotFoundMessage},
		{name: "global-key-on-an-active-endpoint-without-push", method: http.MethodGet, path: "/api/push/" + pushTestGlobalKey + "/erp_other", expectedMessage: pushNotFoundMessage},
		{name: "without-token", method: http.MethodGet, path: "/api/push", expectedMessage: pushNotFoundMessage},
		{name: "too-many-segments", method: http.MethodPut, path: "/api/push/a/b/c", expectedMessage: pushNotFoundMessage},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			status, header, body := doPush(t, router, scenario.method, scenario.path)
			expectedBody := `{"ok":false,"msg":"` + scenario.expectedMessage + `"}`
			if status != http.StatusNotFound || body != expectedBody {
				t.Fatalf("expected 404 with %s, got %d %s", expectedBody, status, body)
			}
			if len(header.Get("WWW-Authenticate")) > 0 {
				t.Errorf("expected no authentication challenge, got %q", header.Get("WWW-Authenticate"))
			}
		})
	}
	if _, after := latestResult(t, "jobs_backup"); after != before {
		t.Errorf("expected no result from the rejected pushes, got %d new results", after-before)
	}
}

func TestPush_ActiveEndpoints(t *testing.T) {
	router := newPushTestRouter(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, count := latestResult(t, "erp_site"); count > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	status, _, body := doPush(t, router, http.MethodGet, "/api/push/"+pushTestGlobalKey+"/ERP_site?status=down&msg=Latencia%20alta")
	if status != http.StatusOK || body != `{"ok":true}` {
		t.Fatalf("expected the global key to be accepted for an active endpoint with push, got %d %s", status, body)
	}
	result, count := latestResult(t, "erp_site")
	if result == nil || result.Success || result.Message != "Latencia alta" || result.Origin != endpoint.ResultOriginPush || count < 2 {
		t.Fatalf("expected the push in the history of the checks, got %+v (%d results)", result, count)
	}
	if status, _, _ := doPush(t, router, http.MethodPost, "/api/push/"+pushTestActiveToken+"?msg=Recuperado"); status != http.StatusOK {
		t.Errorf("expected the token of the active endpoint to be accepted, got %d", status)
	}
	if result, _ := latestResult(t, "erp_site"); result == nil || !result.Success || result.Message != "Recuperado" {
		t.Errorf("expected the push with the token of the endpoint, got %+v", result)
	}
}

func TestPush_LimitsOnlyRejections(t *testing.T) {
	router := newPushTestRouter(t)
	for i := 1; i <= pushRejectionsPerMinute+1; i++ {
		status, header, body := doPush(t, router, http.MethodGet, "/api/push/guess-"+strings.Repeat("x", i))
		if i <= pushRejectionsPerMinute && status != http.StatusNotFound {
			t.Fatalf("expected 404 for rejection %d, got %d", i, status)
		}
		if i == pushRejectionsPerMinute+1 && (status != http.StatusTooManyRequests || body != `{"ok":false,"msg":"Too many requests"}` || len(header.Get("Retry-After")) == 0) {
			t.Fatalf("expected 429 with Retry-After after %d rejections, got %d %s", pushRejectionsPerMinute, status, body)
		}
	}
	if status, _, _ := doPush(t, router, http.MethodGet, "/api/push/"+pushTestExternalToken); status != http.StatusOK {
		t.Errorf("expected a valid push to be accepted above the limit of rejections, got %d", status)
	}
}

func TestParsePushPing(t *testing.T) {
	scenarios := map[string]float64{"": 0, "abc": 0, "0": 0, "120": 120, "12.5ms": 12.5, " 7": 7, ".5": 0.5, "1e3": 1000, "+3": 3}
	for value, expected := range scenarios {
		ping, err := parsePushPing(value)
		if err != nil {
			t.Errorf("expected no error for %q, got %v", value, err)
			continue
		}
		if (ping == nil) != (expected == 0) || (ping != nil && *ping != expected) {
			t.Errorf("expected %v for %q, got %v", expected, value, ping)
		}
	}
	for _, value := range []string{"-5", "100000000001", "Infinity"} {
		if _, err := parsePushPing(value); err == nil {
			t.Errorf("expected an error for %q", value)
		}
	}
}
