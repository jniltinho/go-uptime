// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"golang.org/x/crypto/bcrypt"
)

// The contract of the HTTP server: for every route, what a client receives — the status, the headers that matter and
// the body. It is recorded from the running server into testdata/http_contract.golden.json and compared on every run,
// through a real TCP connection, so that replacing the HTTP framework cannot change an answer without it showing up here
// (change migrate-to-echo-and-cobra, D3). Record it again with:
//
//	go test ./api/ -run TestHTTPContract -update-contract
//
// and read the diff of the golden file: every changed line is a change of behavior that a client can see.
var updateContract = flag.Bool("update-contract", false, "record the contract of the HTTP server instead of comparing it")

const contractGoldenFile = "testdata/http_contract.golden.json"

// contractHeaders are the headers of the contract. Date, Content-Length and the headers of the connection are left
// out: they are not decided by the handlers.
var contractHeaders = []string{
	"Allow", "Cache-Control", "Content-Disposition", "Content-Encoding", "Content-Type", "Etag", "Location",
	"Referrer-Policy", "Retry-After", "Set-Cookie", "Vary", "Www-Authenticate", "X-Accel-Buffering",
	"X-Content-Type-Options", "X-Robots-Tag",
}

type contractCase struct {
	Name    string
	Method  string
	Path    string
	Headers map[string]string
	Body    string
	// Credentials is "user:password" for Authorization: Basic
	Credentials string
	// OnlyStatusAndHeaders leaves the body out of the contract, for the bodies that change on every run
	OnlyStatusAndHeaders bool
	// RawPath sends the path as it is, through a plain connection: net/http refuses to build a request with an invalid
	// escape, which a client can nevertheless send
	RawPath bool
}

type contractAnswer struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

var (
	contractTimestamp   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	contractSession     = regexp.MustCompile(`go_uptime_session=[^;]*`)
	contractCookieDate  = regexp.MustCompile(`(?i)expires=[^;]*`)
	contractBuildAssets = regexp.MustCompile(`(app|chunk-vendors)\.[0-9a-f]+\.(js|css)`)
	contractBackupName  = regexp.MustCompile(`go-uptime-backup-\d{8}-\d{6}`)
)

func normalizeContract(value string) string {
	value = contractTimestamp.ReplaceAllString(value, "<timestamp>")
	value = contractSession.ReplaceAllString(value, "go_uptime_session=<token>")
	value = contractCookieDate.ReplaceAllString(value, "expires=<date>")
	value = contractBackupName.ReplaceAllString(value, "go-uptime-backup-<date>")
	return contractBuildAssets.ReplaceAllString(value, "$1.<hash>.$2")
}

func contractConfig(t *testing.T) *config.Config {
	t.Helper()
	hash := func(password string) string {
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		return base64.URLEncoding.EncodeToString(hashed)
	}
	newEndpoint := func(name, group string) *endpoint.Endpoint {
		// The name may have a "%", which is not valid in the URL of the endpoint
		ep := &endpoint.Endpoint{Name: name, Group: group, URL: "https://example.org/" + url.PathEscape(name), Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}}
		if err := ep.ValidateAndSetDefaults(); err != nil {
			t.Fatal(err)
		}
		return ep
	}
	enabled, disabled, rateLimit := true, false, 0
	statusPages := &pageconfig.Config{Enabled: &enabled, RateLimit: &rateLimit, Pages: []*pageconfig.Page{
		{Slug: "infra", Title: "Infra", Description: "Public page", Groups: []string{"core"}, Featured: []string{"core_api"}, ShowMessages: true},
		{Slug: "clients", Title: "Clients", Groups: []string{"core"}, GroupsCollapsed: true, Auth: &pageconfig.PageAuth{Username: "client", PasswordBcryptHashBase64Encoded: hash("page-secret")}},
		{Slug: "hidden", Title: "Hidden", Groups: []string{"core"}, Enabled: &disabled},
	}}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	external := &endpoint.ExternalEndpoint{Name: "backup", Group: "jobs", Token: "contract-token-0000000000000000"}
	if err := external.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		Security:    &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: hash("secret")}},
		Admin:       &admin.Config{Enabled: true},
		Maintenance: &maintenance.Config{Enabled: &disabled},
		Storage:     &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		// "100%" and "%61pi" are there for the escaping: %61 is "a", so a key unescaped once too many becomes core_api,
		// which is ANOTHER endpoint, with another state
		Endpoints:         []*endpoint.Endpoint{newEndpoint("api", "core"), newEndpoint("db", "core"), newEndpoint("100%", "core"), newEndpoint("%61pi", "core")},
		ExternalEndpoints: []*endpoint.ExternalEndpoint{external},
		StatusPages:       statusPages,
	}
}

// startContractServer serves the router through a real TCP connection and returns its address. It is the only part of
// this file that knows the HTTP framework.
func startContractServer(t *testing.T, cfg *config.Config) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: New(cfg).Router(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return "http://" + listener.Addr().String()
}

func setupContract(t *testing.T) string {
	t.Helper()
	cfg := contractConfig(t)
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		_ = store.Initialize(nil)
		_, _ = managedendpoint.Load(&config.Config{})
		statuspage.Load(&config.Config{})
		pushkey.Load(&config.Config{})
	})
	now := time.Now()
	results := []struct {
		ep     *endpoint.Endpoint
		result *endpoint.Result
	}{
		{cfg.Endpoints[0], &endpoint.Result{Success: true, HTTPStatus: 200, Connected: true, Duration: 120 * time.Millisecond, Timestamp: now.Add(-2 * time.Minute), ConditionResults: []*endpoint.ConditionResult{{Condition: "[STATUS] == 200", Success: true}}}},
		{cfg.Endpoints[0], &endpoint.Result{Success: true, HTTPStatus: 200, Connected: true, Duration: 80 * time.Millisecond, Timestamp: now.Add(-time.Minute), ConditionResults: []*endpoint.ConditionResult{{Condition: "[STATUS] == 200", Success: true}}}},
		// core_100% is up and core_%61pi is down, while core_api is up: the badge tells which endpoint answered
		{cfg.Endpoints[2], &endpoint.Result{Success: true, HTTPStatus: 200, Connected: true, Duration: 10 * time.Millisecond, Timestamp: now.Add(-time.Minute)}},
		{cfg.Endpoints[3], &endpoint.Result{Success: false, Duration: 10 * time.Millisecond, Timestamp: now.Add(-time.Minute)}},
		{cfg.Endpoints[1], &endpoint.Result{Success: false, Duration: 5 * time.Millisecond, Timestamp: now.Add(-time.Minute), Errors: []string{`Get "https://example.org/db": dial tcp 10.0.0.5:443: connect: connection refused`}, ConditionResults: []*endpoint.ConditionResult{{Condition: "[STATUS] (0) == 200", Success: false}}}},
	}
	for _, entry := range results {
		if err := store.Get().InsertEndpointResult(entry.ep, entry.result); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	statuspage.Load(cfg)
	pushkey.Load(cfg)
	// The monitoring runs without the endpoints of the configuration: the administration and the push need its registry,
	// but a check of an endpoint would add results at a moment of its own, and the recorded bodies would change
	monitored := *cfg
	monitored.Endpoints = nil
	watchdog.Monitor(&monitored)
	t.Cleanup(func() { watchdog.Shutdown(&monitored) })
	return startContractServer(t, cfg)
}

func contractCases() []contractCase {
	const adminCredentials, pageCredentials = "admin:secret", "client:page-secret"
	yaml := map[string]string{"Content-Type": "application/yaml"}
	jsonBody := map[string]string{"Content-Type": "application/json"}
	// A closed port of the loopback: the check of the managed endpoint never leaves this machine
	managed := "name: site\ngroup: web\ninterval: 1h\nurl: http://127.0.0.1:1/site\nconditions: [\"[STATUS] == 200\"]\n"
	cases := []contractCase{
		// Public, outside of /api
		{Name: "health", Method: "GET", Path: "/health"},
		{Name: "health head", Method: "HEAD", Path: "/health"},
		{Name: "health post", Method: "POST", Path: "/health"},
		{Name: "spa root", Method: "GET", Path: "/", OnlyStatusAndHeaders: true},
		{Name: "spa root head", Method: "HEAD", Path: "/"},
		{Name: "spa login", Method: "GET", Path: "/login", OnlyStatusAndHeaders: true},
		{Name: "spa endpoint", Method: "GET", Path: "/endpoints/core_api", OnlyStatusAndHeaders: true},
		{Name: "spa suite", Method: "GET", Path: "/suites/core_suite", OnlyStatusAndHeaders: true},
		{Name: "spa admin", Method: "GET", Path: "/admin", OnlyStatusAndHeaders: true},
		{Name: "spa admin endpoint edit", Method: "GET", Path: "/admin/endpoints/core_api/edit", OnlyStatusAndHeaders: true},
		{Name: "spa admin status pages", Method: "GET", Path: "/admin/status-pages", OnlyStatusAndHeaders: true},
		{Name: "spa admin backup", Method: "GET", Path: "/admin/backup", OnlyStatusAndHeaders: true},
		{Name: "index.html redirect", Method: "GET", Path: "/index.html"},
		{Name: "index.html redirect with query", Method: "GET", Path: "/index.html?a=1"},
		{Name: "custom css", Method: "GET", Path: "/css/custom.css"},
		{Name: "static css", Method: "GET", Path: "/css/app.css", OnlyStatusAndHeaders: true},
		{Name: "static css head", Method: "HEAD", Path: "/css/app.css"},
		{Name: "static css gzip", Method: "GET", Path: "/css/app.css", Headers: map[string]string{"Accept-Encoding": "gzip"}, OnlyStatusAndHeaders: true},
		{Name: "static font", Method: "GET", Path: "/fonts/inter-4-1-latin.woff2", OnlyStatusAndHeaders: true},
		{Name: "static font range", Method: "GET", Path: "/fonts/inter-4-1-latin.woff2", Headers: map[string]string{"Range": "bytes=0-3"}},
		// A path with "..", however it is escaped, must never reach the template of the SPA as a file
		{Name: "static dot dot", Method: "GET", Path: "/css/../index.html", RawPath: true},
		{Name: "static escaped dot dot", Method: "GET", Path: "/css/%2e%2e/index.html", RawPath: true},
		{Name: "static escaped slash after dot dot", Method: "GET", Path: "/css/..%2findex.html", RawPath: true},
		{Name: "static dot dot out of the file system", Method: "GET", Path: "/css/../../../../etc/passwd", RawPath: true},
		{Name: "static missing", Method: "GET", Path: "/js/missing.js"},
		{Name: "static missing with credentials", Method: "GET", Path: "/js/missing.js", Credentials: adminCredentials},
		{Name: "static directory", Method: "GET", Path: "/js/", OnlyStatusAndHeaders: true},
		{Name: "unknown path", Method: "GET", Path: "/nothing-here"},
		{Name: "unknown path with credentials", Method: "GET", Path: "/nothing-here", Credentials: adminCredentials},
		{Name: "uppercase path", Method: "GET", Path: "/HEALTH"},
		{Name: "trailing slash", Method: "GET", Path: "/health/"},

		// Public API
		{Name: "config without session", Method: "GET", Path: "/api/v1/config"},
		{Name: "config with credentials", Method: "GET", Path: "/api/v1/config", Credentials: adminCredentials},
		{Name: "config uppercase", Method: "GET", Path: "/API/v1/config"},
		{Name: "badge health", Method: "GET", Path: "/api/v1/endpoints/core_api/health/badge.svg"},
		{Name: "badge health head", Method: "HEAD", Path: "/api/v1/endpoints/core_api/health/badge.svg"},
		{Name: "badge health shields", Method: "GET", Path: "/api/v1/endpoints/core_api/health/badge.shields"},
		{Name: "badge health unknown key", Method: "GET", Path: "/api/v1/endpoints/core_missing/health/badge.svg"},
		{Name: "badge uptime", Method: "GET", Path: "/api/v1/endpoints/core_api/uptimes/24h/badge.svg"},
		{Name: "badge uptime invalid duration", Method: "GET", Path: "/api/v1/endpoints/core_api/uptimes/3d/badge.svg"},
		{Name: "raw uptime", Method: "GET", Path: "/api/v1/endpoints/core_api/uptimes/24h"},
		{Name: "badge response time", Method: "GET", Path: "/api/v1/endpoints/core_api/response-times/24h/badge.svg"},
		{Name: "raw response time", Method: "GET", Path: "/api/v1/endpoints/core_api/response-times/24h"},
		{Name: "chart response time", Method: "GET", Path: "/api/v1/endpoints/core_api/response-times/24h/chart.svg", OnlyStatusAndHeaders: true},
		{Name: "history response time", Method: "GET", Path: "/api/v1/endpoints/core_api/response-times/24h/history", OnlyStatusAndHeaders: true},
		{Name: "key with encoded slash", Method: "GET", Path: "/api/v1/endpoints/core%2Fapi/health/badge.svg"},
		{Name: "key with double encoded slash", Method: "GET", Path: "/api/v1/endpoints/core%252Fapi/health/badge.svg"},
		{Name: "key with plus", Method: "GET", Path: "/api/v1/endpoints/core+api/health/badge.svg"},
		// Keys that exist: a wrong number of unescapes answers the badge of another endpoint, or none
		{Name: "key with a percent sign", Method: "GET", Path: "/api/v1/endpoints/core_100%25/health/badge.svg"},
		{Name: "key with an escaped escape", Method: "GET", Path: "/api/v1/endpoints/core_%2561pi/health/badge.svg"},
		{Name: "key with a needless escape", Method: "GET", Path: "/api/v1/endpoints/%63ore_api/health/badge.svg"},
		{Name: "key with a needless escape and a trailing slash", Method: "GET", Path: "/api/v1/endpoints/%63ore_api/health/badge.svg/"},
		{Name: "key with a percent sign and a trailing slash", Method: "GET", Path: "/api/v1/endpoints/core_100%25/health/badge.svg/"},
		{Name: "key with invalid escape", Method: "GET", Path: "/api/v1/endpoints/core%zzapi/health/badge.svg", RawPath: true},
		{Name: "external result without token", Method: "POST", Path: "/api/v1/endpoints/jobs_backup/external?success=true"},
		{Name: "external result wrong token", Method: "POST", Path: "/api/v1/endpoints/jobs_backup/external?success=true", Headers: map[string]string{"Authorization": "Bearer wrong"}},
		{Name: "external result", Method: "POST", Path: "/api/v1/endpoints/jobs_backup/external?success=false&error=disk+full", Headers: map[string]string{"Authorization": "Bearer contract-token-0000000000000000"}},
		{Name: "external result with success repeated", Method: "POST", Path: "/api/v1/endpoints/jobs_backup/external?success=true&success=invalid", Headers: map[string]string{"Authorization": "Bearer contract-token-0000000000000000"}},
		{Name: "external result unknown key", Method: "POST", Path: "/api/v1/endpoints/jobs_missing/external?success=true", Headers: map[string]string{"Authorization": "Bearer contract-token-0000000000000000"}},

		// Push compatible with the Uptime Kuma
		{Name: "push by token", Method: "GET", Path: "/api/push/contract-token-0000000000000000?status=up&msg=OK&ping=12"},
		{Name: "push by token post", Method: "POST", Path: "/api/push/contract-token-0000000000000000?status=down&msg=Disk%20full"},
		{Name: "push unknown token", Method: "GET", Path: "/api/push/unknown-token-000000000000000000"},
		{Name: "push invalid ping", Method: "GET", Path: "/api/push/contract-token-0000000000000000?ping=-1"},
		{Name: "push repeated query", Method: "GET", Path: "/api/push/contract-token-0000000000000000?status=up&status=down&msg="},
		// A ";" is part of the value and an invalid escape is kept: net/url drops such pairs, and a missing status means up
		{Name: "push with a semicolon", Method: "GET", Path: "/api/push/contract-token-0000000000000000?status=down;maintenance&msg=disk;full"},
		{Name: "push with an invalid escape in the ping", Method: "GET", Path: "/api/push/contract-token-0000000000000000?ping=%zz&status=up"},
		{Name: "push root", Method: "GET", Path: "/api/push"},
		{Name: "push too deep", Method: "GET", Path: "/api/push/a/b/c"},
		{Name: "push body above the limit", Method: "POST", Path: "/api/push/contract-token-0000000000000000?status=up", Body: strings.Repeat("a", 5<<20)},

		// Public status pages
		{Name: "status page", Method: "GET", Path: "/api/v1/status-pages/infra"},
		{Name: "status page head", Method: "HEAD", Path: "/api/v1/status-pages/infra"},
		{Name: "status page gzip", Method: "GET", Path: "/api/v1/status-pages/infra", Headers: map[string]string{"Accept-Encoding": "gzip"}, OnlyStatusAndHeaders: true},
		{Name: "status page trailing slash", Method: "GET", Path: "/api/v1/status-pages/infra/"},
		{Name: "status page uppercase slug", Method: "GET", Path: "/api/v1/status-pages/INFRA"},
		{Name: "status page missing", Method: "GET", Path: "/api/v1/status-pages/missing"},
		{Name: "status page disabled", Method: "GET", Path: "/api/v1/status-pages/hidden"},
		{Name: "status page root", Method: "GET", Path: "/api/v1/status-pages"},
		{Name: "status page too deep", Method: "GET", Path: "/api/v1/status-pages/a/b/c"},
		{Name: "status page post", Method: "POST", Path: "/api/v1/status-pages/infra"},
		{Name: "status page options", Method: "OPTIONS", Path: "/api/v1/status-pages/infra"},
		{Name: "status page with admin credentials", Method: "GET", Path: "/api/v1/status-pages/missing", Credentials: adminCredentials},
		{Name: "status page details", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_api"},
		{Name: "status page details failed check", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_db"},
		{Name: "status page details unknown key", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/jobs_backup"},
		{Name: "status page chart", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=recent", OnlyStatusAndHeaders: true},
		{Name: "status page chart invalid period", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_api/response-time-chart?period=year"},
		{Name: "status page badge", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_api/health/badge.svg"},
		{Name: "status page events head", Method: "HEAD", Path: "/api/v1/status-pages/infra/endpoints/core_api/events"},
		{Name: "status page events unknown key", Method: "GET", Path: "/api/v1/status-pages/infra/endpoints/core_missing/events"},
		{Name: "status page html", Method: "GET", Path: "/status/infra", OnlyStatusAndHeaders: true},
		{Name: "status page html trailing slash", Method: "GET", Path: "/status/infra/", OnlyStatusAndHeaders: true},
		{Name: "status page html nested", Method: "GET", Path: "/status/infra/endpoints/core_api", OnlyStatusAndHeaders: true},
		{Name: "status page html missing", Method: "GET", Path: "/status/missing", OnlyStatusAndHeaders: true},

		// Status page with a login of its own
		{Name: "protected page without credentials", Method: "GET", Path: "/api/v1/status-pages/clients"},
		{Name: "protected page", Method: "GET", Path: "/api/v1/status-pages/clients", Credentials: pageCredentials},
		{Name: "protected page with admin credentials", Method: "GET", Path: "/api/v1/status-pages/clients", Credentials: adminCredentials},
		{Name: "protected page browser request", Method: "GET", Path: "/api/v1/status-pages/clients", Headers: map[string]string{"Sec-Fetch-Site": "same-origin", "X-Requested-With": "XMLHttpRequest"}},
		{Name: "protected page unknown key before 404", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_missing"},
		{Name: "protected page unknown key", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_missing", Credentials: pageCredentials},
		{Name: "protected page details", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api", Credentials: pageCredentials},
		{Name: "protected page badge", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api/health/badge.svg", Credentials: pageCredentials},
		{Name: "protected page badge response time", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api/response-times/24h/badge.svg", Credentials: pageCredentials},
		{Name: "protected page events head", Method: "HEAD", Path: "/api/v1/status-pages/clients/endpoints/core_api/events", Credentials: pageCredentials},
		{Name: "protected page events without credentials", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api/events"},
		{Name: "protected page html", Method: "GET", Path: "/status/clients", Credentials: pageCredentials, OnlyStatusAndHeaders: true},
		{Name: "protected page html without credentials", Method: "GET", Path: "/status/clients"},
		{Name: "protected page html head without credentials", Method: "HEAD", Path: "/status/clients"},
		{Name: "protected page html nested without credentials", Method: "GET", Path: "/status/clients/endpoints/core_api"},
		{Name: "protected page html trailing slash without credentials", Method: "GET", Path: "/status/clients/"},
		{Name: "protected page badge invalid duration", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api/response-times/invalid/badge.svg", Credentials: pageCredentials},
		{Name: "protected page badge without credentials", Method: "GET", Path: "/api/v1/status-pages/clients/endpoints/core_api/health/badge.svg"},
		// The tenth failure of this client on this page was the one above: from here on the page answers 429, even to the
		// right credentials, and only this page
		{Name: "protected page after ten failures", Method: "GET", Path: "/api/v1/status-pages/clients", Credentials: pageCredentials},
		{Name: "public page while another one is limited", Method: "GET", Path: "/api/v1/status-pages/infra", OnlyStatusAndHeaders: true},

		// Protected API
		{Name: "statuses without credentials", Method: "GET", Path: "/api/v1/endpoints/statuses"},
		{Name: "statuses event source without credentials", Method: "GET", Path: "/api/v1/endpoints/core_api/events", Headers: map[string]string{"Accept": "text/event-stream"}},
		{Name: "statuses browser without credentials", Method: "GET", Path: "/api/v1/endpoints/statuses", Headers: map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{Name: "statuses wrong password", Method: "GET", Path: "/api/v1/endpoints/statuses", Credentials: "admin:wrong"},
		{Name: "statuses", Method: "GET", Path: "/api/v1/endpoints/statuses", Credentials: adminCredentials},
		{Name: "statuses head", Method: "HEAD", Path: "/api/v1/endpoints/statuses", Credentials: adminCredentials},
		{Name: "statuses paged", Method: "GET", Path: "/api/v1/endpoints/statuses?page=1&pageSize=1", Credentials: adminCredentials},
		{Name: "endpoint status", Method: "GET", Path: "/api/v1/endpoints/core_api/statuses", Credentials: adminCredentials},
		// What the pushes above RECORDED, and not only what they answered
		{Name: "what the pushes recorded", Method: "GET", Path: "/api/v1/endpoints/jobs_backup/statuses", Credentials: adminCredentials},
		{Name: "endpoint status unknown key", Method: "GET", Path: "/api/v1/endpoints/core_missing/statuses", Credentials: adminCredentials},
		{Name: "endpoint status without credentials", Method: "GET", Path: "/api/v1/endpoints/core_api/statuses"},
		{Name: "endpoint events head", Method: "HEAD", Path: "/api/v1/endpoints/core_api/events", Credentials: adminCredentials},
		{Name: "endpoint events unknown key", Method: "GET", Path: "/api/v1/endpoints/core_missing/events", Credentials: adminCredentials},
		{Name: "endpoint chart", Method: "GET", Path: "/api/v1/endpoints/core_api/response-time-chart?period=recent", Credentials: adminCredentials, OnlyStatusAndHeaders: true},
		{Name: "endpoint chart without credentials", Method: "GET", Path: "/api/v1/endpoints/core_api/response-time-chart?period=recent"},
		{Name: "suite statuses", Method: "GET", Path: "/api/v1/suites/statuses", Credentials: adminCredentials},
		{Name: "suite statuses without credentials", Method: "GET", Path: "/api/v1/suites/statuses"},
		{Name: "suite status unknown key", Method: "GET", Path: "/api/v1/suites/missing/statuses", Credentials: adminCredentials},
		{Name: "unknown api path without credentials", Method: "GET", Path: "/api/v1/nothing-here"},
		{Name: "unknown api path with credentials", Method: "GET", Path: "/api/v1/nothing-here", Credentials: adminCredentials},
		{Name: "api root without credentials", Method: "GET", Path: "/api"},
		{Name: "unknown api path head", Method: "HEAD", Path: "/api/v1/nothing-here"},
		{Name: "unknown api path options", Method: "OPTIONS", Path: "/api/v1/nothing-here"},
		{Name: "unknown api path post with credentials", Method: "POST", Path: "/api/v1/nothing-here", Credentials: adminCredentials},

		// Login screen of security.basic
		{Name: "login cross site", Method: "POST", Path: "/api/v1/auth/login", Headers: map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"}, Body: `{"username":"admin","password":"secret"}`},
		{Name: "login foreign origin", Method: "POST", Path: "/api/v1/auth/login", Headers: map[string]string{"Content-Type": "application/json", "Origin": "https://evil.example"}, Body: `{"username":"admin","password":"secret"}`},
		{Name: "login wrong content type", Method: "POST", Path: "/api/v1/auth/login", Headers: map[string]string{"Content-Type": "text/plain"}, Body: `{"username":"admin","password":"secret"}`},
		{Name: "login malformed", Method: "POST", Path: "/api/v1/auth/login", Headers: jsonBody, Body: `{"username":`},
		{Name: "login body above the limit", Method: "POST", Path: "/api/v1/auth/login", Headers: jsonBody, Body: `{"username":"` + strings.Repeat("a", 8<<10) + `"}`},
		{Name: "login wrong password", Method: "POST", Path: "/api/v1/auth/login", Headers: jsonBody, Body: `{"username":"admin","password":"wrong"}`},
		{Name: "login", Method: "POST", Path: "/api/v1/auth/login", Headers: jsonBody, Body: `{"username":"admin","password":"secret"}`},
		{Name: "login with forged https", Method: "POST", Path: "/api/v1/auth/login", Headers: map[string]string{"Content-Type": "application/json", "X-Forwarded-Proto": "https", "X-Forwarded-Ssl": "on"}, Body: `{"username":"admin","password":"secret"}`},
		// Only the first value of X-Forwarded-Proto counts, as before: never the other headers echo.Context.Scheme trusts
		{Name: "login with the headers the framework would trust", Method: "POST", Path: "/api/v1/auth/login", Headers: map[string]string{"Content-Type": "application/json", "X-Forwarded-Ssl": "on", "X-Forwarded-Protocol": "https", "X-Url-Scheme": "https"}, Body: `{"username":"admin","password":"secret"}`},
		{Name: "login get", Method: "GET", Path: "/api/v1/auth/login"},
		{Name: "logout", Method: "POST", Path: "/api/v1/auth/logout", Headers: jsonBody},

		// Administration
		{Name: "admin without credentials", Method: "GET", Path: "/api/v1/admin/endpoints"},
		{Name: "admin metadata", Method: "GET", Path: "/api/v1/admin/metadata", Credentials: adminCredentials},
		{Name: "admin list", Method: "GET", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials},
		{Name: "admin create cross site", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "Sec-Fetch-Site": "cross-site"}, Body: managed},
		{Name: "admin create foreign origin", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "Origin": "https://evil.example"}, Body: managed},
		{Name: "admin create forged forwarded host", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "Origin": "https://evil.example", "X-Forwarded-Host": "evil.example", "X-Forwarded-Proto": "https"}, Body: managed},
		{Name: "admin create wrong content type", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "text/plain"}, Body: managed},
		{Name: "admin create invalid", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: yaml, Body: "name: site\n"},
		{Name: "admin create body above the limit", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: yaml, Body: managed + "# " + strings.Repeat("a", 300<<10) + "\n"},
		{Name: "admin validate", Method: "POST", Path: "/api/v1/admin/endpoints/validate", Credentials: adminCredentials, Headers: yaml, Body: managed},
		{Name: "admin parse", Method: "POST", Path: "/api/v1/admin/endpoints/parse", Credentials: adminCredentials, Headers: yaml, Body: managed},
		{Name: "admin create", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: yaml, Body: managed},
		{Name: "admin create again", Method: "POST", Path: "/api/v1/admin/endpoints", Credentials: adminCredentials, Headers: yaml, Body: managed},
		{Name: "admin get", Method: "GET", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials},
		{Name: "admin get config endpoint", Method: "GET", Path: "/api/v1/admin/endpoints/core_api", Credentials: adminCredentials},
		{Name: "admin get unknown", Method: "GET", Path: "/api/v1/admin/endpoints/web_missing", Credentials: adminCredentials},
		{Name: "admin update without if-match", Method: "PUT", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials, Headers: yaml, Body: managed},
		{Name: "admin update invalid if-match", Method: "PUT", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "If-Match": "abc"}, Body: managed},
		{Name: "admin update outdated if-match", Method: "PUT", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "If-Match": `"9"`}, Body: managed},
		{Name: "admin update weak if-match", Method: "PUT", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials, Headers: map[string]string{"Content-Type": "application/yaml", "If-Match": `W/"1"`}, Body: managed},
		{Name: "admin disable", Method: "POST", Path: "/api/v1/admin/endpoints/web_site/disable", Credentials: adminCredentials, Headers: map[string]string{"If-Match": `"2"`}},
		{Name: "admin config endpoint is read only", Method: "DELETE", Path: "/api/v1/admin/endpoints/core_api", Credentials: adminCredentials, Headers: map[string]string{"If-Match": `"1"`}},
		{Name: "admin delete", Method: "DELETE", Path: "/api/v1/admin/endpoints/web_site", Credentials: adminCredentials, Headers: map[string]string{"If-Match": `"3"`}},
		{Name: "admin status pages list", Method: "GET", Path: "/api/v1/admin/status-pages", Credentials: adminCredentials},
		{Name: "admin status pages options", Method: "GET", Path: "/api/v1/admin/status-pages/options", Credentials: adminCredentials},
		{Name: "admin status pages exposure without query", Method: "GET", Path: "/api/v1/admin/status-pages/exposure", Credentials: adminCredentials},
		{Name: "admin status pages exposure", Method: "GET", Path: "/api/v1/admin/status-pages/exposure?group=core", Credentials: adminCredentials},
		{Name: "admin status page get", Method: "GET", Path: "/api/v1/admin/status-pages/clients", Credentials: adminCredentials},
		{Name: "admin status page preview", Method: "GET", Path: "/api/v1/admin/status-pages/clients/preview", Credentials: adminCredentials},
		{Name: "admin status page create", Method: "POST", Path: "/api/v1/admin/status-pages", Credentials: adminCredentials, Headers: yaml, Body: "slug: managed\ntitle: Managed\ngroups: [core]\nenabled: true\nauth:\n  username: visitor\n  password: a-good-password\n"},
		{Name: "admin status page create short password", Method: "POST", Path: "/api/v1/admin/status-pages", Credentials: adminCredentials, Headers: yaml, Body: "slug: other\ntitle: Other\ngroups: [core]\nauth:\n  username: visitor\n  password: short\n"},
		{Name: "admin push keys list", Method: "GET", Path: "/api/v1/admin/push-keys", Credentials: adminCredentials},
		{Name: "admin push keys create invalid", Method: "POST", Path: "/api/v1/admin/push-keys", Credentials: adminCredentials, Headers: jsonBody, Body: `{"name":""}`},
		{Name: "admin backup wrong content type", Method: "POST", Path: "/api/v1/admin/backup", Credentials: adminCredentials, Headers: yaml, Body: "{}"},
		{Name: "admin backup", Method: "POST", Path: "/api/v1/admin/backup", Credentials: adminCredentials, Headers: jsonBody, Body: "{}", OnlyStatusAndHeaders: true},
		{Name: "admin restore preview invalid", Method: "POST", Path: "/api/v1/admin/restore/preview", Credentials: adminCredentials, Headers: jsonBody, Body: `{"file":"not json"}`},
		{Name: "admin restore preview of 3 MiB", Method: "POST", Path: "/api/v1/admin/restore/preview", Credentials: adminCredentials, Headers: jsonBody, Body: `{"file":"` + strings.Repeat("a", 3<<20) + `"}`},
		{Name: "admin restore preview above its limit", Method: "POST", Path: "/api/v1/admin/restore/preview", Credentials: adminCredentials, Headers: jsonBody, Body: `{"file":"` + strings.Repeat("a", 3800<<10) + `"}`},
		{Name: "admin restore preview above the global limit", Method: "POST", Path: "/api/v1/admin/restore/preview", Credentials: adminCredentials, Headers: jsonBody, Body: `{"file":"` + strings.Repeat("a", 5<<20) + `"}`},
		{Name: "admin unknown path", Method: "GET", Path: "/api/v1/admin/nothing-here", Credentials: adminCredentials},
	}
	return cases
}

// rawContractResponse sends the request line as it is and reads the answer
func rawContractResponse(base string, testCase contractCase) (*http.Response, error) {
	address := strings.TrimPrefix(base, "http://")
	connection, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = io.WriteString(connection, testCase.Method+" "+testCase.Path+" HTTP/1.1\r\nHost: "+address+"\r\nConnection: close\r\n\r\n"); err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(connection), nil)
}

func runContractCase(t *testing.T, client *http.Client, base string, testCase contractCase) contractAnswer {
	t.Helper()
	if testCase.RawPath {
		response, err := rawContractResponse(base, testCase)
		if err != nil {
			return contractAnswer{Status: -1, Body: "connection error"}
		}
		return contractAnswerOf(response, testCase)
	}
	var body io.Reader
	if len(testCase.Body) > 0 {
		body = strings.NewReader(testCase.Body)
	}
	request, err := http.NewRequest(testCase.Method, base+testCase.Path, body)
	if err != nil {
		t.Fatalf("%s: %v", testCase.Name, err)
	}
	for name, value := range testCase.Headers {
		request.Header.Set(name, value)
	}
	if user, password, found := strings.Cut(testCase.Credentials, ":"); found {
		request.SetBasicAuth(user, password)
	}
	response, err := client.Do(request)
	if err != nil {
		// A server may close the connection while refusing a body that is too large: that is part of the contract too
		return contractAnswer{Status: -1, Body: "connection error"}
	}
	return contractAnswerOf(response, testCase)
}

func contractAnswerOf(response *http.Response, testCase contractCase) contractAnswer {
	defer response.Body.Close()
	raw, readErr := io.ReadAll(response.Body)
	answer := contractAnswer{Status: response.StatusCode, Headers: map[string][]string{}}
	for _, name := range contractHeaders {
		values := response.Header.Values(name)
		if len(values) == 0 {
			continue
		}
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			if name == "Retry-After" {
				value = "<seconds>"
			}
			normalized = append(normalized, normalizeContract(value))
		}
		answer.Headers[name] = normalized
	}
	if readErr != nil {
		answer.Body = "error reading the body: " + readErr.Error()
		return answer
	}
	if !testCase.OnlyStatusAndHeaders {
		answer.Body = normalizeContract(string(raw))
	}
	return answer
}

func TestHTTPContract(t *testing.T) {
	base := setupContract(t)
	client := &http.Client{
		Timeout: 30 * time.Second,
		// The contract is what the server answers, not what a client makes of it
		Transport:     &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	cases := contractCases()
	actual := make(map[string]contractAnswer, len(cases))
	order := make([]string, 0, len(cases))
	for _, testCase := range cases {
		if _, duplicated := actual[testCase.Name]; duplicated {
			t.Fatalf("duplicated case name %q", testCase.Name)
		}
		actual[testCase.Name] = runContractCase(t, client, base, testCase)
		order = append(order, testCase.Name)
	}
	if *updateContract {
		encoded, err := json.MarshalIndent(actual, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(contractGoldenFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(contractGoldenFile, append(encoded, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d cases in %s", len(actual), contractGoldenFile)
		return
	}
	raw, err := os.ReadFile(contractGoldenFile)
	if err != nil {
		t.Fatalf("no recorded contract, run with -update-contract: %v", err)
	}
	var expected map[string]contractAnswer
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for _, name := range order {
		want, recorded := expected[name]
		if !recorded {
			t.Errorf("%s: not in the recorded contract, run with -update-contract", name)
			continue
		}
		got := actual[name]
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if !bytes.Equal(wantJSON, gotJSON) {
			t.Errorf("%s: the answer changed\n  recorded: %s\n  now:      %s", name, truncateContract(wantJSON), truncateContract(gotJSON))
		}
	}
	var stale []string
	for name := range expected {
		if _, exists := actual[name]; !exists {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("recorded cases that no longer exist: %s", strings.Join(stale, ", "))
	}
}

func truncateContract(value []byte) string {
	const maximum = 6000
	if len(value) > maximum {
		return string(value[:maximum]) + "…"
	}
	return string(value)
}
