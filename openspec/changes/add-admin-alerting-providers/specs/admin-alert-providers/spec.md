## ADDED Requirements

### Requirement: Persistence of managed alerting providers
The system MUST store every alerting provider created from the web administration in the `managed_alert_providers` table of the configured storage (SQLite, PostgreSQL, MySQL or MariaDB), with at most one row per alert type (`slack`, `telegram`, `email`, ...). Each row MUST keep the submitted definition without default values, an integer version incremented on every change, the creation time, the time of the last change and the author of the last change. With a storage that does not support managed providers (memory), writes MUST answer 501, as they do for push keys.

#### Scenario: Provider survives a restart
- **WHEN** an administrator creates the `telegram` provider and Go Uptime restarts
- **THEN** the `telegram` provider is available again, with the same definition and the same version

#### Scenario: Repeated type
- **WHEN** the managed `slack` provider already exists and an administrator tries to create another `slack`
- **THEN** the API answers 409 saying the type is already configured

### Requirement: Load independent of the administration
With a SQL storage, the system MUST load the managed providers at startup and on every configuration reload, **before** the managed endpoints, whatever the value of `admin.enabled`. A row whose definition is no longer valid MUST be kept in the database and listed as invalid with its error, and MUST NOT prevent the startup nor the load of the other rows.

#### Scenario: Administration turned off
- **WHEN** managed providers exist and a reload sets `admin.enabled` to `false`
- **THEN** managed endpoints keep notifying through the managed providers

#### Scenario: Invalid definition in the database
- **WHEN** a managed `ntfy` row has a field that a newer version rejects
- **THEN** Go Uptime starts, the log records the error and the list shows `ntfy` as invalid

### Requirement: Coexistence with the configuration file
A type configured in the `alerting` section of the file MUST take precedence. The administration MUST:
- show a type of the file as read-only, with secrets masked;
- answer 409 to the creation of a type the file already configures.

A type counts as configured in the file when the file declares its block, even when that block is invalid and was discarded by the validation (it is then listed as "YAML (invalid)"). When a reload makes the file configure a type that is already managed, the system MUST use the file's provider, MUST keep the managed row in the database with the state `shadowed` and MUST log a warning. A shadowed row MUST be viewable and deletable, never editable. When the type leaves the file, the managed row MUST apply again on the next reload.

Endpoints of the file MUST resolve only the providers of the file, so that `go-uptime config validate` alone decides whether the file is valid. Managed endpoints MUST resolve the providers of the file and the valid managed ones.

#### Scenario: Type of the file
- **WHEN** the file configures `slack` and an administrator tries to create `slack` from the web
- **THEN** the API answers 409 saying the type is configured in the file

#### Scenario: File starts configuring the type
- **WHEN** the managed `discord` provider exists and a reload adds `discord` to the file
- **THEN** `discord` alerts use the file's configuration and the list shows the managed row as shadowed

#### Scenario: Invalid block of the file still shadows
- **WHEN** the managed `ntfy` is valid and a reload adds to the file an `ntfy` block without `topic`
- **THEN** the file's block is discarded as today, the list shows `ntfy` as "YAML (invalid)" with the managed row shadowed, and managed endpoints' `ntfy` alerts are not sent

#### Scenario: Endpoint of the file with a managed-only type
- **WHEN** the only `telegram` provider is managed and an endpoint of the file has a `telegram` alert
- **THEN** that endpoint's alert is not sent and the log says the provider is not configured, as it does today

### Requirement: Format and strict validation
The system MUST accept a definition in YAML or JSON with the same keys as the type's block in `alerting` (including `default-alert` and `overrides`). It MUST validate everything an alert can use: the provider's `Validate()`, **each group override merged over the base configuration**, and the `default-alert`. In addition, it MUST reject with 400:
- unknown keys;
- a `type` that is not one of Go Uptime's alert types;
- `aws-ses` without explicit `access-key-id` and `secret-access-key`, which would use the machine's credentials;
- at any depth (base, `overrides`), in `client`: `identity-aware-proxy`, `tls.certificate-file`, `tls.private-key-file`, `tunnel` or any other field that reads a file or a credential of the server. The same check MUST apply to the `provider-override` of the alerts of managed endpoints.

The system MUST NOT expand environment variables (`$VAR`, `${VAR}`) in managed definitions.

#### Scenario: Minimal definition
- **WHEN** an administrator validates `{"type": "ntfy", "topic": "ops"}`
- **THEN** the returned effective definition has `url: https://ntfy.sh` and priority 3
- **AND** the storable definition does not contain those defaults

#### Scenario: Invalid provider
- **WHEN** an administrator submits `{"type": "slack"}` without `webhook-url`
- **THEN** the API answers 400 with the provider's validation message and nothing is stored

#### Scenario: Environment variable
- **WHEN** an administrator submits `{"type": "telegram", "token": "${TELEGRAM_TOKEN}", "id": "1"}`
- **THEN** the stored and used token is the literal text `${TELEGRAM_TOKEN}`

#### Scenario: Machine credentials
- **WHEN** an administrator submits `aws-ses` without `access-key-id`
- **THEN** the API answers 400

### Requirement: Masked secrets
Every read of the administration (detail, YAML, list, validation and write responses) MUST replace with `********` the secret fields of the provider — tokens, passwords, API keys, webhooks, URLs carrying credentials —, inside `overrides` too. Receiving `********` back in a secret field MUST keep the value stored at the same path (same type and, in `overrides`, same group); without a stored value at that path, the API MUST answer 400. Every configuration field of every provider MUST be classified as secret or public, and a missing classification MUST fail the tests.

#### Scenario: Editing without typing the token again
- **WHEN** the administrator opens the `telegram` provider, changes `id` and saves with `token: "********"`
- **THEN** the stored token stays the previous one and `id` changes

#### Scenario: Webhook masked in the list
- **WHEN** the managed `slack` provider has a `webhook-url` and an override for the group `core`
- **THEN** neither the list nor the detail carries either URL

#### Scenario: New field without classification
- **WHEN** a provider gains a field that is not in the classification table
- **THEN** the classification test fails naming the type and the field

### Requirement: Administration API of the providers
Under `/api/v1/admin`, with the same authorization, CSRF, body limit and audit rules as managed endpoints, the system MUST offer:
- `GET /alert-providers`: every alert type, each with its origin (`yaml`, `web`, `shadowed`, `none`), validity, version and the number of endpoints (of the file and managed) with an alert of that type;
- `GET /alert-providers/{type}`: the masked definition, in JSON and in YAML, with an `ETag` carrying the version;
- `POST /alert-providers` and `PUT /alert-providers/{type}`;
- `DELETE /alert-providers/{type}`;

`PUT` and `DELETE` MUST require `If-Match` with the version, like managed endpoints: 428 without it, 412 when malformed or stale. When the managed providers cannot be loaded, the list MUST say so (`managedUnavailable`) instead of looking empty.
- `POST /alert-providers/validate`: effective and storable definitions, without storing.

An unknown type MUST answer 404; updating or deleting a type of the file MUST answer 409.

#### Scenario: Concurrency
- **WHEN** two administrators open `email` at version 2 and the first one saves
- **THEN** the second one gets 412 when saving with `If-Match: "2"`

#### Scenario: Complete list
- **WHEN** the file configures `slack` and the web configures `ntfy`
- **THEN** the list has the 41 types, `slack` as `yaml`, `ntfy` as `web` and the others as `none`

### Requirement: Test notification
`POST /api/v1/admin/alert-providers/test` MUST send a test notification through the received definition, saved or not (with `********` resolved from the stored value), without changing any alert state. The response MUST wait at most 15 seconds and then answer that the provider did not answer in time and that the notification may still be delivered; the test MUST keep its slot until the send really ends. Endpoint tests and provider tests MUST share the same two concurrent slots (429 beyond them). The response MUST be structured (success, duration, HTTP status when known, error) and MUST NOT carry secrets, raw or encoded. The notification MUST identify itself as a Go Uptime test, and the optional group of the request MUST select the matching override. With the `resolve` option, the resolution MUST be sent right after an accepted notification, with the resolve key the provider returned, so that a test leaves no incident open; the result MUST say whether the resolution was accepted.

#### Scenario: Test before saving
- **WHEN** the administrator fills in `telegram` and sends a test without saving
- **THEN** the test message reaches the chat and nothing is stored

#### Scenario: Provider error
- **WHEN** the `telegram` token is invalid
- **THEN** the response carries the failure with the status returned by Telegram, without the token

#### Scenario: Test that opens and closes an incident
- **WHEN** the administrator tests a PagerDuty provider with the resolve option
- **THEN** PagerDuty opens an incident and resolves it with the same dedup key, and the result has `resolved: true`

#### Scenario: Provider that does not answer
- **WHEN** the SMTP server of `email` accepts the connection and never answers
- **THEN** the response arrives after 15 seconds saying the notification may still be delivered, and a third concurrent test answers 429 while the send is still running

### Requirement: Effect on managed endpoints
Creating, updating or deleting a managed provider MUST apply without restarting Go Uptime and only after the commit, through the same atomic snapshot the watchdog reads. Before storing, the system MUST prepare again, against the new provider, every managed endpoint whose definition has an alert of the type — enabled, disabled, invalid or in conflict — and MUST answer 409 with the keys and errors if one of them would become invalid, changing nothing. After the commit, it MUST stop the affected endpoints, publish the new configuration and start them again, so that no endpoint ever runs with the new destination and the old `default-alert`.

Triggered alerts MUST survive the restart even when the inherited thresholds, description or send-on-resolved change: the resolve key and the counters of each triggered alert MUST move to the alert at the same position and of the same type in the new endpoint, so that no second "triggered" notification is sent and the resolution still closes the incident. Deleting a provider used by any managed definition MUST answer 409 with the list of keys. Provider writes MUST be serialized with endpoint writes, with startup and with reload, so that an endpoint can never attach a provider being deleted.

#### Scenario: default-alert changed
- **WHEN** the managed endpoint `core_api` has an `ntfy` alert without `failure-threshold` and the `default-alert` of the managed `ntfy` changes from 3 to 5
- **THEN** the alert of `core_api` triggers after 5 failures, without restarting the other endpoints

#### Scenario: Deleting a provider in use
- **WHEN** an administrator deletes the `ntfy` provider used by `core_api`
- **THEN** the API answers 409 naming `core_api` and the provider stays active

#### Scenario: Triggered alert kept
- **WHEN** the `ntfy` alert of `core_api` is triggered and the administrator changes the `ntfy` topic
- **THEN** the resolution is sent to the new topic when `core_api` recovers

#### Scenario: Triggered alert kept when the default thresholds change
- **WHEN** the `ntfy` alert of `core_api` is triggered with the inherited `failure-threshold` 3 and the `default-alert` changes it to 5
- **THEN** no new "triggered" notification is sent and the next success threshold reached sends the resolution with the original resolve key

#### Scenario: Update that breaks an endpoint
- **WHEN** `core_api` has an `ntfy` alert with a `provider-override` that the new `ntfy` definition makes invalid
- **THEN** the update answers 409 naming `core_api` and the provider keeps its previous definition

#### Scenario: Disabled endpoint counts as a user
- **WHEN** the disabled managed endpoint `old_api` has a `telegram` alert and the administrator deletes `telegram`
- **THEN** the API answers 409 naming `old_api`

### Requirement: Safe diagnostics
No text built from a managed definition MUST reach a response or a log with a secret of that definition: validation errors, load errors, test results, restore results and the delivery errors of managed providers MUST replace every secret value, raw or URL- or JSON-encoded, by `********`, and MUST be truncated to 512 characters.

#### Scenario: Webhook quoted by the provider
- **WHEN** the Slack webhook of a managed provider answers 404 and the HTTP client error quotes the whole URL
- **THEN** the test response and the log show the error with the URL replaced by `********`
