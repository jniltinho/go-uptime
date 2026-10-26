// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/admintest"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"gopkg.in/yaml.v3"
)

// Sources of an Item
const (
	// SourceYAML is a provider declared in the configuration file, read-only in the administration
	SourceYAML = "yaml"

	// SourceWeb is a provider managed through the administration
	SourceWeb = "web"

	// SourceNone is an alert type without provider
	SourceNone = "none"
)

// States of an Item
const (
	// StateValid is a provider that sends alerts
	StateValid = "valid"

	// StateInvalid is a provider whose definition is not valid, which sends nothing
	StateInvalid = "invalid"
)

var (
	// ErrStorageNotSupported is returned when the storage does not support managed providers
	ErrStorageNotSupported = errors.New("the storage does not support alerting providers managed through the administration")

	// ErrCycleInProgress is returned when a start or configuration reload is in progress
	ErrCycleInProgress = errors.New("a start or configuration reload is in progress, try again later")

	// ErrNotFound is returned when no managed provider has the given type
	ErrNotFound = errors.New("alerting provider not found")

	// ErrAlreadyExists is returned when a managed provider of the same type already exists
	ErrAlreadyExists = errors.New("an alerting provider of this type is already configured")

	// ErrConfiguredInFile is returned when trying to create or update a type declared in the configuration file
	ErrConfiguredInFile = errors.New("this alerting provider is configured in the configuration file and cannot be changed through the administration")

	// ErrVersionMismatch is returned when the managed provider was changed since the given version
	ErrVersionMismatch = errors.New("the alerting provider was changed by someone else: reload it")

	// ErrTypeMismatch is returned when the type of a definition differs from the type of the path
	ErrTypeMismatch = errors.New("the type of the definition differs from the type being edited")

	// ErrTestTimeout is returned when a test notification did not end within TestTimeout
	ErrTestTimeout = errors.New("the provider did not answer in time; the notification may still be delivered")

	// ErrNotConfigured is returned when testing a type that has no usable provider
	ErrNotConfigured = errors.New("this alerting provider is not configured")
)

// TestTimeout is how long a test notification is waited for
const TestTimeout = 15 * time.Second

// Usage are the endpoints with an alert of a type
type Usage struct {
	// Managed are the keys of the endpoints managed through the administration with an alert of the type, enabled or
	// not, valid or not, sorted
	Managed []string `json:"managed"`

	// File are the keys of the endpoints, external endpoints and suite endpoints of the configuration file with an
	// alert of the type, sorted
	File []string `json:"file"`
}

// Item is an alert type in the administration list, configured or not. It is an item of the providers of Listing.
type Item struct {
	// Type is the alert type, e.g. "slack"
	Type string `json:"type"`

	// Name is the readable name of the provider
	Name string `json:"name"`

	// Category is the category of the provider, see TypeInfo
	Category string `json:"category"`

	// Source is where the provider in use comes from: "yaml" (configuration file, read-only), "web" (administration)
	// or "none" (not configured)
	Source string `json:"source"`

	// State is "valid" or "invalid" for a configured provider, and empty for "none"
	State string `json:"state,omitempty"`

	// Error is why the provider is invalid, without secrets; empty when it is valid
	Error string `json:"error,omitempty"`

	// Shadowed is whether a provider managed through the administration exists for the type but is not used because
	// the configuration file declares the type
	Shadowed bool `json:"shadowed,omitempty"`

	// Version is the version of the managed provider, also when shadowed; 0 without one
	Version int64 `json:"version,omitempty"`

	// UpdatedAt is the instant of the last change of the managed provider, RFC 3339; omitted without one
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`

	// UpdatedBy is the author of the last change of the managed provider; omitted without one or when unknown
	UpdatedBy string `json:"updatedBy,omitempty"`

	// Usage are the endpoints with an alert of the type
	Usage Usage `json:"usage"`
}

// Listing is the body of GET /api/v1/admin/alert-providers
type Listing struct {
	// StorageSupported is whether the storage can keep providers managed through the administration (false with the
	// memory storage)
	StorageSupported bool `json:"storageSupported"`

	// ManagedUnavailable is whether the managed providers could not be loaded: the list then only shows the providers
	// of the configuration file, and must not be mistaken for an empty one
	ManagedUnavailable bool `json:"managedUnavailable"`

	// Providers has every alert type, in alphabetical order of type
	Providers []*Item `json:"providers"`

	// Types describes every alert type for the forms, in the same order
	Types []*TypeInfo `json:"types"`
}

// Definition is a definition of a provider with its secrets masked, as a JSON object and as YAML, without the type key
type Definition struct {
	// JSON is the definition as an object
	JSON map[string]any `json:"json"`

	// YAML is the definition as YAML
	YAML string `json:"yaml"`
}

// Detail is the body of GET /api/v1/admin/alert-providers/{type}: the item and its definitions
type Detail struct {
	Item

	// Info describes the type for the form
	Info *TypeInfo `json:"info"`

	// Definition is the definition in use, secrets masked: the one of the configuration file for "yaml", the managed
	// one for "web"; omitted for "none"
	Definition *Definition `json:"definition,omitempty"`

	// ManagedDefinition is the definition of the managed provider shadowed by the configuration file, secrets masked;
	// omitted when there is none
	ManagedDefinition *Definition `json:"managedDefinition,omitempty"`
}

// Validation is the body of POST /api/v1/admin/alert-providers/validate
type Validation struct {
	// Type is the alert type of the definition
	Type string `json:"type"`

	// Effective is the definition with every default value applied, secrets masked
	Effective *Definition `json:"effective"`

	// Storable is the definition as it would be stored, without default values, secrets masked
	Storable *Definition `json:"storable"`
}

// WriteResult is the body of a successful create or update
type WriteResult struct {
	Detail

	// Restarted are the keys of the managed endpoints restarted to use the new definition
	Restarted []string `json:"restarted"`

	// Warnings are problems found after the change was stored, e.g. an endpoint that could not be restarted
	Warnings []string `json:"warnings"`
}

// TestResult is the body of POST /api/v1/admin/alert-providers/test
type TestResult struct {
	// Success is whether the provider accepted the notification
	Success bool `json:"success"`

	// DurationMs is how long the send took, in milliseconds
	DurationMs int64 `json:"durationMs"`

	// Error is why the send failed, without secrets and truncated to 512 characters; empty on success
	Error string `json:"error,omitempty"`

	// Resolved is whether the resolution was sent and accepted after the notification, when it was asked for. A
	// provider that opens an incident (PagerDuty, Opsgenie, GitLab, incident.io, issues) then closes it.
	Resolved bool `json:"resolved"`
}

// InUseError is returned when deleting a provider used by managed endpoints
type InUseError struct {
	// Keys are the keys of the managed endpoints using the provider
	Keys []string
}

// Error lists the keys of the endpoints that use the provider
func (err *InUseError) Error() string {
	return fmt.Sprintf("the alerting provider is used by %d managed endpoints: %s", len(err.Keys), strings.Join(err.Keys, ", "))
}

// DependentsError is returned when a change would make managed endpoints invalid
type DependentsError struct {
	// Errors are the errors of each endpoint, by key
	Errors map[string]string
}

// Error lists, sorted by key, why each endpoint would become invalid
func (err *DependentsError) Error() string {
	keys := make([]string, 0, len(err.Errors))
	for key := range err.Errors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+err.Errors[key])
	}
	return "the change would make managed endpoints invalid: " + strings.Join(parts, "; ")
}

// List returns every alert type with its provider, if any
func List() *Listing {
	_, supported := getManagedAlertProviderStore()
	listing := &Listing{StorageSupported: supported, Providers: []*Item{}, Types: Catalog()}
	snap := current.Load()
	if snap == nil {
		return listing
	}
	listing.ManagedUnavailable = snap.unavailable
	for _, alertType := range Types() {
		listing.Providers = append(listing.Providers, snap.item(alertType))
	}
	return listing
}

// Get returns the detail of alertType, configured or not, or ErrNotFound when it is not an alert type
func Get(name string) (*Detail, error) {
	alertType := alertTypeOf(name)
	if !IsType(alertType) {
		return nil, ErrNotFound
	}
	snap := current.Load()
	if snap == nil {
		return nil, ErrCycleInProgress
	}
	return snap.detail(alertType)
}

// Validate validates a definition without storing it. When name is set, the definition updates the managed provider
// of that type: masked secrets stand for its stored values.
func Validate(raw []byte, name string) (*Validation, error) {
	prepared, err := prepareSubmitted(raw, name)
	if err != nil {
		return nil, err
	}
	effective, err := effectiveDefinition(prepared)
	if err != nil {
		return nil, err
	}
	storable, err := newDefinition(MaskSecrets(prepared.Type, prepared.Document))
	if err != nil {
		return nil, err
	}
	return &Validation{Type: string(prepared.Type), Effective: effective, Storable: storable}, nil
}

// Create creates the managed provider of the type of the definition
func Create(raw []byte, author string) (*WriteResult, error) {
	prepared, err := prepareSubmitted(raw, "")
	if err != nil {
		return nil, err
	}
	return write(prepared.Type, author, "created", func(snap *snapshot) (*managedEntry, func(store.ManagedAlertProviderStore) error, error) {
		if snap.declared[prepared.Type] {
			return nil, nil, ErrConfiguredInFile
		}
		if _, exists := snap.managed[prepared.Type]; exists {
			return nil, nil, ErrAlreadyExists
		}
		stored := &common.ManagedAlertProvider{Type: string(prepared.Type), Definition: string(prepared.Definition), UpdatedBy: author}
		entry := &managedEntry{stored: stored, document: prepared.Document, prepared: prepared}
		return entry, func(managedStore store.ManagedAlertProviderStore) error {
			err := managedStore.CreateManagedAlertProvider(stored, nil)
			if errors.Is(err, common.ErrManagedAlertProviderAlreadyExists) {
				return ErrAlreadyExists
			}
			return err
		}, nil
	})
}

// Update replaces the definition of the managed provider of the given type if its version is expectedVersion
func Update(name string, raw []byte, expectedVersion int64, author string) (*WriteResult, error) {
	prepared, err := prepareSubmitted(raw, name)
	if err != nil {
		return nil, err
	}
	return write(prepared.Type, author, "updated", func(snap *snapshot) (*managedEntry, func(store.ManagedAlertProviderStore) error, error) {
		if snap.declared[prepared.Type] {
			return nil, nil, ErrConfiguredInFile
		}
		previous, exists := snap.managed[prepared.Type]
		if !exists {
			return nil, nil, ErrNotFound
		}
		if previous.stored.Version != expectedVersion {
			return nil, nil, ErrVersionMismatch
		}
		stored := &common.ManagedAlertProvider{Type: string(prepared.Type), Definition: string(prepared.Definition), UpdatedBy: author}
		entry := &managedEntry{stored: stored, document: prepared.Document, prepared: prepared}
		return entry, func(managedStore store.ManagedAlertProviderStore) error {
			return storeError(managedStore.UpdateManagedAlertProvider(stored, expectedVersion, nil))
		}, nil
	})
}

// Delete deletes the managed provider of the given type if its version is expectedVersion. A provider used by a managed
// endpoint cannot be deleted, unless it is shadowed by the configuration file.
func Delete(name string, expectedVersion int64, author string) error {
	alertType := alertTypeOf(name)
	if !IsType(alertType) {
		return ErrNotFound
	}
	_, err := write(alertType, author, "deleted", func(snap *snapshot) (*managedEntry, func(store.ManagedAlertProviderStore) error, error) {
		previous, exists := snap.managed[alertType]
		if !exists {
			return nil, nil, ErrNotFound
		}
		if previous.stored.Version != expectedVersion {
			return nil, nil, ErrVersionMismatch
		}
		return nil, func(managedStore store.ManagedAlertProviderStore) error {
			return storeError(managedStore.DeleteManagedAlertProvider(string(alertType), expectedVersion, nil))
		}, nil
	})
	return err
}

// write applies a change of the managed provider of alertType: change returns the new entry (nil to delete) and the
// storage operation. The change is planned against the managed endpoints that use the type, stored, and only then
// published: the affected endpoints are stopped, the snapshot published and the endpoints started again.
func write(alertType alert.Type, author, operation string, change func(*snapshot) (*managedEntry, func(store.ManagedAlertProviderStore) error, error)) (*WriteResult, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	participant := getDependents()
	if participant != nil {
		unlock := participant.Lock()
		defer unlock()
	}
	mutex.Lock()
	defer mutex.Unlock()
	managedStore, ok := getManagedAlertProviderStore()
	if !ok {
		return nil, ErrStorageNotSupported
	}
	snap := current.Load()
	if snap == nil {
		return nil, ErrCycleInProgress
	}
	entry, storeOperation, err := change(snap)
	if err != nil {
		return nil, err
	}
	next := snap.clone()
	if entry != nil {
		next.managed[alertType] = entry
	} else {
		delete(next.managed, alertType)
	}
	next.effective = next.compose()
	var plan DependentsPlan
	if participant != nil && !snap.declared[alertType] {
		if entry == nil {
			if users := participant.Users(alertType); len(users) > 0 {
				return nil, &InUseError{Keys: users}
			}
		} else {
			plan, err = participant.Plan(next.effective, alertType)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := storeOperation(managedStore); err != nil {
		return nil, err
	}
	result := &WriteResult{Restarted: []string{}, Warnings: []string{}}
	if plan != nil {
		plan.Stop()
	}
	publishSnapshot(next)
	if plan != nil {
		result.Restarted = plan.Keys()
		result.Warnings = append(result.Warnings, plan.Start()...)
	}
	logr.Infof("[alertprovider.write] Alerting provider %s %s by %s", alertType, operation, auditAuthor(author))
	if entry != nil {
		detail, err := next.detail(alertType)
		if err != nil {
			return nil, err
		}
		result.Detail = *detail
	}
	return result, nil
}

// prepareSubmitted decodes and validates a submitted definition. With name, the type of the definition must be name,
// or absent, and masked secrets are restored from the managed provider of that type.
func prepareSubmitted(raw []byte, name string) (*Prepared, error) {
	document, err := decodeDocument(raw)
	if err != nil {
		return nil, err
	}
	submittedType, _ := document[typeField].(string)
	delete(document, typeField)
	alertType := alertTypeOf(submittedType)
	if len(name) > 0 {
		if len(submittedType) > 0 && alertTypeOf(submittedType) != alertTypeOf(name) {
			return nil, ErrTypeMismatch
		}
		alertType = alertTypeOf(name)
	}
	if len(alertType) == 0 {
		return nil, fmt.Errorf("%w: the type is required", ErrInvalidDefinition)
	}
	if !IsType(alertType) {
		return nil, fmt.Errorf("%w: %s", ErrUnknownType, alertType)
	}
	var stored map[string]any
	if snap := current.Load(); snap != nil {
		if entry, exists := snap.managed[alertType]; exists {
			stored = entry.document
		}
	}
	if err := RestoreSecrets(alertType, document, stored); err != nil {
		return nil, err
	}
	prepared, err := PrepareDocument(alertType, document)
	if err != nil {
		return nil, redactError(alertType, document, err)
	}
	return prepared, nil
}

// Test sends a test notification through a submitted definition, or, when raw is empty, through the provider of name
// in use. group selects the group override. With resolve, the resolution is sent right after an accepted notification,
// with the resolve key the provider returned, so that an incident opened by the test is closed. The answer comes after
// TestTimeout at the latest; the slot of the test is only released when the sends really end.
func Test(raw []byte, name, group string, resolve bool) (*TestResult, error) {
	alertType, alertProvider, document, err := testProvider(raw, name)
	if err != nil {
		return nil, err
	}
	release, ok := admintest.TryAcquire()
	if !ok {
		return nil, admintest.ErrTooManyTests
	}
	description := "This is a test notification sent from the administration of Go Uptime."
	// Providers that open an incident only keep its resolve key when the alert sends on resolved
	testAlert := &alert.Alert{Type: alertType, Description: &description, FailureThreshold: 1, SuccessThreshold: 1, SendOnResolved: &resolve}
	if defaultAlert := alertProvider.GetDefaultAlert(); defaultAlert != nil {
		copied := *defaultAlert
		testAlert.ProviderOverride = copied.ProviderOverride
	}
	testEndpoint := &endpoint.Endpoint{Name: "Go Uptime test", Group: group, URL: "https://example.org"}
	result := &endpoint.Result{Success: false, Timestamp: time.Now(), Errors: []string{description}}
	started := time.Now()
	done := make(chan *TestResult, 1)
	go func() {
		defer release()
		testResult := &TestResult{}
		if err := alertProvider.Send(testEndpoint, testAlert, result, false); err != nil {
			testResult.Error = redact(alertType, document, err.Error())
		} else {
			testResult.Success = true
			if resolve {
				resolvedResult := &endpoint.Result{Success: true, Timestamp: time.Now()}
				if err := alertProvider.Send(testEndpoint, testAlert, resolvedResult, true); err != nil {
					testResult.Error = "the notification was sent, but not its resolution: " + redact(alertType, document, err.Error())
				} else {
					testResult.Resolved = true
				}
			}
		}
		testResult.DurationMs = time.Since(started).Milliseconds()
		done <- testResult
	}()
	timer := time.NewTimer(TestTimeout)
	defer timer.Stop()
	select {
	case testResult := <-done:
		return testResult, nil
	case <-timer.C:
		return &TestResult{Success: false, DurationMs: time.Since(started).Milliseconds(), Error: ErrTestTimeout.Error()}, nil
	}
}

// testProvider returns the provider a test notification goes through, and the definition whose secrets are redacted
func testProvider(raw []byte, name string) (alert.Type, interface {
	Send(*endpoint.Endpoint, *alert.Alert, *endpoint.Result, bool) error
	GetDefaultAlert() *alert.Alert
}, map[string]any, error) {
	if len(strings.TrimSpace(string(raw))) > 0 {
		prepared, err := prepareSubmitted(raw, name)
		if err != nil {
			return "", nil, nil, err
		}
		return prepared.Type, prepared.Provider, prepared.Document, nil
	}
	alertType := alertTypeOf(name)
	if !IsType(alertType) {
		return "", nil, nil, ErrNotFound
	}
	snap := current.Load()
	if snap == nil {
		return "", nil, nil, ErrCycleInProgress
	}
	alertProvider := providerOf(snap.effective, alertType)
	if alertProvider == nil {
		return "", nil, nil, ErrNotConfigured
	}
	document, _ := snap.definitionDocument(alertType)
	return alertType, alertProvider, document, nil
}

// item returns the list item of alertType
func (snap *snapshot) item(alertType alert.Type) *Item {
	info := typeInfo(alertType)
	item := &Item{Type: string(alertType), Name: info.Name, Category: info.Category, Source: SourceNone, Usage: snap.usage(alertType)}
	entry, managed := snap.managed[alertType]
	if managed {
		item.Version = entry.stored.Version
		updatedAt := entry.stored.UpdatedAt
		item.UpdatedAt, item.UpdatedBy = &updatedAt, entry.stored.UpdatedBy
	}
	switch {
	case snap.declared[alertType]:
		item.Source, item.State, item.Shadowed = SourceYAML, StateValid, managed
		if snap.cfg == nil || providerOf(snap.cfg.Alerting, alertType) == nil {
			item.State, item.Error = StateInvalid, "the provider of the configuration file is invalid: see the log of the start"
		}
	case managed:
		item.Source, item.State = SourceWeb, StateValid
		if entry.err != nil {
			item.State, item.Error = StateInvalid, entry.err.Error()
		}
	}
	return item
}

// detail returns the detail of alertType
func (snap *snapshot) detail(alertType alert.Type) (*Detail, error) {
	detail := &Detail{Item: *snap.item(alertType), Info: typeInfo(alertType)}
	if detail.Source != SourceNone {
		document, err := snap.definitionDocument(alertType)
		if err != nil {
			return nil, err
		}
		if detail.Definition, err = newDefinition(MaskSecrets(alertType, document)); err != nil {
			return nil, err
		}
	}
	if detail.Shadowed {
		var err error
		if detail.ManagedDefinition, err = newDefinition(MaskSecrets(alertType, snap.managed[alertType].document)); err != nil {
			return nil, err
		}
	}
	return detail, nil
}

// definitionDocument returns the definition in use for alertType, decoded: the managed one, as stored, or the one of the
// configuration file, with the values applied by its validation
func (snap *snapshot) definitionDocument(alertType alert.Type) (map[string]any, error) {
	if !snap.declared[alertType] {
		if entry, exists := snap.managed[alertType]; exists {
			return copyDocument(entry.document), nil
		}
		return map[string]any{}, nil
	}
	var fileProvider any
	if snap.cfg != nil {
		if alertProvider := providerOf(snap.cfg.Alerting, alertType); alertProvider != nil {
			fileProvider = alertProvider
		}
	}
	if fileProvider == nil || reflect.ValueOf(fileProvider).IsNil() {
		return map[string]any{}, nil
	}
	encoded, err := yaml.Marshal(fileProvider)
	if err != nil {
		return nil, err
	}
	document := map[string]any{}
	if err := yaml.Unmarshal(encoded, &document); err != nil {
		return nil, err
	}
	return document, nil
}

// usage returns the endpoints with an alert of alertType
func (snap *snapshot) usage(alertType alert.Type) Usage {
	usage := Usage{Managed: []string{}, File: []string{}}
	if participant := getDependents(); participant != nil {
		usage.Managed = append(usage.Managed, participant.Users(alertType)...)
	}
	if snap.cfg != nil {
		hasType := func(alerts []*alert.Alert) bool {
			for _, endpointAlert := range alerts {
				if endpointAlert != nil && endpointAlert.Type == alertType {
					return true
				}
			}
			return false
		}
		for _, ep := range snap.cfg.Endpoints {
			if hasType(ep.Alerts) {
				usage.File = append(usage.File, ep.Key())
			}
		}
		for _, externalEndpoint := range snap.cfg.ExternalEndpoints {
			if hasType(externalEndpoint.Alerts) {
				usage.File = append(usage.File, externalEndpoint.Key())
			}
		}
		for _, suite := range snap.cfg.Suites {
			for _, ep := range suite.Endpoints {
				if hasType(ep.Alerts) {
					usage.File = append(usage.File, ep.Key())
				}
			}
		}
	}
	sort.Strings(usage.Managed)
	sort.Strings(usage.File)
	return usage
}

// effectiveDefinition returns the definition of a prepared provider with its default values, secrets masked
func effectiveDefinition(prepared *Prepared) (*Definition, error) {
	encoded, err := yaml.Marshal(prepared.Provider)
	if err != nil {
		return nil, err
	}
	document := map[string]any{}
	if err := yaml.Unmarshal(encoded, &document); err != nil {
		return nil, err
	}
	return newDefinition(MaskSecrets(prepared.Type, document))
}

func newDefinition(document map[string]any) (*Definition, error) {
	if document == nil {
		document = map[string]any{}
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	text := string(encoded)
	if text == "{}\n" {
		text = ""
	}
	return &Definition{JSON: document, YAML: text}, nil
}

func storeError(err error) error {
	switch {
	case errors.Is(err, common.ErrManagedAlertProviderNotFound):
		return ErrNotFound
	case errors.Is(err, common.ErrManagedAlertProviderVersionMismatch):
		return ErrVersionMismatch
	}
	return err
}

func auditAuthor(author string) string {
	if len(author) == 0 {
		return "unknown"
	}
	return author
}
