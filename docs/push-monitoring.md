# Push monitoring

> Not in Gatus, the project that Go Uptime derives from, which only accepts
> results of external endpoints at `/api/v1/endpoints/{key}/external`, with its own parameters.

Scripts, cron jobs, backups and external services report their status to Go Uptime by calling a URL, instead of Go Uptime
checking them. The URL, the parameters and the responses are the same as the Push monitors of the
[Uptime Kuma](https://github.com/louislam/uptime-kuma): a script that pushes to the Uptime Kuma works with Go Uptime by only
changing the address of the server.

## Push URL

```text
https://status.example.com/api/push/<token>?status=up&msg=OK&ping=
https://status.example.com/api/push/<global-key>/<endpoint-key>?status=up&msg=OK&ping=
```

Any HTTP method is accepted (`GET`, `POST`, ...), without the authentication of Go Uptime. Only the query is read:

| Parameter | Default | Meaning |
|-----------|---------|---------|
| `status` | `up` | `up` records a success; `pending` records a **Pending** result (yellow), an extension of Go Uptime; any other value (`down`, `warning`, ...) records a failure, or a Pending result while the endpoint has [retries](#pending-status-and-retries) left |
| `msg` | `OK` | Message of the result, shown in the dashboard (up to 1024 bytes). On a failure it is also the error used by the alerts |
| `ping` | empty | Response time in milliseconds. Empty, non-numeric or `0` is ignored; below 0 or above 100000000000 is rejected |

| Response | When |
|----------|------|
| `200` `{"ok":true}` | The push was accepted |
| `404` `{"ok":false,"msg":"Monitor not found or not active."}` | Unknown token or key, endpoint disabled, removed or that does not receive push |
| `404` `{"ok":false,"msg":"Invalid ping value. Must be between 0 and 100000000000 ms."}` | `ping` out of range |
| `429` `{"ok":false,"msg":"Too many requests"}` | More than 30 rejected pushes in the same minute from the same IP |

Every response has `Cache-Control: no-store`. A push never creates an endpoint.

## Tokens and keys

- **Token of the endpoint:** `/api/push/<token>` authorizes only that endpoint. It has 8 to 128 letters, digits, `-` or
  `_`, is generated with 32 random letters and digits or typed, and cannot be used by two endpoints (409 in the
  administration).
- **Global key:** `/api/push/<global-key>/<endpoint-key>` authorizes every enabled endpoint that receives push. The
  endpoint key is `<group>_<name>`, as in the other URLs of Go Uptime (e.g. `jobs_backup`, or `_backup` without group).
  - Created through the web in the **Push keys** tab of the administration: the key is shown only once and stored as a
    SHA-256 hash, with its last 4 characters as a hint. Revoking it rejects the pushes immediately.
  - Or in the configuration file, read-only in the administration:

    ```yaml
    push:
      keys:
        - name: akamai
          token: "a-long-random-secret-with-at-least-16-characters"
    ```

The global key does not depend on the name or the group of the endpoint: renaming an endpoint only changes the
`<endpoint-key>` part of the URL. The token of the endpoint does not change when it is renamed.

## Push endpoints

A Push endpoint is passive: Go Uptime does not check anything, it records the pushes and a failure for every heartbeat
interval without push.

### Through the web

At `/admin/endpoints/new`, choose **Push (passive)** as the monitor type. The form asks for the heartbeat interval
(1 minute by default, at least 10 seconds) and opens the **Push** section, which generates a token (or paste the token
of an Uptime Kuma monitor) and shows the push URL and a `curl` example, each with a **Copy** button. When editing, the
**Push** section starts collapsed and its header shows the last characters of the token. There is no **Test** button:
send a push to the URL instead. In YAML mode:

```yaml
type: push
name: backup
group: jobs
token: keSDu7G855jvVat1xWiY2Gk4CkL1End5   # optional: generated when missing
heartbeat:
  interval: 25h
alerts:
  - type: slack
    failure-threshold: 1
```

Push endpoints accept `name`, `group`, `token`, `heartbeat`, `alerts`, `maintenance-windows` and `enabled`. They have
the same lifecycle as the other endpoints managed through the web (versions, renaming with the history, removal) and
can be selected by the status pages. See [docs/admin-endpoints.md](admin-endpoints.md).

### In the configuration file

The `external-endpoints` of the configuration file are Push endpoints, with their `token`:

```yaml
external-endpoints:
  - name: backup
    group: jobs
    token: "keSDu7G855jvVat1xWiY2Gk4CkL1End5"
    heartbeat:
      interval: 25h   # optional in the configuration file
      retries: 2      # optional, requires interval, see "Pending status and retries"
    alerts:
      - type: slack
```

They are listed in the administration for reference only. The route of the original Gatus
(`/api/v1/endpoints/{key}/external` with `Authorization: Bearer <token>`) keeps working for them.

## Push on active endpoints

An active endpoint (HTTP, TCP, ICMP, DNS, SSH...) can also receive push, e.g. alerts of metrics sent by the Akamai
calling the URL in the format of the Uptime Kuma. It is off by default:

- **Through the web:** check **Accept push** in the form. The token is optional: without it, only the global keys are
  accepted. In YAML mode:

  ```yaml
  name: site
  group: erp
  url: https://erp.example.com/health
  conditions: ["[STATUS] == 200"]
  push:
    enabled: true
    token: erp-site-token   # optional
  ```

- **In the configuration file:** list the key of the endpoint in `push.endpoints`, without changing the endpoint:

  ```yaml
  push:
    endpoints:
      - key: erp_site
        token: erp-site-token   # optional
  ```

Results sent to the external endpoint API of the original Gatus (`POST /api/v1/endpoints/<key>/external`) are marked
as Push too, and the text of their `error=` is never published on a status page.

A push is recorded in the same history as the checks of Go Uptime, marked as Push, and counts for the uptime, the events,
the metrics and the alerts. The status of the endpoint is the status of the last result, whether it came from a check or
from a push: a `down` push is followed by the next successful check. Heartbeats do not apply to active endpoints.

Example of the Akamai (or any alerting system) calling a global key:

```text
https://status.example.com/api/push/<global-key>/erp_site?status=down&msg=Latency%20above%202s
```

To register many hosts at once, each with a token of its own, and to export the tokens later, see
[Managing endpoints from the command line](admin-endpoints.md#managing-endpoints-from-the-command-line).

## Heartbeat

- For every full heartbeat interval without an accepted push, a failure is recorded with the error
  `heartbeat: no update received within <interval>`, except during maintenance windows. Consecutive intervals without
  push record one failure each.
- An accepted push restarts the count. After a start or a configuration reload, the count starts at that moment.
- Disabling, removing or renaming an endpoint stops its heartbeat before the response.
- The text of the heartbeat is also the message of the result, so that the public status pages with `show-messages` can
  show it.

## Pending status and retries

Besides up (green) and down (red), a result can be **Pending** (yellow). It is a feature of Go Uptime: the Uptime Kuma
records `status=pending` as down and only has green and red.

- `status=pending` records a Pending result with the `msg` of the push, on Push endpoints and on active endpoints that
  accept push. It also restarts the count of the heartbeat, so that a job that keeps reporting "in progress" does not
  fail.
- **Retries** (`heartbeat.retries`, 0 to 100, default 0; "Retries" in the form of Push endpoints) record the next
  failures as Pending before a failure: a push with a status other than `up` and `pending`, or an interval without push,
  is Pending while fewer than `retries` failures were converted since the last success, and only then a failure, with
  alerts and events. A push `up` resets the count; a push `pending` and the maintenance windows do not change it. They
  also apply to the route of the original Gatus (`/api/v1/endpoints/{key}/external`) of the external endpoints of the
  configuration file. A converted result keeps no errors: they become its message.
- A Pending result neither triggers nor resolves alerts, does not change their counters and does not create events
  (became healthy, became unhealthy). It counts as an execution without success in the uptime and in the Prometheus
  metrics, and as a failure in the counters and filters of the dashboard.
- The count of the retries is kept in memory, per endpoint, across configuration reloads. A restart of Go Uptime starts it
  again, so up to `retries` more Pending results can be recorded before the next failure. Removing or renaming the
  endpoint, or removing it from the configuration file, forgets it.

## Dashboard

Below the name of the endpoint, a small line shows when its TLS certificate expires ("Certificate expires in 73 days ·
Dec 1, 2026"), from the most recent check with a certificate: in the secondary color, amber from 14 days and red from
7 days or once expired. Pushes do not hide it, and endpoints without TLS do not show it.

The details page of an endpoint (`/endpoints/<key>`) follows the order of the monitor page of the Uptime Kuma:
- the bars of **Recent Checks**;
- a panel of numbers: **Response (Current)** and **Avg. Response (24h)** (**Ping** on Push endpoints) and the
  **Uptime** over 24 hours, 7 days and 30 days, with "—" without data. The average includes the pushes without `ping`
  as 0 ms;
- the **Response Time Trend** chart, shown as soon as there is a result, in the format of the chart of the Uptime
  Kuma: Recent (one point per push, Down in red and Pending in yellow columns), 3h, 6h, 24h and 1w, see
  [response time chart](status-pages.md#response-time-chart);
- the table of checks, with the pagination;
- the **Events**, collapsed by default like the table of checks.

The details page updates in real time: a push shows up on the bars, the panel, the chart and the table
within about 2 seconds, without reloading the page, even with the table on another page of results. See
[real-time updates](status-pages.md#real-time-updates) for the routes, the limits and the reverse proxy.

**Checks table** expands the table of the results of the page, from the most recent to the oldest, with the status (Up,
Down or Pending), the date and time, the message (the `msg` of the push, otherwise the errors, otherwise the HTTP status
of the check) and the origin (Push or Check). The table starts collapsed and the browser remembers whether it was
expanded. The public status pages only show messages on the details pages of the pages with `show-messages`, and never
the errors of the checks.

## Migrating scripts from the Uptime Kuma

1. Copy the token of the Push monitor from its URL in the Uptime Kuma (`/api/push/<token>?...`).
2. Create a Push endpoint in Go Uptime pasting that token, with a heartbeat interval equal to the one of the monitor.
3. Replace the address of the Uptime Kuma server with the address of Go Uptime in the scripts. Nothing else changes.

Differences:

- There is no PENDING state nor retries: a failure is recorded immediately. Use `failure-threshold` in the alerts to
  wait for several failures before alerting.
- There is no "upside down" mode.
- Monitors are not imported from the database of the Uptime Kuma.

## Examples

Cron job every 5 minutes (heartbeat interval of 10 minutes):

```bash
*/5 * * * * curl -fsS -m 10 --retry 3 "https://status.example.com/api/push/<token>?status=up&msg=OK&ping=" >/dev/null
```

Backup that reports success or failure with a message and its duration:

```bash
#!/usr/bin/env bash
PUSH_URL="https://status.example.com/api/push/<token>"
start=$(date +%s%3N)
if output=$(/usr/local/bin/backup.sh 2>&1); then
  status=up; msg="Backup OK"
else
  status=down; msg="Backup failed: $(tail -n 1 <<<"$output")"
fi
curl -fsS -m 10 -G "$PUSH_URL" --data-urlencode "status=$status" --data-urlencode "msg=$msg" \
  --data-urlencode "ping=$(( $(date +%s%3N) - start ))" >/dev/null
```

`--data-urlencode` encodes spaces and special characters of the message.

## Security

- The routes `/api/push` and `/api/push/*` never ask for the login of Go Uptime (no 401 nor `WWW-Authenticate`).
- Global keys are compared by hash. Tokens and keys are not written to the logs of Go Uptime, which only show the key of the
  endpoint.
- Valid pushes are always accepted. Rejected pushes are limited to 30 per minute per IP (IPv6 by /64); after that, the
  rejected pushes of that IP respond 429. The IP of the client comes from `X-Forwarded-For` only when the proxy is in
  `status-pages.trusted-proxies` (see [docs/status-pages.md](status-pages.md#behind-a-reverse-proxy)); otherwise every
  push behind the proxy shares the same limit.
- As in the Uptime Kuma, the token is part of the URL: use HTTPS and keep the URLs out of the access logs of the proxy.
  With nginx:

  ```nginx
  location /api/push/ {
      access_log off;
      proxy_pass http://127.0.0.1:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
  }
  ```

- Rotate a global key by creating a new one, updating the scripts and revoking the old one.

## Administration API

Every route requires administrator authentication (see [docs/admin-endpoints.md](admin-endpoints.md#api)).

| Method and route | Description |
|------------------|-------------|
| `GET /api/v1/admin/push-keys` | Lists the global keys of the file and of the web, with the hint and without the key |
| `POST /api/v1/admin/push-keys` | Creates a key: `{"name":"akamai"}`; responds 201 with the key, only this time |
| `DELETE /api/v1/admin/push-keys/{id}` | Revokes a key created through the web |

Status codes: 400 invalid name (1 to 64 characters); 404 key not found; 409 name in use; 501 storage without support
(`memory`). The detail of an endpoint (`GET /api/v1/admin/endpoints/{key}`) returns its token in `pushToken`; the
definition shows it masked.

## Multiple instances with the same PostgreSQL, MySQL or MariaDB

- A push is recorded by the instance that receives it. Every instance runs the heartbeat of its Push endpoints, so an
  instance that receives no push records heartbeat failures: send the pushes to a single instance.
- Endpoints, tokens and global keys changed on one instance only take effect on the others after they restart or reload
  their configuration.
- Only the instance that records a result notifies it in real time: a browser connected to another instance sees it on
  the periodic refresh.

## Going back to the original Gatus

The original image ignores the `push_keys` and `endpoint_result_messages` tables, does not answer `/api/push` and marks
the Push endpoints managed through the web as invalid, keeping their history. The `push` section of the configuration
file must be removed, because the original image rejects unknown keys.

Previous versions of the fork show the Pending results as failures. Before going back to them, remove
`heartbeat.retries` from the Push endpoints and `show-messages` from the status pages managed through the web: their
definitions are decoded strictly and would be rejected. In the configuration file both are ignored.

## End-to-end tests

`test/e2e/push.sh` starts a local Go Uptime with a temporary SQLite database, creates keys and endpoints through the screens
with [agent-browser](https://github.com/vercel-labs/agent-browser), sends pushes with `curl` and saves screenshots in
`dist/prints/push/` (outside of git).
