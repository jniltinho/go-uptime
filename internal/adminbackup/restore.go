package adminbackup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/TwiN/logr"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const (
	// Types of the items of a restore, in the order in which they are planned and applied
	TypePushKey    = "pushKey"
	TypeEndpoint   = "endpoint"
	TypeStatusPage = "statusPage"

	// Actions of the items of a plan
	ActionCreate    = "create"
	ActionUpdate    = "update"
	ActionUnchanged = "unchanged"
	ActionSkip      = "skip"

	// Results of the items of an applied restore
	ResultCreated   = "created"
	ResultUpdated   = "updated"
	ResultUnchanged = "unchanged"
	ResultSkipped   = "skipped"
	ResultFailed    = "failed"

	reasonAlreadyExists  = "already exists"
	reasonNameOrGroup    = "name or group changes"
	reasonReloadProgress = "configuration reload in progress"
	reasonChanged        = "changed during the restore"
)

// ErrFingerprintMismatch is returned when the plan changed since the preview
var ErrFingerprintMismatch = errors.New("the backup, the options or the registered items changed since the preview: preview the restore again")

// Options are the options of a restore
type Options struct {
	// Overwrite updates the items that already exist with a different definition
	Overwrite bool

	// DisableEndpoints creates and updates the endpoints disabled
	DisableEndpoints bool
}

// Plan is what a restore would do, without effects. It is the response of POST /api/v1/admin/restore/preview and is
// output-only, except for Fingerprint, which the client sends back to apply the restore.
type Plan struct {
	// Summary counts the items by action.
	Summary Summary `json:"summary"`

	// Notices tell what applying the restore will start.
	Notices Notices `json:"notices"`

	// Fingerprint is the SHA-256 hash, as 64 lowercase hexadecimal characters, of the decrypted backup file, the
	// overwrite and disableEndpoints options, the generation of the loaded status pages and, for every item, its type,
	// identifier, action and the version and hash of the definition it would replace. It must be sent as the fingerprint
	// of POST /api/v1/admin/restore with the same file and options: when the plan computed then has another fingerprint,
	// nothing is applied and the route answers 409.
	Fingerprint string `json:"fingerprint"`

	// Items are the items of the backup in the order in which they are applied: push keys by name, endpoints by key,
	// then status pages by slug. It is an empty list, never null, for an empty backup.
	Items []*Item `json:"items"`
}

// Summary counts the items of a plan by action. It travels in Plan.Summary and the four counts add up to the number of
// items.
type Summary struct {
	// Create is the number of items with the "create" action.
	Create int `json:"create"`

	// Update is the number of items with the "update" action, which is only planned with the overwrite option.
	Update int `json:"update"`

	// Unchanged is the number of items with the "unchanged" action.
	Unchanged int `json:"unchanged"`

	// Skip is the number of items with the "skip" action.
	Skip int `json:"skip"`
}

// Notices tell what the restore will start. It travels in Plan.Notices.
type Notices struct {
	// MonitoringStarts is the number of enabled endpoints that will be created or updated, and so monitored. It is 0 with
	// the disableEndpoints option.
	MonitoringStarts int `json:"monitoringStarts"`

	// WithAlerts is the number of the endpoints counted in MonitoringStarts that have at least one alert, and so may
	// notify as soon as they are restored.
	WithAlerts int `json:"withAlerts"`
}

// Item is an item of a plan: what the restore would do with one push key, endpoint or status page of the backup. It is
// an element of Plan.Items and is output-only.
type Item struct {
	// Type is the kind of item: "pushKey", "endpoint" or "statusPage".
	Type string `json:"type"`

	// ID identifies the item within its type: the name of a push key, the key of an endpoint in the group_name form or
	// the slug of a status page.
	ID string `json:"id"`

	// Action is what the restore would do: "create" when the item does not exist, "update" when it exists with another
	// definition and the overwrite option is set, "unchanged" when it exists with the same definition (or, for a push
	// key, the same name and token hash) and "skip" when it is left out. A push key is never updated.
	Action string `json:"action"`

	// Reason explains, in English, a "skip" action: "already exists" without the overwrite option, or the validation
	// error. For an "update" of an endpoint it may be "name or group changes", which warns that the display name or the
	// group changes while the key stays the same. It is empty otherwise.
	Reason string `json:"reason"`

	// Warnings are the selections of a status page (group, endpoint, featured or charts) that match nothing among the
	// current endpoints and the enabled endpoints the restore would create or update, as English sentences ending in
	// "selects nothing", and the sentence of a page that selects more endpoints than
	// status-pages.maximum-endpoints-per-page, which is shown truncated. It is an empty list, never null, for the other
	// types and when every selection matches within the limit.
	Warnings []string `json:"warnings"`

	// What is applied, as previewed
	definition    []byte
	version       int64
	currentSHA256 string
	tokenHash     string
	hint          string
}

// Result is the result of an applied restore. It is the response of POST /api/v1/admin/restore, answered with 200 even
// when items failed: the restore is not atomic and the failure of an item does not undo or stop the others. It is
// output-only.
type Result struct {
	// Summary counts the items by result.
	Summary ResultSummary `json:"summary"`

	// Results are the results of the items, in the order of Plan.Items. It is an empty list, never null, for an empty
	// backup.
	Results []*ItemResult `json:"results"`
}

// ResultSummary counts the items of an applied restore by result. It travels in Result.Summary and the five counts add
// up to the number of items.
type ResultSummary struct {
	// Created is the number of items with the "created" result.
	Created int `json:"created"`

	// Updated is the number of items with the "updated" result.
	Updated int `json:"updated"`

	// Unchanged is the number of items with the "unchanged" result.
	Unchanged int `json:"unchanged"`

	// Skipped is the number of items with the "skipped" result, whether planned as "skip" or skipped because a reload of
	// the configuration started during the restore.
	Skipped int `json:"skipped"`

	// Failed is the number of items with the "failed" result.
	Failed int `json:"failed"`
}

// ItemResult is the result of an item of an applied restore. It is an element of Result.Results and is output-only.
type ItemResult struct {
	// Type is the kind of item: "pushKey", "endpoint" or "statusPage".
	Type string `json:"type"`

	// ID identifies the item within its type: the name of a push key, the key of an endpoint in the group_name form or
	// the slug of a status page.
	ID string `json:"id"`

	// Result is what happened to the item: "created", "updated", "unchanged", "skipped" or "failed".
	Result string `json:"result"`

	// Message explains the result, in English: the reason of the plan for a skipped or updated item, "configuration
	// reload in progress" for an item skipped because a reload started, and for a failed item the error or "changed
	// during the restore" when its version changed since the plan. It is empty otherwise.
	Message string `json:"message"`

	// Warnings are the selections of a status page that match nothing, in English, computed again once the page is
	// created or updated. It is an empty list, never null, for the other types and when every selection matches.
	Warnings []string `json:"warnings"`
}

// Restorer plans and applies the restores of backups with the services of the administration
type Restorer struct {
	Endpoints   *managedendpoint.Service
	StatusPages *statuspage.Service
}

// fingerprintInput is encoded with its fields in this order to compute the fingerprint of a plan. It never travels:
// only the SHA-256 hash of its JSON encoding does, as Plan.Fingerprint.
type fingerprintInput struct {
	// PlaintextSHA256 is the SHA-256 hash of the backup file once decrypted, in lowercase hexadecimal.
	PlaintextSHA256 string `json:"plaintextSHA256"`

	// Overwrite is the overwrite option of the restore.
	Overwrite bool `json:"overwrite"`

	// DisableEndpoints is the disableEndpoints option of the restore.
	DisableEndpoints bool `json:"disableEndpoints"`

	// Generation is the generation of the loaded status pages, 0 before their first load.
	Generation uint64 `json:"generation"`

	// Items are the items of the plan, in the order of Plan.Items.
	Items []fingerprintEntry `json:"items"`
}

// fingerprintEntry is what an item of a plan contributes to the fingerprint
type fingerprintEntry struct {
	// Type is the kind of item: "pushKey", "endpoint" or "statusPage".
	Type string `json:"type"`

	// ID is the name of the push key, the key of the endpoint or the slug of the status page.
	ID string `json:"id"`

	// Action is the planned action: "create", "update", "unchanged" or "skip".
	Action string `json:"action"`

	// Version is the current version of the existing endpoint or status page with that identifier. It is 0 for a push
	// key and when the item does not exist.
	Version int64 `json:"version"`

	// CurrentSHA256 is the SHA-256 hash, in lowercase hexadecimal, of the stored definition of the existing endpoint or
	// status page, or the token hash of another push key that already uses the name. It is empty otherwise.
	CurrentSHA256 string `json:"currentSHA256"`
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (item *Item) skip(reason string) *Item {
	item.Action, item.Reason = ActionSkip, reason
	return item
}

// Plan returns what restoring the backup would do, without effects. plaintext is the backup file as decoded, whose
// hash is part of the fingerprint. The plan simulates the items in the order in which they are applied: push keys,
// endpoints and status pages.
func (r *Restorer) Plan(plaintext []byte, options Options) (*Plan, error) {
	file, err := Decode(plaintext)
	if err != nil {
		return nil, err
	}
	if isAnyRegistryUnavailable() {
		return nil, ErrUnavailable
	}
	plan := &Plan{Items: []*Item{}}
	// Every push token of the endpoints, including the invalid managed endpoints and the endpoints of the backup, so that
	// a push key never accepts the token of an endpoint
	endpointTokenHashes := make(map[[sha256.Size]byte]bool)
	for _, token := range r.Endpoints.EndpointPushTokens() {
		endpointTokenHashes[pushconfig.HashToken(token)] = true
	}
	for _, item := range file.Endpoints {
		if token := managedendpoint.DefinitionPushToken([]byte(item.Definition)); len(token) > 0 {
			endpointTokenHashes[pushconfig.HashToken(token)] = true
		}
	}
	ctx := &managedendpoint.RestoreContext{PlannedTokens: make(map[string]string), PlannedKeyHashes: make(map[[sha256.Size]byte]bool)}
	sort.Slice(file.PushKeys, func(i, j int) bool { return file.PushKeys[i].Name < file.PushKeys[j].Name })
	for _, pushKey := range file.PushKeys {
		plan.Items = append(plan.Items, planPushKey(pushKey, endpointTokenHashes, ctx))
	}
	sort.Slice(file.Endpoints, func(i, j int) bool { return file.Endpoints[i].Key < file.Endpoints[j].Key })
	var plannedRefs []statuspage.EndpointRef
	for _, backupEndpoint := range file.Endpoints {
		item, prepared := r.planEndpoint(backupEndpoint, options, ctx)
		plan.Items = append(plan.Items, item)
		if prepared != nil && (item.Action == ActionCreate || item.Action == ActionUpdate) && prepared.IsEnabled() {
			plan.Notices.MonitoringStarts++
			if prepared.AlertCount() > 0 {
				plan.Notices.WithAlerts++
			}
			plannedRefs = append(plannedRefs, statuspage.EndpointRef{Key: prepared.Key(), Name: prepared.Name(), Group: prepared.Group()})
		}
	}
	refs := append(statuspage.Endpoints(), plannedRefs...)
	sort.Slice(file.StatusPages, func(i, j int) bool { return file.StatusPages[i].Slug < file.StatusPages[j].Slug })
	for _, backupStatusPage := range file.StatusPages {
		plan.Items = append(plan.Items, r.planStatusPage(backupStatusPage, options, refs))
	}
	fingerprint := fingerprintInput{PlaintextSHA256: sha256Hex(plaintext), Overwrite: options.Overwrite, DisableEndpoints: options.DisableEndpoints, Generation: statuspage.Generation(), Items: []fingerprintEntry{}}
	for _, item := range plan.Items {
		switch item.Action {
		case ActionCreate:
			plan.Summary.Create++
		case ActionUpdate:
			plan.Summary.Update++
		case ActionUnchanged:
			plan.Summary.Unchanged++
		default:
			plan.Summary.Skip++
		}
		fingerprint.Items = append(fingerprint.Items, fingerprintEntry{Type: item.Type, ID: item.ID, Action: item.Action, Version: item.version, CurrentSHA256: item.currentSHA256})
	}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return nil, err
	}
	plan.Fingerprint = sha256Hex(encoded)
	return plan, nil
}

func planPushKey(backupKey PushKey, endpointTokenHashes map[[sha256.Size]byte]bool, ctx *managedendpoint.RestoreContext) *Item {
	item := &Item{Type: TypePushKey, ID: backupKey.Name, Warnings: []string{}, tokenHash: backupKey.TokenHash, hint: backupKey.Hint}
	hash, err := pushkey.ValidateRestoredKey(backupKey.Name, backupKey.TokenHash, backupKey.Hint)
	if err != nil {
		return item.skip(err.Error())
	}
	if existing, exists := pushkey.FindByName(backupKey.Name, hash); exists {
		switch {
		case existing.Origin == pushkey.OriginConfig:
			return item.skip("name in use by a push key of the configuration file")
		case existing.SameHash:
			item.Action = ActionUnchanged
			return item
		default:
			item.currentSHA256 = hex.EncodeToString(existing.Hash[:])
			return item.skip("name in use by another push key")
		}
	}
	if pushkey.IsHashInUse(hash) || endpointTokenHashes[hash] || ctx.PlannedKeyHashes[hash] {
		return item.skip(pushkey.ErrHashInUse.Error())
	}
	ctx.PlannedKeyHashes[hash] = true
	item.Action = ActionCreate
	return item
}

func (r *Restorer) planEndpoint(backupEndpoint Endpoint, options Options, ctx *managedendpoint.RestoreContext) (*Item, *managedendpoint.Prepared) {
	item := &Item{Type: TypeEndpoint, ID: backupEndpoint.Key, Warnings: []string{}}
	raw := []byte(backupEndpoint.Definition)
	if options.DisableEndpoints {
		document, err := managedendpoint.ToDocument(raw)
		if err != nil {
			return item.skip(err.Error()), nil
		}
		document["enabled"] = false
		if raw, err = managedendpoint.FromDocument(document); err != nil {
			return item.skip(err.Error()), nil
		}
	}
	parsed, err := managedendpoint.ParseDefinition(raw)
	if err != nil {
		return item.skip(err.Error()), nil
	}
	if parsed.Key() != backupEndpoint.Key {
		return item.skip(fmt.Sprintf("the key does not match the definition (%s)", parsed.Key())), nil
	}
	state := managedendpoint.Get(backupEndpoint.Key)
	if state != nil {
		item.version, item.currentSHA256 = state.Stored.Version, sha256Hex([]byte(state.Stored.Definition))
	}
	prepared, err := r.Endpoints.ValidateRestore(raw, ctx)
	if err != nil {
		return item.skip(err.Error()), nil
	}
	item.definition = prepared.Definition
	if state != nil {
		stored, err := managedendpoint.NormalizeDefinition([]byte(state.Stored.Definition))
		if err == nil && string(stored) == string(prepared.Definition) {
			item.Action = ActionUnchanged
			return item, prepared
		}
		if !options.Overwrite {
			return item.skip(reasonAlreadyExists), prepared
		}
		item.Action = ActionUpdate
		if managedendpoint.NameOrGroupChanges(&prepared.Parsed) {
			item.Reason = reasonNameOrGroup
		}
	} else {
		item.Action = ActionCreate
		ctx.PlannedKeys = append(ctx.PlannedKeys, prepared.Key())
	}
	if token := prepared.PushToken(); len(token) > 0 {
		ctx.PlannedTokens[token] = "another endpoint of the backup"
	}
	return item, prepared
}

func (r *Restorer) planStatusPage(backupStatusPage StatusPage, options Options, refs []statuspage.EndpointRef) *Item {
	item := &Item{Type: TypeStatusPage, ID: backupStatusPage.Slug, Warnings: []string{}}
	storedDefinition, version, exists := statuspage.ManagedDefinition(backupStatusPage.Slug)
	if exists {
		item.version, item.currentSHA256 = version, sha256Hex([]byte(storedDefinition))
	}
	page, definition, warnings, err := r.StatusPages.ValidateRestore([]byte(backupStatusPage.Definition), refs)
	if err != nil {
		return item.skip(err.Error())
	}
	if page.Slug != backupStatusPage.Slug {
		return item.skip(fmt.Sprintf("the slug does not match the definition (%s)", page.Slug))
	}
	item.definition, item.Warnings = definition, describeWarnings(warnings)
	if exists {
		if _, stored, err := statuspage.NormalizeDefinition([]byte(storedDefinition)); err == nil && string(stored) == string(definition) {
			item.Action = ActionUnchanged
			return item
		}
		if !options.Overwrite {
			return item.skip(reasonAlreadyExists)
		}
		item.Action = ActionUpdate
		return item
	}
	item.Action = ActionCreate
	return item
}

// describeWarnings puts the warnings of a status page into words: what it selects without match, and the limit that
// truncates it
func describeWarnings(warnings []statuspage.Warning) []string {
	described := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning.Type == statuspage.WarningTypeTruncated {
			described = append(described, fmt.Sprintf("selects more endpoints than status-pages.maximum-endpoints-per-page (%s): only the first %s are shown", warning.Value, warning.Value))
			continue
		}
		described = append(described, fmt.Sprintf("%s %q selects nothing", warning.Type, warning.Value))
	}
	return described
}

// Apply applies the restore previewed with the given fingerprint, item by item, with the services of the
// administration and author as the author of every change. The failure of an item does not stop the others; when a
// reload of the configuration starts, the remaining items are skipped.
func (r *Restorer) Apply(plaintext []byte, options Options, fingerprint, author string) (*Result, error) {
	plan, err := r.Plan(plaintext, options)
	if err != nil {
		return nil, err
	}
	if plan.Fingerprint != fingerprint {
		return nil, ErrFingerprintMismatch
	}
	result := &Result{Results: make([]*ItemResult, 0, len(plan.Items))}
	reloading := false
	for _, item := range plan.Items {
		itemResult := &ItemResult{Type: item.Type, ID: item.ID, Message: item.Reason, Warnings: item.Warnings}
		switch {
		case item.Action == ActionUnchanged:
			itemResult.Result = ResultUnchanged
		case item.Action == ActionSkip:
			itemResult.Result = ResultSkipped
		case reloading:
			itemResult.Result, itemResult.Message = ResultSkipped, reasonReloadProgress
		default:
			err := r.applyItem(item, author, itemResult)
			switch {
			case err == nil:
			case isCycleInProgress(err):
				reloading = true
				itemResult.Result, itemResult.Message = ResultSkipped, reasonReloadProgress
			case errors.Is(err, common.ErrManagedEndpointVersionMismatch) || errors.Is(err, common.ErrManagedStatusPageVersionMismatch):
				itemResult.Result, itemResult.Message = ResultFailed, reasonChanged
			default:
				itemResult.Result, itemResult.Message = ResultFailed, err.Error()
			}
		}
		result.count(itemResult.Result)
		result.Results = append(result.Results, itemResult)
	}
	logr.Infof("[adminbackup.Apply] Restore by %s: %d created, %d updated, %d unchanged, %d skipped, %d failed", auditAuthor(author), result.Summary.Created, result.Summary.Updated, result.Summary.Unchanged, result.Summary.Skipped, result.Summary.Failed)
	return result, nil
}

func (r *Restorer) applyItem(item *Item, author string, itemResult *ItemResult) error {
	switch item.Type {
	case TypePushKey:
		if _, err := pushkey.Restore(item.ID, item.tokenHash, item.hint, author, r.Endpoints.EndpointTokenGuard()); err != nil {
			return err
		}
		itemResult.Result = ResultCreated
	case TypeEndpoint:
		var err error
		if item.Action == ActionCreate {
			_, err = r.Endpoints.RestoreCreate(item.definition, author)
			itemResult.Result = ResultCreated
		} else {
			_, err = r.Endpoints.Update(item.ID, item.definition, item.version, author)
			itemResult.Result = ResultUpdated
		}
		if err != nil {
			return err
		}
	case TypeStatusPage:
		var detail *statuspage.Detail
		var err error
		if item.Action == ActionCreate {
			detail, err = r.StatusPages.Create(item.definition, author)
			itemResult.Result = ResultCreated
		} else {
			detail, err = r.StatusPages.Update(item.ID, item.definition, item.version, author)
			itemResult.Result = ResultUpdated
		}
		if err != nil {
			return err
		}
		if detail != nil && detail.Definition != nil {
			itemResult.Warnings = describeWarnings(statuspage.SelectionWarnings(detail.Definition))
		}
	}
	return nil
}

func isCycleInProgress(err error) bool {
	return errors.Is(err, managedendpoint.ErrCycleInProgress) || errors.Is(err, statuspage.ErrCycleInProgress) || errors.Is(err, pushkey.ErrCycleInProgress)
}

func (result *Result) count(itemResult string) {
	switch itemResult {
	case ResultCreated:
		result.Summary.Created++
	case ResultUpdated:
		result.Summary.Updated++
	case ResultUnchanged:
		result.Summary.Unchanged++
	case ResultSkipped:
		result.Summary.Skipped++
	default:
		result.Summary.Failed++
	}
}
