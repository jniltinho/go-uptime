## Context

`alerting.Config` has one field per alert type (41 types), found through reflection by its YAML tag (`GetAlertingProviderByAlertType`). Today it only comes from the file: `ValidateAlertingConfig` validates each provider and merges its `default-alert` into the alerts of the file's endpoints, and the watchdog reads `cfg.Alerting` on every result (`HandleAlerting`). Managed endpoints (`internal/managedendpoint`) validate their alerts against `cfg.Alerting` and only accept types configured in the file.

The administration already has three registries built on the same pattern — `managedendpoint`, `statuspage`, `pushkey`: a definition stored without defaults, a version, an atomic snapshot published after the commit, writes serialized with startup and reload, masked secrets, backup and restore. This change adds a fourth registry, `alertprovider`, on the same mould.

## Goals / Non-Goals

**Goals:**
- Configure, test and delete any of the 41 types from the panel, with a form for the common types and YAML for all of them.
- No restart: a change applies to the next result, and a new `default-alert` is applied again to the managed endpoints that use the type.
- No secret ever returned by the API; no access to the server's credentials or files.
- The file stays self-sufficient: `config validate` does not depend on the database.

**Non-Goals:**
- Several instances of the same type (two named Slacks). Go Uptime's model is one provider per type with per-group `overrides`; changing it touches the alert format of the endpoints and belongs to another change.
- Endpoints of the file using managed providers.
- Encrypting secrets in the database: the tokens of managed endpoints are already stored in plaintext; encrypting one and not the other does not change the threat model.
- Migrating the file's providers to the database automatically.

## Decisions

### 1. One row per type, keyed by `type`
The `managed_alert_providers` table has `type` as primary key, `definition` (YAML without defaults and without the `type` key), `version`, `created_at`, `updated_at`, `updated_by`. It follows `managed_endpoints`, in the three SQL dialects and in the memory store (which refuses writes, as today).
*Alternative:* an id and a free name, allowing several of a type — rejected by the Non-Goal above.

### 2. Effective configuration composed and published atomically
`alertprovider` publishes in an `atomic.Pointer` an **effective** `*alerting.Config`: a shallow copy of the file's `cfg.Alerting` with the fields of the valid, non-shadowed managed types filled through reflection (the same tag → field mapping as `GetAlertingProviderByAlertType`). The snapshot is composed again on load, after every committed write and on every reload. `cfg.Alerting` is never changed.

### 3. Who sees what
The watchdog already records the source of every monitored endpoint (`SourceConfig`, `SourceAdmin`). Every `HandleAlerting` call site (checks, pushes, heartbeats, `SubmitEndpointResult`, suites) resolves its configuration through one function, `alertingConfigFor(key, cfg)`: an endpoint registered with `SourceAdmin` gets the effective snapshot that `alertprovider` hands to the watchdog (`watchdog.SetManagedAlertingConfig`), anything else gets `cfg.Alerting`. The watchdog never imports `alertprovider`.
*Alternative:* everyone uses the effective one — rejected because the file's endpoints would get alerts without the merged `default-alert` (the merge happens while parsing, before the database is open) and `config validate` would no longer predict the behaviour.

### 4. Precedence of the file, including invalid blocks
`ValidateAlertingConfig` sets an invalid provider of the file to nil, which would make "absent" and "rejected" look the same. The configuration keeps the list of types **declared** in the file (`config.Config.AlertingTypesInFile`, filled before validation). A declared type shadows the managed row even when the file's block is invalid: the file states the intent, and the list shows "YAML (invalid)". A shadowed managed row can be viewed and deleted, never edited.

### 5. Strict decoding and full validation
A definition arrives as a `{type, ...fields}` document. The service finds the Go type of the `alerting.Config` field by its tag, strictly decodes the rest (`KnownFields`) into a new value and validates **everything an alert can use**, not only `Validate()`:
- `Validate()` of the provider (base configuration and group uniqueness);
- each group override merged over the base (`Merge` then `Validate` of the provider's `Config`), so that an override cannot produce an unusable configuration;
- `default-alert` through `alert.ValidateAndSetDefaults`.

Server resources are refused **at any depth** of the document (base, `overrides[]`, and also the `provider-override` of the alerts of managed endpoints, which today only checks `client` at the top): `client.identity-aware-proxy`, `client.tls.certificate-file`, `client.tls.private-key-file`, `client.tunnel`; and `aws-ses` without both keys. There is no `os.ExpandEnv` on this path.
The storable version is the received document, normalized without defaults, as in `managedendpoint`.

### 6. Classification of secret fields tested through reflection
`internal/alertprovider/secrets.go` has, per type, the secret paths (e.g. `slack: webhook-url`, `telegram: token`, `email: password`, `custom: url, headers.*, body`) and the public paths. A test walks through reflection every leaf with a YAML tag of each provider (nested structs, maps as `.*`, slices of structs as `[]`, inline fields flattened, `client` included) and fails if a path is in neither list. `overrides[].<path>` shares the classification of `<path>`; `default-alert.*` and `overrides[].group` are public. When in doubt, a field is secret (`custom.body` and `placeholders` can carry tokens).
Masking and restoring follow `managedendpoint/mask.go`; a `********` without a stored value at the same path (same type, same override group) is a 400.

### 7. Safe diagnostics
No text built from a definition reaches a response or a log as is. One function, `redact(text, definition)`, replaces every secret value of the definition and its encoded forms (raw, `url.QueryEscape`, `url.PathEscape`, JSON-escaped) by `********`, and truncates to 512 characters. It is applied to validation errors, load errors (log), the test response, restore results and the delivery errors that `HandleAlerting` logs for managed providers. The test response is structured: `{success, durationMs, status?, error?}`, where `status` is the HTTP status when the provider error carries one.

### 8. Locks and dependency direction
`managedendpoint` imports `alertprovider` (to validate alerts against the effective snapshot). `alertprovider` must not import `managedendpoint`, so the endpoint side registers itself as a **dependents participant** (`alertprovider.RegisterDependents`, called from an `init` of `managedendpoint`, like `statuspage` with `KeyRenameParticipant`). The participant offers:
- `Lock() (unlock func())` — takes `managedendpoint.statesMutex`;
- `Users(type) []string` — keys of **every** managed definition (enabled, disabled, invalid, in conflict) whose raw definition has an alert of the type, read from the stored definitions, not from `Parsed`;
- `Plan(candidate *alerting.Config, type) (Plan, error)` — prepares again, against the candidate snapshot, every managed definition that uses the type, without starting anything;
- `Apply(Plan)` — stops, replaces and starts the affected endpoints.

Lock order: lifecycle change (shared) → `statesMutex` → `alertprovider.mutex` → transaction. Endpoint writes already take `statesMutex`, so a provider write and an endpoint write are fully serialized, and an endpoint can never attach a provider that is being deleted. `alertprovider` never takes `statesMutex` by itself outside this order.

### 9. Applying a write: prepare, commit, stop, publish, start
1. Prepare the provider (decision 5) and build the **candidate** effective snapshot.
2. `Plan` every managed endpoint that uses the type against the candidate. If one of them fails (an override no longer valid for the new provider), the write answers 409 with the keys and the errors, and nothing changes. A deletion fails with 409 if `Users` is not empty.
3. Store in a transaction.
4. Stop the affected endpoints (the registry waits for in-flight executions), **migrate their triggered alerts** (below), publish the candidate snapshot, start them again with the prepared objects.
5. If a start fails after the commit, the endpoint's state becomes invalid with the error, as a reload would leave it, and the response lists it under `warnings`.

**Triggered alerts.** `RestorePersistedTriggeredAlerts` deletes persisted triggered alerts whose checksum changed, and the checksum covers thresholds, description, enabled and send-on-resolved: a new inherited `default-alert` would make an open incident forget its resolve key and send a second "triggered". A new `watchdog.MigrateTriggeredAlerts(old, new *endpoint.Endpoint)` matches alerts by position and type, and for each one that was triggered upserts the persisted row with the new checksum, the same resolve key and the counters, before `RestorePersistedTriggeredAlerts` runs on the new object. Several alerts of the same type keep their own state because the match is positional.

### 10. Order at startup and on reload
In `cmd/serve.go`, after opening the storage: load managed providers → compose the snapshot (marking `shadowed` the types declared in the file, and `invalid` the ones failing validation) → hand it to the watchdog → load managed endpoints. A reload resets the snapshot from scratch, so a storage change never keeps the rows of the previous database. When the managed providers cannot be listed, the snapshot has only the file's providers and the list answers with `managedUnavailable: true`, like the push keys. The rule that `ValidateAlertingConfig` runs before `ValidateEndpointsConfig` keeps applying to the file, unchanged.

### 11. Test notification
`AlertProvider.Send` has no context: a deadline can stop *waiting*, not the send. So:
- the test runs in a goroutine and holds its **slot until `Send` returns**, not until the deadline;
- the response waits up to 15 seconds and then answers `{success: false, error: "the provider did not answer within 15 seconds; the notification may still be delivered"}`;
- the two-slot test semaphore of `managedendpoint` moves to a small shared package (`internal/admintest`) used by both tests, so that endpoint and provider tests share the same two slots;
- most providers use `client.GetHTTPClient`, whose timeout bounds the send; the slot is the backstop for the ones that do not (SMTP, AWS SES).

The synthetic endpoint is `Go Uptime test` in the optional group of the request (to pick the override), with a synthetic failed result whose error says it is a test.

### 12. API contract
Same conventions as the managed endpoints: `GET` answers an `ETag` with the version; `PUT` and `DELETE` require `If-Match` (428 without it, 412 when malformed or stale, as for the endpoints). Every error goes through one mapping helper, `adminAlertProviderError`.

### 13. Frontend
- `AdminAlertProviders.vue` reuses `AdminListLayout`, the fixed table, sticky header, cards under 768 px and `AdminActionButton`. **Source** (Web / YAML) and **State** (Valid / Invalid / Shadowed) are separate columns. The usage count is a button that opens the list of endpoints using the provider, split into managed (with edit links) and file. "Managed providers unavailable" is a notice distinct from an empty list.
- `AdminAlertProviderForm.vue`: Save is primary, Validate and Send test are secondary; Send test has an optional group selector when there are overrides, and its result stays inline under the buttons as well as in a toast. A stored secret is shown as "Configured — leave unchanged" with a **Replace** action; switching between form and YAML is lossless (unknown keys kept). A version conflict keeps the draft and offers to compare with the current version.
- Fields per type live in the pure module `utils/alertProviderFields.js`, unit-tested against a list generated by Go (`testdata/alert-provider-fields.json`, written by a Go test with `-update`).
- `ConfirmDialog.vue` gains a default slot, so that the deletion dialog can list the dependent endpoints with links instead of embedding HTML in the message.
- Endpoint form, Alerts section: send-on-resolved becomes **Provider default / Yes / No** (today an unchecked box deletes the key, so `false` cannot be chosen over a `true` default); empty fields show the effective default of the provider as placeholder; Enabled and the reminder are under *Advanced*; unknown keys and the order of the alerts are kept in both modes; "Configure notifications" opens the Notifications tab in a new tab, so the unsaved endpoint is not lost.

## Risks / Trade-offs

- [A `custom` or `n8n` provider pointing to an internal address (SSRF)] → an administrator can already do the same with an HTTP endpoint; the route is for administrators only and audited. Document it.
- [A new upstream provider without classification leaks a secret] → the reflection test fails before the merge.
- [A provider error carrying a secret in its text, possibly encoded] → `redact` (decision 7) covers raw and encoded forms and truncates; tested for Slack, Telegram and Custom. A secret the administrator typed into a *public* field is outside what classification can protect.
- [A test notification that outlives its deadline] → its slot stays taken until `Send` returns, so at most two sends run at once; the response says the notification may still arrive.
- [Restarting many endpoints when a popular provider changes] → the registry restarts each endpoint without stopping the others; the same cost as a partial reload. The response lists the restarted keys.
- [A shadowed type confuses the administrator] → badge and explanation in the list; the row is not deleted so that an accidental reload loses nothing.

## Migration Plan

New table created by each dialect's schema at startup; no data migration. Rollback: an older version ignores the table; managed endpoints with alerts of managed-only types show as invalid in its list and work again once the new version is reinstalled. Older versions refuse the new backup (the `alertProviders` list is an unknown key for them): restores must always go from the older version to the newer one.

## Open Questions

- Final list of the types with a dedicated form in the first delivery (proposed: the 14 of the UI spec).
- Showing in the provider's detail which endpoints use it (the spec only has the count).
