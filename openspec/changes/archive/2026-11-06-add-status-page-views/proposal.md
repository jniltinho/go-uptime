## Why

A status page today has a single layout: a list of groups, each with its endpoints. Teams that publish one page per
product to many customers (for example a SaaS operated for several clients, each with production, staging and QA
environments and several applications) cannot answer the questions their readers ask — "is my client affected?",
"is it production?", "which application?" — without reading every row. Two reference mockups (kept out of the repository), "by client"
and "matrix", show the two layouts those readers need. The public header of the page is also taller
and heavier than the content it frames.

## What Changes

- A new optional `views` block in the definition of a status page (YAML and administration API):
  - `enabled`: the views the page offers, among `list` (today's layout, the default), `grouped` (groups → environments
    → applications, with filters and summary cards) and `matrix` (groups × applications, one cell per pair).
  - `default`: the view a visitor sees first; it must be one of the enabled views.
  - `group-label`: the noun used for a group in the texts of the views (for example `Client`), defaulting to `Group`.
  - `environments`: the ordered list of environments of the page (`id`, `name`, `short`); the first one is the
    **primary** environment (the "production" of the incident counters and of the uptime of the matrix).
  - `applications`: the optional order of the applications (columns of the matrix and filter chips).
  - `groups`: optional metadata per group shown by the views (a `description` and up to 5 `tags`).
  - `endpoints`: the classification of each endpoint, by key, into an `application` and an `environment`. An
    endpoint without a classification uses its name as the application and has no environment.
- The public payload gains a `views` object and, per endpoint and per group, only the values the administrator typed
  in that block (`application`, `environment`, `description`, `tags`). No key, label or other endpoint metadata is
  published.
- The public page shows a view switcher when more than one view is enabled, keeps the chosen view in the URL
  (`?view=matrix`), and renders the `grouped` and `matrix` views with environment and application filters and a
  "only groups with problems" toggle. All aggregation of the views is done in the browser from the payload, in a pure
  module with unit tests.
- The administration form of a status page gets a **Views** section: the three views as checkboxes, the default view,
  the group label, the environments, the applications, the group metadata and a classification table of the endpoints
  the page shows.
- The public header becomes slimmer and minimalist: a single compact bar (smaller logo and title, compact theme
  selector), a compact title block and a one-line summary banner.
- Pages without `views` behave exactly as today (`list` only, no switcher).

## Capabilities

### New Capabilities
- `status-page-views`: the `views` block of a status page definition, its validation, its public payload and the
  `list`, `grouped` and `matrix` views of the public page and of the administration form.
- `status-page-public-header`: the slim, minimalist header and summary of the public status pages.

### Modified Capabilities
<!-- None: the existing requirements keep their behaviour; the new fields are additive and covered by the new specs. -->

## Impact

- Backend: `internal/config/statuspage` (new `Views` type, validation, limits), `internal/statuspage/payload.go`
  (views, classification of the endpoints and group metadata in the payload), sanitization and contract tests,
  `internal/api/testdata/http_contract.golden.json` if a recorded payload changes.
- Frontend: `web/app/src/views/public/StatusPage.vue`, new components under `web/app/src/components/public/`
  (`ViewSwitcher`, `GroupedView`, `MatrixView`), new `web/app/src/utils/statusPageViews.js` with unit tests,
  `PublicLayout.vue` and `StatusSummary.vue` (slim header), `AdminStatusPageForm.vue` (Views section).
- Documentation: `docs/status-pages.md` and the README.
- No new dependency, no database migration (the definition of a managed page is stored as YAML), no change for
  existing pages; backups of managed pages carry the new block as part of the definition.
