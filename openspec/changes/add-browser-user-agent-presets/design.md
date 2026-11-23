# Design

## Context

`internal/config/endpoint/endpoint.go` defines `DefaultUserAgent = "go-uptime/1.0"`. `ValidateAndSetDefaults` adds it
to `e.Headers` when no `User-Agent` header exists (`hasHeader` compares case-insensitively), and the WebSocket branch of
`EvaluateHealth` adds it to a copy of the headers for the handshake. gRPC, TCP, UDP, SCTP, ICMP, DNS, SSH, STARTTLS and
TLS checks do not use `e.Headers`; gRPC sends its own HTTP/2 headers, grpc-go's `user-agent` included, and
`client.PerformGRPCHealthCheck` ignores the endpoint headers. Note that `ValidateAndSetDefaults` adds the default header
before it knows the type, and returns early for DNS and SSH. Suites call `ValidateAndSetDefaults` on each of their endpoints, so they inherit
whatever this change does there.

Managed endpoints are decoded strictly into `endpoint.Endpoint` (`managedendpoint/definition.go`, `KnownFields(true)`)
and persisted without the defaults that `ValidateAndSetDefaults` adds (`TestPrepare` checks that `User-Agent` is not
in the stored definition). The endpoint form edits a whitelist of keys and keeps the others from the loaded document.

## Goals / Non-Goals

**Goals:**

- Pick a realistic browser agent per endpoint with one word, without copying strings.
- Keep every existing configuration and stored definition working and byte-identical in what it sends.
- Make conflicting or meaningless settings a validation error, caught by `go-uptime config validate`.

**Non-Goals:**

- Impersonating a browser beyond the header: no `Accept`, `Accept-Language`, `Sec-CH-UA`, `Sec-Fetch-*`, no HTTP/2
  settings or TLS fingerprint (JA3/JA4) of a browser. A filter that checks those still sees Go.
- A global default for every endpoint (see Open Questions).
- Mobile or non-Windows variants, random rotation, or a free-text value in `user-agent` (the `User-Agent` header
  already covers a custom string).
- The agent of the alerting providers (Zulip keeps `go-uptime/1.0`) and of the OAuth2/IAP token requests.

## Decisions

### 1. A preset field on the endpoint, not on `client`

`user-agent: chrome` lives at the top level of the endpoint, next to `headers`, because it is resolved into a header
and only matters for the types that send headers. `client` is shared by every type and validated by
`client.Config.ValidateAndSetDefaults`, which does not know the endpoint type.

Alternative: `client.user-agent`. Rejected: it would be accepted on TCP or DNS endpoints without meaning, and the
validation would have to cross packages to tell the type.

### 2. One table of presets, versioned with the project

`internal/config/endpoint/useragent.go`:

```go
// UserAgentPreset is the value of the user-agent field of an endpoint.
type UserAgentPreset string

const (
	UserAgentGoUptime UserAgentPreset = "go-uptime"
	UserAgentChrome   UserAgentPreset = "chrome"
	UserAgentFirefox  UserAgentPreset = "firefox"
	UserAgentEdge     UserAgentPreset = "edge"
)

var userAgentPresets = map[UserAgentPreset]string{
	UserAgentGoUptime: DefaultUserAgent,
	UserAgentChrome:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/<M>.0.0.0 Safari/537.36",
	UserAgentFirefox:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:<M>.0) Gecko/20100101 Firefox/<M>.0",
	UserAgentEdge:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/<M>.0.0.0 Safari/537.36 Edg/<M>.0.0.0",
}
```

`<M>` is the stable major version of each browser when the change is implemented, written literally. The strings follow
the reduced user agent that Chrome and Edge ship (minor, build and patch are `0`) and the format Firefox ships;
Windows 10/11 both report `Windows NT 10.0`. A test pins the format (regular expression per preset) so that a refresh
cannot break it, and a comment above the table states the date of the last refresh and the source (the release pages of
each browser). Refreshing them is a one-line change in a minor release; `create-release` gains a checklist item.

Alternative: compute the version from the date. Rejected: a guessed version that does not exist yet is easier to flag
than a slightly old one.

Alternative: fetch the current version at runtime. Rejected: a network call at startup, and a check whose behaviour
changes without a release.

### 3. Validation in `ValidateAndSetDefaults`, resolution at request time

The preset is **never written into `e.Headers`**. Writing it there would make the validated endpoint carry both
`user-agent: chrome` and `User-Agent: Mozilla/...`: `Effective` (the effective YAML of the detail) would show a
definition that fails its own validation when pasted back, and a second `ValidateAndSetDefaults` on the same object
would report a conflict. Instead:

**Validation**, at the top of `ValidateAndSetDefaults`, before the headers default and before the DNS and SSH early
returns, whenever `user-agent` is not empty (explicit `go-uptime` included — it is a value like the others):

1. Not a key of the table → `ErrEndpointWithInvalidUserAgent` (lists the accepted values).
2. `e.Type()` is not `HTTP` or `WEBSOCKET` (GraphQL is HTTP; a `dns` block wins over an HTTP-looking URL, as in
   `Type()`) → `ErrEndpointUserAgentNotSupported`.
3. `hasHeader(e.Headers, "User-Agent")` → `ErrEndpointUserAgentConflict`.

**Defaulting:** the `go-uptime/1.0` header is added to `e.Headers` only when there is no `User-Agent` header **and** no
`user-agent` field. With the field set, `e.Headers` stays as written. The function stays idempotent.

**Resolution:** `func (e *Endpoint) userAgent() string` returns the string of the preset, or `""` without one. The
three paths that send endpoint headers set it after copying `e.Headers`: `buildHTTPRequest`, the WebSocket branch of
`EvaluateHealth`, and so the copy made by `EvaluateHealthWithContext` for suites. The WebSocket fallback to
`DefaultUserAgent` stays for endpoints evaluated without validation.

Support is defined by "the request path consumes `e.Headers`", which is exactly HTTP(S), GraphQL and WS(S).

Why reject the conflict instead of letting the header win: the header winning silently would hide a preset someone
believed active; YAML users who want a custom string drop the preset.

### 4. Managed endpoints and persistence

`UserAgent UserAgentPreset \`yaml:"user-agent,omitempty"\`` on `endpoint.Endpoint`, so the strict decoding accepts it
with no change in `managedendpoint`. `Prepare` persists the submitted document as today: a submitted
`user-agent: go-uptime` is kept as written (it is valid and harmless); the form never submits it (§5). Because the
preset never reaches `e.Headers`, the effective YAML shows `user-agent: chrome` and no generated header, and pasting it
back validates. The registry reload, the backup (definition bytes) and the restore therefore round-trip without change.

A passive push definition decodes strictly into `endpoint.ExternalEndpoint`, which has no `user-agent`: a raw push
definition that carries it is rejected, as any unknown field. In the configuration file, `external-endpoints` is not
decoded strictly, so the key is ignored there like any other unknown key; the validation of §3 covers `endpoints` and the
steps of suites.

Not a secret: `secrets`/`MaskSecrets` do not touch it.

### 5. The form

`AdminEndpointForm.vue` gets a `Select` "User agent" with Go Uptime (default), Chrome, Firefox and Edge.

- **Visibility** follows the backend, not the monitor type of the form (which falls back to `http` for any unknown
  scheme): shown only when the URL starts with `http://`, `https://`, `ws://` or `wss://` and the document has no `dns`
  block. When it is hidden, the serialization drops `user-agent`, so changing the URL to `grpcs://` after picking a
  preset cannot submit an unsupported preset.
- The shared `Select` (`components/ui/select/Select.vue`) has no `disabled` prop today: the change adds one on its
  interactive button, which also closes an open list.
- **Header row:** a header row whose trimmed name equals `user-agent` case-insensitively (the comparison the
  serialization uses) makes the select disabled, shows the hint "Set by the User-Agent header" and clears the preset
  from the form state, whatever the editing order (preset first or header first). Removing the row enables the select
  again on Go Uptime. The form can thus never serialize both.
- Choosing Go Uptime deletes the key (`setOrDelete`).
- `user-agent` joins `activeFields`: on a new, unsaved endpoint switched to Push the field is dropped (a saved endpoint
  cannot change to Push).
- The server still rejects any conflict that comes from the YAML editor or the API.
- Labels in English, colours from the role utilities.

## Risks / Trade-offs

- **Stale strings** → old versions still pass most filters; the refresh is listed in the release checklist and pinned
  by a format test. A very old browser version may itself be flagged by strict WAFs.
- **False sense of bypass** → the docs state that only the header changes; Cloudflare-type bot management that checks
  TLS fingerprints or JavaScript challenges will still block the check.
- **Terms of service / abuse** → presenting as a browser is the operator's choice on sites they monitor; the default
  stays honest (`go-uptime/1.0`) and the docs recommend allow-listing `go-uptime/1.0` where possible.
- **Stray key on upgrade** → a file that carried an ignored `user-agent` key starts using it; covered in the Migration
  Plan and the release notes.
- **Firewall rules on the agent** → an endpoint switched to a preset no longer matches a rule that allows
  `go-uptime/1.0`; documented in the migration note.

## Migration Plan

No data migration.

- **Upgrade:** the file loader does not decode strictly, so a file that already had a stray `user-agent` key (ignored
  before) becomes meaningful: a browser value changes the agent, an unknown value or a conflict makes the configuration
  invalid. The release notes mention it.
- **Rollback:** an older binary ignores `user-agent` in the YAML file (no strict decoding) and goes back to
  `go-uptime/1.0`. Managed definitions with the field fail its strict decoding: those rows load as invalid, and a backup
  restored there skips them in the plan. Remove the field from managed endpoints before going back.

## Open Questions

- A global default (`endpoint-defaults.user-agent` or similar) for files with many endpoints. Left out: Go Uptime has
  no endpoint defaults section yet, and adding one is a design of its own.
- Whether to also offer `safari` (macOS). Easy to add to the table later.
