<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
# Endpoint administration through the web

> Not in Gatus, the project that Go Uptime derives from, which only accepts
> endpoints in the configuration file ([TwiN/gatus#1345](https://github.com/TwiN/gatus/issues/1345)).

With the administration enabled, endpoints can be created, edited, disabled and removed at `/admin`, without access to
the server and without restarting Go Uptime. The endpoints of the configuration file keep working as always and are shown
in the administration for reference only.

## Requirements

- `security.basic` or `security.oidc` configured. With OIDC, `admin.allowed-subjects` is required.
- `storage.type` set to `sqlite`, `postgres` or `mysql` ([MySQL and MariaDB](storage-mysql.md)): the endpoints managed
  through the web are stored in the `managed_endpoints` table.

## Configuration

```yaml
storage:
  type: sqlite
  path: /data/data.db

security:
  basic:
    username: admin
    # bcrypt hash of the password, base64 encoded (see below)
    password-bcrypt-base64: "JDJhJDEwJHRiMnRFakxWazZLdXBzRERQazB1TE8vckRLY05Yb1hSdnoxWU0yQ1FaYXZRSW1McmladDYu"

admin:
  enabled: true
  # Required with security.oidc: subjects (sub claim) allowed to administer, case-insensitive
  # allowed-subjects: ["ops@example.com"]
  # Origins accepted for changes; needed behind a proxy that exposes another port
  # allowed-origins: ["https://status.example.com:8443"]

# Without any endpoint in the file, Go Uptime starts normally when admin.enabled is true
endpoints: []
```

These three blocks — a SQL storage, a login and `admin.enabled` — are the minimum. The default
[config.yaml](../config.yaml) carries them commented out, and
[.examples/docker-compose-admin](../.examples/docker-compose-admin) is a Docker Compose setup with nothing else.

To generate `password-bcrypt-base64`, run `go-uptime password hash` (see [cli.md](cli.md#go-uptime-password-hash)). The value
must be the base64 of a bcrypt hash: anything else — a plain password, a placeholder left in the file — makes the
configuration invalid, which `go-uptime config validate` reports before the server is restarted. Without the binary at hand,
[generate-admin-password.py](generate-admin-password.py) does the same and only needs Python 3:

```bash
python3 docs/generate-admin-password.py --username admin   # asks for the password and prints the config.yaml block
python3 docs/generate-admin-password.py                    # prints only the value
printf '%s' 'your-password' | python3 docs/generate-admin-password.py --stdin
```

It uses the Python `bcrypt` module when it is installed and computes bcrypt in pure Python otherwise (a few seconds
with the default cost of 10). The result is the same as with `htpasswd`:

```bash
htpasswd -bnBC 10 "" 'your-password' | tr -d ':\n' | sed 's/$2y/$2a/' | base64 -w0 | tr '+/' '-_'
```

Go Uptime decodes this value with the URL base64 alphabet; `tr '+/' '-_'` prevents failures with hashes that produce `+`
or `/` in standard base64.

With `security.basic`, the only basic user is the administrator. With `security.oidc`, only the subjects of
`admin.allowed-subjects` can administer; the other authenticated users keep seeing the dashboard.

## Login screen

With `security.basic` and without `security.oidc`, the dashboard, the details pages and the administration open a login
screen at `/login` instead of the native credentials dialog of the browser. After signing in, the browser goes back to
the page it was opening, and the **Logout** button of the header closes the session. The public status pages keep
opening without login.

```yaml
security:
  basic:
    username: admin
    password-bcrypt-base64: "JDJhJDEwJHRiMnRFakxWazZLdXBzRERQazB1TE8vckRLY05Yb1hSdnoxWU0yQ1FaYXZRSW1McmladDYu"
    # Validity of the sessions of the login screen: 8h by default, between 5m and 720h (30 days)
    session-ttl: 12h
```

- **Sessions:** a successful login creates a session in the `login_sessions` table, where only the SHA-256 hash of the
  token is stored. With `storage.type: memory`, sessions are kept in memory and every restart asks for a new login.
  The `go_uptime_session` cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` when the connection is TLS or
  `X-Forwarded-Proto` is `https`.
- Sessions survive restarts and reloads of the configuration. The logout, and a change of `username` or
  `password-bcrypt-base64`, end them immediately on every instance that uses the same database.
- **Scripts:** `Authorization: Basic` keeps working on every protected route, so `curl -u admin:your-password` needs no
  change. Without credentials, the API answers 401 with `WWW-Authenticate: Basic`, except for the requests of browsers
  and of the frontend (`Sec-Fetch-Site`, `Sec-Fetch-Mode` or `X-Requested-With`), which would open the native dialog.
- **Failed logins:** 10 failures in the same minute from the same IP address (IPv6 addresses by /64) block the login
  and `Authorization: Basic` from that address until the minute ends, with 429 and `Retry-After`, even with the right
  password. Wrong credentials on the login screen, on `Authorization: Basic` and on `GET /api/v1/config` count. The
  count belongs to each process: instances do not share it, and a reload of the configuration resets it.
- **Behind a reverse proxy:** list the proxy in `status-pages.trusted-proxies` (see [Behind a reverse
  proxy](#behind-a-reverse-proxy)), so that the limit counts the address of each client from `X-Forwarded-For`.
  Otherwise every request seems to come from the proxy, and 10 failures of anyone block the login of everyone.

| Method and route | Description |
|------------------|-------------|
| `POST /api/v1/auth/login` | JSON `{"username": "...", "password": "..."}` up to 4 KB: 204 with the session cookie, 401 wrong credentials, 429 blocked |
| `POST /api/v1/auth/logout` | Closes the session of the cookie, if any, and expires the cookie (204) |

Both routes answer with `Cache-Control: no-store`, follow the origin rules of the changes of the administration and
answer 404 without `security.basic` or with `security.oidc`. `GET /api/v1/config` informs `login` (`basic`, `oidc` or
empty) and `authenticated`.

## Usage

- **List (`/admin`):** search by name, group or URL; shows the source (Web or YAML), endpoints in conflict with the YAML
  and invalid endpoints; lets you enable, disable and remove the endpoints managed through the web. On larger screens
  the lists of the administration (Endpoints, Status pages and Push keys) fill the window with a compact header, and
  only the table scrolls, with its header fixed.
- **Form (`/admin/endpoints/new` and `/admin/endpoints/<key>/edit`):** form mode (name, group, URL, method, interval,
  conditions, headers, alerts, enabled, follow redirects and skip TLS certificate verification) and YAML mode, with the
  same keys as an item of `endpoints` in the configuration file. The group is picked from the groups of the existing
  endpoints or typed as a new group. Changing the name or the group renames the endpoint (see [Renaming](#renaming)).
- **Follow redirects** (`client.ignore-redirect`, on by default) and **Skip TLS certificate verification**
  (`client.insecure`) help with sites that redirect HTTP to HTTPS or send an incomplete certificate chain. The other
  `client` options are only available in YAML mode.
- **Monitor type:** the active types (HTTP, TCP, ICMP, DNS, SSH) or **Push (passive)**, which receives pushes in the
  format of the Uptime Kuma. **Accept push** makes an active endpoint also receive pushes. The **Push keys** tab manages
  the global keys. See [docs/push-monitoring.md](push-monitoring.md).
- The form is split into sections (General, Check, Push, Conditions, Headers and Alerts). The **Push** section is
  collapsible: it opens when Push is chosen or Accept push is checked, starts collapsed when editing (its header shows
  the last characters of the token) and has **Copy** buttons for the push URL and the `curl` example.
- **Validate** checks the definition without saving it. **Test** runs a single check, without storing the result or
  triggering alerts, and shows each condition (not available for Push endpoints). **Save** stores and applies it.
- **Remove** deletes the definition and the whole history of the endpoint. Triggered alerts are not resolved with the
  alerting providers.

## Behavior

- Creating, changing, enabling, disabling or removing an endpoint takes effect immediately, without restarting Go Uptime
  and without restarting the other endpoints. A check in progress finishes before the change and its result is
  discarded.
- The history of the endpoints managed through the web is kept across restarts and reloads of the configuration file.
- If the configuration file starts defining the same key (group and name) as an endpoint managed through the web, the
  one from the file wins and the one from the web is marked as in conflict, without being deleted. Removing the web one,
  in that case, does not affect the one from the file.
- Turning `admin.enabled` off only hides the administration: the endpoints managed through the web keep being monitored.
- During startup or a configuration reload, changes respond 503 and nothing is stored.
- With `skip-invalid-config-update: true`, an invalid configuration file no longer brings Go Uptime down: the previous
  configuration stays in use until the file is fixed.

## Renaming

The key of an endpoint is `<group>_<name>`. Changing the name or the group of an endpoint managed through the web
renames its key when saving, and the form shows the old and the new key before that:

- The history (results, events, uptime and triggered alerts) moves to the new key in the same transaction as the
  definition, with SQLite, PostgreSQL, MySQL and MariaDB. The state of the triggered alerts whose configuration did not
  change is kept, so an ongoing incident is not triggered again.
- The new key must be free: an endpoint, external endpoint or suite of the configuration file, another endpoint managed
  through the web, or history still stored under that key (e.g. of an endpoint removed from the configuration file
  before the next reload) makes the change fail with 409, without changing anything.
- The URLs of the badges and of the details page, and the Prometheus series, use the new key. The old URLs stop
  working.
- Status pages managed through the web that select the endpoint by key (`endpoints` or `featured`) are updated in the
  same transaction, with a new version. Status pages of the configuration file cannot be changed: the form lists them
  before saving, and they stop showing the endpoint until the file uses the new key. Pages that select by group follow
  the new group.
- An endpoint in conflict with the configuration file can be renamed: only its definition moves, and the new key starts
  without history, because the history of the old key belongs to the endpoint of the configuration file.

## Restrictions of the endpoints managed through the web

- Environment variables (`$VAR`, `${VAR}`) are not expanded.
- `client.identity-aware-proxy`, `client.tls.certificate-file`, `client.tls.private-key-file`, `store` and `always-run`,
  which would use credentials or files of the server, are not accepted.
- `client.tunnel` must exist in `tunneling`.
- Alerts need a provider configured in `alerting`; invalid overrides are rejected.
- `extra-labels` can only use labels that already exist in the endpoints of the configuration file (Prometheus metrics).
- The key cannot be the same as the key of an endpoint, external endpoint, suite or suite endpoint of the file.

## Secrets

Every response masks with `********`: headers whose name contains `authorization`, `cookie`, `token`, `secret`,
`password` or `key`; the password and sensitive parameters of the URL; `client.oauth2.client-secret`; `ssh.password`;
`ssh.private-key`; and the values of `alerts[].provider-override`. When saving, a value equal to the mask keeps the
stored value. Secrets are stored as plain text in the database, as they would be in the configuration file: protect
your backups.

## API

Every route requires administrator authentication. Changes require `Content-Type` `application/json` or
`application/yaml` when there is a body, and the `Origin` of the browser must match the address of Go Uptime.

| Method and route | Description |
|------------------|-------------|
| `GET /api/v1/admin/endpoints` | Lists the endpoints of the file and the ones managed through the web |
| `GET /api/v1/admin/endpoints/{key}` | Stored and effective definition (with `ETag`) |
| `POST /api/v1/admin/endpoints` | Creates (201) |
| `PUT /api/v1/admin/endpoints/{key}` | Changes, or renames when the name or the group changes (requires `If-Match`) |
| `POST /api/v1/admin/endpoints/{key}/enable` and `/disable` | Enables or disables (requires `If-Match`) |
| `DELETE /api/v1/admin/endpoints/{key}` | Removes (requires `If-Match`) |
| `POST /api/v1/admin/endpoints/validate[?key=]` | Validates without saving |
| `POST /api/v1/admin/endpoints/test[?key=]` | Tests once (at most 2 concurrent tests) |
| `POST /api/v1/admin/endpoints/parse` | Converts YAML into a JSON document, without validating |
| `GET /api/v1/admin/metadata` | Available alert types, tunnels and labels |

Status codes: 400 invalid definition; 404 not found; 409 key in use, endpoint of the configuration file, or new key
with stored history when renaming; 412 outdated version; 413 body above 256 KB; 415 invalid content type; 428 missing `If-Match`;
429 too many tests; 503 startup or reload in progress.

```bash
curl -u admin:your-password -H 'Content-Type: application/yaml' \
  --data-binary $'name: site\ngroup: web\nurl: https://example.com\nconditions: ["[STATUS] == 200"]\n' \
  http://127.0.0.1:8080/api/v1/admin/endpoints
```

## Managing endpoints from the command line

[docs/manager-go-uptime.py](manager-go-uptime.py) does, through this API, what would take many clicks in the administration:
registers a list of hosts, renames a group in every endpoint that uses it and exports the push tokens. It only uses the
standard library.

```bash
export GO_UPTIME_URL=https://status.example.com
export GO_UPTIME_PASSWORD='your-password'

python3 docs/manager-go-uptime.py csv --csv inventory.csv --out endpoints-prod.csv   # only rewrites the CSV
python3 docs/manager-go-uptime.py import --csv endpoints-prod.csv --dry-run          # shows what it would register
python3 docs/manager-go-uptime.py import --csv endpoints-prod.csv                    # registers
python3 docs/manager-go-uptime.py endpoints                                          # lists the endpoints
python3 docs/manager-go-uptime.py status-pages                                       # lists the status pages
python3 docs/manager-go-uptime.py groups                                             # lists the groups
python3 docs/manager-go-uptime.py rename-group --from analytics --to data            # renames the group of every endpoint
python3 docs/manager-go-uptime.py export-tokens --out hosts-tokens.csv               # host,token
```

`--url`, `--username` and `--password` also come from `GO_UPTIME_URL`, `GO_UPTIME_USERNAME` and `GO_UPTIME_PASSWORD`, which
keeps the password out of the shell history. Every subcommand that changes something accepts `--verbose`, and `import`
and `rename-group` accept `--dry-run`. `--timeout` and `--insecure` change the timeout of each request and skip the
verification of the TLS certificate.

Go Uptime applies each change in a cycle of its own and answers `503` to the changes that arrive meanwhile, which a
sequence of registrations runs into. The script repeats a request answered with `503` up to 8 times, waiting what
`Retry-After` says or, without it, from 0.25 to 5 seconds. A `503` means that nothing was changed, so repeating is safe.

### csv and import

The CSV can be an inventory of hostnames, with the columns `Serviço`, `Ambiente`, `Hostname` and `Status` — the service
becomes the group, and by default only the rows of production whose status is `OK` are kept, one per hostname — or the
simple format the script itself writes, with the columns `grupo`, `nome` and `url`. Each host becomes an HTTP endpoint
that also accepts [push](push-monitoring.md#push-on-active-endpoints), with a token of its own.

| Flag | What it does |
|------|--------------|
| `--csv` | The CSV to read. |
| `--out` | The CSV to write (`csv` and `export-tokens`). |
| `--csv` (in `endpoints` and `status-pages`) | Writes the listing as a CSV instead of printing the table. |
| `--group`, `--source` (in `endpoints`) | Only these groups, and only the endpoints of the configuration file or the ones managed through the web. |
| `--environment`, `--every-environment`, `--every-status` | What is kept from the inventory: by default the environment that starts with `Produção` and the rows with status `OK`. |
| `--group` | Only these groups, by the slug of the service; can be repeated. |
| `--scheme`, `--path` | How the URL is built from the hostname: `https://<hostname>` by default. |
| `--interval`, `--condition` | Interval and conditions of the check: `5m` and `[STATUS] == 200` by default; `--condition` can be repeated. |
| `--no-push` | Registers without push, so no token is generated. |
| `--limit` | Keeps at most N endpoints. |

Running `import` again only creates what is missing: an endpoint whose key already exists is left untouched, so an
interrupted import can simply be repeated. The tokens are generated by the script, which means the export has to be
kept safe, exactly like a backup of the administration.

### endpoints and status-pages

`endpoints` lists what is registered — the endpoints of the configuration file and the ones managed through the web —
with the key, the type, the source, whether it is enabled, whether it accepts push and its URL:

```
KEY                           TYPE  SOURCE  STATE    PUSH  URL
analytics_api-example-com     HTTP  web     enabled  yes   https://api.example.com
core_health                   HTTP  file    enabled        https://example.org/health
jobs_backup                   PUSH  file    enabled  yes
4 endpoints · 2 of the configuration file · 0 disabled · 3 accepting push
```

`--group` (repeatable) and `--source config|admin` narrow the listing. `status-pages` does the same for the pages, with
the slug, the title, the source, the state (`published`, `enabled`, `disabled`, `conflict` or `invalid`), the number of
endpoints, whether the page requires a login and its address; it also warns when `status-pages.enabled` is false, when
the managed pages could not be loaded and when every visitor seems to come from the same proxy.

Both accept `--csv PATH`, which writes the same columns as a CSV instead of printing the table.

### rename-group

`rename-group --from <current> --to <new>` changes the group of every endpoint already registered in that group. The
group is a label, written exactly as it is typed (`--to "Dados / BI"` keeps the capitals, the spaces and the accents);
only the key is normalized, to `<group>_<name>`. `--from` finds the group by the label or by its key, so `--from
tradimus` also finds the group written `Tradimus`.

When the key changes, this is the [rename](#renaming) of the administration, one endpoint at a time: the history, the
uptime and the events are kept, the push token does not change, and the managed status pages follow the new key by
themselves. When only the capitals or the accents of the label change, the key stays the same and it is an ordinary
update — the script says `keys unchanged`.

- endpoints of the **configuration file** are not touched, because the administration cannot change them: the script
  lists them so that they can be changed in the YAML;
- a status page **of the configuration file** that selects the old key keeps selecting it, and the script prints which
  one;
- `--dry-run` shows every `old key -> new key` without changing anything, which is worth running first.

## Backup and restore

The **Backup** tab (`/admin/backup`) downloads and restores what was registered through the web: the managed endpoints
(with their groups, which are part of their definitions), the managed status pages and the push keys created through
the web. It never includes the history (results, events, uptimes), the items of the configuration file, the login
sessions nor the global configuration.

### Download

- The file is JSON (`go-uptime-backup-<date>.json`) with the complete definitions, **including tokens, passwords, headers and
  webhooks of alerts in plain text**. Keep it safe.
- **Encrypt with a password** (12 to 1024 bytes) writes `go-uptime-backup-<date>.enc.json` instead, encrypted with AES-256-GCM
  and a key derived from the password with Argon2id. Without the password the file cannot be read, and a wrong
  password and a changed file give the same error.
- Push keys are backed up with the hash and the hint of their token only: after a restore, the scripts keep pushing with
  the original key, whose token is never shown.
- A backup holds at most 2 MiB, 1,000 endpoints, 200 status pages and 500 push keys; above that, the download answers 422.

### Restore

1. Choose the file (and type the password of an encrypted file) and click **Preview**. Nothing is changed: the preview
   shows, for each item, whether it will be created, updated, left unchanged or skipped, and why. It simulates the items
   in order (push keys, endpoints, status pages), so it predicts what the restore will do, and tells how many enabled
   endpoints will start being monitored and how many have alerts.
2. Items are skipped when:
   - their key or slug is used by the configuration file;
   - their definition is refused by the same validations as the forms (for example an alert provider that is not
     configured, or a push token already used, including by another endpoint of the backup);
   - they exist with a different definition and **Overwrite existing items** is not checked;
   - they have a secret masked by the API (`********`, for example copied from the YAML editor), a push endpoint has no
     token, or an endpoint would change its type;
   - a push key has a name in use, or the hash of another key or of the token of any endpoint.
3. **Restore endpoints as disabled** restores the endpoints with `enabled: false`, useful to restore into a test
   installation without checking the production services nor sending alerts.
4. Click **Restore** and confirm. Each item is applied like a change made through the forms, and the result of each one is
   shown. Nothing is deleted, keys and slugs never change, and restoring the same file again leaves the applied items
   unchanged. If anything changed since the preview, the restore answers 409 and nothing is applied: preview again.
5. If a configuration reload starts during the restore, the remaining items are skipped: restore again afterwards.

After 10 wrong passwords in 15 minutes, a client must wait before trying again; this limit does not affect the login.

### API

```bash
# Backup, optionally encrypted
curl -u admin:password -H 'Content-Type: application/json' -d '{"password":"a long password"}' \
  -o go-uptime-backup.enc.json https://status.example.com/api/v1/admin/backup

# Preview, then restore with the fingerprint of the preview
jq -n --slurpfile file go-uptime-backup.enc.json '{file: $file[0], password: "a long password", overwrite: false}' > restore.json
curl -u admin:password -H 'Content-Type: application/json' -d @restore.json https://status.example.com/api/v1/admin/restore/preview
jq --arg fingerprint "<fingerprint>" '. + {fingerprint: $fingerprint}' restore.json > apply.json
curl -u admin:password -H 'Content-Type: application/json' -d @apply.json https://status.example.com/api/v1/admin/restore
```

The three routes only accept `application/json`. Errors: 400 (invalid file or password), 409 (changed since the
preview), 413 (body above 3.5 MiB), 415, 422 (backup above the limits), 429 (too many wrong passwords or encryptions
in progress, with `Retry-After`) and 503 (reload in progress or registered items unavailable).

## Behind a reverse proxy

The origin expected for changes is derived from the `Host` header and the scheme (`X-Forwarded-Proto`). With nginx,
forward these headers:

```nginx
proxy_set_header Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

If the public address uses a port different from the one forwarded in `Host`, list it in `admin.allowed-origins`.

With the [login screen](#login-screen), list the proxy in `status-pages.trusted-proxies`, so that the limit of failed
logins counts each client instead of the proxy. Go Uptime logs a warning when a private, loopback, link-local or CGNAT
address sends `X-Forwarded-For` without being in that list:

```yaml
status-pages:
  trusted-proxies: ["127.0.0.1"]
```

A restore sends the backup file in the body of the request: raise the body limit of nginx, whose default is 1 MiB, and
keep the read timeout above the time of a large restore:

```nginx
client_max_body_size 4m;
proxy_read_timeout 120s;
```

## Multiple instances with the same PostgreSQL, MySQL or MariaDB

A change made on one instance only takes effect on the others after they restart or reload their configuration. The
same applies to the items of a restore. In the
meantime, an instance that still monitors a removed or renamed endpoint may recreate the history of the old key, which
is deleted when that instance restarts or reloads.

## Versions and images

- Fork releases use `v<upstream-version>-fork.<N>` tags (e.g. `v5.36.0-fork.1`). Because of SemVer precedence, these
  tags sort below the upstream version with the same base in tools such as Renovate and `sort -V`.
- Docker Hub image: `jniltinho/go-uptime:<tag>` (`linux/amd64` and `linux/arm64`), published with
  `make docker-release VERSION=<version without the v>`. The `latest` tag is not published.

## Going back to the original Gatus

The original image ignores the `managed_endpoints` table: the endpoints managed through the web stop being monitored
and their history is deleted on the first start. Before going back, back up the database and make sure the
configuration file has at least one endpoint, otherwise the original image does not start.

The original image and the previous fork versions ignore the `login_sessions` table and `security.basic.session-ttl`,
which can stay in the configuration file, and ask for the credentials with the native dialog of the browser again.

## End-to-end tests

`test/e2e/admin.sh` starts a local Go Uptime with a temporary SQLite database and goes through the screens with
[agent-browser](https://github.com/vercel-labs/agent-browser), saving screenshots in `dist/prints/` (outside of git).
`test/e2e/login.sh` covers the login screen: redirections, refused redirects, wrong password, limit of failed logins,
logout, public status pages without login and `curl -u`.
