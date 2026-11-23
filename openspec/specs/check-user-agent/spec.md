# check-user-agent Specification

## Purpose
The `User-Agent` the HTTP(S), GraphQL and WebSocket checks send: the `go-uptime/1.0` default, the browser presets of the
`user-agent` field of an endpoint, their validation, and how managed endpoints and the endpoint form expose the choice.

## Requirements

### Requirement: Default agent of the checks
HTTP(S), GraphQL and WebSocket checks SHALL send `User-Agent: go-uptime/1.0` when the endpoint sets neither the
`user-agent` field nor a `User-Agent` header. On those checks, a `User-Agent` header of the endpoint, in any letter
case, SHALL be sent as written when the `user-agent` field is not set.

#### Scenario: Endpoint without agent settings
- **WHEN** an HTTPS endpoint has no `user-agent` field and no `User-Agent` header
- **THEN** its requests carry `User-Agent: go-uptime/1.0`

#### Scenario: Custom header
- **WHEN** an HTTPS endpoint has the header `user-agent: CustomAgent/1.0` and no `user-agent` field
- **THEN** its requests carry `User-Agent: CustomAgent/1.0`

### Requirement: Browser presets
An endpoint SHALL accept an optional field `user-agent` with one of the values `go-uptime`, `chrome`, `firefox` or
`edge`. `go-uptime` SHALL send `go-uptime/1.0`. Each browser preset SHALL send the desktop Windows `User-Agent` string
of a stable release of that browser, from one table in the code, and SHALL change no header other than `User-Agent`.

#### Scenario: Chrome preset
- **WHEN** an HTTPS endpoint sets `user-agent: chrome`
- **THEN** its requests carry a `User-Agent` that starts with `Mozilla/5.0 (Windows NT 10.0; Win64; x64)`, contains
  `Chrome/<major>.0.0.0` and does not contain `Edg/`

#### Scenario: Edge preset
- **WHEN** an HTTPS endpoint sets `user-agent: edge`
- **THEN** its requests carry the Chrome format followed by `Edg/<major>.0.0.0`

#### Scenario: Firefox preset
- **WHEN** an HTTPS endpoint sets `user-agent: firefox`
- **THEN** its requests carry a `User-Agent` that contains `Gecko/20100101 Firefox/<major>.0`

#### Scenario: Explicit default
- **WHEN** an HTTPS endpoint sets `user-agent: go-uptime`
- **THEN** its requests carry `User-Agent: go-uptime/1.0`

#### Scenario: WebSocket handshake
- **WHEN** a `wss://` endpoint sets `user-agent: firefox`
- **THEN** the handshake request carries the Firefox string

#### Scenario: HTTP step of a suite
- **WHEN** an HTTPS step of a suite sets `user-agent: edge` and the suite runs against a local receiver
- **THEN** the receiver sees the Edge string on that step

#### Scenario: No other header
- **WHEN** an HTTPS endpoint sets `user-agent: chrome` and no other header
- **THEN** the request carries no `Accept-Language`, `Sec-CH-UA` or `Sec-Fetch-*` header added by Go Uptime

### Requirement: Effective definition stays valid
The preset SHALL be resolved when the request is built and SHALL NOT be written into the headers of the endpoint.
Validating an endpoint twice SHALL give the same result, and the effective YAML of an endpoint with a preset SHALL
contain the `user-agent` field and no generated `User-Agent` header.

#### Scenario: Repeated validation
- **WHEN** `ValidateAndSetDefaults` runs twice on an endpoint with `user-agent: chrome`
- **THEN** both runs succeed and the endpoint headers have no `User-Agent`

#### Scenario: Effective YAML pasted back
- **WHEN** the effective YAML of a managed endpoint with `user-agent: firefox` is submitted as a new definition
- **THEN** it validates without a conflict error

### Requirement: Validation of the agent settings
When an entry of `endpoints` or a step of a suite sets `user-agent` (`go-uptime` included), the configuration SHALL be invalid, fatal at startup and
reported by `go-uptime config validate` if the value is unknown, if the endpoint is not an HTTP(S), GraphQL or
WebSocket check, or if a `User-Agent` header is also set in any letter case. The error SHALL name the endpoint, or the
suite and step.

#### Scenario: Unknown preset
- **WHEN** an endpoint sets `user-agent: opera`
- **THEN** `go-uptime config validate` exits with an error that lists `go-uptime`, `chrome`, `firefox` and `edge`

#### Scenario: Preset and header together
- **WHEN** an HTTPS endpoint sets `user-agent: chrome` and the header `User-Agent: Mine/1.0`
- **THEN** the configuration is invalid and the server does not start

#### Scenario: Preset on a TCP endpoint
- **WHEN** a `tcp://` endpoint sets `user-agent: firefox`
- **THEN** the configuration is invalid

#### Scenario: DNS block with an HTTP-looking URL
- **WHEN** an endpoint with a `dns` block and URL `https://1.1.1.1` sets `user-agent: chrome`
- **THEN** the configuration is invalid

#### Scenario: External endpoint of the file
- **WHEN** an entry of `external-endpoints` in the configuration file has `user-agent: chrome`
- **THEN** the key is ignored like any other unknown key of the file (the file is not decoded strictly), and the
  configuration stays valid

#### Scenario: Invalid step of a suite
- **WHEN** a step of a suite sets `user-agent: opera`
- **THEN** the error names the suite and the step

### Requirement: Agent on hot reload
A change of `user-agent` in the configuration file SHALL take effect through the usual reload cycle. A reload whose new
configuration has an invalid agent setting SHALL follow the existing rule for invalid configurations: with
`skip-invalid-config-update: true` the running checks keep their previous agent; otherwise the process stops with the
error.

#### Scenario: Valid change
- **WHEN** the file changes an HTTPS endpoint from no `user-agent` to `user-agent: edge` while the server runs
- **THEN** after the reload a local receiver sees the Edge string

#### Scenario: Invalid change with skip enabled
- **WHEN** `skip-invalid-config-update` is true and the file gets `user-agent: opera`
- **THEN** the reload is skipped and the checks keep sending the previous agent

### Requirement: Agent of managed endpoints
Endpoints managed through the administration SHALL accept the `user-agent` field under their strict validation, with
the same rules as the file. The stored definition SHALL keep the field as submitted and SHALL NOT contain a generated
`User-Agent` header. The YAML editor, the detail, the backup and the restore SHALL carry the field; it SHALL NOT be
masked. A passive push definition SHALL reject the field.

#### Scenario: Save with a preset
- **WHEN** an administrator saves a managed HTTPS endpoint with `user-agent: firefox`
- **THEN** the stored definition contains `user-agent: firefox` and no `User-Agent` header, and the next check sends
  the Firefox string without a restart

#### Scenario: Reload of a stored preset
- **WHEN** the server restarts with that stored definition
- **THEN** the definition loads without a conflict error

#### Scenario: Conflict from the YAML editor
- **WHEN** a managed definition sets `user-agent: edge` and a `User-Agent` header
- **THEN** validation and save answer the validation error and nothing is stored

#### Scenario: Restore into an empty destination
- **WHEN** a backup with an endpoint that sets `user-agent: chrome` is restored into an instance without that endpoint
- **THEN** the restored endpoint keeps `user-agent: chrome` and its checks send the Chrome string

#### Scenario: Push definition
- **WHEN** a raw push definition with `user-agent: chrome` is submitted
- **THEN** it is rejected as an unknown field

### Requirement: Agent in the endpoint form
The endpoint form SHALL offer a "User agent" select (Go Uptime, Chrome, Firefox, Edge) only for `http(s)://` and
`ws(s)://` URLs without a `dns` block, and SHALL NOT submit `user-agent` when the select is hidden, when Go Uptime is
chosen, when a `User-Agent` header row exists (the select is then disabled with a hint), or for a Push endpoint.

#### Scenario: Choosing a browser
- **WHEN** an administrator picks Chrome on an HTTPS endpoint and saves
- **THEN** the saved document contains `user-agent: chrome`

#### Scenario: Preset first, then header
- **WHEN** an administrator picks Edge and then adds a header row named ` User-agent `
- **THEN** the select is disabled with the hint and the saved document has the header and no `user-agent`

#### Scenario: Header removed
- **WHEN** that header row is removed
- **THEN** the select is enabled on Go Uptime

#### Scenario: URL changed to an unsupported scheme
- **WHEN** an administrator picks Firefox and then changes the URL to `grpcs://example.com:443`
- **THEN** the select is hidden and the saved document has no `user-agent`

#### Scenario: New endpoint switched to Push
- **WHEN** a new, unsaved endpoint with Edge picked is switched to Push and saved
- **THEN** the saved definition has no `user-agent`
