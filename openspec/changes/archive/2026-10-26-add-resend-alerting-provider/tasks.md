## 1. Provider

- [x] 1.1 Package `internal/alerting/provider/resend`: `Config` (`Validate` with at most 50 recipients, `Merge`), `AlertProvider` (`Validate`, `Send`, `GetDefaultAlert`, `GetConfig`, `ValidateOverrides`), payload and message
- [x] 1.2 Tests of the package with the HTTP client mocked: validation, overrides, request (URL, bearer token, JSON body), subject and body, refused delivery

## 2. Registration (the six places of AGENTS.md)

- [x] 2.1 `alert.TypeResend` in `internal/alerting/alert/type.go`
- [x] 2.2 Field `resend` of `alerting.Config`
- [x] 2.3 Compile-time checks of `AlertProvider` and `Config` in `internal/alerting/provider/provider.go`
- [x] 2.4 `alertTypes` of `ValidateAlertingConfig`
- [x] 2.5 README, in alphabetical order

## 3. Administration

- [x] 3.1 Classification of the fields in `internal/alertprovider/secrets.go` (`api-key` secret, `from` and `to` public, client section)
- [x] 3.2 Catalog entry in `internal/alertprovider/docs.json` (Email category, help of the guide in English)
- [x] 3.3 API test counts `alertprovider.Types()` instead of 41; docs that say "41 types"

## 4. Verification

- [x] 4.1 `make lint`, `go test ./... -race`, `npm run test:unit`
- [x] 4.2 The form of Resend in the Notifications tab, through the browser
