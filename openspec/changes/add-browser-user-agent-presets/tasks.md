## 1. Backend

- [x] 1.1 `internal/config/endpoint/useragent.go`: `UserAgentPreset`, the constants, the table with the current stable major of Chrome, Firefox and Edge, a comment with the date and source of the refresh, and `(*Endpoint).userAgent()`
- [x] 1.2 `Endpoint.UserAgent` (`yaml:"user-agent,omitempty"`) with its doc comment; the errors `ErrEndpointWithInvalidUserAgent`, `ErrEndpointUserAgentNotSupported` and `ErrEndpointUserAgentConflict`
- [x] 1.3 Validation at the top of `ValidateAndSetDefaults` (design.md §3), before the headers default and the DNS/SSH early returns; the `go-uptime/1.0` default only without header and without field; update the doc comment
- [x] 1.4 Resolution in `buildHTTPRequest` and in the WebSocket branch, after the copy of `e.Headers`; check that the copy of `EvaluateHealthWithContext` goes through them
- [x] 1.5 Unit tests (table-driven, `t.Parallel()`): each preset and explicit `go-uptime` on HTTP, GraphQL and WebSocket (local servers that record the header), unknown value, every non-header type, `dns` block with an HTTPS URL, conflict in any letter case, repeated validation, no other header added, format regex per preset
- [x] 1.6 Suite tests: `Suite.Execute` against a local receiver sees the Edge string; an invalid step names the suite and the step
- [x] 1.7 Config tests: YAML with a preset loads; an `external-endpoints` entry with `user-agent` loads and the key is ignored; invalid ones fail `parseAndValidateConfigBytes`; `go-uptime config validate` reports the error (`cmd` test)
- [ ] 1.8 Hot reload test (`cmd`): a valid change of the preset is seen by a local receiver; an invalid one with `skip-invalid-config-update: true` keeps the previous agent

## 2. Managed endpoints

- [x] 2.1 `managedendpoint` tests: `Prepare` keeps `user-agent`, stores no `User-Agent`; `Effective` has the field and no generated header, and submitting it back validates; reload of the stored definition; conflict rejected
- [x] 2.2 `managedendpoint` test: a raw push definition with `user-agent` is rejected by the strict decoding
- [ ] 2.3 `adminbackup` test: restore into an empty destination keeps the preset
- [x] 2.4 Check that masking is untouched and the HTTP contract golden still matches (re-record only if an admin fixture changes, and read the diff)

## 3. Frontend

- [x] 3.0 `components/ui/select/Select.vue`: a `disabled` prop on the interactive button (`disabled`, `aria-disabled`), closing an open list when it turns disabled; unit test for mouse and keyboard
- [x] 3.1 `AdminEndpointForm.vue`: "User agent" select shown by URL scheme (`http`, `https`, `ws`, `wss`) and absence of `dns`, dropped from the document when hidden; `setOrDelete` of `user-agent`; a header row named `user-agent` (trimmed, any case) disables it with the hint and clears the preset, in both editing orders; `user-agent` in `activeFields`; role utilities only
- [x] 3.2 `npx vp fmt`, `make frontend-check`, `make frontend-test`, `make frontend-build`
- [ ] 3.3 `test/e2e/admin.sh`: pick Firefox, save, check the YAML and that a local receiver saw the Firefox string; preset then header row (and the reverse); URL changed to `grpcs://` hides the select; YAML → form → YAML round trip keeps the preset
- [ ] 3.4 `make build`, then `docs/screenshots/capture.sh`, with the capture of the endpoint form showing the select

## 4. Documentation and release

- [x] 4.1 README: the `user-agent` field in the endpoint table, with the accepted values and the limits (only the header changes; TLS fingerprint and other headers stay the ones of Go)
- [x] 4.2 `docs/admin-endpoints.md`: the select of the form; `docs/migrating-from-gatus.md`: an endpoint on a preset no longer matches a rule on `go-uptime/1.0`
- [ ] 4.3 `create-release` skill: checklist item to refresh the browser versions of the table; release notes mention the stray-key upgrade case and the rollback of managed definitions
- [x] 4.4 `make lint`, `make test`, `openspec validate add-browser-user-agent-presets --strict`
