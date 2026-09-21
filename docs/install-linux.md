<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
# Installing the binary on Linux with systemd

Go Uptime is a single static binary: no runtime, no libraries, no container. This page installs the release tarball in
`/opt/go-uptime` and runs it as a systemd service, under a user of its own and with the file system locked down.

```text
/opt/go-uptime/go-uptime                 the binary
/opt/go-uptime/config/config.yaml    the configuration (several YAML files in this directory are merged)
/opt/go-uptime/data/                 the SQLite database and nothing else: the only place the service writes
/etc/systemd/system/go-uptime.service
```

## Install

```bash
VERSION=7.0.0
ARCH=amd64        # or arm64

sudo useradd --system --home-dir /opt/go-uptime --shell /usr/sbin/nologin go-uptime
sudo install -d -o root -g go-uptime -m 0750 /opt/go-uptime /opt/go-uptime/config
sudo install -d -o go-uptime -g go-uptime -m 0750 /opt/go-uptime/data

curl -fsSL -o /tmp/go-uptime.tar.gz \
  "https://github.com/jniltinho/go-uptime/releases/download/v${VERSION}/go-uptime_${VERSION}_linux_${ARCH}.tar.gz"
sudo tar xzf /tmp/go-uptime.tar.gz -C /opt/go-uptime go-uptime
sudo chown root:go-uptime /opt/go-uptime/go-uptime && sudo chmod 0750 /opt/go-uptime/go-uptime
sudo -u go-uptime /opt/go-uptime/go-uptime version
```

The binary and the configuration belong to `root` and are only readable by the `go-uptime` group: the service cannot
rewrite its own binary nor its configuration, which holds the password hash and the secrets of the alerts.

## Configure

```bash
sudo -u go-uptime /opt/go-uptime/go-uptime password hash      # asks for the password, prints the hash
sudoedit /opt/go-uptime/config/config.yaml
```

```yaml
web:
  address: 127.0.0.1      # publish it through nginx or another reverse proxy
  port: 8080
storage:
  type: sqlite
  path: /opt/go-uptime/data/data.db
security:
  basic:
    username: admin
    password-bcrypt-base64: "JDJhJDEwJD..."
admin:
  enabled: true
endpoints:
  - name: website
    url: "https://example.org"
    interval: 1m
    conditions:
      - "[STATUS] == 200"
```

```bash
sudo chown root:go-uptime /opt/go-uptime/config/config.yaml && sudo chmod 0640 /opt/go-uptime/config/config.yaml
sudo -u go-uptime /opt/go-uptime/go-uptime config validate --config /opt/go-uptime/config/config.yaml
```

With PostgreSQL, MySQL or MariaDB as the storage, `/opt/go-uptime/data` stays empty and `storage.path` is the DSN: see
[storage-mysql.md](storage-mysql.md).

## The service

Copy [systemd/go-uptime.service](systemd/go-uptime.service) and start it:

```bash
sudo curl -fsSL -o /etc/systemd/system/go-uptime.service \
  https://raw.githubusercontent.com/jniltinho/go-uptime/v7.0.0/docs/systemd/go-uptime.service
sudo systemctl daemon-reload
sudo systemctl enable --now go-uptime
systemctl status go-uptime
curl -s http://127.0.0.1:8080/health          # {"status":"UP"}
journalctl -u go-uptime -f                        # the log, see "Logs" below
```

What the unit does, and why:

| | |
|---|---|
| `ExecStartPre=... config validate` | A restart with an invalid configuration fails before the running process is replaced, with the error in `journalctl`. |
| No `ExecReload` | Go Uptime reloads the configuration by itself, up to 30 seconds after the file changes. `systemctl restart go-uptime` is only needed after replacing the binary. Endpoints and status pages created at `/admin` live in the database and need neither. |
| `User=go-uptime`, `NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths=/opt/go-uptime/data` | The service runs without root and sees the whole file system as read-only, except its data directory. |
| `AmbientCapabilities=CAP_NET_RAW` | ICMP endpoints (`icmp://host`) open a raw socket, which a user other than root only gets through this capability. Remove the two capability lines if you do not monitor by ping. |
| `RestrictAddressFamilies`, `MemoryDenyWriteExecute`, `SystemCallArchitectures=native`, ... | System call filters. `systemd-analyze security go-uptime` rates the unit at 3.2 (OK); an unhardened service is around 9.6. |

If an endpoint needs something the sandbox denies — a client certificate in `/home`, an SSH key, a Unix socket — the
check fails with a permission error in the log: grant that path with another `ReadOnlyPaths=` or `ReadWritePaths=`
line in a drop-in (`sudo systemctl edit go-uptime`) instead of removing the protection.

## Logs

Go Uptime writes its log to the standard output — it has no log file of its own and no HTTP access log — and systemd sends
it to the journal, under the identifier `go-uptime`:

```bash
journalctl -u go-uptime -f                          # follow
journalctl -u go-uptime --since "1 hour ago"
journalctl -u go-uptime -b | grep -E "WARN|ERROR"   # since the last boot, only the problems
journalctl -u go-uptime -o cat | grep "key=core_website"   # one endpoint
```

The level is part of the text of each line, not a priority of the journal, so `journalctl -p warning` does not filter
it: use `grep`. Every line starts with its origin, such as `[watchdog.executeEndpoint]` or `[api.pushHandler]`. Since `v6.0.0` the
lines of the start and of the reload begin with `[cmd.` (before, `[main.`): adjust filters and alerts that match on the
old prefix.

- **Level:** `GO_UPTIME_LOG_LEVEL` (`DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`), in the unit or in a drop-in
  (`sudo systemctl edit go-uptime`, then `[Service]` and `Environment=GO_UPTIME_LOG_LEVEL=DEBUG`), followed by
  `sudo systemctl restart go-uptime`. `INFO` logs one line per check, which with many endpoints and short intervals is most
  of the volume; `WARN` keeps only the problems.
- **Keeping the journal across reboots:** on a distribution where `/var/log/journal` does not exist the journal lives in
  memory. `sudo mkdir -p /var/log/journal && sudo systemctl restart systemd-journald` makes it persistent, and
  `SystemMaxUse=1G` in `/etc/systemd/journald.conf` bounds its size.
- **A log file as well**, for a collector that reads files: in a drop-in,

  ```ini
  [Service]
  LogsDirectory=go-uptime
  StandardOutput=append:/var/log/go-uptime/go-uptime.log
  StandardError=inherit
  ```

  systemd creates `/var/log/go-uptime` for the `go-uptime` user and opens the file itself, so the sandbox needs no other
  change. Rotate it with `copytruncate`, because the file stays open while the service runs:

  ```text
  # /etc/logrotate.d/go-uptime
  /var/log/go-uptime/go-uptime.log {
      daily
      rotate 14
      compress
      missingok
      notifempty
      copytruncate
  }
  ```

## Upgrade

> Coming from `jniltinho/gatus` v6, installed in `/opt/gatus`? See [migrating-from-gatus.md](migrating-from-gatus.md): you
> can stay in `/opt/gatus` and only swap the binary, or move here, which needs `storage.path` and a few other paths edited.

```bash
VERSION=x.y.z; ARCH=amd64     # the new version
curl -fsSL -o /tmp/go-uptime.tar.gz \
  "https://github.com/jniltinho/go-uptime/releases/download/v${VERSION}/go-uptime_${VERSION}_linux_${ARCH}.tar.gz"
sudo cp -a /opt/go-uptime/go-uptime /opt/go-uptime/go-uptime.previous
sudo tar xzf /tmp/go-uptime.tar.gz -C /opt/go-uptime go-uptime
sudo chown root:go-uptime /opt/go-uptime/go-uptime && sudo chmod 0750 /opt/go-uptime/go-uptime
sudo systemctl restart go-uptime && systemctl status go-uptime
```

The database is migrated when the new version starts. To go back, restore `go-uptime.previous` and restart; before an
upgrade that skips several versions, download a backup at `/admin/backup` first
(see [admin-endpoints.md](admin-endpoints.md#backup-and-restore)).

## Behind nginx

```nginx
server {
    listen 443 ssl;
    server_name status.example.com;
    # ssl_certificate ...;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # Real-time updates are an event stream: it must not be buffered, and it stays open for minutes
    location ~ ^/api/v1/(endpoints|status-pages)/.*/events$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_read_timeout 7m;
    }
}
```

`Host` and `X-Forwarded-Proto` are what the administration uses to accept a change as coming from its own address, and
what marks the session cookie as `Secure`: see [admin-endpoints.md](admin-endpoints.md#behind-a-reverse-proxy).

## Remove

```bash
sudo systemctl disable --now go-uptime
sudo rm /etc/systemd/system/go-uptime.service && sudo systemctl daemon-reload
sudo rm -rf /opt/go-uptime          # includes the database
sudo userdel go-uptime
```
