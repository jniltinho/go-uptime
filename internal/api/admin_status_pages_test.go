package api

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/admin"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"

	"golang.org/x/crypto/bcrypt"
)

func newAdminStatusPageEnvironment(t *testing.T, statusPagesEnabled bool) *adminTestEnvironment {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	ep := &endpoint.Endpoint{Name: "api", Group: "core", URL: "https://example.org", Interval: time.Hour, Conditions: []endpoint.Condition{"[STATUS] == 200"}}
	if err := ep.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	rateLimit := 0
	statusPages := &pageconfig.Config{Enabled: &statusPagesEnabled, RateLimit: &rateLimit, Pages: []*pageconfig.Page{{Slug: "infra", Title: "Infra", Groups: []string{"core"}}}}
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Security:    &security.Config{Basic: &security.BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: base64.URLEncoding.EncodeToString(hash)}},
		Admin:       &admin.Config{Enabled: true},
		Storage:     &storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "gatus.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50},
		Endpoints:   []*endpoint.Endpoint{ep},
		StatusPages: statusPages,
	}
	if err := store.Initialize(cfg.Storage); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() {
		store.Get().Close()
		_ = store.Initialize(nil)
		_, _ = managedendpoint.Load(&config.Config{})
		statuspage.Load(&config.Config{})
	})
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	statuspage.Load(cfg)
	return &adminTestEnvironment{app: New(cfg).Router()}
}

func TestAdminStatusPagesAPI(t *testing.T) {
	env := newAdminStatusPageEnvironment(t, true)
	const apps = "slug: apps\ntitle: Apps\ngroups: [core]\n"
	publicStatus := func(t *testing.T, slug string) int {
		t.Helper()
		status, _, _ := env.do(t, http.MethodGet, "/api/v1/status-pages/"+slug, "", map[string]string{"Authorization": ""})
		return status
	}

	t.Run("requires-authentication", func(t *testing.T) {
		if status, _, _ := env.do(t, http.MethodGet, "/api/v1/admin/status-pages", "", map[string]string{"Authorization": ""}); status != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", status)
		}
	})
	t.Run("create-disabled", func(t *testing.T) {
		header, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", apps, nil, http.StatusCreated)
		if header.Get("ETag") != `"1"` || body["enabled"] != false || body["published"] != false || body["origin"] != "admin" {
			t.Errorf("unexpected creation: etag=%s body=%v", header.Get("ETag"), body)
		}
		if status := publicStatus(t, "apps"); status != http.StatusNotFound {
			t.Errorf("expected the disabled page not to be public, got %d", status)
		}
	})
	t.Run("create-rejections", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", apps, nil, http.StatusConflict)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: infra\ntitle: Infra\ngroups: [core]\n", nil, http.StatusConflict)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: typo\ntitle: Typo\ngroup: [core]\n", nil, http.StatusBadRequest)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: options\ntitle: Options\ngroups: [core]\n", nil, http.StatusBadRequest)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", `{"slug":"json-page","title":"JSON","groups":["core"]}`, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", apps+"description: "+strings.Repeat("a", 300*1024)+"\n", nil, http.StatusRequestEntityTooLarge)
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: csrf\ntitle: CSRF\ngroups: [core]\n", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden)
	})
	t.Run("create-json", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", `{"slug":"json-page","title":"JSON","groups":["core"],"enabled":true}`, map[string]string{"Content-Type": "application/json"}, http.StatusCreated)
		if status := publicStatus(t, "json-page"); status != http.StatusOK {
			t.Errorf("expected the page created enabled to be public, got %d", status)
		}
	})
	t.Run("update-rejections", func(t *testing.T) {
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/apps", apps, nil, http.StatusPreconditionRequired)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/apps", apps, map[string]string{"If-Match": `"7"`}, http.StatusPreconditionFailed)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/apps", strings.Replace(apps, "slug: apps", "slug: other", 1), map[string]string{"If-Match": `"1"`}, http.StatusBadRequest)
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/infra", "slug: infra\ntitle: Infra\ngroups: [core]\n", map[string]string{"If-Match": `"1"`}, http.StatusConflict)
		env.expectStatus(t, http.MethodDelete, "/api/v1/admin/status-pages/infra", "", map[string]string{"If-Match": `"1"`}, http.StatusConflict)
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/missing", "", nil, http.StatusNotFound)
	})
	t.Run("enable-and-update", func(t *testing.T) {
		header, body := env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages/apps/enable", "", map[string]string{"If-Match": `"1"`}, http.StatusOK)
		if header.Get("ETag") != `"2"` || body["published"] != true {
			t.Errorf("unexpected enable response: etag=%s body=%v", header.Get("ETag"), body)
		}
		if status := publicStatus(t, "apps"); status != http.StatusOK {
			t.Errorf("expected the enabled page to be public, got %d", status)
		}
		env.expectStatus(t, http.MethodPut, "/api/v1/admin/status-pages/apps", "slug: apps\ntitle: Aplicações\ngroups: [core]\nenabled: true\n", map[string]string{"If-Match": `"2"`}, http.StatusOK)
		_, body = env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/apps", "", nil, http.StatusOK)
		if body["title"] != "Aplicações" || !strings.Contains(body["yaml"].(string), "Aplicações") {
			t.Errorf("expected the updated definition, got %v", body)
		}
	})
	t.Run("queries", func(t *testing.T) {
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages", "", nil, http.StatusOK)
		if body["publicationEnabled"] != true || len(body["statusPages"].([]any)) != 3 {
			t.Errorf("unexpected list: %v", body)
		}
		_, body = env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/options", "", nil, http.StatusOK)
		if len(body["groups"].([]any)) != 1 || len(body["endpoints"].([]any)) != 1 {
			t.Errorf("unexpected options: %v", body)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/exposure", "", nil, http.StatusBadRequest)
		_, body = env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/exposure?group=core", "", nil, http.StatusOK)
		if len(body["statusPages"].([]any)) != 3 {
			t.Errorf("expected the core group on the 3 pages, got %v", body)
		}
		_, body = env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages/validate", "slug: draft\ntitle: Draft\ngroups: [missing]\n", nil, http.StatusOK)
		if len(body["warnings"].([]any)) != 1 {
			t.Errorf("expected a warning for the missing group, got %v", body)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/draft", "", nil, http.StatusNotFound)
	})
	t.Run("preview-disabled-page", func(t *testing.T) {
		env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: draft\ntitle: Draft\ngroups: [core]\n", nil, http.StatusCreated)
		_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/draft/preview", "", nil, http.StatusOK)
		if body["slug"] != "draft" || len(body["groups"].([]any)) != 1 {
			t.Errorf("expected the preview of the disabled page, got %v", body)
		}
	})
	t.Run("cycle-in-progress", func(t *testing.T) {
		lifecycle.BeginCycle()
		status, _, _ := env.do(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: during-cycle\ntitle: Cycle\ngroups: [core]\n", nil)
		lifecycle.EndCycle()
		if status != http.StatusServiceUnavailable {
			t.Errorf("expected 503 during a cycle, got %d", status)
		}
		env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/during-cycle", "", nil, http.StatusNotFound)
	})
	t.Run("delete", func(t *testing.T) {
		env.expectStatus(t, http.MethodDelete, "/api/v1/admin/status-pages/apps", "", map[string]string{"If-Match": `"2"`}, http.StatusPreconditionFailed)
		env.expectStatus(t, http.MethodDelete, "/api/v1/admin/status-pages/apps", "", map[string]string{"If-Match": `"3"`}, http.StatusOK)
		if status := publicStatus(t, "apps"); status != http.StatusNotFound {
			t.Errorf("expected the deleted page not to be public, got %d", status)
		}
	})
}

func TestAdminStatusPagesAPI_PublicationDisabled(t *testing.T) {
	env := newAdminStatusPageEnvironment(t, false)
	_, body := env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages", "", nil, http.StatusOK)
	if body["publicationEnabled"] != false {
		t.Errorf("expected publicationEnabled to be false, got %v", body)
	}
	env.expectStatus(t, http.MethodGet, "/api/v1/admin/status-pages/infra/preview", "", nil, http.StatusOK)
	env.expectStatus(t, http.MethodPost, "/api/v1/admin/status-pages", "slug: apps\ntitle: Apps\ngroups: [core]\nenabled: true\n", nil, http.StatusCreated)
	if status, _, _ := env.do(t, http.MethodGet, "/api/v1/status-pages/apps", "", map[string]string{"Authorization": ""}); status != http.StatusNotFound {
		t.Errorf("expected no public page with status-pages.enabled set to false, got %d", status)
	}
}
