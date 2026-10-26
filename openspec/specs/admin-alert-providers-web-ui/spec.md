# admin-alert-providers-web-ui Specification

## Purpose
TBD - created by archiving change add-admin-alerting-providers. Update Purpose after archive.
## Requirements
### Requirement: Notifications tab
The administration MUST have a **Notifications** tab in `AdminTabs`, with the routes `/admin/alert-providers`, `/admin/alert-providers/new` and `/admin/alert-providers/{type}/edit`, which MUST open directly from the URL, including on a page reload.

#### Scenario: Direct opening
- **WHEN** the browser opens `/admin/alert-providers/telegram/edit`
- **THEN** the frontend loads the `telegram` form

### Requirement: List of providers
The `/admin/alert-providers` screen MUST list the configured providers (origin Web, YAML or shadowed) and, behind an **Add notification** button, the types not configured yet, searchable by type name. Each configured provider MUST show the readable name of the type, its **source** (Web or YAML) and its **state** (Valid, Invalid with the error, Shadowed with the explanation) as separate columns, the number of endpoints using it and the actions. The usage count MUST be a button that opens the list of the endpoints using the provider, split into managed endpoints (with links to their edit screens) and endpoints of the file. When the managed providers cannot be loaded, the screen MUST show that notice instead of an empty list. The list MUST follow the width, density, icon and card rules of the other administration lists. A YAML provider MUST only have the view action; a Web provider MUST have edit, test and delete; a shadowed Web provider MUST have view and delete.

#### Scenario: Nothing configured
- **WHEN** neither the file nor the web configure a provider
- **THEN** the screen explains that no notification is configured and highlights **Add notification**

#### Scenario: Provider of the file
- **WHEN** the file configures `slack`
- **THEN** `slack` shows the YAML badge without edit or delete, and viewing it shows the webhook masked

### Requirement: Provider form
For the most used types (Slack, Discord, Telegram, Email, ntfy, Microsoft Teams Workflows, Google Chat, Mattermost, Pushover, Gotify, Matrix, PagerDuty, Opsgenie, Custom), the form MUST offer dedicated fields with a label, short help and a link to the type's documentation; secret fields MUST be password inputs, able to reveal what was typed and never the stored value; a stored secret MUST show as "Configured — leave unchanged" with a **Replace** action. For every type, the form MUST have the `default-alert` fields (failure and success thresholds, send on resolved, description) and a YAML mode with the whole block, `overrides` included. A type without dedicated fields MUST open in YAML mode with a commented template. Switching modes MUST keep what was typed and every key the form does not show.

#### Scenario: Telegram through the form
- **WHEN** the administrator picks Telegram, fills in the token and the chat id and saves
- **THEN** the provider is created and the list shows Telegram with origin Web

#### Scenario: Type without a form
- **WHEN** the administrator picks SIGNL4
- **THEN** the form opens in YAML mode with the type's template

### Requirement: Validate, test and save
The form MUST have **Save** as the primary button and **Validate** and **Send test** as secondary ones. *Send test* MUST use the definition being edited, without saving, MUST offer the group to test when the definition has overrides, MUST offer **Trigger and resolve**, on by default, which also sends the resolution, and MUST show the outcome in a toast **and** inline under the buttons, where it stays until the next test, with the provider's error message on failure. A validation error MUST be shown next to its field when the message identifies it, and at the top of the form otherwise. A version conflict MUST keep the draft and offer to load the current definition.

#### Scenario: Failed test
- **WHEN** the administrator tests an `ntfy` with an unreachable URL
- **THEN** an error toast shows the failure and the form keeps what was typed

### Requirement: Deletion
Deletion MUST ask for confirmation in the administration's `ConfirmDialog`, naming the type. When the API answers 409 because endpoints use the provider, the dialog MUST list the keys of those endpoints with links to their edit screens. `ConfirmDialog` MUST gain a slot for that content instead of rendering HTML from its message.

#### Scenario: Provider in use
- **WHEN** the administrator tries to delete `ntfy`, used by `core_api`
- **THEN** the dialog shows `core_api` with a link to its edit screen and nothing is deleted

### Requirement: Alerts in the endpoint form
The **Alerts** section of the managed endpoint form MUST keep accepting one or more alerts per endpoint, including several alerts of the same type (for example, two `ntfy` alerts with different thresholds or descriptions), each with its own type, description, failure and success thresholds and send-on-resolved, and MUST keep the other keys of each alert (`provider-override`, `enabled`, ...) as they are.

Send-on-resolved MUST be a choice of **Provider default / Yes / No**, so that `false` can be chosen over a provider default of `true`; Enabled and the reminder MUST be under *Advanced*. The type selector of each alert MUST offer the configured types — from the file and valid from the web — with their readable name and origin, and the thresholds left empty MUST show as placeholder the value of that provider's `default-alert`. The section description MUST say that the notifications are sent by the providers configured in the Notifications tab or in the configuration file, instead of only the file. With no provider configured, the section MUST explain it and offer a link to `/admin/alert-providers/new`, instead of only "No alerting provider configured.". The links to the Notifications tab MUST open in a new tab, so that an unsaved endpoint is not lost. An existing alert whose type is not configured (or is invalid) MUST stay in the form with a warning and a link to the Notifications tab, and saving MUST keep answering 400 for it.

#### Scenario: Newly configured type
- **WHEN** the administrator creates `telegram` from the web and opens a managed endpoint
- **THEN** Telegram is among the offered alert types

#### Scenario: Several alerts on one endpoint
- **WHEN** the administrator adds a `telegram` alert and two `ntfy` alerts with failure thresholds 3 and 10 to `core_api` and saves
- **THEN** the stored definition has the three alerts, in that order, and each one triggers on its own

#### Scenario: No provider configured
- **WHEN** neither the file nor the web configure a provider and the administrator opens the endpoint form
- **THEN** the Alerts section explains that no notification is configured and links to the creation of one

### Requirement: Standards and tests of the screen
The screens MUST follow the frontend standards (Composition API, props, variable classes and `dark:` in the three themes, English texts) and MUST have an end-to-end test `test/e2e/alert-providers.sh` with `agent-browser` that creates a `custom` provider pointing to a local HTTP receiver, sends the test, attaches the provider to a Push endpoint, makes the endpoint fail and checks that the notification reached the receiver. `docs/screenshots` MUST include the list and the form.

#### Scenario: End-to-end notification
- **WHEN** the script sends enough `status=down` pushes to trigger the alert
- **THEN** the local receiver gets the endpoint's notification

### Requirement: Backup tab
The Backup tab MUST count, label and warn about alerting providers like the other item types: the totals of the backup, the items of the preview (type `alertProvider`, identified by the readable name of the type) and the results of the restore.

#### Scenario: Preview with a provider
- **WHEN** the administrator previews a backup with the `telegram` provider and the `core_api` endpoint
- **THEN** the preview lists "Telegram" as an alerting provider before `core_api`

