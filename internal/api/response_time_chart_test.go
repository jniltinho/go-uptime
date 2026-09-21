package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/heartbeat"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/labstack/echo/v5"
)

// strictChartPayload mirrors the payload of the response time chart, decoded with DisallowUnknownFields, so that a new
// field (e.g. an error or a hostname) fails the tests
type strictChartPayload struct {
	Period          string    `json:"period"`
	IntervalSeconds *int64    `json:"intervalSeconds"`
	BucketSeconds   int       `json:"bucketSeconds"`
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	Results         *[]struct {
		Timestamp  time.Time `json:"timestamp"`
		Status     string    `json:"status"`
		DurationMs *int64    `json:"durationMs"`
	} `json:"results"`
	Buckets *[]struct {
		Timestamp time.Time `json:"timestamp"`
		Up        int       `json:"up"`
		Down      int       `json:"down"`
		Pending   int       `json:"pending"`
		AvgMs     *int64    `json:"avgMs"`
		MinMs     *int64    `json:"minMs"`
		MaxMs     *int64    `json:"maxMs"`
	} `json:"buckets"`
}

func decodeChartPayload(t *testing.T, body string) strictChartPayload {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	var payload strictChartPayload
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("failed to decode the chart payload strictly: %v: %s", err, body)
	}
	return payload
}

type chartTestEnvironment struct {
	app      *echo.Echo
	endpoint *endpoint.Endpoint
}

func newChartTestEnvironment(t *testing.T) *chartTestEnvironment {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	ep := &endpoint.Endpoint{Name: "api", Group: "core", URL: "https://example.org", Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := ep.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	withoutHeartbeat := &endpoint.ExternalEndpoint{Name: "nobeat", Group: "jobs", Token: "chart-test-token-000000000000000"}
	if err := withoutHeartbeat.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	statusPages := statusPagesTestConfig(true, 0)
	statusPages.Pages[0].Groups = []string{"core", "jobs"}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 120; i++ {
		result := &endpoint.Result{Success: i%10 != 0, Timestamp: now.Add(time.Duration(i-120) * time.Minute), Duration: time.Duration(10+i) * time.Millisecond, Hostname: "10.0.0.5"}
		if i%10 == 0 {
			result.Errors = []string{"secret-error"}
		}
		if i%15 == 0 {
			result.Success, result.Pending, result.Message = false, true, "secret-message"
		}
		if err := store.Get().InsertEndpointResult(ep, result); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{Security: statusPageBasicSecurity(t), Storage: &storage.Config{MaximumNumberOfResults: 100}, Endpoints: []*endpoint.Endpoint{ep}, ExternalEndpoints: []*endpoint.ExternalEndpoint{withoutHeartbeat}, StatusPages: statusPages}
	statuspage.Load(cfg)
	return &chartTestEnvironment{app: New(cfg).Router(), endpoint: ep}
}

func (env *chartTestEnvironment) get(t *testing.T, target string, authenticated bool) (*http.Response, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authenticated {
		request.SetBasicAuth("admin", "secret")
	}
	response, err := testHTTP(env.app, request)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	_, _ = body.ReadFrom(response.Body)
	_ = response.Body.Close()
	return response, body.String()
}

func TestResponseTimeChart_Protected(t *testing.T) {
	env := newChartTestEnvironment(t)
	response, body := env.get(t, "/api/v1/endpoints/core_api/response-time-chart?period=recent", true)
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected 200 with no-store, got %d %v: %s", response.StatusCode, response.Header, body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "10.0.0.5") {
		t.Errorf("the chart must not publish messages, errors or hostnames: %s", body)
	}
	recent := decodeChartPayload(t, body)
	if recent.Results == nil || len(*recent.Results) != 100 || recent.Buckets != nil || recent.IntervalSeconds == nil || *recent.IntervalSeconds != 60 {
		t.Fatalf("expected the 100 latest results with the interval of the endpoint, got %s", body)
	}
	results := *recent.Results
	statuses := map[string]int{}
	for i, result := range results {
		statuses[result.Status]++
		if result.DurationMs == nil || (i > 0 && result.Timestamp.Before(results[i-1].Timestamp)) {
			t.Fatalf("expected ordered results with durationMs, got %s", body)
		}
	}
	if statuses["up"] == 0 || statuses["down"] == 0 || statuses["pending"] == 0 || !recent.From.Equal(results[0].Timestamp) || !recent.To.Equal(results[99].Timestamp) {
		t.Errorf("unexpected statuses or bounds: %v %s %s", statuses, recent.From, recent.To)
	}

	_, body = env.get(t, "/api/v1/endpoints/core_api/response-time-chart?period=3h", true)
	buckets := decodeChartPayload(t, body)
	if buckets.Buckets == nil || buckets.Results != nil || buckets.BucketSeconds != common.MinuteBucketSeconds || len(*buckets.Buckets) == 0 || len(*buckets.Buckets) > 180 {
		t.Fatalf("expected the minute buckets of 3 hours, got %s", body)
	}
	if expectedFrom := time.Now().UTC().Truncate(time.Minute).Add(-179 * time.Minute); !buckets.From.Equal(expectedFrom) {
		t.Errorf("expected the window to start at %s, got %s", expectedFrom, buckets.From)
	}
	total := 0
	for _, bucket := range *buckets.Buckets {
		total += bucket.Up + bucket.Down + bucket.Pending
		if bucket.Up > 0 && (bucket.AvgMs == nil || bucket.MinMs == nil || bucket.MaxMs == nil) {
			t.Errorf("expected the response times of a bucket with up results: %s", body)
		}
	}
	if total != 120 {
		t.Errorf("expected the 120 results in the buckets, got %d", total)
	}
	_, body = env.get(t, "/api/v1/endpoints/core_api/response-time-chart?period=1w", true)
	if week := decodeChartPayload(t, body); week.BucketSeconds != common.HourBucketSeconds || week.Buckets == nil || len(*week.Buckets) > 168 {
		t.Errorf("expected the hour buckets of a week, got %s", body)
	}

	// Endpoint without results and without interval
	_, body = env.get(t, "/api/v1/endpoints/jobs_nobeat/response-time-chart?period=recent", true)
	if empty := decodeChartPayload(t, body); empty.Results == nil || len(*empty.Results) != 0 || empty.IntervalSeconds != nil || !empty.From.Equal(empty.To) {
		t.Errorf("expected an empty recent chart without interval, got %s", body)
	}
	_, body = env.get(t, "/api/v1/endpoints/jobs_nobeat/response-time-chart?period=24h", true)
	if empty := decodeChartPayload(t, body); empty.Buckets == nil || len(*empty.Buckets) != 0 {
		t.Errorf("expected an empty list of buckets, got %s", body)
	}

	for target, expected := range map[string]int{
		"/api/v1/endpoints/core_api/response-time-chart?period=30d":     http.StatusBadRequest,
		"/api/v1/endpoints/core_api/response-time-chart":                http.StatusBadRequest,
		"/api/v1/endpoints/core_missing/response-time-chart?period=30d": http.StatusNotFound,
	} {
		if response, body := env.get(t, target, true); response.StatusCode != expected || !json.Valid([]byte(body)) {
			t.Errorf("%s: expected %d with a JSON body, got %d %s", target, expected, response.StatusCode, body)
		}
	}
	if response, _ := env.get(t, "/api/v1/endpoints/core_api/response-time-chart?period=recent", false); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without authentication, got %d", response.StatusCode)
	}
}

func TestResponseTimeChart_Public(t *testing.T) {
	env := newChartTestEnvironment(t)
	response, body := env.get(t, "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=recent", false)
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("expected 200 with the public headers, got %d %v: %s", response.StatusCode, response.Header, body)
	}
	recent := decodeChartPayload(t, body)
	if recent.Results == nil || len(*recent.Results) != statuspage.MaximumPublicResults || strings.Contains(body, "secret") || strings.Contains(body, "10.0.0.5") {
		t.Fatalf("expected the 50 latest results without sensitive data, got %s", body)
	}
	_, body = env.get(t, "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=6h", false)
	if buckets := decodeChartPayload(t, body); buckets.Buckets == nil || len(*buckets.Buckets) == 0 || len(*buckets.Buckets) > 360 {
		t.Errorf("expected the minute buckets of 6 hours, got %s", body)
	}

	// A new result renews the cached recent chart
	last := (*recent.Results)[len(*recent.Results)-1].Timestamp
	watchdog.UpdateEndpointStatus(env.endpoint, &endpoint.Result{Success: true, Timestamp: last.Add(time.Second), Duration: 7 * time.Millisecond})
	_, body = env.get(t, "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=recent", false)
	if renewed := decodeChartPayload(t, body); !(*renewed.Results)[len(*renewed.Results)-1].Timestamp.Equal(last.Add(time.Second)) {
		t.Errorf("expected the new result in the recent chart, got %s", body)
	}

	// The identical 404 comes before the validation of the period
	reference, referenceBody := doStatusPageRequest(t, env.app, http.MethodGet, "/api/v1/status-pages/missing")
	for _, target := range []string{
		"/api/v1/status-pages/infra/endpoints/core_missing/response-time-chart?period=30d",
		"/api/v1/status-pages/hidden/endpoints/core_api/response-time-chart?period=recent",
		"/api/v1/status-pages/missing/endpoints/core_api/response-time-chart?period=recent",
	} {
		response, body := env.get(t, target, false)
		if response.StatusCode != reference.StatusCode || body != referenceBody {
			t.Errorf("%s: expected the identical 404, got %d %s", target, response.StatusCode, body)
		}
		for _, header := range comparedStatusPageHeaders {
			if response.Header.Get(header) != reference.Header.Get(header) {
				t.Errorf("%s: expected header %s=%q, got %q", target, header, reference.Header.Get(header), response.Header.Get(header))
			}
		}
	}
	response, body = env.get(t, "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=30d", false)
	if response.StatusCode != http.StatusBadRequest || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Robots-Tag") != "noindex, nofollow" || !json.Valid([]byte(body)) {
		t.Errorf("expected 400 with the public headers, got %d %v %s", response.StatusCode, response.Header, body)
	}
}

func TestEndpointIntervalSeconds(t *testing.T) {
	cfg := &config.Config{
		Storage:   &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "gatus.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		Endpoints: []*endpoint.Endpoint{{Name: "site", Group: "file", Interval: 2 * time.Minute}},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{
			{Name: "beat", Group: "file", Heartbeat: heartbeat.Config{Interval: 30 * time.Second}},
			{Name: "nobeat", Group: "file"},
		},
	}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		_ = store.Initialize(nil)
		_, _ = managedendpoint.Load(&config.Config{})
	})
	managedStore, _ := store.GetManagedEndpointStore()
	definitions := map[string]string{
		"managed_web": "name: web\ngroup: managed\nurl: https://example.org\ninterval: 3m\nconditions:\n  - \"[STATUS] == 200\"\n",
		"managed_job": "type: push\nname: job\ngroup: managed\ntoken: keSDu7G855jvVat1xWiY2Gk4CkL1End5\nheartbeat:\n  interval: 5m\n",
	}
	for key, definition := range definitions {
		if err := managedStore.CreateManagedEndpoint(&common.ManagedEndpoint{Key: key, Definition: definition}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]int64{"file_site": 120, "file_beat": 30, "file_nobeat": 0, "managed_web": 180, "managed_job": 300, "unknown_key": 0} {
		actual := endpointIntervalSeconds(cfg, key)
		if (expected == 0 && actual != nil) || (expected != 0 && (actual == nil || *actual != expected)) {
			t.Errorf("%s: expected %d, got %v", key, expected, actual)
		}
	}
}
