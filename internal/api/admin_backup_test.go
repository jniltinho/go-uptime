package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/maintenance"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

const backupTestPassword = "correct horse battery"

type backupTestEnvironment struct {
	app       *echo.Echo
	serverURL string
	close     func()
}

// newBackupTestEnvironment starts an installation with its own SQLite database and the given status pages in its
// configuration file. close must be called before starting another installation, because the registries are global.
func newBackupTestEnvironment(t *testing.T, configPages []*pageconfig.Page) *backupTestEnvironment {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	yamlEndpoint := &endpoint.Endpoint{Name: "api", Group: "core", URL: server.URL, Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := yamlEndpoint.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	enabled, disabled, rateLimit := true, false, 0
	statusPages := &pageconfig.Config{Enabled: &enabled, RateLimit: &rateLimit, Pages: configPages}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Security:    &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}},
		Admin:       &admin.Config{Enabled: true},
		Storage:     &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "gatus.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		Maintenance: &maintenance.Config{Enabled: &disabled},
		Endpoints:   []*endpoint.Endpoint{yamlEndpoint},
		StatusPages: statusPages,
	}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatal(err)
	}
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	pushkey.Load(cfg)
	statuspage.Load(cfg)
	watchdog.Monitor(cfg)
	closed := false
	closeEnvironment := func() {
		if closed {
			return
		}
		closed = true
		watchdog.Shutdown(cfg)
		store.Get().Close()
		_ = store.Initialize(nil)
		_, _ = managedendpoint.Load(&config.Config{})
		pushkey.Load(&config.Config{})
		statuspage.Load(&config.Config{})
		server.Close()
	}
	t.Cleanup(closeEnvironment)
	return &backupTestEnvironment{app: New(cfg).Router(), serverURL: server.URL, close: closeEnvironment}
}

func (env *backupTestEnvironment) request(t *testing.T, method, path, contentType, body string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "status.example.com"
	request.SetBasicAuth("admin", "secret")
	if len(contentType) > 0 {
		request.Header.Set("Content-Type", contentType)
	}
	for name, value := range headers {
		if len(value) == 0 {
			request.Header.Del(name)
		} else {
			request.Header.Set(name, value)
		}
	}
	response, err := testHTTP(env.app, request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response, raw
}

func (env *backupTestEnvironment) expect(t *testing.T, method, path, contentType, body string, expected int) []byte {
	t.Helper()
	response, raw := env.request(t, method, path, contentType, body, nil)
	if response.StatusCode != expected {
		t.Fatalf("%s %s: expected %d, got %d: %s", method, path, expected, response.StatusCode, raw)
	}
	return raw
}

type testPlan struct {
	Summary     map[string]int `json:"summary"`
	Notices     map[string]int `json:"notices"`
	Fingerprint string         `json:"fingerprint"`
	Items       []testPlanItem `json:"items"`
}

type testPlanItem struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Action   string   `json:"action"`
	Reason   string   `json:"reason"`
	Warnings []string `json:"warnings"`
}

func (plan *testPlan) action(itemType, id string) testPlanItem {
	for _, item := range plan.Items {
		if item.Type == itemType && item.ID == id {
			return item
		}
	}
	return testPlanItem{}
}

func restoreBody(file json.RawMessage, password string, overwrite, disableEndpoints bool, fingerprint string) string {
	body := map[string]any{"file": file, "overwrite": overwrite, "disableEndpoints": disableEndpoints}
	if len(password) > 0 {
		body["password"] = password
	}
	if len(fingerprint) > 0 {
		body["fingerprint"] = fingerprint
	}
	encoded, _ := json.Marshal(body)
	return string(encoded)
}

func (env *backupTestEnvironment) preview(t *testing.T, file json.RawMessage, password string, overwrite, disableEndpoints bool) *testPlan {
	t.Helper()
	raw := env.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(file, password, overwrite, disableEndpoints, ""), http.StatusOK)
	var plan testPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	return &plan
}

func TestAdminBackupAndRestore(t *testing.T) {
	source := newBackupTestEnvironment(t, nil)
	const pushToken = "keSDu7G855jvVat1xWiY2Gk4CkL1End5"
	source.expect(t, http.MethodPost, "/api/v1/admin/endpoints", "application/yaml", "type: push\nname: backup\ngroup: jobs\ntoken: "+pushToken+"\n", http.StatusCreated)
	source.expect(t, http.MethodPost, "/api/v1/admin/endpoints", "application/yaml", "name: site\ngroup: web\nenabled: false\ninterval: 1h\nurl: "+source.serverURL+"\nheaders:\n  Authorization: Bearer abc123\nconditions: [\"[STATUS] == 200\"]\n", http.StatusCreated)
	source.expect(t, http.MethodPost, "/api/v1/admin/status-pages", "application/yaml", "slug: jobs\ntitle: Jobs\ngroups: [jobs]\nenabled: true\n", http.StatusCreated)
	created := source.expect(t, http.MethodPost, "/api/v1/admin/push-keys", "application/json", `{"name":"akamai"}`, http.StatusCreated)
	var createdKey struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(created, &createdKey)

	// Protections of the routes
	if response, _ := source.request(t, http.MethodPost, "/api/v1/admin/backup", "", "", map[string]string{"Authorization": ""}); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without authentication, got %d", response.StatusCode)
	}
	if response, _ := source.request(t, http.MethodPost, "/api/v1/admin/backup", "application/json", "{}", map[string]string{"Sec-Fetch-Site": "cross-site"}); response.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for a cross-site request, got %d", response.StatusCode)
	}
	source.expect(t, http.MethodPost, "/api/v1/admin/backup", "application/yaml", "{}", http.StatusUnsupportedMediaType)
	source.expect(t, http.MethodPost, "/api/v1/admin/backup", "", "", http.StatusUnsupportedMediaType)
	source.expect(t, http.MethodPost, "/api/v1/admin/backup", "application/json", `{"password":"short"}`, http.StatusBadRequest)

	// Plain backup
	response, plain := source.request(t, http.MethodPost, "/api/v1/admin/backup", "application/json", "{}", nil)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Disposition"), `attachment; filename="go-uptime-backup-`) || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected backup response: %d %v %s", response.StatusCode, response.Header, plain)
	}
	var backup struct {
		Endpoints []struct {
			Key        string `json:"key"`
			Definition string `json:"definition"`
		} `json:"endpoints"`
		StatusPages []struct {
			Slug string `json:"slug"`
		} `json:"statusPages"`
		PushKeys []struct {
			Name      string `json:"name"`
			TokenHash string `json:"tokenHash"`
		} `json:"pushKeys"`
	}
	if err := json.Unmarshal(plain, &backup); err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256.Sum256([]byte(createdKey.Token))
	if len(backup.Endpoints) != 2 || !strings.Contains(backup.Endpoints[0].Definition, pushToken) || !strings.Contains(backup.Endpoints[1].Definition, "Bearer abc123") || !strings.Contains(backup.Endpoints[1].Definition, "enabled: false") ||
		len(backup.StatusPages) != 1 || len(backup.PushKeys) != 1 || backup.PushKeys[0].TokenHash != hex.EncodeToString(tokenHash[:]) || strings.Contains(string(plain), "core_api") {
		t.Fatalf("unexpected backup: %s", plain)
	}

	// Encrypted backup
	response, encrypted := source.request(t, http.MethodPost, "/api/v1/admin/backup", "application/json", `{"password":"`+backupTestPassword+`"}`, nil)
	if response.StatusCode != http.StatusOK || !strings.HasSuffix(response.Header.Get("Content-Disposition"), `.enc.json"`) || strings.Contains(string(encrypted), pushToken) || strings.Contains(string(encrypted), "abc123") {
		t.Fatalf("unexpected encrypted backup: %d %v", response.StatusCode, response.Header)
	}

	// Backup during a reload
	lifecycle.BeginCycle()
	source.expect(t, http.MethodPost, "/api/v1/admin/backup", "application/json", "{}", http.StatusServiceUnavailable)
	lifecycle.EndCycle()
	source.close()

	// Another installation, whose configuration file has the status page jobs
	target := newBackupTestEnvironment(t, []*pageconfig.Page{{Slug: "jobs", Title: "Jobs of the file", Groups: []string{"jobs"}}})
	plan := target.preview(t, plain, "", false, false)
	if plan.action("pushKey", "akamai").Action != "create" || plan.action("endpoint", "jobs_backup").Action != "create" || plan.action("endpoint", "web_site").Action != "create" ||
		plan.action("statusPage", "jobs").Action != "skip" || !strings.Contains(plan.action("statusPage", "jobs").Reason, "configuration file") || plan.Notices["monitoringStarts"] != 1 || len(plan.Fingerprint) != 64 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if managedendpoint.Get("jobs_backup") != nil {
		t.Fatal("the preview must not create anything")
	}
	// The same plan twice has the same fingerprint
	if again := target.preview(t, plain, "", false, false); again.Fingerprint != plan.Fingerprint {
		t.Error("expected a deterministic fingerprint")
	}
	target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(plain, "", false, false, strings.Repeat("0", 64)), http.StatusConflict)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(plain, "", true, false, plan.Fingerprint), http.StatusConflict)
	raw := target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(plain, "", false, false, plan.Fingerprint), http.StatusOK)
	var result struct {
		Summary map[string]int `json:"summary"`
	}
	_ = json.Unmarshal(raw, &result)
	if result.Summary["created"] != 3 || result.Summary["skipped"] != 1 || result.Summary["failed"] != 0 {
		t.Fatalf("unexpected result: %s", raw)
	}
	if name, ok := pushkey.Lookup(createdKey.Token); !ok || name != "akamai" {
		t.Error("expected the restored push key to accept the original token")
	}
	if state := managedendpoint.Get("web_site"); state == nil || state.Parsed().IsEnabled() || !strings.Contains(state.Stored.Definition, "Bearer abc123") {
		t.Errorf("expected web_site restored disabled with its secret, got %+v", state)
	}
	// The same restore again changes nothing
	again := target.preview(t, plain, "", true, false)
	for _, id := range []string{"akamai", "jobs_backup", "web_site"} {
		itemType := "endpoint"
		if id == "akamai" {
			itemType = "pushKey"
		}
		if item := again.action(itemType, id); item.Action != "unchanged" {
			t.Errorf("expected %s to be unchanged, got %+v", id, item)
		}
	}

	// Encrypted file: password required, wrong password, right password
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(encrypted, "", false, false, ""), http.StatusBadRequest)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(encrypted, "wrong password!!", false, false, ""), http.StatusBadRequest)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(plain, backupTestPassword, false, false, ""), http.StatusBadRequest)
	if decrypted := target.preview(t, encrypted, backupTestPassword, false, false); decrypted.action("endpoint", "web_site").Action != "unchanged" {
		t.Errorf("expected the encrypted backup to be read, got %+v", decrypted)
	}

	// Changes: overwrite, masked secret, push key with the hash of the token of an endpoint, disabled endpoints
	edited := strings.Replace(string(plain), "interval: 1h", "interval: 2h", 1)
	edited = strings.Replace(edited, "Bearer abc123", "'********'", 1)
	if plan := target.preview(t, json.RawMessage(edited), "", true, false); !strings.Contains(plan.action("endpoint", "web_site").Reason, "masked secret") {
		t.Errorf("expected the masked secret to be skipped, got %+v", plan.action("endpoint", "web_site"))
	}
	edited = strings.Replace(string(plain), "interval: 1h", "interval: 2h", 1)
	if plan := target.preview(t, json.RawMessage(edited), "", false, false); plan.action("endpoint", "web_site").Reason != "already exists" {
		t.Errorf("expected already exists without overwrite, got %+v", plan.action("endpoint", "web_site"))
	}
	if plan := target.preview(t, json.RawMessage(edited), "", true, false); plan.action("endpoint", "web_site").Action != "update" {
		t.Errorf("expected an update with overwrite, got %+v", plan.action("endpoint", "web_site"))
	}
	endpointTokenHash := sha256.Sum256([]byte(pushToken))
	forged := strings.Replace(string(plain), `"name": "akamai"`, `"name": "forged"`, 1)
	forged = strings.Replace(forged, hex.EncodeToString(tokenHash[:]), hex.EncodeToString(endpointTokenHash[:]), 1)
	forged = strings.Replace(forged, `name: backup\n`, `name: other\n`, 1)
	forged = strings.Replace(forged, `"key": "jobs_backup"`, `"key": "jobs_other"`, 1)
	if plan := target.preview(t, json.RawMessage(forged), "", false, false); plan.action("pushKey", "forged").Reason != pushkey.ErrHashInUse.Error() || !strings.Contains(plan.action("endpoint", "jobs_other").Reason, "push token") {
		t.Errorf("expected the forged push key and the repeated token to be skipped, got %+v", plan.Items)
	}
	newEndpoint := strings.Replace(string(plain), `"key": "web_site"`, `"key": "web_new"`, 1)
	newEndpoint = strings.Replace(newEndpoint, `name: site\n`, `name: new\n`, 1)
	newEndpoint = strings.Replace(newEndpoint, `enabled: false\n`, ``, 1)
	disabledPlan := target.preview(t, json.RawMessage(newEndpoint), "", false, true)
	if disabledPlan.action("endpoint", "web_new").Action != "create" || disabledPlan.Notices["monitoringStarts"] != 0 {
		t.Fatalf("expected web_new to be created disabled, got %+v", disabledPlan)
	}
	target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(json.RawMessage(newEndpoint), "", false, true, disabledPlan.Fingerprint), http.StatusOK)
	if state := managedendpoint.Get("web_new"); state == nil || state.Parsed().IsEnabled() || watchdog.IsEndpointMonitored("web_new") {
		t.Errorf("expected web_new to be created without monitoring, got %+v", state)
	}

	// A restore that starts during a reload skips its items
	reloadFile := strings.Replace(newEndpoint, `"key": "web_new"`, `"key": "web_later"`, 1)
	reloadFile = strings.Replace(reloadFile, `name: new\n`, `name: later\n`, 1)
	reloadPlan := target.preview(t, json.RawMessage(reloadFile), "", false, false)
	lifecycle.BeginCycle()
	raw = target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(json.RawMessage(reloadFile), "", false, false, reloadPlan.Fingerprint), http.StatusOK)
	lifecycle.EndCycle()
	if !strings.Contains(string(raw), "configuration reload in progress") || managedendpoint.Get("web_later") != nil {
		t.Errorf("expected the items to be skipped during a reload, got %s", raw)
	}

	// Bodies: larger than the limit of the administration but accepted by the restore, and above the limit of the restore
	padded := strings.Replace(string(plain), `"createdBy"`, strings.Repeat(" ", 300*1024)+`"createdBy"`, 1)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(json.RawMessage(padded), "", false, false, ""), http.StatusOK)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/yaml", restoreBody(plain, "", false, false, ""), http.StatusUnsupportedMediaType)
	tooLarge := `{"file":` + string(plain) + `,"overwrite":false` + strings.Repeat(" ", adminRestoreMaximumBodySize) + `}`
	target.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", tooLarge, http.StatusRequestEntityTooLarge)
}

func TestAdminRestorePasswordLimiter(t *testing.T) {
	previous := restorePasswordLimiter
	restorePasswordLimiter = security.NewFailureLimiter(restorePasswordWindow, 2, 100)
	defer func() { restorePasswordLimiter = previous }()
	env := newBackupTestEnvironment(t, nil)
	_, encrypted := env.request(t, http.MethodPost, "/api/v1/admin/backup", "application/json", `{"password":"`+backupTestPassword+`"}`, nil)
	for i := 0; i < 2; i++ {
		env.expect(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(encrypted, "wrong password!!", false, false, ""), http.StatusBadRequest)
	}
	response, _ := env.request(t, http.MethodPost, "/api/v1/admin/restore/preview", "application/json", restoreBody(encrypted, backupTestPassword, false, false, ""), nil)
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") == "" {
		t.Errorf("expected 429 with Retry-After after the wrong passwords, got %d", response.StatusCode)
	}
	// The login is not blocked by the wrong passwords of the restores
	env.expect(t, http.MethodGet, "/api/v1/admin/endpoints", "", "", http.StatusOK)
}

// TestAdminBackupAndRestore_GroupsCollapsed carries the groups-collapsed option of a status page through a backup and a
// restore: the backup stores the whole definition, so the format does not change, and the restored page publishes it.
func TestAdminBackupAndRestore_GroupsCollapsed(t *testing.T) {
	source := newBackupTestEnvironment(t, nil)
	source.expect(t, http.MethodPost, "/api/v1/admin/endpoints", "application/yaml", "name: site\ngroup: web\nenabled: false\ninterval: 1h\nurl: "+source.serverURL+"\nconditions:\n  - \"[STATUS] == 200\"\n", http.StatusCreated)
	source.expect(t, http.MethodPost, "/api/v1/admin/status-pages", "application/yaml", "slug: web\ntitle: Web\ngroups: [web]\nenabled: true\ngroups-collapsed: true\n", http.StatusCreated)
	_, plain := source.request(t, http.MethodPost, "/api/v1/admin/backup", "application/json", "{}", nil)
	if !strings.Contains(string(plain), "groups-collapsed: true") {
		t.Fatalf("expected the definition of the backup to carry groups-collapsed, got %s", plain)
	}

	target := newBackupTestEnvironment(t, nil)
	plan := target.preview(t, plain, "", false, false)
	target.expect(t, http.MethodPost, "/api/v1/admin/restore", "application/json", restoreBody(plain, "", false, false, plan.Fingerprint), http.StatusOK)
	detail := target.expect(t, http.MethodGet, "/api/v1/admin/status-pages/web", "", "", http.StatusOK)
	if !strings.Contains(string(detail), "groups-collapsed") {
		t.Errorf("expected the restored definition to carry groups-collapsed, got %s", detail)
	}
	public := target.expect(t, http.MethodGet, "/api/v1/status-pages/web", "", "", http.StatusOK)
	if !strings.Contains(string(public), `"groupsCollapsed":true`) {
		t.Errorf("expected the restored page to publish groupsCollapsed, got %s", public)
	}
}
