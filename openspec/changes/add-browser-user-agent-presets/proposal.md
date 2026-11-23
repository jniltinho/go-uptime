# Proposal

## Why

Every HTTP(S), GraphQL and WebSocket check sends `User-Agent: go-uptime/1.0` (`endpoint.DefaultUserAgent`), and the
only way to change it is to type a full `User-Agent` header on each endpoint. Sites behind a WAF, a CDN or a bot filter
often answer a non-browser agent with 403, a challenge page or a different body, so the check measures the filter
instead of the site. Administrators end up copying browser strings by hand, which go stale and differ from one
endpoint to the next.

## What Changes

- New optional endpoint field `user-agent`, a preset: `go-uptime` (the default, today's `go-uptime/1.0`), `chrome`,
  `firefox` or `edge`. Each browser preset sends the desktop `User-Agent` string of the current stable release of that
  browser on Windows, kept in one table in the code and refreshed when the project updates it.
- The preset applies to the checks that send the endpoint headers — HTTP(S), GraphQL and WebSocket — including the
  steps of suites. Setting it on another type (gRPC, TCP, UDP, SCTP, ICMP, DNS, SSH, STARTTLS, TLS) is a configuration
  error. The preset is resolved when the request is built and never written into the headers, so the effective YAML
  stays valid when pasted back.
- An endpoint that sets both `user-agent` and a `User-Agent` header (any case) is a configuration error: the two would
  silently compete. A `User-Agent` header alone keeps working exactly as today.
- An unknown preset is a configuration error: fatal at startup and reported by `go-uptime config validate`.
- Managed endpoints accept the field under the strict decoding; the endpoint form of the administration gets a
  "User agent" select (Go Uptime, Chrome, Firefox, Edge) on HTTP endpoints; the YAML editor, the backup and the restore
  carry it like any other field. The form never sends the default (choosing Go Uptime removes the key), never sends a
  preset together with a `User-Agent` header row, and hides the select for URLs that are not HTTP(S) or WS(S).
- Documentation: README (endpoint fields), `docs/admin-endpoints.md` and the note of `docs/migrating-from-gatus.md`
  about allowing the checks through a firewall by their agent.
- Not a change: the default stays `go-uptime/1.0`; the Zulip provider keeps its own agent; no other header (`Accept`,
  `Sec-CH-UA`, ...) is added and the TLS fingerprint stays the one of Go.

## Capabilities

### New Capabilities

- `check-user-agent`: the `User-Agent` the checks send — the default, the browser presets, the precedence with the
  `User-Agent` header, the validation, and how managed endpoints and the endpoint form expose the choice.

### Modified Capabilities

None. `project-identity` requires the default agent to be `go-uptime/1.0` and replaceable by the `User-Agent` header of
each endpoint; both stay true.

## Impact

- `internal/config/endpoint/`: new field and validation, a `useragent.go` with the preset table, the validation in
  `ValidateAndSetDefaults`, the resolution in `buildHTTPRequest` and in the WebSocket branch; new errors.
- `internal/managedendpoint/`: nothing to change in the decoding (the field is part of `endpoint.Endpoint`); tests that
  the stored and the effective definitions keep the preset without a generated `User-Agent` header.
- `web/app/src/pages/admin/AdminEndpointForm.vue`: the select, `user-agent` in `activeFields` (removed from a push
  definition).
- Contract test of the HTTP API: unchanged unless an admin response is recorded with an endpoint that sets the field.
- Docs: `README.md`, `docs/admin-endpoints.md`, `docs/migrating-from-gatus.md`; the screenshot of the endpoint form
  (`docs/screenshots/capture.sh`).
- No new dependency, no storage migration.
