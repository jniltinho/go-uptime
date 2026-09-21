<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
# Public status pages

> Not in Gatus, the project that Go Uptime derives from: there, the
> request for public status pages was closed as *not planned* ([TwiN/gatus#1311](https://github.com/TwiN/gatus/issues/1311)).

A status page is a public page, open **without login** at `/status/<slug>`, that only shows the selected groups and
endpoints, like the *Status Pages* of Uptime Kuma. The dashboard, the administration and the status API stay protected
by `security.basic` or `security.oidc`.

Each endpoint is shown with its name, current status, bars of the latest checks and uptime over 24 hours, 7 days and
30 days, and its name opens a public details page with the response time chart, like the endpoint details page of the
dashboard.

## Configuration

```yaml
status-pages:
  enabled: true                          # defaults to true; false unpublishes every page
  trusted-proxies: ["172.30.0.1/32"]     # see "Behind a reverse proxy"
  rate-limit: 120                        # 404 responses per minute per IP; 0 disables it
  maximum-endpoints-per-page: 400        # how many endpoints a page shows; 1 to 1000, defaults to 400
  pages:
    - slug: services
      title: "Services"
      description: "External websites and APIs"
      groups: [sites, apis]
      featured: [sites_github]           # shown at the top of the page, with more details
      show-certificate-expiration: true  # days until the TLS certificate expires, below the name
    - slug: infrastructure
      title: "Infrastructure"
      groups: [dns]
      endpoints: [core_database]         # keys of individual endpoints (group_name)
      enabled: false                     # pages of the file are published by default
```

| Field | Rule |
|-------|------|
| `slug` | 1 to 64 characters: lowercase letters, digits and hyphens, not starting or ending with a hyphen. Unique. `new`, `options`, `preview`, `validate` and `exposure` are reserved. |
| `title` | Required, up to 100 characters. |
| `description` | Optional, up to 1000 characters, plain text (no markdown or HTML). |
| `groups` | Up to 50 groups. Every enabled endpoint of the group is shown, including the ones created later. |
| `endpoints` | Up to 1000 keys in the `group_name` format (the same key as the badges). How many of them are shown is [`maximum-endpoints-per-page`](#maximum-number-of-endpoints-per-page). |
| `featured` | Up to 10 endpoint keys shown at the top of the page, in cards with more details. They are part of the selection of the page and are not repeated in their group. |
| `charts` | **Deprecated and ignored.** Every endpoint of the page now has a details page with its response time chart. Still accepted, with a warning, so that pages saved by `v5.36.0-fork.2` stay valid; saving the page in the administration removes it. |
| `show-certificate-expiration` | Optional, defaults to `false`. Shows below the name of each endpoint how many days are left until its TLS certificate expires, like the *Show Certificate Expiry* option of Uptime Kuma. |
| `show-messages` | Optional, defaults to `false`. Shows, on the details page of each endpoint, the same table of checks as the dashboard, with the message and the origin of each result. Only the messages of pushes and heartbeats, the HTTP status of the checks (`HTTP 200`) and the [reason of a failure](#reason-of-a-failure) are published, never the errors of the checks. The message of a push is published as it was sent. |
| `groups-collapsed` | Optional, defaults to `false`. The groups of the page start collapsed, each one showing its status and its counts; a visitor expands the ones they want. A group that is not operational always appears expanded, see [Collapsible groups](#collapsible-groups). |
| `auth` | Optional. Requires a username and a password to see the page: `username` and `password-bcrypt-base64`. See [Login of a page](#login-of-a-page). |
| `enabled` | Pages of the file: defaults to `true`. Pages managed through the web: defaults to `false`. |

A page must select at least one group, endpoint or featured endpoint. A group or key that does not exist yet does not invalidate the page:
the load logs a warning and the administration shows the warning when validating.

### Maximum number of endpoints per page

`status-pages.maximum-endpoints-per-page` is how many endpoints a page shows: an integer from `1` to `1000`, `400` by
default (it was a fixed `200` up to `v6.2.0`). Anything else (`0`, `1001`, `2.5`) invalidates the configuration, also in
`go-uptime config validate`. It applies to every page, of the file and managed through the web, and a change takes effect
when Go Uptime reloads its configuration file, without restart.

A page that selects more endpoints than the limit stays published and shows the first ones in display order (the
featured endpoints, then the sections), with the notice "Showing the first N services". The administration marks it in
the listing (`400+`) and warns when validating the page.

- **The limit is an access rule, not only a display rule.** An endpoint beyond the cut is not on the page: its details
  page, its chart, its event stream and its badges answer the same 404 as a key that does not exist. Raising the limit
  therefore **publishes more endpoints** on the pages that were truncated: look at what those pages select before
  raising it.
- **Two different limits.** A definition accepts up to 1000 keys in `endpoints` whatever the limit in force; the limit
  decides how many endpoints are shown, counting the ones reached through `groups`.
- **Cost of a high limit.** Measured with 1000 endpoints and 50 results each: the payload of a page is about 4 MiB
  (under 40 KiB on the wire with gzip) and takes about 20 ms to assemble, against 1.5 MiB and 10 ms with 400. In the
  browser, a page with everything expanded has about 70,000 DOM nodes and 56 MiB of heap, and expanding 20 groups at
  once takes half a second; with [`groups-collapsed: true`](#collapsible-groups) it has 275 nodes and 6 MiB. Use
  `groups-collapsed` on pages with many hundreds of endpoints. The numbers come from `TestMeasureEndpointLimit`
  (`GO_UPTIME_MEASURE_ENDPOINT_LIMIT=1 go test ./internal/statuspage/ -run TestMeasureEndpointLimit -v`).

The default `config.yaml` of the fork (Docker image and release tarballs) already ships the `/status/services` and
`/status/infrastructure` pages with example endpoints.

## Pages managed through the web

With the [administration](admin-endpoints.md) enabled, the **Status pages** tab at `/admin/status-pages` lets you create,
edit, preview, publish, unpublish and remove pages. They are stored in the `managed_status_pages` table of the same
database as the endpoints (SQLite, PostgreSQL, MySQL or MariaDB).

- A new page is created **disabled**, also through the API: check the preview and tick **Published**.
- The form is split into sections (General, Groups and Endpoints): the slug has **Copy link** and **Open** buttons,
  groups are cards with a counter, the list of endpoints has a search, an **Only selected** filter and the count of
  featured endpoints (at most 10), and the preview of the saved version is collapsible.
- The pages of the configuration file are shown in the list for reference only.
- If the configuration file starts using the slug of a page managed through the web, the file wins: the web page is
  marked as in conflict and is not published until the file stops using the slug.
- Turning `admin.enabled` off does **not** unpublish the pages managed through the web (only the administration goes
  away). To take every page down, use `status-pages.enabled: false`.
- The endpoint form warns on which status pages the endpoint will be shown, by group or by key.

## What the page shows

- A **status banner** with the state of the page in words and colour ("All systems operational", "Partial outage",
  "Major outage" or "No data"), how many endpoints are up and down (`12 up · 2 down`, with pending and without data
  only when there are any) and when the page was assembled. The counts come from the server, so they stay right on a
  page that is truncated by [`maximum-endpoints-per-page`](#maximum-number-of-endpoints-per-page).
- **Featured** endpoints first, in cards with the uptime and the average response time over 24 hours, 7 days and
  30 days, the last response time, the check bars and a **View details** link.
- The name of every endpoint links to its details page (see below).
- With `show-certificate-expiration: true`, a small line below the name of each endpoint with a TLS certificate says how
  many days are left until it expires ("Certificate expires in 73 days"), in the secondary color, amber from 14 days and
  red from 7 days or once expired. The date is not published. HTTP, TCP and ICMP endpoints without TLS show nothing.
- Sections in the order of `groups`, then the groups only reached through `endpoints` (in alphabetical order) and, last,
  **Other services** with the endpoints without group. Within each section, endpoints are sorted by name.
- Endpoints of the file, external endpoints and endpoints managed through the web, as long as they are enabled. Suites
  and `remote` instances are left out.
- At most [`maximum-endpoints-per-page`](#maximum-number-of-endpoints-per-page) endpoints per page, 400 by default; above
  that, the page says how many it shows ("Showing the first 400 services").
- The latest 50 results of each endpoint (or `storage.maximum-number-of-results`, if lower).

Statuses:

| Status | Endpoint | Group and page |
|--------|----------|----------------|
| Operational / Up | last result succeeded | every endpoint with results is up |
| Pending (yellow) | last result is [Pending](push-monitoring.md#pending-status-and-retries) | — |
| Partial outage | — | any other combination, including only pending endpoints |
| Major outage / Down | last result failed | every endpoint with results is down |
| No data | no result yet | no endpoint has results |

The uptime is shown as "—" when there was no check during the period. With SQLite, PostgreSQL and MySQL, the history older
than 48 hours is aggregated per day, so the edges of the 7 and 30 day periods are approximate, as in the badges.

## Collapsible groups

The header of every group is a button that collapses and expands its endpoints, with the mouse, `Enter` or `Space`.
Collapsed or not, the header shows the name of the group, its status and how many of its endpoints are in each state
(`3 up`, `2 up · 1 down`), so a collapsed group still answers "is it fine?". The featured endpoints are not collapsible.

Which state a group is shown in is decided again every time the page gets fresh data, in this order:

1. **A group that is not operational is expanded.** A problem never starts hidden: not with `groups-collapsed: true`,
   and not when the visitor had collapsed that group. A group whose endpoints have no data yet counts as not operational.
2. Otherwise, **the choice of the visitor** for that group.
3. Otherwise, the default of the page: collapsed with `groups-collapsed: true`, expanded without it.

A visitor may collapse a group during an incident, but it does not stick: the next refresh opens it again, and it is not
remembered. When the group recovers, the choice the visitor had made before applies again.

- **The page refreshes every 60 seconds** and when its tab becomes visible again, over a payload cached for up to 30
  seconds. A group that starts failing is opened on the next refresh, up to 90 seconds later — the same delay with which
  the page shows any change. Meanwhile the collapsed header already shows the new status and counts of the last payload.
- **The choice is remembered in the browser of the visitor**, per page and per group, and holds for the visit even when
  the browser cannot store anything. What is stored is a SHA-256 of the slug and of the name of the group, never the
  name itself, because a page can require a login and should leave no readable trace on a shared computer. The keys
  need a secure context (HTTPS or localhost): over plain HTTP the choice lasts until the page is closed.
- The preview of the administration shows what a new visitor sees: it ignores, and does not change, the choices stored
  by the public page.

## Endpoint details page

`/status/<slug>/endpoints/<key>` is open without login for every endpoint shown on a published page, and has the layout
of the endpoint details page of the dashboard (`/endpoints/<key>`), in the order of the monitor page of the Uptime Kuma:

- the days until the TLS certificate expires, below the title, when the page has `show-certificate-expiration: true`;
- the bars of the latest checks;
- the panel of numbers of the dashboard: response time of the last check, average response time over 24 hours and uptime
  over 24 hours, 7 days and 30 days;
- **Response Time Trend**: the same chart as the dashboard, in the format of the chart of the Uptime Kuma, with the
  Recent / 3h / 6h / 24h / 1w selector (see [response time chart](#response-time-chart));
- **Checks table**, collapsed by default: status (Up, Down or Pending), date and time and response time of the latest
  checks, or, with `show-messages: true`, the same columns as the dashboard (status, date and time, message and origin),
  never with the errors of the checks — a check that failed without answering shows the
  [reason of the failure](#reason-of-a-failure);
- response time and health badges;
- **Events**, collapsed by default like the table of checks (the browser remembers when it is expanded): monitoring
  started, became healthy, was unhealthy for…, the latest 50.

A key of an endpoint that is not on the page, or of a page that is not published, shows "Page not found". The page
updates in [real time](#real-time-updates) and also refreshes every 60 seconds, pausing while the tab is hidden.

## Response time chart

The **Response Time Trend** chart of the endpoint details pages, on the dashboard and on the public status pages,
follows the chart of the monitor page of the Uptime Kuma:

- **Recent** (the default): one point per check or push, the latest 100 results (50 on a public page). A Down result
  is a translucent red column and a Pending result a translucent yellow column.
- **3h, 6h and 24h**: one point per minute; **1w**: one point per hour. The green line is the average response time,
  with lighter lines for the minimum and the maximum. A minute or an hour without Up results and with Down results is
  a red column; one with Pending results, or with Up results mixed with Down or Pending results, is a yellow column.
- The line only has the Up results with a response time of at least 1 ms: a push without `ping` shows only its
  column.
- A long gap without results (more than 10 intervals of the endpoint) breaks the line.
- The browser remembers the chosen period. Recent updates as soon as a result arrives, the other periods at most once
  a minute.

Go Uptime stores the aggregates of every result per minute for 24 hours and per hour for 7 days, in the
`endpoint_response_time_buckets` table (or in memory). **After upgrading, the 3h to 1w periods start empty and fill up
over time**; Recent shows the results already stored right away.

| Route | Access |
|-------|--------|
| `GET /api/v1/endpoints/<key>/response-time-chart?period=recent\|3h\|6h\|24h\|1w` | Same authentication as `/api/v1/endpoints/<key>/statuses`; 404 for an unknown key |
| `GET /api/v1/status-pages/<slug>/endpoints/<key>/response-time-chart?period=...` | Public; the same 404 as the status pages for a key that is not on a published page, then 400 for an invalid period |

```json
{"period":"24h","intervalSeconds":60,"bucketSeconds":60,"from":"2026-09-15T18:01:00Z","to":"2026-09-16T18:00:00Z",
 "buckets":[{"timestamp":"2026-09-16T17:59:00Z","up":1,"down":0,"pending":0,"avgMs":35,"minMs":35,"maxMs":35}]}
```

The responses only have the time, the status and the response time: never messages, errors or hostnames. The public
route is cached for up to 30 seconds (Recent is renewed by a new result recorded by the same instance).

## Real-time updates

The endpoint details pages, on the dashboard (`/endpoints/<key>`) and on the public status pages
(`/status/<slug>/endpoints/<key>`), show a new result within about 2 seconds, without reloading the page: a check, a
push (including `status=pending`), a heartbeat failure or a result of the external endpoint API. The bars, the panel of
numbers, **Response Time Trend** and the table of checks update without a loading indicator. The Recent period of the
chart reloads right away, the other periods at most once every 60 seconds. The page of the status page itself (`/status/<slug>`)
keeps refreshing every 60 seconds.

The browser opens a [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) channel
that only says that the endpoint has a new result; the page then fetches the data through the usual routes. The events
carry no data of the result:

| Route | Access |
|-------|--------|
| `GET /api/v1/endpoints/<key>/events` | Same authentication as `/api/v1/endpoints/<key>/statuses`; 404 for an unknown key |
| `GET /api/v1/status-pages/<slug>/endpoints/<key>/events` | Public; the same 404 as the status pages for a key that is not on a published page |

```text
retry: 3000
id: 41

event: result
id: 42
data: {}

: ping
```

- The stream starts with `retry` and the current sequence of the endpoint, sends `: ping` every 15 seconds and ends after
  5 minutes; the browser reconnects with `Last-Event-ID` (or `?lastEventId=`) and receives an event right away if a
  result arrived in between. `HEAD` only answers the headers.
- **Limits:** 500 open streams in total and 10 per client IP address, both routes together (the address is exact: an
  IPv6 client with temporary addresses gets 10 streams per address). Above the limit: `429 {"error":"too many
  requests"}` with `Retry-After: 30`. During a configuration reload or shutdown the streams are closed and new ones get
  503.
- When the channel cannot open (401, 404, 429, 502 or 503), the page keeps the periodic refresh and tries again after 30
  seconds, doubling up to 5 minutes, or right after a successful refresh.
- Each tab has its own stream, closed while the tab is hidden.
- The cached details of a public page are renewed as soon as the endpoint has a new result, so the page shows it without
  waiting for the 30 seconds of the cache.
- With several instances sharing the same database, only the instance that records a result notifies it: a visitor
  connected to another instance sees it on the periodic refresh.

To test a stream with `curl` (with `security.basic`, the `Accept` header makes a missing login respond 401 without the
`WWW-Authenticate` challenge, like a browser):

```bash
curl -N -H 'Accept: text/event-stream' https://status.example.com/api/v1/status-pages/infra/endpoints/core_api/events
```

## Public API

`GET /api/v1/status-pages/<slug>` responds without authentication:

```json
{
  "slug": "services", "title": "Services", "description": "External websites and APIs",
  "status": "degraded", "updatedAt": "2026-09-14T19:30:00Z", "truncated": false, "groupsCollapsed": false,
  "featured": [{
    "name": "github", "group": "sites", "status": "up",
    "uptime": {"24h": 1, "7d": 0.999, "30d": 0.998},
    "responseTime": {"24h": 180, "7d": 175, "30d": 190},
    "results": [{"timestamp": "2026-09-14T19:29:30Z", "success": true, "durationMs": 171}]
  }],
  "groups": [{
    "name": "apis", "status": "degraded",
    "summary": {"total": 3, "up": 2, "down": 1, "pending": 0, "unknown": 0},
    "endpoints": [{
      "name": "github-api", "status": "up",
      "uptime": {"24h": 0.9993, "7d": 0.998, "30d": null},
      "responseTime": {"24h": 123, "7d": 130, "30d": null},
      "results": [{"timestamp": "2026-09-14T19:29:00Z", "success": true, "durationMs": 123}]
    }]
  }]
}
```

With `show-certificate-expiration: true`, every endpoint with a TLS certificate also has `certificateExpiresInDays`: the
whole number of days until the certificate expires (negative once it expired), from the most recent check with a
certificate. The same field is in the endpoint details API.

`GET /api/v1/status-pages/<slug>/endpoints/<key>` responds, also without authentication, the details of an endpoint
shown on the page, with its latest events (type and time only):

```json
{
  "page": {"slug": "services", "title": "Services"},
  "name": "github-api", "group": "apis", "status": "up", "updatedAt": "2026-09-14T19:30:00Z",
  "uptime": {"24h": 0.9993, "7d": 0.998, "30d": null},
  "responseTime": {"24h": 123, "7d": 130, "30d": null},
  "results": [{"timestamp": "2026-09-14T19:29:00Z", "success": true, "durationMs": 123}],
  "events": [{"type": "START", "timestamp": "2026-09-14T10:00:00Z"}, {"type": "HEALTHY", "timestamp": "2026-09-14T10:00:00Z"}]
}
```

The chart and the badges of the details page use the routes by key of the original Gatus
(`/api/v1/endpoints/<key>/response-times/<duration>/history` and `.../badge.svg`), which are already public.

**Never published by these routes:** URL, hostname, IP, port, HTTP status, errors, conditions, certificate or domain
expiration, alerts and `extra-labels`; the page payload has no key and no event.

- A missing, disabled or conflicting page, an invalid slug, a key of an endpoint that is not on the page and any other
  path under `/api/v1/status-pages` all respond the same `404 {"error":"status page not found"}`, with the same headers.
- `503 {"error":"status page temporarily unavailable"}` when the database could not be read; the error only goes to
  the log.
- The HTML route `/status/<anything>` always responds 200 with the application, which queries the API and shows
  "Page not found" when needed.
- Headers: `X-Robots-Tag: noindex, nofollow`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: strict-origin-when-cross-origin`, `Cache-Control: no-cache` (200) and `no-store` (404, 429 and 503).
- The page can be embedded in an iframe (for example, on a monitoring TV). The public API does not send CORS headers.

## Reason of a failure

With `show-messages`, a check that failed **without answering** — no message and no HTTP status — publishes why it
failed, from a closed set of five texts:

| Message | When |
|---------|------|
| `Certificate error` | Error of TLS or of the certificate: it does not match the host, it expired, it was signed by an unknown authority. |
| `DNS error` | The name does not resolve. |
| `Timeout` | The check ran out of time. |
| `Connection failed` | Connection refused, host unreachable, connection closed, and any check that did not connect — which is how TCP, UDP, SCTP and ICMP fail, without registering an error. |
| `Check failed` | Any other failure, including a condition that failed with the service answering. |

**The error itself is never published**, not even in part: the five texts are constants, chosen by Go Uptime from the
errors of the check. That is on purpose, because the errors carry the infrastructure: `dial tcp: lookup
sso.example.org on 127.0.0.11:53: no such host` gives away the DNS resolver of the installation, and `x509:
certificate is valid for *.example.com` gives away the certificate the host serves. The whole text stays on the
dashboard, which is protected by `security`; a page with a [login of its own](#login-of-a-page) serves the same
sanitized payload, because its credential belongs to a customer, not to whoever runs Go Uptime.

The five texts are stable and are not translated: they are part of the public API of the page.

A push keeps publishing the message it sent, a heartbeat keeps publishing its own text, and a result that is Pending
never gets a reason, because it is not a failure.

## Login of a page

A page can ask for a username and a password of its own. Tick **Require login to view this page** in the form, or add
`auth` in the configuration file:

```yaml
status-pages:
  pages:
    - slug: clients
      title: "Clients"
      groups: [core]
      auth:
        username: client
        password-bcrypt-base64: "JDJhJDEwJD..."   # the same encoding as security.basic
```

The hash is generated by [docs/generate-admin-password.py](generate-admin-password.py), the same script as the
password of the administration. In the form you type the password itself and Go Uptime stores only the hash: editing the
page shows the field empty, and leaving it empty keeps the current password.

What changes with `auth`:

- **everything the page serves asks for the credential**: the page at `/status/<slug>`, the details page of each
  endpoint, the API of the page, its real-time channel, its response time chart and its badges. The browser asks with
  its own dialog (HTTP Basic), and `curl -u client:password https://go-uptime.example.org/status/clients` works the same;
- **it is not the login of the administration.** The credential of `security.basic`, its session and OIDC do not open
  the page: only the credential of that page does. And a credential of one page does not open another;
- **there is no way to sign out** other than closing the browser: that is how HTTP Basic works;
- **the badges and numbers by key stay public.** `/api/v1/endpoints/<key>/health/badge.svg` and the other routes by key
  are public in the original Gatus and keep being public, because the dashboard uses them. The page with a login has
  badges of its own under `/api/v1/status-pages/<slug>/endpoints/<key>/...`, which is what its details page shows. To
  hide those numbers from anyone, the installation needs `security`;
- **the 401 says the page exists.** Someone who tries the address learns that the slug is published, which is inherent
  to HTTP Basic. What stays hidden is the page itself: which endpoints it has, their state and their messages;
- **wrong passwords are limited per page and per client**: 10 failures in 5 minutes answer `429` with `Retry-After`,
  and only that page and that client wait. Behind a reverse proxy this needs [`trusted-proxies`](#behind-a-reverse-proxy):
  without it every visitor arrives with the IP of the proxy and shares the same limit.

A page with a login on an installation **without** `security` logs a warning on load: the page is protected while the
dashboard API keeps publishing more than the page shows.

## Cache and cost

- Each page is assembled at most once every 30 seconds, however many visitors it has: concurrent requests share the
  same assembly. A change made through the administration takes effect on the next request.
- The assembly reads the database in one transaction with three queries, whatever the number of endpoints, and at most
  4 pages are assembled at the same time.
- The details of an endpoint are assembled at most once every 30 seconds per page and endpoint, and a key that is not
  on the page responds 404 without reading the database.
- If the database fails, the error is cached for 5 seconds so as not to overload it.
- The page in the browser refreshes every 60 seconds and pauses while the tab is hidden; the details pages also update
  in [real time](#real-time-updates).
- The key of the cached details includes the sequence of the results of the endpoint, so a new result renews them.
- The data of the response time chart has a cache of its own, so that the charts never evict the pages.
- The cache of the pages and of the details holds at most 1000 payloads and 128 MiB. Above that the least recently used
  payloads are discarded and assembled again when asked.

## Rate limit

- Only **404** responses count towards the limit (`rate-limit` per minute per IP; IPv6 aggregated by /64). A published
  page is never blocked: its cost is already limited by the cache.
- When the limit is exceeded: `429 {"error":"too many requests"}` with `Retry-After`.

## Behind a reverse proxy

Without configuration, the IP of the visitor is the IP of the connection. Behind nginx, every visitor would arrive with
the same IP and share the same limit. Set in `trusted-proxies` where the proxy connects to Go Uptime from: `X-Forwarded-For`
is only read from those connections, from right to left, and the first IP that is not a trusted proxy identifies the
visitor.

When a connection from an untrusted private or local IP brings `X-Forwarded-For`, Go Uptime logs a warning (once per load)
with the IP to add to `trusted-proxies`, and the status page list of the administration shows the same warning.

`trusted-proxies` is required behind a proxy for the [real-time updates](#real-time-updates): without it, every visitor
shares the 10 streams of the IP address of the proxy, and from the 11th on the pages only refresh every 60 seconds.

### Docker with nginx on the host

With the port published on `127.0.0.1` only, Go Uptime sees connections coming from the **gateway of the Docker network**,
not from `127.0.0.1`. Pin the subnet of the compose network so that the gateway has a known IP:

```yaml
services:
  go-uptime:
    image: jniltinho/go-uptime:v7.0.0
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - ./config:/config:ro
      - ./data:/data
    networks:
      - go-uptime

networks:
  go-uptime:
    ipam:
      config:
        - subnet: 172.30.0.0/24
```

```yaml
status-pages:
  trusted-proxies: ["172.30.0.1/32"]
```

The nginx vhost must send `X-Forwarded-For`:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

The event streams work through this `location`: Go Uptime sends `X-Accel-Buffering: no`, which turns off the buffering of
nginx for them, and a `: ping` every 15 seconds, below the default `proxy_read_timeout` of 60 seconds. With a longer
chain of proxies or a CDN, turn off the buffering of the event routes (`proxy_buffering off;`) and keep the read timeout
above 15 seconds. Serve the site over HTTP/2 (`listen 443 ssl; http2 on;`): over HTTP/1.1 the browser allows only 6
connections per site, shared by every tab.

Alternatives: `network_mode: host` with `web.address: 127.0.0.1`, or the binary directly on the host; in both cases,
use `trusted-proxies: ["127.0.0.1/32", "::1/128"]`.

## Security

- **The slug is not access control.** Anyone with the address can see the page; do not publish names of services that
  must not be seen by third parties.
- The names of the endpoints and groups become public. Since selecting a group automatically includes new endpoints,
  check the exposure warning in the endpoint form.
- The badges, uptimes and response times by key (`/api/v1/endpoints/<key>/...`) are already public in the original
  Go Uptime and stay that way, including for the endpoints of a page with a [login of its own](#login-of-a-page).
- A page with a login answers `private, no-store` and never lands in a shared cache. The password is stored as a bcrypt
  hash, every read of the administration answers `********` in its place, and the backup carries the hash, like the
  other secrets of the administration.

## Administration API

Routes under `/api/v1/admin`, with the same requirements as the [endpoint administration](admin-endpoints.md#api)
(authentication, permission, CSRF protection, body of up to 256 KB in JSON or YAML):

| Method and route | Purpose |
|------------------|---------|
| `GET /status-pages` | Lists the pages, with `publicationEnabled`, `managedUnavailable` and `sharedRateLimitWarning` |
| `GET /status-pages/options` | Groups and endpoints that can be selected |
| `GET /status-pages/exposure?group=<g>&key=<k>` | Pages on which an endpoint would be shown |
| `POST /status-pages/validate` | Validates a definition and returns warnings (`?slug=` to validate a change) |
| `POST /status-pages` | Creates (201 with `ETag`) |
| `GET /status-pages/<slug>` | Gets (with `ETag`) |
| `PUT /status-pages/<slug>` | Changes (requires `If-Match`) |
| `POST /status-pages/<slug>/enable` and `/disable` | Publishes or unpublishes (requires `If-Match`) |
| `DELETE /status-pages/<slug>` | Removes (requires `If-Match`) |
| `GET /status-pages/<slug>/preview` | Public payload of any page, including disabled ones, without cache |

Errors: 400 (invalid definition, reserved or changed slug), 404, 409 (slug in use or page of the file), 412 (outdated
version), 428 (missing `If-Match`), 501 (storage without support) and 503 (startup or reload in progress).

## Multiple instances with the same PostgreSQL, MySQL or MariaDB

A page created through the web on one instance only shows up on the others after they reload their configuration or
restart. Behind a load balancer, visitors alternate between the page and "Page not found" until every instance
reloads.

## Going back to the original Gatus

The `status-pages` section and the `managed_status_pages` table are ignored by the original Gatus, and the pages stop
existing. Back up the database before switching versions. The `endpoint_response_time_buckets` table of the response
time chart is also ignored and can be dropped. Previous versions of the fork reject the pages managed through
the web with `show-messages`: turn it off before going back.

The same goes for `groups-collapsed`, from `v6.1.0`: a version before it marks a managed page that has the option as
invalid and stops publishing it, so untick "Start with the groups collapsed" on those pages before going back, and do
not restore on an older version a backup made with the option on. In the configuration file it is the opposite: an
older version ignores `groups-collapsed` silently, and the page just opens expanded.

The same care goes for `maximum-endpoints-per-page`, from `v6.3.0`. An older version ignores the option and shows 200
endpoints per page, and it refuses any definition with more than 200 keys in `endpoints`: a managed page like that is
marked invalid and stops being published, and a page **of the configuration file** like that keeps Go Uptime from starting.
Bring those pages down to 200 keys before going back.

## End-to-end tests

```bash
test/e2e/status-pages.sh
test/e2e/status-page-groups.sh    # the collapsible groups, with Push endpoints to take a group down on demand
test/e2e/status-page-limit.sh     # maximum-endpoints-per-page: the cut as an access rule, changed with Go Uptime running
```

Starts Go Uptime with a temporary SQLite database, basic auth and the administration, goes through the public page without
credentials (light and dark modes, 390 px, page not found, simulated OIDC) and the administration screens, with
screenshots in `dist/prints/status-pages/`.
