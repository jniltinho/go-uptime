## MODIFIED Requirements

### Requirement: Administration API of the providers
Under `/api/v1/admin`, with the same authorization, CSRF, body limit and audit rules as managed endpoints, the system MUST offer:
- `GET /alert-providers`: every alert type of `alerting.Config`, whatever their number, each with its origin (`yaml`, `web`, `shadowed`, `none`), validity, version and the number of endpoints (of the file and managed) with an alert of that type;
- `GET /alert-providers/{type}`: the masked definition, in JSON and in YAML, with an `ETag` carrying the version;
- `POST /alert-providers` and `PUT /alert-providers/{type}`;
- `DELETE /alert-providers/{type}`;

`PUT` and `DELETE` MUST require `If-Match` with the version, like managed endpoints: 428 without it, 412 when malformed or stale. When the managed providers cannot be loaded, the list MUST say so (`managedUnavailable`) instead of looking empty.
- `POST /alert-providers/validate`: effective and storable definitions, without storing.

An unknown type MUST answer 404; updating or deleting a type of the file MUST answer 409.

#### Scenario: Concurrency
- **WHEN** two administrators open `email` at version 2 and the first one saves
- **THEN** the second one gets 412 when saving with `If-Match: "2"`

#### Scenario: Complete list
- **WHEN** the file configures `slack` and the web configures `ntfy`
- **THEN** the list has every alert type (42 with `resend`), `slack` as `yaml`, `ntfy` as `web` and the others as `none`
