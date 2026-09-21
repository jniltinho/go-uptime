<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
# Command line

The `go-uptime` binary has a command line. Without a command it starts the server, exactly as before, so the image, the
systemd units and everyone who runs `./go-uptime` keep working without any change.

```text
go-uptime                       start the server (same as `go-uptime serve`)
go-uptime serve                 start the server
go-uptime version               version, commit and build date
go-uptime config validate       validate the configuration without starting anything
go-uptime password hash         hash a password for security.basic and for the login of a status page
go-uptime healthcheck           check /health of the local server, for the HEALTHCHECK of the image
go-uptime completion <shell>    shell completion (bash, zsh, fish, powershell)
```

`--help` works on every command.

## Configuration path and log level

| Flag | Environment | Default |
|------|-------------|---------|
| `--config` | `GO_UPTIME_CONFIG_PATH` (and the deprecated `GATUS_CONFIG_FILE`) | `config/config.yaml`, then `config/config.yml` |
| `--log-level` | `GO_UPTIME_LOG_LEVEL` | `INFO` |

The flag wins over the environment, and the environment over the default. Both flags work on every command, before or
after it: `go-uptime --config x.yaml` is `go-uptime serve --config x.yaml`. `GO_UPTIME_DELAY_START_SECONDS` still delays the start
of the server, and only of the server.

Two details that matter:

- **A path given with `--config` must exist.** Through the environment, a missing path falls back on the default paths,
  as it always did; typed on the command line, it is an error — otherwise `go-uptime config validate --config typo.yaml`
  would happily validate another file.
- **A reload uses the same path the server started with.** The configuration is checked every 30 seconds, and the file
  that is loaded again is the one of `--config`, even when `GO_UPTIME_CONFIG_PATH` points elsewhere.

## go-uptime config validate

Loads and validates the configuration exactly as the server does, and nothing else: no storage is opened, no server is
started and no request is made. It exits with `0` when the configuration is valid and with `1`, showing the error, when
it is not — which makes it usable in a pipeline before a deploy:

```bash
go-uptime config validate --config ./config.yaml && docker compose up -d
```

```text
$ go-uptime config validate --config ./config.yaml
The configuration is valid: 9 endpoints, 0 external endpoints, 0 suites
```

## go-uptime password hash

Prints the bcrypt hash of a password encoded in base64, the value of `security.basic.password-bcrypt-base64` and of
`auth.password-bcrypt-base64` of a [status page with a login](status-pages.md#login-of-a-page). It replaces
[generate-admin-password.py](generate-admin-password.py), which keeps working.

On a terminal the password is asked twice, without echo. Through a pipe, one line is read and only its line break is
dropped, so `echo` and `printf` give the same hash, and spaces stay part of the password:

```bash
go-uptime password hash                          # asks for the password
printf 'the-password' | go-uptime password hash  # from a pipe
```

The password is **never** accepted as an argument or as a flag: it would stay in the history of the shell and in the
list of processes. Passwords are limited to 72 bytes, the limit of bcrypt, which silently ignores everything past it.

## go-uptime healthcheck

Requests `/health` and exits with `0` when the answer is `200`, and with `1` otherwise, in at most 5 seconds. It exists
because the image is built `FROM scratch`: there is no shell, no `curl` and no `wget` to check the server with, so the
binary checks itself. Both images declare it:

```dockerfile
HEALTHCHECK --interval=30s --timeout=6s --start-period=30s --retries=3 CMD ["/go-uptime", "healthcheck"]
```

Without `--url`, the address, the port and the scheme come from the `web` section of the configuration, and only from
it: the rest of the configuration is not validated, and nothing the server starts is started (no start delay, no
storage, no OIDC).

- A wildcard address (`0.0.0.0`, `::`) is where the server listens, not where it can be reached: the check goes to the
  loopback.
- With `web.tls`, the check speaks HTTPS **without verifying the certificate**, which may be self-signed or issued for
  another name than the loopback. It only ever talks to the server of its own configuration.
- A redirect is not a healthy answer, and the proxy of the environment is never used.
- `--url` checks another address, and then the configuration is not read at all. The certificate is only left
  unverified for the loopback (`127.0.0.1`, `::1`, `localhost`): a `--url` to another host gets the normal verification.
- The 5 seconds cover the whole command, the read of the configuration included.
- The `HEALTHCHECK` is another process: it inherits the environment of the container, **not the arguments** of the
  server. In a container, choose the configuration with `GO_UPTIME_CONFIG_PATH`, which both see, rather than with
  `command: ["serve", "--config", "/other.yaml"]`, which the healthcheck would not know about.

With `GO_UPTIME_DELAY_START_SECONDS`, raise the `--start-period` of the `HEALTHCHECK` in your compose file accordingly.

## What changed for whoever already ran the binary

- An invalid or missing configuration at start used to end in a Go `panic` with a stack trace and the exit code 2. It
  now prints one line, `Error: ...`, and exits with 1. A storage that cannot be opened still panics, as before.
- The prefix of the log lines of the lifecycle went from `[main.` to `[cmd.` — for instance
  `[cmd.listenToConfigurationFileChanges]`. An alert that matches the old prefix has to be updated.

## go-uptime version

```text
$ go-uptime version
go-uptime 7.0.0 (commit: 46681c7f, built: 2026-09-21T08:13:45Z)
```

The three values are written into the binary at build time by `make build`, `make release-cross` and the `Dockerfile`;
a binary built with a plain `go build` says `dev`.

## Layout

`main.go`, at the root of the project, only calls `cmd.Execute()`. The `cmd` package has one file per command, with
its tests next to it: `root.go`, `serve.go` (the lifecycle: start, reload, signals), `reload.go`, `version.go`,
`config.go`, `password.go`, `healthcheck.go` and `options.go` (the precedence of the flags). `test/e2e/cli.sh` checks
the commands and the reload against the real binary.
