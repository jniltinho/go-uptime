<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
# Migrating from `jniltinho/gatus` v6 to Go Uptime v7

Go Uptime is the same project as `jniltinho/gatus`, under its new name from `v7.0.0` on. The database, the configuration
file and the API routes are the same, and **there is no database migration**. This page lists what you have to change,
what you may have to change, and what changes by itself.

Back up first: download a backup at **Admin → Backup** on v6 (it restores on v7), and copy the database. With SQLite
that is three files while Gatus runs (`data.db`, `data.db-wal`, `data.db-shm`), or one after stopping it.

## Docker

**Required: the name of the image.**

```diff
 services:
   gatus:
-    image: jniltinho/gatus:v6.3.0
+    image: jniltinho/go-uptime:v7.0.0
```

**Keep everything else as it is**: the name of the service, the volume, the database and its user, the labels of
Prometheus. Renaming a volume or a database points the installation to empty data, and renaming the service breaks
whoever resolves it by name (a reverse proxy, `prometheus.yml`). The new examples of this repository use new names
because they are for new installations, not because yours must change.

What keeps working without a change:

- `GATUS_CONFIG_PATH`, `GATUS_LOG_LEVEL` and `GATUS_DELAY_START_SECONDS`, as aliases of `GO_UPTIME_CONFIG_PATH`,
  `GO_UPTIME_LOG_LEVEL` and `GO_UPTIME_DELAY_START_SECONDS`. Each one used logs a warning at every start; with both
  set, the new one wins; an empty one counts as not set. `GATUS_CONFIG_FILE`, which was already deprecated, stays
  accepted too.
- An `entrypoint` or a `healthcheck` of your own that calls `/gatus`: the image keeps `/gatus` as a symbolic link to
  `/go-uptime` during the 7.x series.

**Conditional changes**, only if they apply to you:

| If you… | Then… |
|---|---|
| have Prometheus dashboards or alerts on `gatus_results_total` and the other metrics | add `metrics-namespace: gatus` to the configuration, which keeps the names of v6 exactly, or rewrite the queries for the new prefix, `go_uptime_` |
| allow the checks through a firewall or a WAF by their `User-Agent` | allow `go-uptime/1.0` (it was `Gatus/1.0`). The Zulip provider also sends `go-uptime/1.0` now. An endpoint can still set its own `User-Agent` header |
| expand `${GATUS_CONFIG_PATH}` or `${GATUS_LOG_LEVEL}` **inside** the YAML, counting on the value that the image used to set | set the variable in your compose: the image no longer defines them (their defaults would hide the old names), so the expansion would be empty |
| show the JSON badge (`…/health/badge.shields`) through shields.io | the label is now `go-uptime`; add `&label=…` to the badge URL to choose another |

## Linux, installed with `docs/install-linux.md`

There are two ways. The first one changes the least. Both were run for real before the release, under systemd, starting
from the published `v6.3.0` installed by its own guide: the history is kept in both.

### A. Stay in `/opt/gatus`

Nothing of the unit, the user, the paths or the configuration changes, except the binary:

```bash
VERSION=7.0.0; ARCH=amd64
curl -sLO "https://github.com/jniltinho/go-uptime/releases/download/v${VERSION}/go-uptime_${VERSION}_linux_${ARCH}.tar.gz"
tar xzf "go-uptime_${VERSION}_linux_${ARCH}.tar.gz" go-uptime
sudo systemctl stop gatus
sudo install -o root -g gatus -m 0750 go-uptime /opt/gatus/go-uptime   # same owner and mode as the binary of v6
sudo ln -sfn go-uptime /opt/gatus/gatus          # the unit keeps calling /opt/gatus/gatus
/opt/gatus/go-uptime config validate --config /opt/gatus/config/config.yaml
sudo systemctl start gatus && systemctl status gatus --no-pager
```

`Environment=GATUS_LOG_LEVEL=…` in the unit keeps working, with the warning in the journal.

### B. Move to `/opt/go-uptime`

Follow [install-linux.md](install-linux.md) for the new directory, user and unit, then bring your data and **edit what
holds a path**:

1. `sudo systemctl stop gatus`
2. Copy `/opt/gatus/config/` and `/opt/gatus/data/` to `/opt/go-uptime/`, owned by the `go-uptime` user.
3. In `/opt/go-uptime/config/config.yaml`, change `storage.path` (`/opt/gatus/data/data.db` →
   `/opt/go-uptime/data/data.db`) and any other path under `/opt/gatus`. **Without this the service does not start**
   (`unable to open database file` in the journal): the new unit only lets it write in `/opt/go-uptime/data`.
4. Move your drop-ins from `/etc/systemd/system/gatus.service.d/` to `go-uptime.service.d/`, renaming `GATUS_*`
   variables if you want to get rid of the warnings.
5. `sudo systemctl disable --now gatus && sudo systemctl enable --now go-uptime`
6. The journal is now read with `journalctl -u go-uptime`; anything that filters by `SyslogIdentifier=gatus` (rsyslog,
   a log shipper) has to filter by `go-uptime`.

## What changes by itself

- **Everybody signs in once more.** The session cookie is now `go_uptime_session` (and `go_uptime_state` /
  `go_uptime_nonce` during an OIDC login), so the sessions of v6 are not recognised.
- **The preferences of each browser are kept**: sorting, filters, collapsed groups, the period of the chart and the
  rest are migrated from the `gatus:` keys of `localStorage` the first time they are read. The theme is in the `theme`
  cookie, which did not change, and an installed PWA stays the same application.
- **The visible text of the alerts says Go Uptime** (titles on Slack, Teams, Discord, Telegram, ntfy, Gotify, Pushover,
  incident.io and n8n; sender, title and icon on Mattermost and Rocket.Chat).
- **Backups are written in the new format** (`go-uptime-admin-backup`, file `go-uptime-backup-<date>.json`). A backup
  made on v6 still restores on v7, in clear or encrypted, through the API and through the screen.
- `gatus` becomes `go-uptime` on the command line: `go-uptime config validate`, `go-uptime password hash`…

## What does not change, on purpose

Some values keep the old name because **another system** uses them to identify, deduplicate, close or filter, and
changing them during an upgrade would leave open alerts unresolved or silence filters and automations:

| Provider | Kept |
|---|---|
| Opsgenie | `source` (`gatus`), `entity-prefix` (`gatus-`), `alias-prefix` (`gatus-healthcheck-`) |
| SIGNL4, Squadcast | the external id and the `event_id`, `gatus-<key>`; Squadcast also the `source` tag |
| GitHub, Gitea | the title of the issue, `alert(gatus): <endpoint>`, which is how the issue is found again to be closed |
| GitLab | `monitoring-tool` (`gatus`), which also makes the title of the alert |
| Datadog, New Relic, PagerDuty, Splunk | the source, source type, event type and service fields |
| Home Assistant | the event `gatus_alert`; the new `event-type` option changes it |
| iLert | the URL of the integration, which is theirs |
| Zulip | the default topic, `Gatus`, when no `topic` is configured |

Most of them are options: set your own value if you prefer the new name, once no alert opened by v6 is pending.

The scripts keep working too: `docs/manager-gatus.py` is still there, as an identical copy of
`docs/manager-go-uptime.py`, and both accept `--gatus-url`, `GATUS_URL`, `GATUS_USERNAME` and `GATUS_PASSWORD` besides
`--url` and the `GO_UPTIME_*` variables.

## Going back to v6

Reinstalling `jniltinho/gatus:v6.3.0` over the same database works, since nothing was migrated. Two things do not go
back: **v6 cannot read a backup made by v7** (keep the one you made before upgrading), and a configuration with
`metrics-namespace`, or a Home Assistant provider with `event-type`, has options that v6 does not know — `metrics-namespace`
is ignored by v6, and `event-type` too.

## How long the compatibility lasts

The `GATUS_*` variables, the `/gatus` link of the image, the legacy backup formats, the migration of the browser
preferences and the copy of the script stay for the whole 7.x series.
