## ADDED Requirements

### Requirement: Configuration of the Resend provider
The system MUST accept an alerting provider of type `resend` with the keys `api-key`, `from`, `to` and `client`, plus `default-alert` and `overrides` by group like the other providers. `api-key`, `from` and `to` MUST be required, and a value made only of spaces MUST count as empty; `to` MUST hold one to 50 addresses separated by commas; `from` and every address of `to` MUST be valid email addresses, plain or with a name (`Name <user@example.com>`). Each group override MUST be validated merged over the default configuration, so that an override with too many recipients or an invalid address is refused when the configuration is loaded. Two overrides with the same group, or an override without group, MUST be refused.

#### Scenario: Valid provider
- **WHEN** the configuration has `resend` with `api-key: re_123`, `from: "Go Uptime <alerts@example.com>"` and `to: "ops@example.com,dev@example.com"`
- **THEN** the configuration is valid and alerts of type `resend` are sent through it

#### Scenario: Missing API key
- **WHEN** the configuration has `resend` without `api-key`
- **THEN** the provider is invalid with the error `api-key not set`

#### Scenario: Invalid address
- **WHEN** `to` is `ops@example.com, invalid`
- **THEN** the provider is invalid with an error naming `invalid`

#### Scenario: Override with too many recipients
- **WHEN** the default configuration is valid and the override of the group `core` has 51 addresses in `to`
- **THEN** the provider is invalid with an error naming the group `core`

#### Scenario: Too many recipients
- **WHEN** `to` has 51 addresses
- **THEN** the provider is invalid with an error saying that Resend accepts at most 50 recipients

### Requirement: Delivery through the Resend API
An alert MUST be sent with `POST https://api.resend.com/emails`, the header `Authorization: Bearer <api-key>`, and a JSON body with `from`, `to` as a list of the trimmed addresses, `subject`, `text` and `html`. The `html` MUST be the text with `<`, `>`, `&` and quotes escaped and the line breaks as `<br>`, so that the content of an alert never becomes markup. The subject MUST be `[<endpoint>] Alert triggered` or `[<endpoint>] Alert resolved`, and the body MUST carry the description of the alert, the extra labels and the results of the conditions, like the other email providers. A response with a status of 400 or above MUST be returned as an error with the status and the body of the response.

#### Scenario: Triggered alert
- **WHEN** an alert of the endpoint `api` of the group `core` triggers
- **THEN** Resend receives a request with the bearer API key and the subject `[core/api] Alert triggered`

#### Scenario: Group override
- **WHEN** the provider has an override for the group `core` with `to: core@example.com` and an endpoint of `core` alerts
- **THEN** the email goes to `core@example.com` only

#### Scenario: Content with markup
- **WHEN** the description of the alert is `latency <500ms> & errors`
- **THEN** the `text` carries it as it is and the `html` carries `latency &lt;500ms&gt; &amp; errors`

#### Scenario: Refused by Resend
- **WHEN** Resend answers 403 because the domain of `from` is not verified
- **THEN** the delivery fails with an error carrying the status 403 and the message of Resend

### Requirement: Resend in the administration
The `resend` type MUST be configurable through the Notifications tab like every other type: `api-key` MUST be secret (masked in every answer, edited as a password, redacted from errors), and the form MUST show `from` and `to` with their help, in the Email category.

#### Scenario: API key masked
- **WHEN** an administrator creates the `resend` provider through the web and opens it again
- **THEN** the API key is shown as configured and is never answered by the API
