## Context

Status pages are defined in YAML (`status-pages.pages`) or through the administration (stored as the YAML of the
definition, strictly decoded into `internal/config/statuspage.Page`). The public payload (`internal/statuspage/payload.go`)
publishes only sanitized fields: groups in display order, endpoints with status, uptime, response time and the latest
results. The public SPA (`StatusPage.vue`) renders a single layout: collapsible groups with one row per endpoint.

The reference mockups (kept out of the repository) need three dimensions per endpoint: the **group** (shown as
"client"), the **environment** (PROD, STG, QA) and the **application** (Billing, API, …). The group exists already. The
other two do not exist anywhere in an endpoint definition.

The mockups are visual references: the interface stays in English and the bars stay the latest results.

## Goals / Non-Goals

**Goals:**
- Let each page enable the `list`, `grouped` and `matrix` views and choose the default one.
- Describe environment and application per endpoint **in the page**, for endpoints of the file and managed ones alike.
- Publish nothing beyond what the administrator typed in the `views` block.
- Keep the layout and the selection of pages without `views` as they are: the payload gains additive fields (`views`,
  `application`, and so on) and the header becomes slimmer for every page, but the list view, its groups and its
  collapsing rules do not change.
- A slimmer public header.

**Non-Goals:**
- Daily aggregated bars ("last 30 days") — the views reuse the latest results already in the payload.
- New storage, new endpoint fields, or using `extra-labels` for display.
- Views on the dashboard (only public status pages and their preview).
- Persisting the visitor's filters (the view goes in the URL; filters live in memory).

## Decisions

### 1. The classification lives in the page, keyed by endpoint key

`views.endpoints` maps an endpoint key to `{application, environment}`.

Alternatives:
- *`extra-labels` of the endpoint*: tied to Prometheus — a managed endpoint may only use labels already registered by
  the file, adding a label changes the metric series of every endpoint, and publishing label values from a page would
  publish metadata the page administrator did not write. Rejected.
- *A new field in the endpoint*: would touch the endpoint schema, the strict validation of managed endpoints, the
  backup and the rename. Rejected for a display concern.
- *Name conventions* ("Billing PROD"): fragile. Rejected.

The one value of the views that does not come from the block is the default application: the **name** of the endpoint,
which the payload publishes already.

The page key is the same lowercase key of `endpoints`/`featured`, normalized the same way. Two entries that collide
after normalization (`" KEY "` and `"key"`) are an error, never a silent merge.

**Rename.** `keyRenameParticipant.PrepareKeyRename` (`internal/statuspage/rename.go`) today only looks at `endpoints`
and `featured`. It is extended to `views.endpoints`, including pages that reach the endpoint only through a group:
- managed pages: an entry of the old key moves to the new key in the same transaction; if the new key already has an
  entry (a stale one: the key did not exist as an endpoint), the moved entry replaces it and the replacement is logged
  with both values — the classification of the endpoint being renamed is the one in use;
- file pages: listed in `AffectedConfigStatusPages` when `views.endpoints` has the old key, like the key lists;
- a change of group changes the key, so it is covered by the same path. Tests: transaction, failure of the transaction
  (nothing published), pages in conflict, and the collision.

Defaults keep the block small: an endpoint without classification uses its **name** as application and has **no
environment**, so a page whose groups are clients and whose endpoints are named after the applications gets a useful
matrix with only `enabled: [list, matrix]`.

### 2. Shape and limits of `views`

```yaml
views:
  enabled: [list, grouped, matrix]   # default [list]; at least one; no duplicates
  default: grouped                   # default: first enabled; must be enabled
  group-label: Client                # 1..30 chars, default "Group"
  environments:                      # at most 10; first = primary
    - id: prod                       # [a-z0-9-], 1..20, unique
      name: Production               # 1..40, default = id
      short: PROD                    # 1..8,  default = upper(id)
  applications: [Billing, API]          # at most 50, 1..40 chars, unique
  groups:                            # at most 50 entries, by group name
    Client A: {description: Acme customer, tags: [BR, OCI]}   # description <= 100, <= 5 tags of <= 20
  endpoints:                         # at most MaximumEndpointKeys entries
    client-a_billing: {application: Billing, environment: prod}
```

Omitted, `null` and `{}` are the same: the defaults. Inside the block an omitted list means its default, but an explicit
empty `enabled: []` is an error (a page must offer a view). The defaults are **not written back** into the struct:
effective values come from getters (`EffectiveEnabled`, `EffectiveDefault`, `EffectiveGroupLabel`, `Short()`), so
`yaml.Marshal` of a parsed definition keeps what the administrator wrote — the stored YAML, the backup fingerprint and
the restore plan compare the same bytes before and after. The default of `short` is the id in uppercase cut to 8
characters. Keys of the maps: endpoint keys up to `MaximumEndpointKeyLength`, group names up to `MaximumGroupLength`.
Unknown fields anywhere inside the block are refused by the strict decoding (YAML and JSON).

Validation errors (`ErrInvalidViews` wrapping the detail) for: unknown view, empty `enabled`, `default` not enabled,
duplicated environment id or application, an `environments[].id` with invalid characters, an endpoint whose
`environment` is not declared, length and count limits, and an endpoint entry with neither field. An `application` not
listed in `applications` is valid: it is ordered after the listed ones, alphabetically. Groups and endpoints of the
block that the page does not show are ignored (the administration validation reports them as warnings, like missing
groups today).

Because `default` falls back to the first enabled view and `enabled` to `[list]`, an omitted block and an empty block
mean the same thing.

### 3. Payload

`Payload.Views` (always present):

```json
"views": {"enabled": ["list"], "default": "list", "groupLabel": "Group",
          "environments": [{"id":"prod","name":"Production","short":"PROD"}],
          "applications": ["Billing","API"]}
```

The `views` object has public types of its own (`ViewsPayload`, `EnvironmentPayload`); the map by key is never
serialized. `GroupPayload` gains `description` and `tags` (omitted when empty) and `EndpointPayload` gains `application`
(always set: classification or name) and `environment` (omitted when none). They are filled in `buildEndpointPayload`,
which is shared by the groups, the featured endpoints and the details payload, so the three answer the same values,
with or without results. The sanitization test (`DisallowUnknownFields`) is updated and gets a negative test: a
classified page must not contain any endpoint key in its body.

Cache and hot reload: the snapshot of a page is immutable and the cache key is `slug|revision|generation`, so a change of
the block through the administration (new revision), a restore, a rename or a reload (new generation) is never served
from an old entry. Tests change the classification, the environments and the limit in each path.

### 4. Aggregation in the browser

The views only see the **published** selection: the payload is already cut by `Select(..., Published.MaximumEndpoints)`,
and nothing in the views widens the selection or authorizes another route.

Featured endpoints carry their group: in the `grouped` and `matrix` views they are placed back in their group (the
featured section is shown only by the `list` view), so a featured endpoint never leaves an empty cell.

States (per endpoint: `up`, `down`, `pending`, `unknown`):

| Endpoints of a group or cell        | Status       |
|-------------------------------------|--------------|
| all up                              | operational  |
| all down                            | down         |
| any mix of up/down/pending          | degraded     |
| only unknown (or none with results) | unknown      |

This is the rule the server applies to groups (`aggregateStatus`): unknown endpoints are ignored. "Problem" (for the
"only groups with problems" toggle and the affected groups) is any endpoint `down` or `pending`. The counters of the
cards and of the banner are **global** for the published page (not filtered): groups, monitors, down endpoints in the
primary environment, down endpoints in the other environments (endpoints without environment count as "other"), and
affected groups. A page without environments shows only the groups, monitors and incidents cards.

Uptime of a cell: among the filtered environments, the lowest non-null 30-day uptime of the endpoints of the primary
environment; else of the first environment (in declared order, then endpoints without environment) with a non-null
value; else a dash.

Empty cell: "Not configured" when the pair has no endpoint in the published page at all; "Not in this environment" when
it has endpoints but the environment filter removed them; on a truncated page the first becomes "Not shown", because
the pair may exist beyond the limit. A filter that leaves nothing shows "No services match the filters".

`web/app/src/utils/statusPageViews.js` (pure, `npm run test:unit`) computes from the payload:
- the filtered groups (`environment`, `application`, `onlyProblems`), each with its environments → endpoints;
- the counters: groups, monitors, down endpoints in the primary environment, down endpoints elsewhere, affected groups;
- the matrix: rows = groups, columns = applications (ordered as in decision 2), each cell with the aggregated status of
  the endpoints of the pair in the filtered environments (the same rule as the groups: operational/degraded/down/unknown),
  the 30-day uptime of the primary environment (the lowest of its endpoints; else the first environment with data), and
  the short names of the environments present; a pair without endpoint is "not configured".

Server-side aggregation was considered; it would duplicate the payload for each filter combination. The payload is
already complete and capped by `maximum-endpoints-per-page`, so the browser computes it.

### 5. View selection

The view comes from `?view=` when it is enabled, else `views.default`. The switcher (`role="tablist"`-like segmented
buttons with `aria-pressed`) is shown only with two or more views and replaces the query with `router.replace`. The `list`
view is today's layout unchanged. The `grouped` view reuses `isCollapsed` and the remembered choices of
`utils/statusPageGroups.js` with the same storage keys (SHA-256 of slug and group): a group that is not operational is
always expanded, and the choice of a visitor is shared by the two views. After a refresh, a view that is no longer
enabled falls back to the default, and a filter value that no longer exists falls back to "All". Polling,
`credentials: 'same-origin'` and the checks of `requestGeneration` stay as they are.

### 6. Slim header and large screens

`PublicLayout.vue`: height 40px (`h-10`), logo `h-5 w-5`, title `text-sm`, compact theme selector. Large screens: the grouped and matrix views and the header use up to 1760px with 48px of inner padding (128px of space on each side in Full HD) and the list `max-w-6xl`, 1400px from `2xl`, so the side margins stay small; `.public-scale` applies `zoom: 1.25` from 2400px and `1.6` from 3200px of viewport (QHD and 4K at 100% scaling), only on the public layout. `zoom` scales the layout (not only the paint), so widths and hit areas stay right. The page title
block uses `text-xl sm:text-2xl` with the description in `text-sm`, and `StatusSummary.vue` becomes a single compact line
(`py-2 text-sm`). Containers widen to `max-w-6xl` for the matrix to fit five or more columns. The bio theme navy header
keeps redefining its variables inside `.app-header`.

### 7. Administration form

A **Views** section in the left column: three checkboxes, a select for the default, the group label, an editable list
of environments (id, name, short, reorder, remove), a textarea of applications (one per line), a list of group metadata
for the selected groups, and a classification table listing the endpoints the page would show (from `options`: the
endpoints of the selected groups plus the picked keys) with an application input (datalist of the applications) and an
environment select. The table lists the endpoints of the selected groups, the picked keys and the featured ones. Entries
the form cannot place (a classification of a key the page does not show now, metadata of a group not selected) are
**kept** and listed in an "Other entries" area with a remove button: saving never drops them silently. The block is sent
only when it differs from the defaults, so saving a page without views keeps its YAML unchanged. The preview stays the
preview of the **saved** version, now with its view switcher. The preview dialog gets the same view switcher.

## Risks / Trade-offs

- [The classification is per page: the same endpoint on two pages is classified twice] → acceptable; pages usually
  target different audiences. The administration lists only the endpoints of the page.
- [Renaming an endpoint leaves a stale classification on file-defined pages] → same behaviour as `endpoints:` today;
  managed pages are renamed by the existing participant, extended to `views.endpoints`.
- [Large payload on a page with many endpoints] → no new per-endpoint data besides two short strings; the existing limit
  of endpoints per page still applies.
- [The matrix with many applications on a phone] → the matrix scrolls horizontally inside its own container with a
  sticky first column.

## Migration Plan

Additive. No migration; omitted `views` = today's layout and selection. Backup and restore carry the block inside the
definition: tests cover export → restore, a repeated restore (idempotent), overwrite, preview → apply equality, and a
page with a login (the masked hash is merged before validation, as today). Rollback: a definition saved with `views` would be refused
by an older binary's strict decoding — documented in the release notes (remove the block before downgrading).

## Open Questions

- None blocking. Daily bars for 30 days can come later through `endpoint_response_time_buckets`.
