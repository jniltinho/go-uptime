## 1. Definition and validation

- [x] 1.1 Add the `Views` types (views, environments, group metadata, endpoint classification) to `internal/config/statuspage` with `ErrInvalidViews`, limits and doc comments
- [x] 1.2 Validate and normalize `views` in `Page.ValidateAndSetDefaults` (defaults, keys in lowercase, environment references)
- [x] 1.3 Unit tests of the validation (valid block, every error of the spec, omitted block = defaults)
- [x] 1.4 Rename `views.endpoints` keys in the rename participant of `internal/statuspage` (group-only pages, collision, file pages listed), with tests
- [x] 1.5 Getters of the effective values; defaults never written back (marshal round-trip test)

## 2. Payload

- [x] 2.1 Add `Views` to `Payload`, `description`/`tags` to `GroupPayload` and `application`/`environment` to `EndpointPayload`
- [x] 2.2 Fill them in `buildEndpointPayload` (groups, featured and details) and the groups in `BuildPayload`
- [x] 2.3 Update the sanitization test and payload tests; record the HTTP contract again if it changes and read the diff
- [x] 2.4 Negative test: no endpoint key in the body of a classified page; backup/restore round trip with `views`
- [x] 2.5 Administration validation warnings for classified keys and group metadata the page does not show

## 3. Public page

- [x] 3.1 `utils/statusPageViews.js` (filters, counters, environment sections, matrix cells, application order) with `statusPageViews.test.mjs`
- [x] 3.2 `ViewSwitcher.vue`, `GroupedView.vue` and `MatrixView.vue` with `dark:` classes and test ids
- [x] 3.3 `StatusPage.vue`: view from `?view=`, switcher, featured above every view, list view unchanged
- [x] 3.4 Slim header (40 px): `PublicLayout.vue`, title block and `StatusSummary.vue`; wider containers and zoom for Full HD, QHD and 4K

## 4. Administration

- [x] 4.1 Views section in `AdminStatusPageForm.vue` (views, default, group label, environments, applications, group metadata, classification table)
- [x] 4.2 Send the block only when it differs from the defaults; load it back from the definition
- [x] 4.3 View switcher in the preview dialog

## 5. Documentation and checks

- [x] 5.1 Document the `views` block in `docs/status-pages.md` and the README
- [x] 5.2 `make lint`, `make test`, `npm run test:unit`, `make frontend-build`
- [x] 5.3 Run locally with an example configuration reproducing the mockups and check the three views in the browser
