// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// Origin is where a status page is defined
type Origin string

const (
	// OriginConfig is a status page of the configuration file, read-only in the administration
	OriginConfig Origin = "config"

	// OriginAdmin is a status page managed through the administration API
	OriginAdmin Origin = "admin"
)

// State is the runtime state of a status page
type State struct {
	Origin Origin

	// Slug is the slug of the page, also set when the page is invalid
	Slug string

	// Page is the validated page, or nil if the managed status page is invalid. It must not be modified.
	Page *pageconfig.Page

	// Stored is the persisted managed status page, for OriginAdmin
	Stored *common.ManagedStatusPage

	// ConflictOrigin describes what uses the same slug in the configuration file, when the managed status page is in
	// conflict
	ConflictOrigin string

	// Err is the validation error, when the managed status page is invalid
	Err error
}

// InConflict returns whether the slug of the managed status page is used by the configuration file
func (state *State) InConflict() bool {
	return len(state.ConflictOrigin) > 0
}

// IsPublished returns whether the page is valid, not in conflict and enabled. Managed status pages without enabled are
// not published.
func (state *State) IsPublished() bool {
	if state.Page == nil || state.InConflict() {
		return false
	}
	if state.Origin == OriginAdmin {
		return state.Page.Enabled != nil && *state.Page.Enabled
	}
	return state.Page.IsEnabled()
}

// Published is a published status page captured from a snapshot
type Published struct {
	Page *pageconfig.Page

	// Revision changes every time the published status pages change, and identifies the definition of Page
	Revision uint64

	// Generation changes every time the status pages are loaded (start and reload)
	Generation uint64

	// MaximumResults is the number of latest results shown for each endpoint
	MaximumResults int

	// MaximumEndpoints is how many endpoints the page shows (status-pages.maximum-endpoints-per-page), captured with
	// the page: it is the limit to give to Select for anything that is decided or assembled for this Published
	MaximumEndpoints int
}

// MaximumPublicResults is the maximum number of latest results shown for each endpoint of a public status page
const MaximumPublicResults = 50

type snapshot struct {
	revision           uint64
	generation         uint64
	maximumResults     int
	maximumEndpoints   int
	enabled            bool
	managedUnavailable bool
	configStates       map[string]*State
	managedStates      map[string]*State
	configEndpoints    []EndpointRef
}

var (
	// current is an immutable snapshot of the status pages, replaced on every change (copy-on-write)
	current atomic.Pointer[snapshot]

	// mutex serializes the changes of the snapshot
	mutex sync.Mutex

	// revisions is incremented on every publication, so that a revision is never reused
	revisions atomic.Uint64

	// generations is incremented on every Load
	generations atomic.Uint64
)

// Load publishes the status pages of cfg and the managed status pages persisted in the storage, replacing the previous
// ones. It must be called after managedendpoint.Load, whose endpoints can be selected by the pages.
//
// It does not depend on admin.enabled: disabling the administration does not unpublish the managed status pages. If
// the managed status pages cannot be listed, only the pages of the configuration file are published.
func Load(cfg *config.Config) {
	mutex.Lock()
	defer mutex.Unlock()
	next := &snapshot{
		generation:       generations.Add(1),
		maximumResults:   maximumPublicResults(cfg),
		maximumEndpoints: cfg.StatusPages.GetMaximumEndpointsPerPage(),
		enabled:          cfg.StatusPages.IsEnabled(),
		configStates:     make(map[string]*State),
		managedStates:    make(map[string]*State),
		configEndpoints:  configEndpointRefs(cfg),
	}
	if cfg.StatusPages != nil {
		for _, page := range cfg.StatusPages.Pages {
			next.configStates[page.Slug] = &State{Origin: OriginConfig, Slug: page.Slug, Page: page}
		}
	}
	if managedStatusPageStore, ok := store.GetManagedStatusPageStore(); ok {
		storedPages, err := managedStatusPageStore.ListManagedStatusPages()
		if err != nil {
			next.managedUnavailable = true
			logr.Errorf("[statuspage.Load] Failed to load managed status pages, so only the status pages of the configuration file are published: %s", err.Error())
		}
		for _, stored := range storedPages {
			next.managedStates[stored.Slug] = newManagedState(stored, next.configStates)
		}
	}
	publish(next)
	// The cache keys include the revision and the generation, so this only frees memory
	publicCache.Clear()
	// Fork: a load may change the credential of a page, so the remembered verifications and the counted failures go too
	resetAuthState()
	logPublished(next)
	warnAboutLoginWithoutSecurity(cfg, next)
}

// warnAboutLoginWithoutSecurity warns when a page requires a login while the installation has no security: the page is
// protected, but /api/v1/endpoints/statuses and the badges by key stay open and publish more than the page shows (fork)
func warnAboutLoginWithoutSecurity(cfg *config.Config, snap *snapshot) {
	if !snap.enabled || cfg.Security != nil {
		return
	}
	var slugs []string
	for _, state := range append(sortedStates(snap.configStates), sortedStates(snap.managedStates)...) {
		if state.IsPublished() && state.Page.RequiresLogin() {
			slugs = append(slugs, state.Slug)
		}
	}
	if len(slugs) > 0 {
		logr.Warnf("[statuspage.Load] Status pages with a login of their own (%s) while the installation has no security: the dashboard API stays open and publishes more than these pages show", strings.Join(slugs, ", "))
	}
}

func newManagedState(stored *common.ManagedStatusPage, configStates map[string]*State) *State {
	state := &State{Origin: OriginAdmin, Slug: stored.Slug, Stored: stored}
	if _, used := configStates[stored.Slug]; used {
		state.ConflictOrigin = "the configuration file"
		logr.Warnf("[statuspage.Load] Managed status page with slug=%s is not published because its slug is used by the configuration file", stored.Slug)
		return state
	}
	page, err := Parse([]byte(stored.Definition))
	if err == nil && page.Slug != stored.Slug {
		err = fmt.Errorf("%w: the slug of the definition (%s) does not match the stored slug", ErrInvalidDefinition, page.Slug)
	}
	if err != nil {
		state.Err = err
		logr.Errorf("[statuspage.Load] Managed status page with slug=%s is not published because it is invalid: %s", stored.Slug, err.Error())
		return state
	}
	state.Page = page
	return state
}

// Lookup returns the published status page with the given slug. It returns false when the page does not exist, is not
// published, or when status pages are disabled.
func Lookup(slug string) (Published, bool) {
	snap := current.Load()
	if snap == nil || !snap.enabled {
		return Published{}, false
	}
	state := snap.configStates[slug]
	if state == nil {
		state = snap.managedStates[slug]
	}
	if state == nil || !state.IsPublished() {
		return Published{}, false
	}
	return Published{Page: state.Page, Revision: snap.revision, Generation: snap.generation, MaximumResults: snap.maximumResults, MaximumEndpoints: snap.maximumEndpoints}, true
}

// Generation returns the generation of the loaded status pages, 0 before the first Load
func Generation() uint64 {
	if snap := current.Load(); snap != nil {
		return snap.generation
	}
	return 0
}

// maximumPublicResults returns MaximumPublicResults, or storage.maximum-number-of-results if it is lower
func maximumPublicResults(cfg *config.Config) int {
	if cfg.Storage != nil && cfg.Storage.MaximumNumberOfResults > 0 && cfg.Storage.MaximumNumberOfResults < MaximumPublicResults {
		return cfg.Storage.MaximumNumberOfResults
	}
	return MaximumPublicResults
}

// List returns the state of every status page: the pages of the configuration file, then the managed status pages,
// each ordered by slug
func List() []*State {
	snap := current.Load()
	if snap == nil {
		return nil
	}
	list := append(sortedStates(snap.configStates), sortedStates(snap.managedStates)...)
	return list
}

// IsEnabled returns whether status pages are served, as configured by status-pages.enabled
func IsEnabled() bool {
	snap := current.Load()
	return snap != nil && snap.enabled
}

// IsManagedUnavailable returns whether the managed status pages could not be loaded from the storage
func IsManagedUnavailable() bool {
	snap := current.Load()
	return snap != nil && snap.managedUnavailable
}

// Endpoints returns the endpoints that can be published: the enabled endpoints and external endpoints of the
// configuration file, and the valid, enabled and not in conflict managed endpoints
func Endpoints() []EndpointRef {
	refs, _ := endpointsAndLimit()
	return refs
}

// endpointsAndLimit returns the endpoints that can be published and how many of them a page shows, both from one read
// of the snapshot, for the callers that have no Published at hand (the administration)
func endpointsAndLimit() ([]EndpointRef, int) {
	snap := current.Load()
	var refs []EndpointRef
	limit := pageconfig.DefaultMaximumEndpointsPerPage
	if snap != nil {
		refs = append(refs, snap.configEndpoints...)
		limit = snap.maximumEndpoints
	}
	for _, state := range managedendpoint.List() {
		if ep := state.Endpoint; ep != nil && ep.IsEnabled() {
			refs = append(refs, EndpointRef{Key: ep.Key(), Name: ep.Name, Group: ep.Group})
		}
		// Fork: push endpoints managed through the administration
		if pushEndpoint := state.Push; pushEndpoint != nil && pushEndpoint.IsEnabled() {
			refs = append(refs, EndpointRef{Key: pushEndpoint.Key(), Name: pushEndpoint.Name, Group: pushEndpoint.Group})
		}
	}
	return refs, limit
}

func configEndpointRefs(cfg *config.Config) []EndpointRef {
	refs := make([]EndpointRef, 0, len(cfg.Endpoints)+len(cfg.ExternalEndpoints))
	for _, ep := range cfg.Endpoints {
		if ep.IsEnabled() {
			refs = append(refs, EndpointRef{Key: ep.Key(), Name: ep.Name, Group: ep.Group})
		}
	}
	for _, externalEndpoint := range cfg.ExternalEndpoints {
		if externalEndpoint.IsEnabled() {
			refs = append(refs, EndpointRef{Key: externalEndpoint.Key(), Name: externalEndpoint.Name, Group: externalEndpoint.Group})
		}
	}
	return refs
}

func publish(next *snapshot) {
	next.revision = revisions.Add(1)
	current.Store(next)
}

// logPublished logs the published status pages and warns about the groups and endpoint keys they select without match
func logPublished(snap *snapshot) {
	if !snap.enabled {
		if len(snap.configStates)+len(snap.managedStates) > 0 {
			logr.Info("[statuspage.Load] Status pages are disabled by status-pages.enabled")
		}
		return
	}
	refs := Endpoints()
	published := map[Origin][]string{}
	for _, state := range append(sortedStates(snap.configStates), sortedStates(snap.managedStates)...) {
		if !state.IsPublished() {
			continue
		}
		published[state.Origin] = append(published[state.Origin], state.Slug)
		for _, warning := range selectionWarnings(state.Page, refs) {
			if warning.Type == warningTypeCharts {
				logr.Warnf("[statuspage.Load] Status page with slug=%s has charts, which is deprecated and ignored: every endpoint of the page has a details page with its response time chart", state.Slug)
				continue
			}
			logr.Warnf("[statuspage.Load] Status page with slug=%s selects %s=%s, which has no match", state.Slug, warning.Type, warning.Value)
		}
	}
	if len(published) > 0 {
		logr.Infof("[statuspage.Load] Published status pages: config=[%s] admin=[%s]", strings.Join(published[OriginConfig], ", "), strings.Join(published[OriginAdmin], ", "))
	}
}

func sortedStates(states map[string]*State) []*State {
	list := make([]*State, 0, len(states))
	for _, state := range states {
		list = append(list, state)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Slug < list[j].Slug
	})
	return list
}
