# MySQL and MariaDB storage

> Not in Gatus, the project that Go Uptime derives from: there, the
> request for MySQL/MariaDB was closed ([TwiN/gatus#283](https://github.com/TwiN/gatus/issues/283)) and the
> implementation was not merged ([TwiN/gatus#1003](https://github.com/TwiN/gatus/pull/1003)): the maintainer decided not
> to support more storage types.

`storage.type: mysql` stores the results, events, uptimes, triggered alerts and suites in **MySQL 8.4+** or
**MariaDB 10.11+**, with the same behavior as PostgreSQL, including the [administration](admin-endpoints.md) and the
[public status pages](status-pages.md).

## Supported versions

| Database | Minimum | Tested in CI |
|----------|---------|--------------|
| MySQL    | 8.4 LTS | 8.4.11 and 9.7.2 |
| MariaDB  | 10.11 LTS | 10.11.19 and 12.3.3 |

The minimums are the oldest LTS versions still supported by their vendors (MySQL 8.0 and MariaDB 10.6 reached their end
of life in 2026). Go Uptime does not refuse older servers, but it logs a warning at startup and they are not supported.
Amazon Aurora, TiDB and PlanetScale/Vitess are not tested.

## Configuration

```yaml
storage:
  type: mysql
  path: "go_uptime:${MARIADB_PASSWORD}@tcp(mariadb:3306)/go_uptime"
  caching: true
```

`storage.path` is a DSN of [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql#dsn-data-source-name):
`user:password@tcp(host:port)/database?parameters`. An invalid DSN fails the validation of the configuration, and the
password is never written to the logs.

Go Uptime overrides these parameters, whatever the DSN and the server configuration say, because the storage relies on
them:

| What | Value | Why |
|------|-------|-----|
| `parseTime`, `loc` | `true`, `UTC` | times are stored in `DATETIME(6)` columns, without time zone |
| Session `time_zone` | `+00:00` | no surprise with date functions |
| Charset and collation | `utf8mb4`, `utf8mb4_bin` | keys differing only by case are different keys, like in PostgreSQL |
| `clientFoundRows` | `true` | an update that changes nothing still counts the matched row |
| Session `sql_mode` | `ANSI_QUOTES,ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION` | same quoting as the other databases, errors instead of silent truncation |
| Session `innodb_lock_wait_timeout` | `10` seconds | a lock wait becomes an error handled by the storage |
| Transaction isolation | `READ COMMITTED` | the default of PostgreSQL; avoids the deadlocks of concurrent checks |
| `interpolateParams` | `true` | fewer round trips per check result |

The other parameters of the DSN are kept, for example `tls=true`, `timeout=5s` or `readTimeout=30s`. The pool keeps at
most 25 connections, recycled every 3 minutes.

## Creating the database

```sql
CREATE DATABASE go_uptime CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
CREATE USER 'go_uptime'@'%' IDENTIFIED BY 'a-long-password';
GRANT ALL PRIVILEGES ON go_uptime.* TO 'go_uptime'@'%';
```

The tables are created automatically when Go Uptime starts, and starting again on an existing schema keeps the data. They
use InnoDB (required for the foreign keys) and `utf8mb4_bin`.

With Docker, see [`.examples/docker-compose-mariadb-storage`](../.examples/docker-compose-mariadb-storage): the image of
the original Gatus (`twinproduction/gatus`) does not include this storage, use `jniltinho/go-uptime` with a fixed tag.

## Limits

- **Keys of at most 768 characters.** The key of an endpoint, external endpoint, suite or endpoint of a suite
  (`group_name`) must fit in an InnoDB index. Longer keys fail the validation of the configuration and of the
  administration with a message citing the limit. SQLite and PostgreSQL have no such limit.
- **InnoDB page size of 16K** (the default). Go Uptime fails at startup with a smaller `innodb_page_size`.
- **25 connections.** With many endpoints checked at the same time, checks wait for a free connection instead of
  exceeding `max_connections`.

## Behavior

- A check result is written in a transaction. A deadlock or a lock wait timeout rolls back the whole transaction, and
  Go Uptime tries again, up to 3 attempts. Nothing is written halfway.
- The history older than 48 hours is merged into daily uptime entries, like with SQLite and PostgreSQL.
- `ON DELETE CASCADE` foreign keys remove the results, events, uptimes and triggered alerts of a removed endpoint.

## Multiple instances

Several Go Uptime instances can share the same database, as with PostgreSQL. Endpoints and status pages changed through the
administration only apply to the other instances after they reload their configuration or restart.

MariaDB Galera clusters are not tested: the insertion of check results retries certification conflicts, but the
changes made through the administration do not.

## Backup

```bash
mariadb-dump -ugo_uptime -p --single-transaction go_uptime > go-uptime.sql   # MariaDB
mysqldump -ugo_uptime -p --single-transaction go_uptime > go-uptime.sql      # MySQL
```

## Migrating and going back

- There is **no migration** of the data of SQLite or PostgreSQL: switching `storage.type` to `mysql` starts with an
  empty database. The endpoints and status pages of the configuration file come back on the first load; the ones
  managed through the administration must be created again.
- Switching back to the previous `storage.type` finds the previous storage untouched.
- The original Gatus does not read MySQL: going back to it with `storage.type: mysql` fails the validation.

## Troubleshooting

| Log | Meaning |
|-----|---------|
| `storage.path is not a valid MySQL DSN` | the DSN could not be parsed; check the format `user:password@tcp(host:port)/database` |
| `MySQL server version=... is older than the minimum supported versions` | the server works, but it is not supported |
| `the InnoDB page size of the MySQL server is too small` | the server uses `innodb_page_size` below 16K |
| `Retrying in ... after a transient MySQL error` | a deadlock or lock wait timeout happened and the result is written again |

## Tests

The tests of the SQL storage also run on MySQL and MariaDB when these variables are set, with a user allowed to create
databases (each test uses a database of its own):

```bash
docker run -d --name go-uptime-test-mysql -p 127.0.0.1:53306:3306 -e MYSQL_ROOT_PASSWORD=go-uptime-root mysql:8.4.11
docker run -d --name go-uptime-test-mariadb -p 127.0.0.1:53307:3306 -e MARIADB_ROOT_PASSWORD=go-uptime-root mariadb:10.11.19

GO_UPTIME_TEST_MYSQL_URL='root:go-uptime-root@tcp(127.0.0.1:53306)/' \
GO_UPTIME_TEST_MARIADB_URL='root:go-uptime-root@tcp(127.0.0.1:53307)/' \
  go test ./storage/... -race
```

The end-to-end tests accept another storage:

```bash
E2E_STORAGE_TYPE=mysql E2E_STORAGE_PATH='root:go-uptime-root@tcp(127.0.0.1:53307)/go_uptime_e2e' test/e2e/status-pages.sh
```

Use an empty database: the scripts expect a storage without data.
