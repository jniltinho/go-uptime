// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/admintest"
	"github.com/jniltinho/go-uptime/v7/internal/alertprovider"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/labstack/echo/v5"
)

// errAlertProviderVersionRequired is the error of a change of an alerting provider sent without If-Match, answered
// with 428
var errAlertProviderVersionRequired = errors.New("the If-Match header with the current version of the alerting provider is required")

// errAlertProviderInvalidVersion is the error of an If-Match that is not a positive integer, answered with 412
var errAlertProviderInvalidVersion = errors.New("invalid If-Match header")

// registerAdminAlertProviderRoutes registers the routes of the administration of the alerting providers (fork). The
// router must already require authentication, administrator permission and request protection.
func registerAdminAlertProviderRoutes(router httpx.Router, securityConfig *security.Config) {
	// GET and HEAD /api/v1/admin/alert-providers: lists every alert type with its provider, if any.
	//
	// Authentication: administrator (protected group, administrator permission).
	// Responses: 200 with alertprovider.Listing (the providers, configured or not, with their source, state and usage,
	// and the description of every type for the forms); the common responses of the administration (401, 403, 429).
	httpx.GetAndHead(router, "/alert-providers", func(c *echo.Context) error {
		return httpx.JSON(c, http.StatusOK, alertprovider.List())
	})
	// POST /api/v1/admin/alert-providers/parse: decodes the YAML or JSON definition in the body and returns it as a JSON
	// document with the YAML keys, without validating the provider, reading stored data or masking anything: it only
	// echoes what the administrator sent. The form of the administration uses it to switch from the YAML to the form.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: body with the definition, in YAML or JSON, with a matching Content-Type.
	// Responses: 200 with {"json": <document>}; 400 when the body is empty, is not a single mapping or cannot be decoded;
	// the common responses of the administration (401, 403, 413, 415, 429).
	router.POST("/alert-providers/parse", func(c *echo.Context) error {
		document, err := alertprovider.Parse(httpx.Body(c))
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, map[string]any{"json": document})
	})
	// POST /api/v1/admin/alert-providers/validate: validates the definition in the body without storing it.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: body with the definition, in YAML or JSON: the type key and the keys of the type's block in the alerting
	// section. The optional query parameter type is the type of the managed provider being edited: masked secrets then
	// stand for its stored values, and the type key of the body can be omitted.
	// Responses: 200 with alertprovider.Validation (effective and storable definitions, secrets masked); 400 when the
	// definition is empty, invalid, of an unknown type or of another type than the query parameter, uses a field that
	// is not allowed, or has a masked secret without stored value; 500 on an unexpected error; the common responses of
	// the administration (401, 403, 413, 415, 429).
	router.POST("/alert-providers/validate", func(c *echo.Context) error {
		validation, err := alertprovider.Validate(httpx.Body(c), httpx.Query(c, "type"))
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, validation)
	})
	// POST /api/v1/admin/alert-providers/test: sends a test notification through the definition in the body, which is
	// not stored. No alert state changes.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: body with the definition, as in validate; optional query parameters type (as in validate), group, the
	// group of the synthetic endpoint, which selects the group override of the provider, and resolve, "true" to send the
	// resolution right after an accepted notification (closing the incident a provider such as PagerDuty opens).
	// Responses: 200 with alertprovider.TestResult, whether the provider accepted the notification or not, at most 15
	// seconds after the request (the notification may still be delivered later); 400 as in validate; 429 when too many
	// tests of the administration are in progress; 500 on an unexpected error; the common responses of the
	// administration (401, 403, 413, 415, 429).
	router.POST("/alert-providers/test", func(c *echo.Context) error {
		if len(strings.TrimSpace(string(httpx.Body(c)))) == 0 {
			return adminAlertProviderError(c, alertprovider.ErrEmptyDefinition)
		}
		result, err := alertprovider.Test(httpx.Body(c), httpx.Query(c, "type"), httpx.Query(c, "group"), httpx.Query(c, "resolve") == "true")
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, result)
	})
	// POST /api/v1/admin/alert-providers: creates the managed provider of the type of the definition in the body.
	//
	// Authentication: administrator (protected group, administrator permission, request protection). The author of the
	// change is the authenticated user.
	// Request: body with the definition, as in validate; If-Match is not used.
	// Responses: 201 with alertprovider.WriteResult (the detail, the restarted endpoints and the warnings) and an ETag
	// with the version; 400 as in validate; 409 when the type is already configured, in the configuration file or
	// through the administration, or when a managed endpoint using the type would become invalid (the body then has
	// "endpoints", the errors by key); 501 when the storage does not support managed providers; 503 while a start or a
	// configuration reload is in progress; 500 on an unexpected error; the common responses of the administration (401,
	// 403, 413, 415, 429).
	router.POST("/alert-providers", func(c *echo.Context) error {
		result, err := alertprovider.Create(httpx.Body(c), securityConfig.RequestAuthor(c))
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return writeAlertProviderResult(c, http.StatusCreated, result)
	})
	// GET and HEAD /api/v1/admin/alert-providers/:type: returns an alert type with its provider, if any, and its
	// definitions, secrets masked.
	//
	// Authentication: administrator (protected group, administrator permission).
	// Request: the path parameter type is the alert type, e.g. "slack".
	// Responses: 200 with alertprovider.Detail and, for a managed provider, an ETag with its version in quotes, to be
	// sent back in If-Match; 404 when type is not an alert type; 503 before the first load; the common responses of the
	// administration (401, 403, 429).
	httpx.GetAndHead(router, "/alert-providers/:type", func(c *echo.Context) error {
		detail, err := alertprovider.Get(c.Param("type"))
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		if detail.Version > 0 {
			httpx.SetHeader(c, "ETag", fmt.Sprintf(`"%d"`, detail.Version))
		}
		return httpx.JSON(c, http.StatusOK, detail)
	})
	// PUT /api/v1/admin/alert-providers/:type: replaces the definition of a managed provider. The managed endpoints with
	// an alert of the type are restarted with the new definition, keeping their triggered alerts.
	//
	// Authentication: administrator (protected group, administrator permission, request protection). The author of the
	// change is the authenticated user.
	// Request: the path parameter type is the alert type; the If-Match header is required and carries the current
	// version, as "3" or W/"3" (the ETag of get); the body is the new definition, as in validate, whose masked secrets
	// keep their stored values.
	// Responses: 200 with alertprovider.WriteResult and an ETag with the new version; 400 as in validate; 404 when no
	// managed provider has the type; 409 when the configuration file declares the type, or when a managed endpoint using
	// it would become invalid (with "endpoints"); 412 when If-Match is not a positive integer or not the current version;
	// 428 without If-Match; 501, 503 and 500 as in create; the common responses of the administration (401, 403, 413,
	// 415, 429).
	router.PUT("/alert-providers/:type", func(c *echo.Context) error {
		expectedVersion, err := alertProviderExpectedVersion(c)
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		result, err := alertprovider.Update(c.Param("type"), httpx.Body(c), expectedVersion, securityConfig.RequestAuthor(c))
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return writeAlertProviderResult(c, http.StatusOK, result)
	})
	// DELETE /api/v1/admin/alert-providers/:type: deletes a managed provider, also when it is shadowed by the
	// configuration file.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: the path parameter type is the alert type; the If-Match header is required, as in update.
	// Responses: 200 with {"type": "<type>"}; 404 when no managed provider has the type; 409 when a managed endpoint
	// has an alert of the type (the body then has "endpoints", their keys); 412 and 428 as in update; 501, 503 and 500 as
	// in create; the common responses of the administration (401, 403, 413, 415, 429).
	router.DELETE("/alert-providers/:type", func(c *echo.Context) error {
		expectedVersion, err := alertProviderExpectedVersion(c)
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		if err := alertprovider.Delete(c.Param("type"), expectedVersion, securityConfig.RequestAuthor(c)); err != nil {
			return adminAlertProviderError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, map[string]any{"type": strings.ToLower(c.Param("type"))})
	})
	// POST /api/v1/admin/alert-providers/:type/test: sends a test notification through the provider of the type in use,
	// of the configuration file or managed. No alert state changes.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: the path parameter type is the alert type; the optional query parameters group and resolve, as in the
	// test of a definition; no body.
	// Responses: 200 with alertprovider.TestResult, as in the test of a definition; 404 when type is not an alert type;
	// 409 when the type has no valid provider; 429 when too many tests of the administration are in progress; 500 on an
	// unexpected error; the common responses of the administration (401, 403, 413, 415, 429).
	router.POST("/alert-providers/:type/test", func(c *echo.Context) error {
		result, err := alertprovider.Test(nil, c.Param("type"), httpx.Query(c, "group"), httpx.Query(c, "resolve") == "true")
		if err != nil {
			return adminAlertProviderError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, result)
	})
}

// writeAlertProviderResult answers the result of a write with the given status and the ETag of the new version
func writeAlertProviderResult(c *echo.Context, status int, result *alertprovider.WriteResult) error {
	if result.Version > 0 {
		httpx.SetHeader(c, "ETag", fmt.Sprintf(`"%d"`, result.Version))
	}
	return httpx.JSON(c, status, result)
}

// alertProviderExpectedVersion returns the version from the If-Match header, e.g. "3" or W/"3": a missing header is
// errAlertProviderVersionRequired (428) and a value that is not a positive integer errAlertProviderInvalidVersion (412)
func alertProviderExpectedVersion(c *echo.Context) (int64, error) {
	value := strings.TrimSpace(httpx.Header(c, "If-Match"))
	if len(value) == 0 {
		return 0, errAlertProviderVersionRequired
	}
	version, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(value, "W/"), `"`), 10, 64)
	if err != nil || version <= 0 {
		return 0, errAlertProviderInvalidVersion
	}
	return version, nil
}

// adminAlertProviderError maps the errors of the administration of the alerting providers to their HTTP status and
// answers {"error": "..."}: 400 invalid definition; 404 not found; 409 already configured, configured in the file, not
// configured, in use or breaking managed endpoints (with "endpoints"); 412 version mismatch or invalid If-Match; 428
// missing If-Match; 429 too many tests; 501 storage not supported; 503 start or reload in progress; 500 for anything
// else, logged and answered without its text.
func adminAlertProviderError(c *echo.Context, err error) error {
	body := map[string]any{"error": err.Error()}
	status := http.StatusInternalServerError
	var inUse *alertprovider.InUseError
	var dependents *alertprovider.DependentsError
	switch {
	case errors.As(err, &inUse):
		status, body["endpoints"] = http.StatusConflict, inUse.Keys
	case errors.As(err, &dependents):
		status, body["endpoints"] = http.StatusConflict, dependents.Errors
	case errors.Is(err, alertprovider.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, alertprovider.ErrAlreadyExists), errors.Is(err, alertprovider.ErrConfiguredInFile),
		errors.Is(err, alertprovider.ErrNotConfigured):
		status = http.StatusConflict
	case errors.Is(err, alertprovider.ErrVersionMismatch), errors.Is(err, errAlertProviderInvalidVersion):
		status = http.StatusPreconditionFailed
	case errors.Is(err, errAlertProviderVersionRequired):
		status = http.StatusPreconditionRequired
	case errors.Is(err, admintest.ErrTooManyTests):
		status = http.StatusTooManyRequests
	case errors.Is(err, alertprovider.ErrStorageNotSupported):
		status = http.StatusNotImplemented
	case errors.Is(err, alertprovider.ErrCycleInProgress):
		status = http.StatusServiceUnavailable
	case errors.Is(err, alertprovider.ErrEmptyDefinition), errors.Is(err, alertprovider.ErrInvalidDefinition),
		errors.Is(err, alertprovider.ErrUnknownType), errors.Is(err, alertprovider.ErrFieldNotAllowed),
		errors.Is(err, alertprovider.ErrMaskedSecretWithoutValue), errors.Is(err, alertprovider.ErrTypeMismatch):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		logr.Errorf("[api.adminAlertProviderError] %s", err.Error())
		body = map[string]any{"error": "internal error"}
	}
	return httpx.JSON(c, status, body)
}
