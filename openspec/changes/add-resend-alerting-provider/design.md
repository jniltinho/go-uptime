## Context

Every alerting provider follows the same shape (`internal/alerting/provider/slack` is the reference of `AGENTS.md`): a `Config` with `Validate` and `Merge`, an `AlertProvider` with `DefaultConfig` inline, `DefaultAlert` and `Overrides`, and `GetConfig`, which merges the group override and the provider-override of the alert. SendGrid is the closest provider: an email API with an API key, a sender and comma-separated recipients. Since v7.1.0 a provider also has to be classified field by field for the administration (`internal/alertprovider/secrets.go`) and described in its catalog (`docs.json`); reflection tests fail otherwise.

## Goals / Non-Goals

**Goals:**
- A `resend` provider with the keys of the alerting guide of the original Gatus (`api-key`, `from`, `to`, `client`), configurable in the file and in the Notifications tab.

**Non-Goals:**
- Templates, attachments, `reply_to`, `cc`/`bcc`, scheduled sending or the idempotency key of the Resend API.
- Sharing code with SendGrid: each provider stays self-contained, as in the rest of the package.

## Decisions

### 1. Copy the SendGrid shape, with the payload of Resend
`Config{APIKey, From, To, ClientConfig}`, the same `Validate`/`Merge`/`GetConfig` and the same subject and body as SendGrid, so that an email reads the same whatever the API. The request is `POST https://api.resend.com/emails` with `Authorization: Bearer <api-key>` and `{"from", "to": [...], "subject", "text", "html"}`; the HTML is the text **escaped** with `html.EscapeString`, then with `<br>` for line breaks. SendGrid only replaces the line breaks, so an alert whose name, description or condition has `<`, `>` or `&` loses part of its text or gains markup; Resend does not copy that defect (fixing SendGrid is outside this change). The URL is a package variable, so the tests can check it, and the HTTP client goes through `client.GetHTTPClient(cfg.ClientConfig)`, which the tests replace with `client.InjectHTTPClient`.
*Alternative:* one generic "email API" provider with per-service payloads — rejected: it would change the configuration of SendGrid.

### 2. Everything that can be checked locally is checked by `Validate`
The Resend API refuses more than 50 addresses in `to`, an invalid address, and empty fields. `Validate` checks all of them (a value of only spaces counts as empty; addresses go through `net/mail.ParseAddress`, which accepts `Name <user@example.com>`), and `AlertProvider.Validate` also validates **each group override merged over the default configuration** — the administration already did, the configuration file did not. That turns a delivery failure at the first alert into a configuration error at startup, in `config validate` and in the form of the administration.

### 3. Classification and catalog
`api-key` is secret; `from` and `to` are public, as for SendGrid; the `client` section gets the shared classification of `typesWithClient`. The catalog places Resend in the Email category with the help of the guide, in English.

## Risks / Trade-offs

- [The domain of `from` must be verified in Resend, and an unverified one fails every delivery] → the help of `from` says so, and the test notification of the administration shows the 403 of Resend before any alert.
- [The API of Resend changes] → the payload only uses the documented required fields.
