# status-page-views Specification

## Purpose
TBD - created by archiving change add-status-page-views. Update Purpose after archive.
## Requirements
### Requirement: Views block of a status page
A status page definition SHALL accept an optional `views` block with `enabled`, `default`, `group-label`,
`environments`, `applications`, `groups` and `endpoints`, in the YAML file and in the administration API. An omitted
or empty block SHALL mean `enabled: [list]`, `default: list` and `group-label: Group`.

#### Scenario: Page without views
- **WHEN** a page has no `views` block
- **THEN** the page offers only the `list` view and its public page shows no view switcher

#### Scenario: Empty block
- **WHEN** a page has `views: {}` or `views: null`
- **THEN** it behaves like a page without the block, and the stored definition keeps what was written (no default is
  written back)

#### Scenario: Default view falls back to the first enabled
- **WHEN** a page has `views.enabled: [matrix, list]` and no `default`
- **THEN** the default view of the page is `matrix`

### Requirement: Validation of the views block
The validation SHALL refuse a `views` block with an unknown view, an empty `enabled`, a duplicated view, a `default`
that is not enabled, more than 10 environments, a duplicated or malformed environment id (other than 1 to 20 lowercase
letters, digits or hyphens), more than 50 applications or a duplicated one, an endpoint entry whose `environment` is
not declared, an endpoint entry without application and environment, more than 5 tags on a group, or any text beyond
its limit. The error SHALL wrap `ErrInvalidViews`, and `go-uptime config validate` SHALL report it like the server.

#### Scenario: Explicit empty list of views
- **WHEN** `views.enabled` is `[]`
- **THEN** the definition is invalid

#### Scenario: Keys colliding after normalization
- **WHEN** `views.endpoints` has the entries `" Group_Billing"` and `group_billing`
- **THEN** the definition is invalid

#### Scenario: Unknown nested field
- **WHEN** an environment of `views.environments` has a field `colour`
- **THEN** the definition is refused by the strict decoding, in YAML and in JSON

#### Scenario: Undeclared environment
- **WHEN** `views.endpoints` classifies a key with `environment: uat` and `views.environments` has no `uat`
- **THEN** the definition is invalid and the error names the key and the environment

#### Scenario: Default not enabled
- **WHEN** `views.enabled` is `[list]` and `views.default` is `matrix`
- **THEN** the definition is invalid

#### Scenario: Application not listed
- **WHEN** an endpoint is classified with an application that `views.applications` does not list
- **THEN** the definition is valid and the application is ordered after the listed ones, alphabetically

### Requirement: Classification of the endpoints
Each endpoint shown by a page SHALL have an application and may have an environment. The classification SHALL come
from `views.endpoints` by endpoint key (lowercase, trimmed); without an entry the application SHALL be the name of the
endpoint and the endpoint SHALL have no environment. The keys of `views.endpoints` of a managed page SHALL be renamed
with the endpoint, like `endpoints` and `featured`.

#### Scenario: Endpoint without classification
- **WHEN** an endpoint named `Billing` is shown and has no entry in `views.endpoints`
- **THEN** its payload has `application: "Billing"` and no `environment`

The application and the environment SHALL be the same in the groups, in the featured endpoints and in the details
payload of an endpoint, with or without results.

#### Scenario: Rename of a classified managed endpoint
- **WHEN** a managed endpoint classified by a managed page that selects it only through its group is renamed
- **THEN** the classification of the page follows the new key in the same transaction, and a failed transaction
  publishes nothing

#### Scenario: Rename onto a stale classification
- **WHEN** the new key already has an entry in `views.endpoints` of a managed page
- **THEN** the entry of the renamed endpoint replaces it and the replacement is logged

#### Scenario: File page classifying the old key
- **WHEN** a page of the configuration file classifies the old key of a renamed endpoint
- **THEN** the rename lists it among the affected pages of the file

### Requirement: Published data of the views
The public payload SHALL carry a `views` object (`enabled`, `default`, `groupLabel`, `environments`, `applications`),
each group SHALL carry the `description` and `tags` of `views.groups` when defined, and each endpoint SHALL carry its
`application` and `environment`. The payload SHALL NOT carry any endpoint key, label, URL or other metadata because of
the views, and the sanitization test SHALL decode it with `DisallowUnknownFields`.

#### Scenario: Group metadata
- **WHEN** `views.groups` defines `Client A: {description: Acme customer, tags: [BR, OCI]}`
- **THEN** the group `Client A` of the payload has that description and those tags, and other groups have neither

The only value of the views not taken from the block SHALL be the default application, the name of the endpoint, which
the payload already publishes. The views SHALL only use the selection published under `maximum-endpoints-per-page`.

#### Scenario: Keys are not published
- **WHEN** a page classifies endpoints by key
- **THEN** no endpoint key appears in the public payload

#### Scenario: Change of the classification is published
- **WHEN** the classification of a page changes through the administration, a restore, a rename or a reload
- **THEN** the next public payload carries the new values and no cached payload of the old revision or generation is
  served

#### Scenario: Backup and restore
- **WHEN** a managed page with a `views` block is backed up and restored, twice, with overwrite
- **THEN** the restored definition is identical, the second restore changes nothing, and the preview of the restore
  predicts the apply

### Requirement: View selection on the public page
The public page SHALL show the view named by the `view` query parameter when it is enabled, else the default view of
the page. With two or more enabled views it SHALL show a view switcher whose buttons expose `aria-pressed`, and choosing
a view SHALL replace the query parameter without reloading the page. Featured endpoints SHALL be shown in their own section by the `list` view and inside their group by the `grouped` and
`matrix` views. After a refresh, a view that is no longer enabled SHALL fall back to the default view and a filter value
that no longer exists SHALL fall back to "All".

#### Scenario: View from the URL
- **WHEN** a visitor opens `/status/acme?view=matrix` on a page that enables `matrix`
- **THEN** the matrix view is shown

#### Scenario: Disabled view in the URL
- **WHEN** a visitor opens `/status/acme?view=matrix` on a page that does not enable `matrix`
- **THEN** the default view of the page is shown

### Requirement: Grouped view
The `grouped` view SHALL show cards with the number of groups, of monitors, of down endpoints in the primary
environment and of down endpoints in the other environments; a banner with the incidents and the affected groups when
there is any; filter chips for the environment and the application ("All" first); an "only groups with problems"
toggle; and one collapsible card per group with its description, tags, environments, applications and status, listing
its endpoints by environment (in the order of `views.environments`, endpoints without environment last) with their
application, name, latest results and 24h, 7d and 30d uptime. The counters SHALL be computed over the whole published
page, not over the filters. A problem SHALL be an endpoint down or pending. The groups SHALL follow the collapsing rules
of the list view (`isCollapsed`), sharing the remembered choices: a group that is not operational SHALL be expanded.

#### Scenario: Filtering by environment
- **WHEN** the visitor selects the environment chip `STG`
- **THEN** only the endpoints of the environment `stg` are listed and the groups without them are hidden

#### Scenario: Nothing matches
- **WHEN** the filters leave no endpoint
- **THEN** the view says that no services match the filters

#### Scenario: Only groups with problems
- **WHEN** the visitor checks "only groups with problems"
- **THEN** only groups with an endpoint down or pending are listed

### Requirement: Matrix view
The `matrix` view SHALL show one row per group and one column per application. Each cell SHALL show the aggregated
status of the endpoints of the pair in the filtered environments, with the rules of the status of a group (unknown
ignored; all up operational; all down down; any other mix degraded), the lowest non-null 30-day uptime of those
endpoints in the primary environment (else in the first environment, in declared order, with a non-null value; else a
dash) and the short names of the environments present. A pair without endpoint in the published page SHALL show "Not
configured", or "Not shown" when the page is truncated; a pair whose endpoints were removed by the environment filter
SHALL show "Not in this environment". A cell SHALL link to the
details page of its endpoint when it has exactly one, and the view SHALL show a legend of the colours.

#### Scenario: Worst status of a cell
- **WHEN** the pair (Client A, Billing) has an endpoint up in `prod` and one down in `qa`, and no environment filter
- **THEN** the cell is "Degraded" and shows the 30-day uptime of the `prod` endpoint

#### Scenario: Removed by the filter
- **WHEN** the pair (Client A, API) has endpoints only in `prod` and the visitor filters `QA`
- **THEN** the cell shows "Not in this environment"

#### Scenario: Not configured
- **WHEN** a group has no endpoint of the application `Reports`
- **THEN** its `Reports` cell shows "Not configured"

### Requirement: Views in the administration
The status page form SHALL have a Views section to enable each view, choose the default one, set the group label,
edit the environments, the order of the applications, the description and tags of the selected groups, and classify
the endpoints of the selected groups, the picked endpoints and the featured ones. Entries the form cannot place SHALL be
kept and listed with a remove button, never dropped silently. The form SHALL send the block only when it differs from
the defaults. The preview SHALL stay the preview of the saved version and SHALL offer its enabled views.

#### Scenario: Saving a page without views
- **WHEN** an administrator saves a page leaving only `list` enabled and nothing else in the section
- **THEN** the stored definition has no `views` block

#### Scenario: Orphan classification kept
- **WHEN** a page classifies a key that is not among the endpoints the form lists, and the administrator saves without
  removing it
- **THEN** the classification is still in the stored definition

#### Scenario: Classifying an endpoint
- **WHEN** the administrator selects the environment `prod` and types `Billing` for an endpoint and saves
- **THEN** the stored definition has that endpoint in `views.endpoints` and the public matrix places it in the `Billing` column

