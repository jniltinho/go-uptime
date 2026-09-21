<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<a href="https://github.com/jniltinho/go-uptime">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/logo-with-light-text.png">
    <img alt="Go Uptime" src=".github/assets/logo-with-dark-text.png" width="420">
  </picture>
</a>

# Go Uptime

[![Release](https://img.shields.io/github/v/release/jniltinho/go-uptime?sort=semver)](https://github.com/jniltinho/go-uptime/releases)
[![CI](https://github.com/jniltinho/go-uptime/actions/workflows/ci.yml/badge.svg)](https://github.com/jniltinho/go-uptime/actions/workflows/ci.yml)
[![Docker pulls](https://img.shields.io/docker/pulls/jniltinho/go-uptime)](https://hub.docker.com/r/jniltinho/go-uptime)
[![License](https://img.shields.io/github/license/jniltinho/go-uptime)](LICENSE)

Uptime monitoring in a single Go binary: it checks HTTP, ICMP, TCP, DNS and other services, evaluates conditions on the
status, response time, body and certificates, sends alerts, and serves a dashboard, an administration and public status
pages.

![Dashboard](docs/screenshots/dashboard.png)

> **Coming from `jniltinho/gatus`?** This is the same project under its new name, from `v7.0.0` on. Change the image to
> `jniltinho/go-uptime` and everything else keeps working: see [docs/migrating-from-gatus.md](docs/migrating-from-gatus.md).

## What it does

| | |
|---|---|
| **Monitoring from a YAML file** | Endpoints, conditions, suites, maintenance windows and more than 40 alerting providers, in a configuration file that is reloaded when it changes. [docs/README.md](docs/README.md) |
| **Endpoint administration through the web** | Create, edit, disable and remove endpoints at `/admin`, without restarting. [docs/admin-endpoints.md](docs/admin-endpoints.md) |
| **Public status pages** | Pages open without login at `/status/<slug>`, with featured endpoints, collapsible groups and a details page for every endpoint, while the dashboard stays protected. A page can also ask for a username and a password of its own. [docs/status-pages.md](docs/status-pages.md) |
| **SQLite, PostgreSQL, MySQL and MariaDB** | `storage.type: sqlite`, `postgres` or `mysql` (MySQL 8.4+ and MariaDB 10.11+). [docs/storage-mysql.md](docs/storage-mysql.md) |
| **Push monitoring compatible with the Uptime Kuma** | Scripts and services report their status at `/api/push/<token>?status=up&msg=OK&ping=`, with Push endpoints, global keys, push on active endpoints and a Pending status with retries. [docs/push-monitoring.md](docs/push-monitoring.md) |
| **Response time chart** | Periods Recent, 3h, 6h, 24h and 1w, with the average, the minimum and the maximum, and columns for the failures. [docs/status-pages.md](docs/status-pages.md#response-time-chart) |
| **TLS certificate expiration** | Days until the certificate expires, below the name of the endpoint on the dashboard and, with `show-certificate-expiration`, on the status pages. [docs/status-pages.md](docs/status-pages.md) |
| **Login screen** | A login page with logout for `security.basic`, with sessions stored in the database and a limit of failed logins, while `curl -u` keeps working. OIDC is supported too. [docs/admin-endpoints.md](docs/admin-endpoints.md#login-screen) |
| **Backup and restore of the administration** | A JSON file with the endpoints, status pages and push keys, optionally encrypted, restored with a preview of what changes. [docs/admin-endpoints.md](docs/admin-endpoints.md#backup-and-restore) |
| **Command line** | `go-uptime config validate` checks a configuration before a deploy, `go-uptime password hash` generates the hash of a password, `go-uptime version` shows the build, and `go-uptime healthcheck` is the `HEALTHCHECK` of the image, which has no shell. [docs/cli.md](docs/cli.md) |
| **Bulk management script** | `docs/manager-go-uptime.py` registers a CSV of hosts, renames a group in every endpoint, lists endpoints and status pages and exports the push tokens, through the administration API. [docs/admin-endpoints.md](docs/admin-endpoints.md#managing-endpoints-from-the-command-line) |
| **Interface of its own** | Three themes — dark (the default), light and **bio**, in teal, blue and navy —, chosen by each visitor or set with `ui.default-theme`; square style, thin scrollbar in the colours of the theme and the Inter font served by Go Uptime itself, without calling any external service. |

**[See every screen →](docs/screenshots/README.md)**

## Quick start

With Docker, using a fixed version (`latest` is never published):

```bash
mkdir -p config && curl -sL -o config/config.yaml https://raw.githubusercontent.com/jniltinho/go-uptime/master/config.yaml
docker run -d --name go-uptime -p 127.0.0.1:8080:8080 -v "$PWD/config:/config" jniltinho/go-uptime:v7.0.0
```

Open http://127.0.0.1:8080. `docker ps` shows the container as `healthy` once `/health` answers.

That keeps nothing across restarts. With Docker Compose, the history in SQLite and the administration enabled:

```yaml
services:
  go-uptime:
    image: jniltinho/go-uptime:v7.0.0
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"   # publish it through a reverse proxy, not directly
    volumes:
      - ./config:/config:ro
      - go-uptime-data:/data
volumes:
  go-uptime-data:
```

```bash
docker run --rm -it jniltinho/go-uptime:v7.0.0 password hash             # asks for the password, prints the hash
docker run --rm -v "$PWD/config:/config:ro" jniltinho/go-uptime:v7.0.0 config validate   # before every deploy
docker compose up -d
```

with the `storage`, `security` and `admin` blocks of the next section in `config/config.yaml`. For MariaDB or MySQL,
start from [.examples/docker-compose-mariadb-storage](.examples/docker-compose-mariadb-storage).

Every example of [.examples](.examples) starts as it is with the administration enabled: open
http://127.0.0.1:8080/admin and sign in with the user `admin` and the password `go-uptime`. That password is public, so
the examples publish the port on `127.0.0.1` only: **change it** (`go-uptime password hash`) before putting a reverse
proxy in front. The `config.yaml` inside the image has no administration enabled, on purpose.

Without Docker, download `go-uptime_<version>_linux_<amd64|arm64>.tar.gz` from the
[releases](https://github.com/jniltinho/go-uptime/releases) and run:

```bash
tar xzf go-uptime_7.0.0_linux_amd64.tar.gz
./go-uptime config validate --config config.yaml
./go-uptime --config config.yaml
```

To run it as a service, [docs/install-linux.md](docs/install-linux.md) installs it in `/opt/go-uptime` with a hardened
systemd unit, logs in the journal and the upgrade steps.

## Minimal configuration

```yaml
endpoints:
  - name: website
    url: "https://example.org"
    interval: 1m
    conditions:
      - "[STATUS] == 200"
      - "[RESPONSE_TIME] < 500"
```

To keep the history across restarts, configure `storage` (`sqlite`, `postgres` or `mysql`):

```yaml
storage:
  type: sqlite
  path: /data/data.db
```

The administration and the login screen need `security` and `admin.enabled: true`. The password is a bcrypt hash in
base64, which `go-uptime password hash` generates (see [docs/cli.md](docs/cli.md#go-uptime-password-hash)):

```yaml
security:
  basic:
    username: admin
    password-bcrypt-base64: "JDJhJDEwJD..."
admin:
  enabled: true
```

The configuration file and the log level can also come from the environment: `GO_UPTIME_CONFIG_PATH` (a file or a
directory of YAML files) and `GO_UPTIME_LOG_LEVEL`.

## Upgrading

- **From `jniltinho/gatus` v6:** [docs/migrating-from-gatus.md](docs/migrating-from-gatus.md). The only required change
  in Docker is the name of the image; the database needs no migration, the `GATUS_*` variables keep working, and a
  backup made on v6 restores on v7. What is incompatible (the name of the binary, one more login, the prefix of the
  metrics, the `User-Agent`) is listed there with the way out of each.
- **Between versions of Go Uptime:** change the tag of the image or the binary. `test/e2e/upgrade.sh` upgrades the
  previous release to the candidate before every tag.

## Documentation

| Topic | Where |
|-------|-------|
| Screens | [docs/screenshots/README.md](docs/screenshots/README.md) |
| Full configuration: endpoints, conditions, alerting, storage, security, UI, suites, deployment and FAQ | [docs/README.md](docs/README.md) |
| Endpoint administration through the web, login screen, backup and restore | [docs/admin-endpoints.md](docs/admin-endpoints.md) |
| Public status pages and response time chart | [docs/status-pages.md](docs/status-pages.md) |
| Installing the binary on Linux: `/opt/go-uptime`, systemd unit, logs, upgrade, nginx | [docs/install-linux.md](docs/install-linux.md) |
| Command line: `serve`, `version`, `config validate`, `password hash` and `healthcheck` | [docs/cli.md](docs/cli.md) |
| MySQL and MariaDB storage | [docs/storage-mysql.md](docs/storage-mysql.md) |
| Push monitoring compatible with the Uptime Kuma | [docs/push-monitoring.md](docs/push-monitoring.md) |
| Bulk management of endpoints and push tokens (`manager-go-uptime.py`) | [docs/admin-endpoints.md](docs/admin-endpoints.md#managing-endpoints-from-the-command-line) |
| Migrating from `jniltinho/gatus` v6 | [docs/migrating-from-gatus.md](docs/migrating-from-gatus.md) |
| Docker Compose examples | [.examples](.examples) |

## Build from source

```bash
make frontend-install frontend-build   # only when changing the web interface
make build                             # binary in dist/go-uptime
make lint test                         # go vet, gofmt and the Go tests
```

`main.go` only calls the commands of `cmd/`; every other Go package lives in `internal/` and is documented with
`go doc` (`go doc ./internal/api`, for example). The end-to-end suites are in `test/e2e/`.

The Go module is `github.com/jniltinho/go-uptime/v7`. It cannot be installed with `go install` yet: `go.mod` replaces
six modules with the local copies of `third_party/`, which `go install` refuses. Build it from a clone.

## Releases

Releases use `vX.Y.Z` tags, with `linux/amd64` and `linux/arm64` tarballs on
[GitHub](https://github.com/jniltinho/go-uptime/releases) and the `jniltinho/go-uptime:<tag>` image on
[Docker Hub](https://hub.docker.com/r/jniltinho/go-uptime). There is no `latest` tag on purpose: pin the version you
run. The releases up to `v6.3.0` were published as `jniltinho/gatus`, and those images stay available.

## Origin and license

Go Uptime derives from [Gatus](https://github.com/TwiN/gatus), by [TwiN](https://github.com/TwiN): it started as a fork
of it in 2026 and has been developed on its own since `v6.0.0`, when the HTTP server and the command line were
rewritten. A configuration file of Gatus still works unchanged. The credit and the license stay: Go Uptime is
distributed under the [Apache License 2.0](LICENSE), like Gatus, and [NOTICE](NOTICE) records the origin.
