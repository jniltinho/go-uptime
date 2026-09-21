// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TwiN/logr"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"gopkg.in/yaml.v3"
)

// warningTypeCharts is the type of the warning of a page with the deprecated charts, which are ignored
const warningTypeCharts = "charts"

// WarningTypeTruncated is the type of the warning of a page that selects more endpoints than
// status-pages.maximum-endpoints-per-page: unlike the other warnings, its value is not something without match but the
// limit in force.
const WarningTypeTruncated = "truncated"

var (
	// ErrReadOnly is returned when trying to change a status page defined in the configuration file
	ErrReadOnly = errors.New("the status page is defined in the configuration file and cannot be changed through the administration")

	// ErrCycleInProgress is returned when a start or configuration reload is in progress
	ErrCycleInProgress = errors.New("a start or configuration reload is in progress, try again later")

	// ErrSlugChanged is returned when an update changes the slug of the status page
	ErrSlugChanged = errors.New("the slug of a status page cannot be changed")

	// ErrSlugInUse is returned when creating a status page whose slug is already used
	ErrSlugInUse = errors.New("the slug is already used by another status page")

	// ErrStorageNotSupported is returned when the storage does not support managed status pages
	ErrStorageNotSupported = errors.New("the storage does not support managed status pages")

	// ErrExposureQueryRequired is returned when the exposure of an endpoint is requested without group and key
	ErrExposureQueryRequired = errors.New("the group or the key of the endpoint is required")

	// getManagedStatusPageStore is replaced in tests
	getManagedStatusPageStore = store.GetManagedStatusPageStore

	// previewSemaphore limits the previews of the administration, which are not cached, without taking the slots of the
	// public status pages
	previewSemaphore = make(chan struct{}, 1)
)

// Item summarizes a status page for the administration: it is an item of the statusPages field of Listing, answered
// by GET /api/v1/admin/status-pages, and its fields are at the top level of Detail. It is output-only.
type Item struct {
	// Slug is the slug of the page, the one of its public path /status/{slug} and of its administration routes: 1 to 64
	// lowercase letters, digits or hyphens.
	Slug string `json:"slug"`

	// Title is the title of the page, with 1 to 100 characters. It is omitted when the definition of a managed status
	// page is invalid and cannot be read.
	Title string `json:"title,omitempty"`

	// Origin is where the page is defined: "config" for the configuration file, read-only in the administration, and
	// "admin" for a status page managed through the administration API.
	Origin Origin `json:"origin"`

	// Enabled is the enabled of the definition of the page. A page of the configuration file without enabled is enabled,
	// a managed status page without enabled is disabled. It is false when the definition cannot be read.
	Enabled bool `json:"enabled"`

	// Published is whether the page is served to the visitors: status-pages.enabled is true and the page is valid, not in
	// conflict and enabled.
	Published bool `json:"published"`

	// Conflict is whether the slug of the managed status page is also used by the configuration file, in which case the
	// managed status page is not published.
	Conflict bool `json:"conflict"`

	// ConflictOrigin describes what uses the same slug, in English: currently always "the configuration file". It is
	// omitted when the page is not in conflict.
	ConflictOrigin string `json:"conflictOrigin,omitempty"`

	// Error is the text of the validation error of a managed status page whose stored definition is invalid, in which
	// case the page is not published. It is omitted for a valid page.
	Error string `json:"error,omitempty"`

	// Endpoints is the number of endpoints that the page shows at this moment, featured ones included, at most
	// status-pages.maximum-endpoints-per-page. It is 0 when the definition cannot be read.
	Endpoints int `json:"endpoints"`

	// Truncated is whether the page selects more endpoints than status-pages.maximum-endpoints-per-page allows: the ones
	// beyond the limit are not shown and their routes of the page answer 404.
	Truncated bool `json:"truncated"`

	// RequiresLogin is whether the page has a login of its own (fork)
	RequiresLogin bool `json:"requiresLogin"`

	// Path is the public path of the page, /status/{slug}, relative to the root of the installation.
	Path string `json:"path"`

	// Version is the version of the managed status page, which starts at 1 and is incremented on every change. It is
	// also answered in the ETag header and must be sent in the If-Match header of every change. It is omitted for a
	// page of the configuration file.
	Version int64 `json:"version,omitempty"`

	// CreatedAt is the instant at which the managed status page was created, as a RFC 3339 timestamp. It is omitted for
	// a page of the configuration file.
	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// UpdatedAt is the instant of the last change of the managed status page, as a RFC 3339 timestamp. It is omitted
	// for a page of the configuration file.
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`

	// UpdatedBy is the author of the last change of the managed status page: the username of the basic authentication
	// or the OIDC subject. It is omitted for a page of the configuration file and when the author is unknown.
	UpdatedBy string `json:"updatedBy,omitempty"`
}

// Listing is the administration list of the status pages, the body of GET /api/v1/admin/status-pages
type Listing struct {
	// PublicationEnabled is status-pages.enabled: when false, no page is published
	PublicationEnabled bool `json:"publicationEnabled"`

	// ManagedUnavailable is whether the managed status pages could not be loaded
	ManagedUnavailable bool `json:"managedUnavailable"`

	// SharedRateLimitWarning is the untrusted proxy IP address from which every visitor seems to come, if any
	SharedRateLimitWarning string `json:"sharedRateLimitWarning,omitempty"`

	// StatusPages are the pages of the configuration file, then the managed status pages, each ordered by slug. It is
	// an empty array, never null, without page.
	StatusPages []*Item `json:"statusPages"`
}

// Detail is a status page with its definition, and the fields of Item at the same level. It is the body answered by
// GET and PUT /api/v1/admin/status-pages/{slug}, by POST /api/v1/admin/status-pages (201) and by
// POST /api/v1/admin/status-pages/{slug}/enable and /disable. It is output-only: the body of the requests is the
// definition itself, in YAML or JSON.
type Detail struct {
	Item

	// Definition is the normalized definition, or nil if the stored definition is invalid
	Definition *pageconfig.Page `json:"definition"`

	// YAML is the stored definition for managed status pages, or the definition from the configuration file
	YAML string `json:"yaml"`
}

// Warning is a group or an endpoint key selected by a page without match, the notice of a deprecated field or the
// notice of a truncated page. It is an item of the warnings field of Validation.
type Warning struct {
	// Type is what has no match: "group" for a group, "endpoint" for an endpoint key and "featured" for the key of a
	// featured endpoint. It is "charts" when the definition still has the deprecated charts, which are ignored, and
	// "truncated" when the page selects more endpoints than status-pages.maximum-endpoints-per-page, in which case only
	// the first ones are shown.
	Type string `json:"type"`

	// Value is the name of the group or the key of the endpoint, in the lowercase group_name form, without match. For
	// the type "charts" it is the keys of the charts joined by ", ", and for the type "truncated" it is the limit in
	// force, in decimal.
	Value string `json:"value"`
}

// Validation is the result of a successful validation, the body of POST /api/v1/admin/status-pages/validate. The
// definition is sent in YAML or JSON as the body of the request, and nothing is persisted.
type Validation struct {
	// Definition is the submitted definition once normalized: texts and group names trimmed, endpoint keys in
	// lowercase, and enabled set to false when it was not sent. The hash of the password of the login of the page is
	// answered masked as "********", never the hash itself.
	Definition *pageconfig.Page `json:"definition"`

	// Warnings are the groups and endpoint keys selected by the definition that match no endpoint at this moment, and
	// the notice of the deprecated charts. They do not prevent saving. It is an empty array, never null, without warning.
	Warnings []Warning `json:"warnings"`

	// Endpoints is the number of endpoints that the page would show at this moment, featured ones included, at most
	// status-pages.maximum-endpoints-per-page.
	Endpoints int `json:"endpoints"`
}

// Options lists what a status page can select, the body of GET /api/v1/admin/status-pages/options
type Options struct {
	// Groups are the groups with at least one endpoint that can be published, ordered by name. Endpoints without group
	// are not counted in any group. It is an empty array, never null, without group.
	Groups []GroupOption `json:"groups"`

	// Endpoints are the endpoints that can be published, ordered by group and then by key: the enabled endpoints and
	// external endpoints of the configuration file and the enabled managed endpoints. It is an empty array, never null.
	Endpoints []EndpointOption `json:"endpoints"`
}

// GroupOption is a group with the number of endpoints that can be published, an item of the groups field of Options
type GroupOption struct {
	// Name is the name of the group, trimmed. It is the value to put in the groups of a definition.
	Name string `json:"name"`

	// Endpoints is the number of endpoints of the group that can be published.
	Endpoints int `json:"endpoints"`
}

// EndpointOption is an endpoint that can be published, an item of the endpoints field of Options
type EndpointOption struct {
	// Key is the key of the endpoint, in the lowercase group_name form in which the characters "/", "_", ".", ",", " ",
	// "#", "+" and "&" of the group and of the name are replaced by "-". It is the value to put in the endpoints and
	// the featured of a definition.
	Key string `json:"key"`

	// Name is the display name of the endpoint, as configured.
	Name string `json:"name"`

	// Group is the name of the group of the endpoint, as configured, or an empty string for an endpoint without group.
	Group string `json:"group"`
}

// Exposure lists the status pages on which an endpoint would appear, the body of
// GET /api/v1/admin/status-pages/exposure, whose query parameters group and key identify the endpoint (at least one
// of them is required)
type Exposure struct {
	// StatusPages are the valid and not in conflict pages that select the endpoint, published or not: the pages of the
	// configuration file, then the managed status pages, each ordered by slug. It is an empty array, never null,
	// without page.
	StatusPages []ExposureItem `json:"statusPages"`
}

// ExposureItem is a status page on which an endpoint would appear, by group or by key. It is an item of the
// statusPages field of Exposure.
type ExposureItem struct {
	// Slug is the slug of the page, the one of its public path /status/{slug}.
	Slug string `json:"slug"`

	// Title is the title of the page, with 1 to 100 characters.
	Title string `json:"title"`

	// Origin is where the page is defined: "config" for the configuration file and "admin" for a status page managed
	// through the administration API.
	Origin Origin `json:"origin"`

	// Published is whether the page is served to the visitors at this moment: status-pages.enabled is true and the page
	// is enabled. When false, the endpoint would only appear once the page is published.
	Published bool `json:"published"`

	// Reason is why the endpoint would appear on the page: "group" when the page selects the group of the query, else
	// "key" when the key of the query is among the endpoints or the featured endpoints of the page.
	Reason string `json:"reason"`
}

// Service administers the status pages
type Service struct{}

// NewService returns the administration service of the status pages
func NewService() *Service {
	return &Service{}
}

// List returns the status pages of the configuration file and the managed status pages
func (s *Service) List() *Listing {
	refs, limit := endpointsAndLimit()
	states := List()
	listing := &Listing{PublicationEnabled: IsEnabled(), ManagedUnavailable: IsManagedUnavailable(), StatusPages: make([]*Item, 0, len(states))}
	listing.SharedRateLimitWarning, _ = SharedRateLimitWarning()
	for _, state := range states {
		listing.StatusPages = append(listing.StatusPages, newItem(state, refs, limit))
	}
	return listing
}

// Get returns the status page with the given slug, the managed one when both origins use the slug
func (s *Service) Get(slug string) (*Detail, error) {
	state := findState(slug)
	if state == nil {
		return nil, ErrPageNotFound
	}
	return newDetail(state)
}

// Options returns the groups and the endpoints that a status page can select
func (s *Service) Options() *Options {
	refs := Endpoints()
	counts := make(map[string]int)
	options := &Options{Groups: []GroupOption{}, Endpoints: make([]EndpointOption, 0, len(refs))}
	for _, ref := range refs {
		if group := pageconfig.NormalizeGroup(ref.Group); len(group) > 0 {
			counts[group]++
		}
		options.Endpoints = append(options.Endpoints, EndpointOption{Key: ref.Key, Name: ref.Name, Group: ref.Group})
	}
	for group, count := range counts {
		options.Groups = append(options.Groups, GroupOption{Name: group, Endpoints: count})
	}
	sort.Slice(options.Groups, func(i, j int) bool {
		return options.Groups[i].Name < options.Groups[j].Name
	})
	sort.Slice(options.Endpoints, func(i, j int) bool {
		a, b := options.Endpoints[i], options.Endpoints[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Key < b.Key
	})
	return options
}

// Exposure returns the status pages on which an endpoint with the given group or key would appear
func (s *Service) Exposure(group, key string) (*Exposure, error) {
	group, key = pageconfig.NormalizeGroup(group), strings.ToLower(strings.TrimSpace(key))
	if len(group) == 0 && len(key) == 0 {
		return nil, ErrExposureQueryRequired
	}
	exposure := &Exposure{StatusPages: []ExposureItem{}}
	for _, state := range List() {
		if state.Page == nil {
			continue
		}
		var reason string
		switch {
		case len(group) > 0 && slices.Contains(state.Page.Groups, group):
			reason = "group"
		case len(key) > 0 && (slices.Contains(state.Page.Endpoints, key) || slices.Contains(state.Page.Featured, key)):
			reason = "key"
		default:
			continue
		}
		exposure.StatusPages = append(exposure.StatusPages, ExposureItem{
			Slug:      state.Slug,
			Title:     state.Page.Title,
			Origin:    state.Origin,
			Published: IsEnabled() && state.IsPublished(),
			Reason:    reason,
		})
	}
	return exposure, nil
}

// Validate validates a definition without persisting it. When slug is not empty, the definition is validated as an
// update of that managed status page.
func (s *Service) Validate(raw []byte, slug string) (*Validation, error) {
	page, err := parseSubmitted(raw, slug)
	if err != nil {
		return nil, err
	}
	refs, limit := endpointsAndLimit()
	// Fork: like every read of the administration, the validation answers with the hash of the credential masked
	selection := Select(page, refs, limit)
	return &Validation{Definition: maskCredential(page), Warnings: withTruncationWarning(selectionWarnings(page, refs), selection, limit), Endpoints: len(selection.Keys())}, nil
}

// Create creates a managed status page. Without enabled, it is created disabled.
func (s *Service) Create(raw []byte, author string) (*Detail, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	managedStatusPageStore, ok := getManagedStatusPageStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	page, err := parseSubmitted(raw, "")
	if err != nil {
		return nil, err
	}
	definition, err := yaml.Marshal(page)
	if err != nil {
		return nil, err
	}
	stored := &common.ManagedStatusPage{Slug: page.Slug, Definition: string(definition), UpdatedBy: author}
	if err := managedStatusPageStore.CreateManagedStatusPage(stored, nil); err != nil {
		return nil, err
	}
	state := publishManaged(stored)
	logr.Infof("[statuspage.Create] Managed status page with slug=%s created by %s", page.Slug, auditAuthor(author))
	return newDetail(state)
}

// Update replaces the definition of a managed status page whose current version is expectedVersion
func (s *Service) Update(slug string, raw []byte, expectedVersion int64, author string) (*Detail, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	managedStatusPageStore, err := managedStoreForChange(slug, expectedVersion)
	if err != nil {
		return nil, err
	}
	page, err := parseSubmitted(raw, slug)
	if err != nil {
		return nil, err
	}
	return save(managedStatusPageStore, page, expectedVersion, author, "updated")
}

// SetEnabled enables or disables a managed status page whose current version is expectedVersion
func (s *Service) SetEnabled(slug string, enabled bool, expectedVersion int64, author string) (*Detail, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	managedStatusPageStore, err := managedStoreForChange(slug, expectedVersion)
	if err != nil {
		return nil, err
	}
	page, err := Parse([]byte(current.Load().managedStates[slug].Stored.Definition))
	if err != nil {
		return nil, err
	}
	page.Enabled = &enabled
	operation := "disabled"
	if enabled {
		operation = "enabled"
	}
	return save(managedStatusPageStore, page, expectedVersion, author, operation)
}

// Delete deletes a managed status page whose current version is expectedVersion
func (s *Service) Delete(slug string, expectedVersion int64, author string) error {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return ErrCycleInProgress
	}
	defer end()
	mutex.Lock()
	defer mutex.Unlock()
	managedStatusPageStore, err := managedStoreForChange(slug, expectedVersion)
	if err != nil {
		return err
	}
	if err := managedStatusPageStore.DeleteManagedStatusPage(slug, expectedVersion, nil); err != nil {
		return err
	}
	// Unpublished only after the commit
	next := cloneCurrentSnapshot()
	delete(next.managedStates, slug)
	publish(next)
	_ = publicCache.DeleteKeysByPattern(slug + "|*")
	logr.Infof("[statuspage.Delete] Managed status page with slug=%s deleted by %s", slug, auditAuthor(author))
	return nil
}

// Preview returns the public payload of any status page, including disabled, in conflict and configuration file ones,
// without cache and without rate limit
func (s *Service) Preview(slug string) ([]byte, error) {
	// The page and the limits of the preview come from one read of the snapshot, like the ones of a published page
	snap := current.Load()
	if snap == nil {
		return nil, ErrPageNotFound
	}
	state := snap.managedStates[slug]
	if state == nil {
		state = snap.configStates[slug]
	}
	if state == nil {
		return nil, ErrPageNotFound
	}
	page := state.Page
	if page == nil {
		var err error
		if page, err = Parse([]byte(state.Stored.Definition)); err != nil {
			return nil, err
		}
	}
	release, acquired := acquireSlot(previewSemaphore)
	if !acquired {
		return nil, ErrPageUnavailable
	}
	defer release()
	maximumResults, maximumEndpoints := snap.maximumResults, snap.maximumEndpoints
	body, err := assemble(page, maximumResults, maximumEndpoints, time.Now())
	if err != nil {
		logr.Errorf("[statuspage.Preview] Failed to assemble status page with slug=%s: %s", slug, err.Error())
		return nil, ErrPageUnavailable
	}
	return body, nil
}

// parseSubmitted parses a submitted definition, in which a missing enabled means disabled. slug is the managed status
// page being updated, or empty for a creation, in which case the slug must not be used.
func parseSubmitted(raw []byte, slug string) (*pageconfig.Page, error) {
	// Fork: the plaintext password of the login of the page becomes a bcrypt hash before anything is parsed, and a
	// password that was not sent keeps the stored hash
	resolved, err := resolveSubmittedCredential(raw, storedPasswordHash(slug))
	if err != nil {
		return nil, err
	}
	page, err := Parse(resolved)
	if err != nil {
		return nil, err
	}
	if page.Enabled == nil {
		disabled := false
		page.Enabled = &disabled
	}
	if len(slug) > 0 {
		if page.Slug != slug {
			return nil, ErrSlugChanged
		}
		return page, nil
	}
	if snap := current.Load(); snap != nil {
		if _, used := snap.configStates[page.Slug]; used {
			return nil, fmt.Errorf("%w: %s is used by the configuration file", ErrSlugInUse, page.Slug)
		}
		if _, used := snap.managedStates[page.Slug]; used {
			return nil, fmt.Errorf("%w: %s is used by another managed status page", ErrSlugInUse, page.Slug)
		}
	}
	return page, nil
}

// managedStoreForChange checks that the managed status page exists at expectedVersion and returns the store. mutex must
// be held.
func managedStoreForChange(slug string, expectedVersion int64) (store.ManagedStatusPageStore, error) {
	var state *State
	if snap := current.Load(); snap != nil {
		state = snap.managedStates[slug]
		if state == nil {
			if _, exists := snap.configStates[slug]; exists {
				return nil, ErrReadOnly
			}
		}
	}
	if state == nil {
		return nil, ErrPageNotFound
	}
	if state.Stored.Version != expectedVersion {
		return nil, common.ErrManagedStatusPageVersionMismatch
	}
	managedStatusPageStore, ok := getManagedStatusPageStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	return managedStatusPageStore, nil
}

// save persists the definition of a managed status page and publishes it after the commit. mutex must be held.
func save(managedStatusPageStore store.ManagedStatusPageStore, page *pageconfig.Page, expectedVersion int64, author, operation string) (*Detail, error) {
	definition, err := yaml.Marshal(page)
	if err != nil {
		return nil, err
	}
	updated := &common.ManagedStatusPage{Slug: page.Slug, Definition: string(definition), UpdatedBy: author}
	if err := managedStatusPageStore.UpdateManagedStatusPage(updated, expectedVersion, nil); err != nil {
		return nil, err
	}
	state := publishManaged(updated)
	logr.Infof("[statuspage.Update] Managed status page with slug=%s %s by %s", page.Slug, operation, auditAuthor(author))
	return newDetail(state)
}

// publishManaged publishes, with a new revision, the state of a managed status page whose change was committed. mutex
// must be held.
func publishManaged(stored *common.ManagedStatusPage) *State {
	next := cloneCurrentSnapshot()
	state := newManagedState(stored, next.configStates)
	next.managedStates[stored.Slug] = state
	publish(next)
	_ = publicCache.DeleteKeysByPattern(stored.Slug + "|*")
	return state
}

// cloneCurrentSnapshot returns a copy of the current snapshot, to be changed and published. mutex must be held.
func cloneCurrentSnapshot() *snapshot {
	next := &snapshot{enabled: true, maximumResults: MaximumPublicResults, maximumEndpoints: pageconfig.DefaultMaximumEndpointsPerPage, configStates: make(map[string]*State), managedStates: make(map[string]*State)}
	if snap := current.Load(); snap != nil {
		next.generation, next.maximumResults, next.enabled = snap.generation, snap.maximumResults, snap.enabled
		next.maximumEndpoints = snap.maximumEndpoints
		next.managedUnavailable, next.configEndpoints = snap.managedUnavailable, snap.configEndpoints
		for slug, state := range snap.configStates {
			next.configStates[slug] = state
		}
		for slug, state := range snap.managedStates {
			next.managedStates[slug] = state
		}
	}
	return next
}

// findState returns the state of the status page with the given slug, the managed one when both origins use the slug
func findState(slug string) *State {
	snap := current.Load()
	if snap == nil {
		return nil
	}
	if state := snap.managedStates[slug]; state != nil {
		return state
	}
	return snap.configStates[slug]
}

// selectionWarnings returns the groups, endpoint keys and featured endpoint keys selected by the page without match
// among refs, and a warning of type charts when the page still has the deprecated charts
func selectionWarnings(page *pageconfig.Page, refs []EndpointRef) []Warning {
	groups := make(map[string]struct{}, len(refs))
	keys := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		groups[pageconfig.NormalizeGroup(ref.Group)] = struct{}{}
		keys[ref.Key] = struct{}{}
	}
	warnings := []Warning{}
	for _, group := range page.Groups {
		if _, exists := groups[group]; !exists {
			warnings = append(warnings, Warning{Type: "group", Value: group})
		}
	}
	for _, key := range page.Endpoints {
		if _, exists := keys[key]; !exists {
			warnings = append(warnings, Warning{Type: "endpoint", Value: key})
		}
	}
	for _, key := range page.Featured {
		if _, exists := keys[key]; !exists {
			warnings = append(warnings, Warning{Type: "featured", Value: key})
		}
	}
	if len(page.Charts) > 0 {
		warnings = append(warnings, Warning{Type: warningTypeCharts, Value: strings.Join(page.Charts, ", ")})
	}
	return warnings
}

// withTruncationWarning adds to warnings the one of a truncated selection, with the limit that cut it
func withTruncationWarning(warnings []Warning, selection Selection, limit int) []Warning {
	if !selection.Truncated {
		return warnings
	}
	return append(warnings, Warning{Type: WarningTypeTruncated, Value: strconv.Itoa(limit)})
}

func newItem(state *State, refs []EndpointRef, limit int) *Item {
	item := &Item{Slug: state.Slug, Origin: state.Origin, Path: "/status/" + state.Slug, Published: IsEnabled() && state.IsPublished()}
	page := state.Page
	if page == nil && state.Stored != nil {
		// In conflict or invalid: describe it from its definition, as far as possible
		page, _ = Parse([]byte(state.Stored.Definition))
	}
	if page != nil {
		item.Title = page.Title
		item.Enabled = page.IsEnabled()
		if state.Origin == OriginAdmin {
			item.Enabled = page.Enabled != nil && *page.Enabled
		}
		selection := Select(page, refs, limit)
		item.Endpoints, item.Truncated = len(selection.Keys()), selection.Truncated
		item.RequiresLogin = page.RequiresLogin()
	}
	if stored := state.Stored; stored != nil {
		createdAt, updatedAt := stored.CreatedAt, stored.UpdatedAt
		item.Version, item.CreatedAt, item.UpdatedAt, item.UpdatedBy = stored.Version, &createdAt, &updatedAt, stored.UpdatedBy
	}
	if state.InConflict() {
		item.Conflict, item.ConflictOrigin = true, state.ConflictOrigin
	}
	if state.Err != nil {
		item.Error = state.Err.Error()
	}
	return item
}

// newDetail describes a status page with its definition. Fork: the hash of the credential of the page is masked, in
// the definition and in the YAML, and submitting the mask back keeps the stored hash.
func newDetail(state *State) (*Detail, error) {
	refs, limit := endpointsAndLimit()
	detail := &Detail{Item: *newItem(state, refs, limit)}
	if state.Stored != nil {
		detail.YAML = maskCredentialInDefinition(state.Stored.Definition)
		if page, err := Parse([]byte(state.Stored.Definition)); err == nil {
			detail.Definition = maskCredential(page)
		}
		return detail, nil
	}
	definition, err := yaml.Marshal(maskCredential(state.Page))
	if err != nil {
		return nil, err
	}
	detail.YAML, detail.Definition = string(definition), maskCredential(state.Page)
	return detail, nil
}

func auditAuthor(author string) string {
	if len(author) == 0 {
		return "unknown"
	}
	return author
}
