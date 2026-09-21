// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// fakeStatusPageStore delegates to a real store, optionally calling onCreate before a creation or failing it
type fakeStatusPageStore struct {
	store.ManagedStatusPageStore
	onCreate  func()
	createErr error
}

func (fake *fakeStatusPageStore) CreateManagedStatusPage(page *common.ManagedStatusPage, apply func() error) error {
	if fake.onCreate != nil {
		fake.onCreate()
	}
	if fake.createErr != nil {
		return fake.createErr
	}
	return fake.ManagedStatusPageStore.CreateManagedStatusPage(page, apply)
}

func setupServiceTest(t *testing.T) (*Service, store.ManagedStatusPageStore) {
	t.Helper()
	managedStatusPageStore := initializeSQLiteStore(t)
	cfg := loadTestConfig(t, testConfig)
	if _, err := managedendpoint.Load(cfg); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	t.Cleanup(publicCache.Clear)
	return NewService(), managedStatusPageStore
}

func replaceManagedStatusPageStore(t *testing.T, replacement func() (store.ManagedStatusPageStore, bool)) {
	t.Helper()
	previous := getManagedStatusPageStore
	getManagedStatusPageStore = replacement
	t.Cleanup(func() { getManagedStatusPageStore = previous })
}

func TestService_Lifecycle(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	detail, err := service.Create([]byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), "ops@example.com")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	if detail.Version != 1 || detail.Origin != OriginAdmin || detail.Enabled || detail.Published || detail.Path != "/status/apps" || detail.Endpoints != 1 {
		t.Errorf("expected a disabled managed page at version 1, got %+v", detail.Item)
	}
	if stored, _ := managedStatusPageStore.GetManagedStatusPage("apps"); stored == nil || !strings.Contains(stored.Definition, "enabled: false") || stored.UpdatedBy != "ops@example.com" {
		t.Errorf("expected the definition to be stored disabled, got %+v", stored)
	}
	if _, published := Lookup("apps"); published {
		t.Error("expected a page created without enabled not to be published")
	}
	revision := current.Load().revision
	if detail, err = service.SetEnabled("apps", true, 1, "ops@example.com"); err != nil || detail.Version != 2 || !detail.Published {
		t.Fatalf("expected the page to be enabled at version 2, got %+v (err=%v)", detail, err)
	}
	if current.Load().revision <= revision {
		t.Error("expected a new revision after enabling the page")
	}
	body, err := PublicPage("apps")
	if err != nil || !strings.Contains(string(body), `"title":"Apps"`) {
		t.Fatalf("expected the enabled page to be public, got %s (err=%v)", body, err)
	}
	if _, err = service.Update("apps", []byte("slug: apps\ntitle: Aplicações\ngroups: [core]\nenabled: true\n"), 2, "ops@example.com"); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	if body, _ := PublicPage("apps"); !strings.Contains(string(body), "Aplicações") {
		t.Errorf("expected the next request to see the update immediately, got %s", body)
	}
	scenarios := []struct {
		name        string
		err         error
		expectedErr error
	}{
		{name: "old-version", err: errorOf(service.Update("apps", []byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), 1, "")), expectedErr: common.ErrManagedStatusPageVersionMismatch},
		{name: "slug-changed", err: errorOf(service.Update("apps", []byte("slug: other\ntitle: Apps\ngroups: [core]\n"), 3, "")), expectedErr: ErrSlugChanged},
		{name: "configuration-file-page", err: errorOf(service.Update("infra", []byte("slug: infra\ntitle: Infra\ngroups: [core]\n"), 1, "")), expectedErr: ErrReadOnly},
		{name: "unknown-page", err: errorOf(service.Update("missing", []byte("slug: missing\ntitle: Missing\ngroups: [core]\n"), 1, "")), expectedErr: ErrPageNotFound},
		{name: "managed-slug-in-use", err: errorOf(service.Create([]byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), "")), expectedErr: ErrSlugInUse},
		{name: "configuration-file-slug-in-use", err: errorOf(service.Create([]byte("slug: infra\ntitle: Infra\ngroups: [core]\n"), "")), expectedErr: ErrSlugInUse},
		{name: "reserved-slug", err: errorOf(service.Create([]byte("slug: exposure\ntitle: Exposure\ngroups: [core]\n"), "")), expectedErr: pageconfig.ErrReservedSlug},
		{name: "unknown-field", err: errorOf(service.Create([]byte("slug: typo\ntitle: Typo\ngroup: [core]\n"), "")), expectedErr: ErrInvalidDefinition},
		{name: "delete-configuration-file-page", err: service.Delete("infra", 1, ""), expectedErr: ErrReadOnly},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if !errors.Is(scenario.err, scenario.expectedErr) {
				t.Errorf("expected %v, got %v", scenario.expectedErr, scenario.err)
			}
		})
	}
	if err := service.Delete("apps", 3, "ops@example.com"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}
	if _, published := Lookup("apps"); published || findState("apps") != nil {
		t.Error("expected the deleted page to be unpublished")
	}
	if _, err := PublicPage("apps"); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("expected the deleted page to be not found, got %v", err)
	}
}

func errorOf[T any](_ T, err error) error {
	return err
}

func TestService_PublishesAfterCommit(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	publishedDuringWrite := true
	replaceManagedStatusPageStore(t, func() (store.ManagedStatusPageStore, bool) {
		return &fakeStatusPageStore{ManagedStatusPageStore: managedStatusPageStore, onCreate: func() {
			_, publishedDuringWrite = Lookup("apps")
		}}, true
	})
	if _, err := service.Create([]byte("slug: apps\ntitle: Apps\ngroups: [core]\nenabled: true\n"), ""); err != nil {
		t.Fatal(err)
	}
	if publishedDuringWrite {
		t.Error("expected the page not to be published before the write")
	}
	if _, published := Lookup("apps"); !published {
		t.Error("expected the page to be published after the commit")
	}
	revision := current.Load().revision
	replaceManagedStatusPageStore(t, func() (store.ManagedStatusPageStore, bool) {
		return &fakeStatusPageStore{ManagedStatusPageStore: managedStatusPageStore, createErr: errors.New("disk full")}, true
	})
	if _, err := service.Create([]byte("slug: failed\ntitle: Failed\ngroups: [core]\nenabled: true\n"), ""); err == nil {
		t.Fatal("expected the failing write to return an error")
	}
	if findState("failed") != nil || current.Load().revision != revision {
		t.Error("expected nothing to be published when the write fails")
	}
	replaceManagedStatusPageStore(t, func() (store.ManagedStatusPageStore, bool) { return nil, false })
	if _, err := service.Create([]byte("slug: memory\ntitle: Memory\ngroups: [core]\n"), ""); !errors.Is(err, ErrStorageNotSupported) {
		t.Errorf("expected ErrStorageNotSupported, got %v", err)
	}
}

func TestService_CycleInProgress(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	lifecycle.BeginCycle()
	_, err := service.Create([]byte("slug: apps\ntitle: Apps\ngroups: [core]\n"), "")
	lifecycle.EndCycle()
	if !errors.Is(err, ErrCycleInProgress) {
		t.Fatalf("expected ErrCycleInProgress, got %v", err)
	}
	if _, err := managedStatusPageStore.GetManagedStatusPage("apps"); !errors.Is(err, common.ErrManagedStatusPageNotFound) {
		t.Errorf("expected nothing to be stored during a cycle, got %v", err)
	}
}

func TestService_Queries(t *testing.T) {
	service, _ := setupServiceTest(t)
	if _, err := service.Create([]byte("slug: jobs\ntitle: Jobs\nendpoints: [jobs_batch]\nenabled: true\n"), ""); err != nil {
		t.Fatal(err)
	}
	exposure, err := service.Exposure(" core ", "")
	if err != nil || len(exposure.StatusPages) != 2 {
		t.Fatalf("expected the core group on the hidden and infra pages, got %+v (err=%v)", exposure, err)
	}
	published := map[string]bool{}
	for _, item := range exposure.StatusPages {
		published[item.Slug] = item.Published
		if item.Reason != "group" {
			t.Errorf("expected the reason to be group, got %+v", item)
		}
	}
	if !published["infra"] || published["hidden"] {
		t.Errorf("unexpected publication of the exposed pages: %v", published)
	}
	if exposure, _ := service.Exposure("", "JOBS_BATCH"); len(exposure.StatusPages) != 1 || exposure.StatusPages[0].Slug != "jobs" || exposure.StatusPages[0].Reason != "key" {
		t.Errorf("expected the jobs page by key, got %+v", exposure)
	}
	if _, err := service.Exposure(" ", ""); !errors.Is(err, ErrExposureQueryRequired) {
		t.Errorf("expected ErrExposureQueryRequired, got %v", err)
	}
	options := service.Options()
	if len(options.Groups) != 2 || options.Groups[0].Name != "core" || options.Groups[0].Endpoints != 1 || len(options.Endpoints) != 2 {
		t.Errorf("expected the core and jobs groups with the enabled endpoints, got %+v", options)
	}
	validation, err := service.Validate([]byte("slug: new-page\ntitle: New\ngroups: [core, missing]\nendpoints: [nope]\n"), "")
	if err != nil || len(validation.Warnings) != 2 || validation.Endpoints != 1 || validation.Definition.Enabled == nil || *validation.Definition.Enabled {
		t.Errorf("expected 2 warnings, 1 endpoint and a disabled definition, got %+v (err=%v)", validation, err)
	}
	if _, err := service.Validate([]byte("slug: jobs\ntitle: Jobs\nendpoints: [jobs_batch]\n"), "jobs"); err != nil {
		t.Errorf("expected the validation of an update not to conflict with the page itself, got %v", err)
	}
	body, err := service.Preview("hidden")
	var payload Payload
	if err != nil || json.Unmarshal(body, &payload) != nil || payload.Slug != "hidden" {
		t.Errorf("expected the preview of the disabled page, got %s (err=%v)", body, err)
	}
	if _, err := service.Preview("missing"); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("expected ErrPageNotFound, got %v", err)
	}
	listing := service.List()
	if !listing.PublicationEnabled || listing.ManagedUnavailable || len(listing.StatusPages) != 3 {
		t.Errorf("unexpected listing: %+v", listing)
	}
}

func TestService_DeleteManagedPageInConflict(t *testing.T) {
	service, managedStatusPageStore := setupServiceTest(t)
	createStoredPage(t, managedStatusPageStore, "infra", "slug: infra\ntitle: Managed infra\ngroups: [core]\nenabled: true\n")
	cfg := loadTestConfig(t, testConfig)
	Load(cfg)
	detail, err := service.Get("infra")
	if err != nil || !detail.Conflict || detail.Origin != OriginAdmin || detail.Title != "Managed infra" {
		t.Fatalf("expected the managed page in conflict, got %+v (err=%v)", detail, err)
	}
	if err := service.Delete("infra", 1, ""); err != nil {
		t.Fatalf("failed to delete the managed page in conflict: %v", err)
	}
	if published, ok := Lookup("infra"); !ok || published.Page.Title != "Infra" {
		t.Error("expected the page of the configuration file to stay published")
	}
}
