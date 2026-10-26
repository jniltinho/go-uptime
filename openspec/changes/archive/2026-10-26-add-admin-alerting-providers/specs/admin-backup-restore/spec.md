## MODIFIED Requirements

### Requirement: Conteúdo e formato do backup
The backup MUST contain the complete, unmasked definitions of what was registered from the web:
- the managed endpoints: active and Push, enabled or not, including those in conflict with the configuration file;
- the managed status pages;
- the global push keys created from the web, with name, lowercase hexadecimal hash, hint and, as information only, creation time and author;
- the managed alerting providers, including those shadowed by the file, with the complete, unmasked definition.

**What is left out:** the backup MUST NOT contain history, triggered alerts, login sessions nor anything registered in the configuration file.

**Format:** JSON with `format: "go-uptime-admin-backup"`, `version: 1`, `createdAt`, `createdBy`, an optional `appVersion` and the lists `endpoints`, `statusPages`, `pushKeys` and `alertProviders`, sorted. The `alertProviders` list MUST be optional when reading, so that a backup made before it stays valid, and MUST always be written.

**Reading:** the system MUST refuse with 400 a file with:
- an unknown `format` or a version greater than the supported one;
- unknown keys;
- missing lists (except `alertProviders`);
- duplicated items (key, slug, name, hash or provider type).

**Limits:** the plaintext MUST have at most 2 MiB, 1,000 endpoints, 200 status pages, 500 keys and one provider per alert type. The backup MUST answer 422 when the registered items exceed these limits.

**Files of v6:** reading MUST also accept `format: "gatus-admin-backup"` and the envelope `format: "gatus-admin-backup-encrypted"`, and the `gatusVersion` field as a synonym of `appVersion`, so that a backup made before the rename is restored. The header of an encrypted file MUST be authenticated as it was read, with the file's own `format`. Writing MUST always use the new format.

#### Scenario: Backup com os três tipos
- **WHEN** the web has the endpoints `jobs_backup` (Push) and `web_site` (disabled), the status page `jobs` and the key `akamai`
- **THEN** the file contains both endpoints with their complete definitions (including the token and `enabled: false`), the page `jobs` and the key `akamai` with only its hash and hint

#### Scenario: Cadastros do YAML ficam de fora
- **WHEN** the configuration file has the endpoint `core_api` and the page `services`
- **THEN** the backup contains neither `core_api` nor `services`

#### Scenario: Arquivo de outro formato
- **WHEN** the administrator submits a file with `format: "other"`, `version: 2` or an unknown key
- **THEN** the API answers 400 without changing anything

#### Scenario: Cadastros acima do limite
- **WHEN** the web has 1,001 managed endpoints
- **THEN** the backup answers 422

#### Scenario: Backup with an alerting provider
- **WHEN** the web has the `telegram` provider
- **THEN** the file contains `telegram` with its token in plaintext, and the encrypted file protects it like the tokens of the endpoints

#### Scenario: Backup made before the providers
- **WHEN** the administrator restores a backup without the `alertProviders` list
- **THEN** the file is accepted and no provider changes

### Requirement: Aplicação do restore
`POST /api/v1/admin/restore` MUST receive the body of the preview plus the `fingerprint`.

**Validation:**
- it MUST compute the plan again and answer 409 without applying anything when the fingerprint differs;
- the fingerprint MUST depend on the content of the file, the options, the configuration generation and the current versions and definitions of the items, **and of every managed alerting provider**, even those absent from the backup, because the endpoints of the plan are validated against them;
- the plan MUST validate the endpoints against a simulated alerting configuration: the file's providers, the current managed ones, and the providers the plan will create or update, respecting the overwrite option and the precedence of the file. The preview MUST list, for each provider it updates, the managed endpoints outside the backup that will be restarted.

**Applying:**
- alerting providers MUST be applied after the push keys and before the endpoints, so that a restored endpoint finds the provider it uses; a provider whose type is configured in the destination's file MUST be `skipped` with the reason;
- it MUST apply the `create` and `update` items in the order of the plan, using the same normalized definition shown in the preview (including with `disableEndpoints`), with the rules and effects of creating and updating from the web (monitoring, publishing after the commit, audit) and the author of the restore, without generating tokens;
- it MUST NOT delete items nor change keys or slugs;
- the hash check of a restored key MUST be serialized with the writes of managed endpoints.

**Result:**
- the response MUST carry the result of each item (`created`, `updated`, `unchanged`, `skipped` or `failed`, with the message) and the selection warnings of the applied pages;
- the failure of one item MUST NOT interrupt the others;
- an item changed between the check and the application MUST be `failed`;
- when a configuration reload starts during the application, the remaining items MUST be `skipped` with "configuration reload in progress".

**After applying:**
- the status cache of the endpoints MUST be cleared;
- the log MUST record the summary with the author;
- a restored push key MUST accept pushes with the original token.

#### Scenario: Restore em outra instalação
- **WHEN** the backup of an installation with SQLite is restored on a new installation with MariaDB
- **THEN** the endpoints are monitored, the page is published and the scripts keep sending pushes with the original global key

#### Scenario: Mudança depois da prévia
- **WHEN** another administrator changes `web_site` between the preview and the application
- **THEN** the application answers 409 and nothing is applied

#### Scenario: Outro arquivo com os mesmos itens
- **WHEN** the application sends a file with the same ids as the preview and different definitions
- **THEN** the application answers 409

#### Scenario: Restore repetido
- **WHEN** the same file is restored twice
- **THEN** the second time every item applied before shows as `unchanged`

#### Scenario: Recarga no meio
- **WHEN** a configuration reload starts after the third item
- **THEN** the following items are `skipped` with "configuration reload in progress" and a new restore after the reload applies what was left

#### Scenario: Endpoint and provider in the same restore
- **WHEN** the backup has the `ntfy` provider and the endpoint `core_api` with an `ntfy` alert, and the destination has no `ntfy`
- **THEN** the provider is created first and `core_api` is created with its alert
