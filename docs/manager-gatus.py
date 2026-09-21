#!/usr/bin/env python3
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
"""Manages the endpoints of Go Uptime through the administration API: imports them from a CSV, renames groups and exports
the push tokens.

Subcommands:

    csv              rewrites an inventory of hostnames as the simple CSV, without calling Go Uptime
    import           registers the endpoints of a CSV, each one accepting push with a token of its own
    endpoints        lists the endpoints, with their source, state and push
    status-pages     lists the status pages, with their state and whether they require a login
    groups           lists the groups with the number of endpoints of each one
    rename-group     changes the group of every endpoint of a group, in one go
    export-tokens    writes host,token of every endpoint that accepts push

Examples:

    # Rewrites the inventory keeping production, one row per hostname, the service as the group
    python3 docs/manager-api.py csv --csv inventory.csv --out endpoints-prod.csv

    # Registers everything, with a token per endpoint (--dry-run shows without sending)
    export GO_UPTIME_URL=https://status.example.com GO_UPTIME_PASSWORD='the-password'
    python3 docs/manager-api.py import --csv endpoints-prod.csv

    # Lists what is registered
    python3 docs/manager-api.py endpoints --group analytics
    python3 docs/manager-api.py status-pages

    # Renames a group in every endpoint that uses it
    python3 docs/manager-api.py rename-group --from outros-institucional --to institucional

    # Exports the push tokens
    python3 docs/manager-api.py export-tokens --out hosts-tokens.csv

The address and the credentials come from --url, --username and --password, or from GO_UPTIME_URL, GO_UPTIME_USERNAME
and GO_UPTIME_PASSWORD, which keeps the password out of the shell history. Only the standard library is used.

The project was called Gatus up to v6: --gatus-url, GATUS_URL, GATUS_USERNAME and GATUS_PASSWORD are still accepted,
the new names winning, and docs/manager-gatus.py stays in the repository as an identical copy of this file, because the
script is downloaded by its address.
"""

from __future__ import annotations

import argparse
import base64
import csv
import json
import os
import re
import secrets
import ssl
import string
import sys
import time
import unicodedata
import urllib.error
import urllib.parse
import urllib.request

# Columns of the inventory of hostnames
INVENTORY_SERVICE = "Serviço"
INVENTORY_ENVIRONMENT = "Ambiente"
INVENTORY_HOSTNAME = "Hostname"
INVENTORY_STATUS = "Status"

# Columns of the simple CSV this script reads and writes
SIMPLE_COLUMNS = ["grupo", "nome", "url"]

TOKEN_ALPHABET = string.ascii_letters + string.digits
TOKEN_LENGTH = 32

# Endpoints of the configuration file cannot be changed through the administration
SOURCE_CONFIG = "config"


# How many times a request answered with 503 is sent, and the longest wait between two of them, in seconds
UNAVAILABLE_ATTEMPTS = 8
UNAVAILABLE_MAXIMUM_DELAY = 5.0


def retry_delay(retry_after: str | None, attempt: int) -> float:
    """The wait before repeating a request: Retry-After when Go Uptime sends it in seconds, otherwise 0.25s doubled at each
    attempt, and never more than UNAVAILABLE_MAXIMUM_DELAY"""
    try:
        delay = float(retry_after) if retry_after else 0.25 * (2**attempt)
    except ValueError:
        delay = 0.25 * (2**attempt)
    return max(0.0, min(delay, UNAVAILABLE_MAXIMUM_DELAY))


def environment(name, default=None):
    """GO_UPTIME_<name>, then GATUS_<name>, the name that the variable had up to v6. An empty value counts as not set."""
    return os.environ.get(f"GO_UPTIME_{name}") or os.environ.get(f"GATUS_{name}") or default


class ManagerError(Exception):
    """An answer of Go Uptime, or a file, that the script cannot go on with"""


def strip_accents(value: str) -> str:
    return "".join(char for char in unicodedata.normalize("NFD", value) if not unicodedata.combining(char))


def slugify(value: str) -> str:
    """Turns a name into a group of Go Uptime: lowercase, without accents and with a single hyphen between the words"""
    return re.sub(r"[^a-z0-9]+", "-", strip_accents(value).lower()).strip("-")


def sanitize_key_part(value: str) -> str:
    """Exactly what Go Uptime does to build a key: lowercase, and each of / _ . , space # + & turned into a hyphen. Accents
    are kept, and repeated separators are not collapsed, so the key is the same one Go Uptime shows."""
    value = value.strip().lower()
    for character in ("/", "_", ".", ",", " ", "#", "+", "&"):
        value = value.replace(character, "-")
    return value


def endpoint_key(group: str, name: str) -> str:
    """The key of an endpoint, as Go Uptime builds it from the group and the name"""
    return f"{sanitize_key_part(group)}_{sanitize_key_part(name)}"


def generate_token() -> str:
    return "".join(secrets.choice(TOKEN_ALPHABET) for _ in range(TOKEN_LENGTH))


def read_rows(path: str, environment: str, keep_every_status: bool) -> list[dict[str, str]]:
    """Reads the CSV and returns one row per endpoint, with the group, the name and the URL"""
    with open(path, newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        columns = reader.fieldnames or []
        if INVENTORY_HOSTNAME in columns:
            rows = list(read_inventory(reader, environment, keep_every_status))
        elif all(column in columns for column in SIMPLE_COLUMNS):
            rows = [
                {"grupo": row["grupo"].strip(), "nome": row["nome"].strip(), "url": row["url"].strip()}
                for row in reader
                if row.get("nome", "").strip()
            ]
        else:
            raise ManagerError(
                f"{path}: expected the columns of the inventory ({INVENTORY_HOSTNAME}, {INVENTORY_SERVICE}, "
                f"{INVENTORY_ENVIRONMENT}) or the simple ones ({', '.join(SIMPLE_COLUMNS)}), found {columns}"
            )
    return deduplicate(rows)


def read_inventory(reader: csv.DictReader, environment: str, keep_every_status: bool):
    """Filters the inventory by environment and by status, and yields the rows of the endpoints"""
    wanted = strip_accents(environment).lower()
    for row in reader:
        hostname = (row.get(INVENTORY_HOSTNAME) or "").strip().rstrip(".")
        if not hostname:
            continue
        if wanted and not strip_accents((row.get(INVENTORY_ENVIRONMENT) or "")).lower().startswith(wanted):
            continue
        if not keep_every_status and (row.get(INVENTORY_STATUS) or "").strip().upper() not in ("", "OK"):
            continue
        yield {"grupo": slugify(row.get(INVENTORY_SERVICE) or "") or "outros", "nome": hostname, "url": ""}


def deduplicate(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    """Keeps one row per name, because the same hostname can appear more than once in the inventory"""
    seen: dict[str, dict[str, str]] = {}
    for row in rows:
        seen.setdefault(row["nome"].lower(), row)
    return sorted(seen.values(), key=lambda row: (row["grupo"], row["nome"]))


def endpoint_url(row: dict[str, str], scheme: str, path: str) -> str:
    return row["url"] or f"{scheme}://{row['nome']}{path}"


class Administration:
    """The administration API of Go Uptime, with basic authentication"""

    def __init__(self, arguments: argparse.Namespace) -> None:
        if not arguments.password:
            raise ManagerError("the password of the administration is required (--password or GO_UPTIME_PASSWORD)")
        self.base_url = arguments.url.rstrip("/")
        credentials = base64.b64encode(f"{arguments.username}:{arguments.password}".encode()).decode()
        self.authorization = f"Basic {credentials}"
        self.timeout = arguments.timeout
        self.context = ssl._create_unverified_context() if arguments.insecure else None

    def request(self, method: str, path: str, body: dict | None = None, headers: dict | None = None) -> tuple[int, object]:
        data = json.dumps(body).encode() if body is not None else None
        request = urllib.request.Request(self.base_url + path, data=data, method=method)
        request.add_header("Authorization", self.authorization)
        request.add_header("Accept", "application/json")
        if data is not None:
            request.add_header("Content-Type", "application/json")
        for name, value in (headers or {}).items():
            request.add_header(name, value)
        # Origin is not sent on purpose: Go Uptime only refuses an Origin that does not match the address it answers on,
        # which behind a reverse proxy is not the address used here. A request without Origin is not a CSRF risk.
        # Go Uptime applies each change in a cycle of its own and answers 503 to the changes that arrive meanwhile ("a start
        # or configuration reload is in progress, try again later"), which a sequence of requests hits all the time.
        # The request is safe to repeat: a 503 means that nothing was changed.
        for attempt in range(UNAVAILABLE_ATTEMPTS):
            try:
                with urllib.request.urlopen(request, timeout=self.timeout, context=self.context) as response:
                    return response.status, decode(response.read())
            except urllib.error.HTTPError as error:
                body = decode(error.read())
                if error.code != 503 or attempt == UNAVAILABLE_ATTEMPTS - 1:
                    return error.code, body
                time.sleep(retry_delay(error.headers.get("Retry-After"), attempt))
            except urllib.error.URLError as error:
                raise ManagerError(f"{method} {path}: {error.reason}") from error
        raise AssertionError("unreachable")

    def list_endpoints(self) -> list[dict]:
        status, body = self.request("GET", "/api/v1/admin/endpoints")
        if status != 200 or not isinstance(body, list):
            raise ManagerError(f"the list of endpoints answered {status}: {describe(body)}")
        return body

    def get_endpoint(self, key: str) -> dict:
        status, body = self.request("GET", "/api/v1/admin/endpoints/" + urllib.parse.quote(key, safe=""))
        if status != 200 or not isinstance(body, dict):
            raise ManagerError(f"the endpoint {key} answered {status}: {describe(body)}")
        return body

    def list_status_pages(self) -> dict:
        status, body = self.request("GET", "/api/v1/admin/status-pages")
        if status != 200 or not isinstance(body, dict):
            raise ManagerError(f"the list of status pages answered {status}: {describe(body)}")
        return body

    def create_endpoint(self, definition: dict) -> tuple[int, object]:
        return self.request("POST", "/api/v1/admin/endpoints", definition)

    def update_endpoint(self, key: str, definition: dict, version: int) -> tuple[int, object]:
        return self.request(
            "PUT",
            "/api/v1/admin/endpoints/" + urllib.parse.quote(key, safe=""),
            definition,
            {"If-Match": f'"{version}"'},
        )


def decode(raw: bytes) -> object:
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw.decode(errors="replace").strip()


def describe(answer: object) -> str:
    if isinstance(answer, dict) and "error" in answer:
        return str(answer["error"])
    return str(answer)


def build_definition(row: dict[str, str], arguments: argparse.Namespace) -> dict:
    definition: dict[str, object] = {
        "name": row["nome"],
        "group": row["grupo"],
        "url": endpoint_url(row, arguments.scheme, arguments.path),
        "interval": arguments.interval,
        "conditions": arguments.condition,
    }
    if not arguments.no_push:
        definition["push"] = {"enabled": True, "token": generate_token()}
    return definition


def command_csv(arguments: argparse.Namespace) -> int:
    rows = filtered_rows(arguments)
    with open(arguments.out, "w", newline="", encoding="utf-8") as handle:
        # Lines ending in \n only, like the CSV files handled by hand: a stray \r turns into part of the value in a
        # shell pipeline
        writer = csv.DictWriter(handle, fieldnames=SIMPLE_COLUMNS, lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow({"grupo": row["grupo"], "nome": row["nome"], "url": endpoint_url(row, arguments.scheme, arguments.path)})
    print(f"{len(rows)} endpoints written to {arguments.out}")
    return 0


def command_import(arguments: argparse.Namespace) -> int:
    rows = filtered_rows(arguments)
    api = None if arguments.dry_run else Administration(arguments)
    existing = {item.get("key", "") for item in api.list_endpoints()} if api else set()
    print(f"{len(rows)} endpoints in {arguments.csv}" + (" (dry run)" if api is None else f" -> {arguments.url}"))
    created = skipped = failed = 0
    for row in rows:
        definition = build_definition(row, arguments)
        key = endpoint_key(str(definition["group"]), str(definition["name"]))
        if key in existing:
            skipped += 1
            if arguments.verbose:
                print(f"  = {key}: already registered")
            continue
        if api is None:
            created += 1
            print(f"  + {key} -> {definition['url']}")
            continue
        status, answer = api.create_endpoint(definition)
        if status == 201:
            created += 1
            if arguments.verbose:
                print(f"  + {key} -> {definition['url']}")
        elif status == 409:
            skipped += 1
            print(f"  = {key}: {describe(answer)}")
        else:
            failed += 1
            print(f"  ! {key}: {status} {describe(answer)}", file=sys.stderr)
    print(f"{created} created, {skipped} already there, {failed} refused")
    return 1 if failed else 0


def command_groups(arguments: argparse.Namespace) -> int:
    counts: dict[str, dict[str, int]] = {}
    for item in Administration(arguments).list_endpoints():
        group = item.get("group") or ""
        entry = counts.setdefault(group, {"total": 0, "config": 0})
        entry["total"] += 1
        if item.get("source") == SOURCE_CONFIG:
            entry["config"] += 1
    if not counts:
        print("no endpoint registered")
        return 0
    width = max(len(group or "(without group)") for group in counts)
    for group, entry in sorted(counts.items(), key=lambda pair: (-pair[1]["total"], pair[0])):
        line = f"{(group or '(without group)'):<{width}}  {entry['total']:4d}"
        if entry["config"]:
            line += f"   ({entry['config']} of the configuration file, read-only)"
        print(line)
    print(f"{len(counts)} groups, {sum(entry['total'] for entry in counts.values())} endpoints")
    return 0


def print_table(headers: list[str], rows: list[list[str]], csv_path: str | None) -> None:
    """Prints the rows in aligned columns, and writes them as a CSV when a path is given"""
    if csv_path:
        with open(csv_path, "w", newline="", encoding="utf-8") as handle:
            writer = csv.writer(handle, lineterminator="\n")
            writer.writerow(headers)
            writer.writerows(rows)
        print(f"{len(rows)} lines written to {csv_path}")
        return
    widths = [len(header) for header in headers]
    for row in rows:
        for index, cell in enumerate(row):
            widths[index] = max(widths[index], len(cell))
    # The last column is not padded, so a long URL does not drag the line
    print("  ".join(header.upper().ljust(widths[index]) for index, header in enumerate(headers)).rstrip())
    for row in rows:
        print("  ".join(cell.ljust(widths[index]) for index, cell in enumerate(row)).rstrip())


def command_endpoints(arguments: argparse.Namespace) -> int:
    """Lists the endpoints of the configuration file and the ones managed through the web"""
    items = Administration(arguments).list_endpoints()
    if arguments.group:
        wanted = {slugify(group) for group in arguments.group}
        items = [item for item in items if slugify(item.get("group") or "") in wanted]
    if arguments.source:
        items = [item for item in items if item.get("source") == arguments.source]
    if not items:
        print("no endpoint found")
        return 0
    rows = [
        [
            item.get("key", ""),
            item.get("type") or "",
            "file" if item.get("source") == SOURCE_CONFIG else "web",
            "enabled" if item.get("enabled") else "disabled",
            "yes" if item.get("acceptsPush") else "",
            item.get("url") or "",
        ]
        for item in items
    ]
    print_table(["key", "type", "source", "state", "push", "url"], rows, arguments.csv)
    if not arguments.csv:
        from_file = sum(1 for item in items if item.get("source") == SOURCE_CONFIG)
        disabled = sum(1 for item in items if not item.get("enabled"))
        accepting_push = sum(1 for item in items if item.get("acceptsPush"))
        print(f"{len(items)} endpoints · {from_file} of the configuration file · {disabled} disabled · {accepting_push} accepting push")
    return 0


def command_status_pages(arguments: argparse.Namespace) -> int:
    """Lists the status pages of the configuration file and the ones managed through the web"""
    listing = Administration(arguments).list_status_pages()
    pages = listing.get("statusPages") or []
    if not pages:
        print("no status page registered")
        return 0
    rows = []
    for page in pages:
        state = "published" if page.get("published") else ("enabled" if page.get("enabled") else "disabled")
        if page.get("conflict"):
            state = "conflict"
        elif page.get("error"):
            state = "invalid"
        rows.append([
            page.get("slug", ""),
            page.get("title") or "",
            "file" if page.get("origin") == "config" else "web",
            state,
            str(page.get("endpoints", 0)),
            "yes" if page.get("requiresLogin") else "",
            page.get("path") or "",
        ])
    print_table(["slug", "title", "source", "state", "endpoints", "login", "path"], rows, arguments.csv)
    if not arguments.csv:
        published = sum(1 for page in pages if page.get("published"))
        with_login = sum(1 for page in pages if page.get("requiresLogin"))
        print(f"{len(pages)} status pages · {published} published · {with_login} with a login of their own")
        if not listing.get("publicationEnabled"):
            print("status-pages.enabled is false: no page is published", file=sys.stderr)
        if listing.get("managedUnavailable"):
            print("the managed status pages could not be loaded: only the ones of the configuration file are listed", file=sys.stderr)
        if warning := listing.get("sharedRateLimitWarning"):
            print(f"every visitor seems to come from {warning}: set status-pages.trusted-proxies", file=sys.stderr)
    return 0


def command_rename_group(arguments: argparse.Namespace) -> int:
    """Changes the group of every endpoint of a group. The new group is written as it is typed, because the group is a
    label: only the key of the endpoint is normalized. When the key changes, this is the rename of the administration,
    which keeps the history and the push token; when only the case or the accents of the label change, the key stays
    the same and it is an ordinary update."""
    api = Administration(arguments)
    source, target = arguments.source.strip(), arguments.target.strip()
    if not target:
        raise ManagerError("the new group is empty")
    if not slugify(target):
        raise ManagerError(f"the new group has no letter or digit: {arguments.target!r}")
    # The group is matched by the label or by its slug, so --from tradimus finds the group written Tradimus
    selected = [
        item
        for item in api.list_endpoints()
        if (item.get("group") or "") == source or slugify(item.get("group") or "") == slugify(source)
    ]
    if not selected:
        raise ManagerError(f"no endpoint is in the group {source}")
    if all((item.get("group") or "") == target for item in selected):
        raise ManagerError("every endpoint of the group already has this exact name")
    from_config = [item for item in selected if item.get("source") == SOURCE_CONFIG]
    managed = [item for item in selected if item.get("source") != SOURCE_CONFIG]
    note = "" if slugify(source) != slugify(target) else ", keys unchanged"
    print(f"{len(selected)} endpoints in {source} -> {target}{note}" + (" (dry run)" if arguments.dry_run else ""))
    for item in from_config:
        print(f"  = {item['key']}: of the configuration file, change it in the YAML", file=sys.stderr)
    renamed = failed = 0
    for item in managed:
        key = item["key"]
        new_key = endpoint_key(target, item.get("name") or "")
        if arguments.dry_run:
            renamed += 1
            print(f"  ~ {key} -> {new_key}")
            continue
        detail = api.get_endpoint(key)
        definition = (detail.get("definition") or {}).get("json")
        version = detail.get("version")
        if not isinstance(definition, dict) or not isinstance(version, int):
            failed += 1
            print(f"  ! {key}: the stored definition could not be read", file=sys.stderr)
            continue
        # The masked secrets go back as they came: a value equal to the mask keeps the stored one
        definition["group"] = target
        status, answer = api.update_endpoint(key, definition, version)
        if status == 200:
            renamed += 1
            if arguments.verbose:
                print(f"  ~ {key} -> {new_key}")
            for page in (answer or {}).get("affectedConfigStatusPages", []) if isinstance(answer, dict) else []:
                print(f"    ! the status page {page.get('slug')} of the configuration file still selects {key}", file=sys.stderr)
        else:
            failed += 1
            print(f"  ! {key}: {status} {describe(answer)}", file=sys.stderr)
    print(f"{renamed} renamed, {len(from_config)} of the configuration file untouched, {failed} refused")
    return 1 if failed else 0


def command_export_tokens(arguments: argparse.Namespace) -> int:
    """Writes host,token with the push token of every endpoint that accepts push"""
    api = Administration(arguments)
    exported = []
    for item in api.list_endpoints():
        if not item.get("acceptsPush"):
            continue
        token = api.get_endpoint(item["key"]).get("pushToken") or ""
        if not token:
            # The endpoint accepts push through the global keys only
            continue
        host = urllib.parse.urlparse(item.get("url") or "").hostname or item.get("name") or item["key"]
        exported.append((host, token))
    exported.sort()
    with open(arguments.out, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(["host", "token"])
        writer.writerows(exported)
    print(f"{len(exported)} endpoints exported to {arguments.out}")
    return 0


def filtered_rows(arguments: argparse.Namespace) -> list[dict[str, str]]:
    rows = read_rows(arguments.csv, "" if arguments.every_environment else arguments.environment, arguments.every_status)
    if arguments.group:
        wanted = {slugify(group) for group in arguments.group}
        rows = [row for row in rows if slugify(row["grupo"]) in wanted]
    if arguments.limit:
        rows = rows[: arguments.limit]
    if not rows:
        raise ManagerError("no endpoint left after the filters")
    return rows


def add_connection_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--url", "--gatus-url", dest="url", default=environment("URL", "http://127.0.0.1:8080"), help="address of Go Uptime (default: %(default)s)")
    parser.add_argument("--username", default=environment("USERNAME", "admin"), help="user of the administration (default: %(default)s)")
    parser.add_argument("--password", default=environment("PASSWORD"), help="password of the administration (or GO_UPTIME_PASSWORD)")
    parser.add_argument("--timeout", type=float, default=30.0, help="timeout of each request in seconds (default: %(default)s)")
    parser.add_argument("--insecure", action="store_true", help="does not verify the TLS certificate of Go Uptime")
    parser.add_argument("--verbose", action="store_true", help="prints one line per endpoint")


def add_csv_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--csv", required=True, help="CSV with the endpoints (inventory of hostnames or the simple format)")
    parser.add_argument("--environment", default="Produção", help="environment kept from the inventory, by the start of the column (default: %(default)s)")
    parser.add_argument("--every-environment", action="store_true", help="keeps every environment of the inventory")
    parser.add_argument("--every-status", action="store_true", help="keeps the rows whose status is not OK")
    parser.add_argument("--group", action="append", metavar="GROUP", help="only these groups, by the slug of the service (can be repeated)")
    parser.add_argument("--scheme", default="https", choices=["https", "http"], help="scheme of the URL built from the hostname (default: %(default)s)")
    parser.add_argument("--path", default="", help="path added to the URL built from the hostname, e.g. /health")
    parser.add_argument("--limit", type=int, metavar="N", help="keeps at most N endpoints")


def parse_arguments(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Manages the endpoints of Go Uptime: imports them from a CSV, renames groups and exports the push tokens",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    subcommands = parser.add_subparsers(dest="command", required=True)

    csv_parser = subcommands.add_parser("csv", help="rewrites an inventory of hostnames as the simple CSV, without calling Go Uptime")
    add_csv_arguments(csv_parser)
    csv_parser.add_argument("--out", required=True, metavar="PATH", help="CSV to write")
    csv_parser.set_defaults(handler=command_csv)

    import_parser = subcommands.add_parser("import", help="registers the endpoints of a CSV, with push and a token of its own")
    add_csv_arguments(import_parser)
    add_connection_arguments(import_parser)
    import_parser.add_argument("--interval", default="5m", help="interval between the checks (default: %(default)s)")
    import_parser.add_argument("--condition", action="append", metavar="CONDITION", help="condition of the endpoint (can be repeated; default: [STATUS] == 200)")
    import_parser.add_argument("--no-push", action="store_true", help="registers without push: no token is generated")
    import_parser.add_argument("--dry-run", action="store_true", help="shows what would be registered, without calling Go Uptime")
    import_parser.set_defaults(handler=command_import)

    endpoints_parser = subcommands.add_parser("endpoints", help="lists the endpoints, with their source, state and push")
    add_connection_arguments(endpoints_parser)
    endpoints_parser.add_argument("--group", action="append", metavar="GROUP", help="only these groups (can be repeated)")
    endpoints_parser.add_argument("--source", choices=["config", "admin"], help="only the endpoints of the configuration file or the ones managed through the web")
    endpoints_parser.add_argument("--csv", metavar="PATH", help="writes the listing as a CSV instead of printing it")
    endpoints_parser.set_defaults(handler=command_endpoints)

    status_pages_parser = subcommands.add_parser("status-pages", help="lists the status pages, with their state and whether they require a login")
    add_connection_arguments(status_pages_parser)
    status_pages_parser.add_argument("--csv", metavar="PATH", help="writes the listing as a CSV instead of printing it")
    status_pages_parser.set_defaults(handler=command_status_pages)

    groups_parser = subcommands.add_parser("groups", help="lists the groups with the number of endpoints of each one")
    add_connection_arguments(groups_parser)
    groups_parser.set_defaults(handler=command_groups)

    rename_parser = subcommands.add_parser("rename-group", help="changes the group of every endpoint of a group")
    add_connection_arguments(rename_parser)
    rename_parser.add_argument("--from", dest="source", required=True, metavar="GROUP", help="current group")
    rename_parser.add_argument("--to", dest="target", required=True, metavar="GROUP", help="new group")
    rename_parser.add_argument("--dry-run", action="store_true", help="shows what would be renamed, without changing anything")
    rename_parser.set_defaults(handler=command_rename_group)

    export_parser = subcommands.add_parser("export-tokens", help="writes host,token of every endpoint that accepts push")
    add_connection_arguments(export_parser)
    export_parser.add_argument("--out", required=True, metavar="PATH", help="CSV to write")
    export_parser.set_defaults(handler=command_export_tokens)

    arguments = parser.parse_args(argv)
    if getattr(arguments, "condition", None) is None and arguments.command == "import":
        arguments.condition = ["[STATUS] == 200"]
    return arguments


def main(argv: list[str]) -> int:
    arguments = parse_arguments(argv)
    return arguments.handler(arguments)


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except ManagerError as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        sys.exit(130)
