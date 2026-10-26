## Why

[Resend](https://resend.com) is an email API that the alerting guide of the original Gatus lists (`alerting.resend`: `api-key`, `from`, `to`, group overrides), but Go Uptime does not have it: today the only way to send alerts by email through an API is SendGrid or AWS SES. Teams that already send their transactional email with Resend need a provider of their own instead of an SMTP relay.

## What Changes

- New alerting provider `resend`, with `api-key`, `from`, `to` (up to 50 recipients separated by commas) and `client`, group overrides, a default alert and the provider-override of an alert, like the other email providers.
- Alerts are sent with `POST https://api.resend.com/emails`, authenticated with the API key as a bearer token, with a plain text and an HTML body, and the subject "Alert triggered" or "Alert resolved".
- Configurable in the configuration file and through the Notifications tab of the administration: the API key is classified as secret (masked, redacted from errors), and the form gets the fields and their help from the catalog of the providers. The number of types goes from 41 to 42.
- Documented in the README, like every provider.

Nothing is **BREAKING**: a configuration without `resend` behaves as before.

## Capabilities

### New Capabilities
- `resend-alerting-provider`: the configuration, the validation and the delivery of the alerts of the Resend provider.

### Modified Capabilities
- `admin-alert-providers`: the complete list of the administration has every alert type, Resend included, instead of a fixed count of 41.

## Impact

- New package `internal/alerting/provider/resend` with its tests (HTTP mocked through `client.InjectHTTPClient`).
- The six places of "Adding an Alerting Provider" of `AGENTS.md`: `alert.TypeResend`, the field of `alerting.Config`, the compile-time checks of `provider.go`, the `alertTypes` of `ValidateAlertingConfig`, and the README.
- `internal/alertprovider`: the classification of the fields (`secrets.go`) and the catalog (`docs.json`); the reflection tests require both.
- Docs that say "41 types"; the API test that counts the types.
