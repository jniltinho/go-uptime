## Why

Alerting providers (the `alerting` section: Slack, Telegram, email, Discord, ntfy and the other 36 types described in [TwiN/gatus#alerting](https://github.com/TwiN/gatus#alerting)) can only be configured in the YAML file. An administrator who already manages endpoints and status pages from the web still needs access to the server to turn on a notification, and a managed endpoint only accepts alerts of types the YAML already configures. This change closes that gap: notifications are configured, tested and removed from the panel instead of the file.

## What Changes

- Alerting providers managed from the web, stored in the `managed_alert_providers` table (one row per alert type, with a version for optimistic concurrency, timestamps and author), loaded at startup and on every configuration reload, regardless of `admin.enabled`.
- Coexistence with the YAML: a type configured in the file is read-only in the panel and cannot be created from the web (409). If a reload makes the file configure a type that is already managed, the file wins, the managed row is kept as **shadowed** and startup does not fail.
- Managed endpoints can use alerts of both file and managed providers; endpoints of the file keep seeing only the file's providers, so `go-uptime config validate` stays truthful without access to the database.
- API `/api/v1/admin/alert-providers`: list the 41 types with the origin of each one (file, web, shadowed, not configured), get (secrets masked), create, update, delete, validate and **send a test notification** with a definition that has not been saved yet.
- A definition has the same keys as the type's block in `alerting` (including `default-alert` and the per-group `overrides`), decoded strictly, without environment variable expansion and without server resources (`aws-ses` without explicit keys would fall back on the machine's credentials; `client` with certificate files or `identity-aware-proxy`).
- Secrets (tokens, passwords, webhooks, keys) are masked as `********` on every read; receiving the mask back keeps the stored value. The classification of every field of every provider is tested through reflection, so a new provider cannot leak a forgotten field.
- Updating a provider restarts only the managed endpoints that have an alert of that type, so the new `default-alert` applies; deleting a provider used by managed endpoints answers 409 with their keys.
- Administration backup and restore include the managed providers (new item type `alertProvider`, restored before the endpoints).
- Screens: a **Notifications** tab in the administration (`/admin/alert-providers`) with the list of types, a form with the common fields of each type, a YAML editor for the rest, and *Validate* and *Send test* buttons; the endpoint form offers the managed types.

Nothing is **BREAKING**: without managed providers, the behaviour is today's.

## Capabilities

### New Capabilities
- `admin-alert-providers`: persistence, coexistence with the YAML, strict validation, secrets, test notification, effect on managed endpoints and API of the alerting providers managed from the web.
- `admin-alert-providers-web-ui`: the Notifications tab of the administration and its end-to-end test.

### Modified Capabilities
- `admin-endpoint-management`: an alert of a managed endpoint is also valid when its provider is managed, and the applied `default-alert` comes from the effective provider.
- `admin-backup-restore`: the file carries the managed providers, restored before the endpoints.

## Impact

- New package `internal/alertprovider` (definition, masking, registry with an atomic snapshot, service); new table in `internal/storage/store/sql` (SQLite, PostgreSQL, MySQL/MariaDB) and in the memory store.
- `internal/watchdog/alerting.go` and its callers: the provider of a managed endpoint is resolved through the effective snapshot.
- `internal/managedendpoint` (alert validation, restart by type), `internal/adminbackup`, `internal/api` (new routes, `contract_test.go` recorded again), `cmd/serve.go` (load in the reload cycle).
- Frontend: `web/app/src/views/admin/AdminAlertProviders.vue`, `AdminAlertProviderForm.vue`, `AdminTabs.vue`, `utils/adminApi.js`, the endpoint form.
- Documentation: `docs/admin-endpoints.md` (new section), `README.md`, `docs/screenshots`.
