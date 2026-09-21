package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// errAdminVersionRequired is the error of a change sent without the If-Match header, answered with 428.
var errAdminVersionRequired = errors.New("the If-Match header with the current version of the endpoint is required")

// adminHandler holds the handlers of the administration of endpoints, under /api/v1/admin.
type adminHandler struct {
	service  *managedendpoint.Service
	security *security.Config
}

// registerAdminRoutes registers the routes of the administration of endpoints. The router must already require
// authentication, administrator permission and request protection.
//
// Handlers always set the status explicitly: the static file middleware registered before the protected routes sets
// 404 when no file matches.
//
// The router is the group /api/v1/admin of the protected group, only registered when the administration is enabled.
// Besides the responses of its own handler, every route registered here can answer, with the JSON body
// {"error": "..."}: 401 without a valid authentication (security middleware; with security.basic also 429 with
// Retry-After while the client is blocked by the failure limiter); 403 when the request is not from an administrator
// (security.Config.AdminMiddleware). For POST, PUT, PATCH and DELETE, adminRequestProtection adds: 403 when
// Sec-Fetch-Site is cross-site or when the Origin (or, without it, the origin of the Referer) is not allowed; 413 when
// the body exceeds 256 KiB (the restore routes check their own limit); 415 when there is a body and the Content-Type
// is not application/json, application/yaml, application/x-yaml or text/yaml. The handlers below call these "the
// common responses of the administration".
func registerAdminRoutes(router httpx.Router, cfg *config.Config) {
	handler := &adminHandler{service: managedendpoint.NewService(cfg), security: cfg.Security}
	httpx.GetAndHead(router, "/metadata", handler.metadata)
	httpx.GetAndHead(router, "/endpoints", handler.list)
	router.POST("/endpoints/parse", handler.parse)
	router.POST("/endpoints/validate", handler.validate)
	router.POST("/endpoints/test", handler.test)
	router.POST("/endpoints", handler.create)
	httpx.GetAndHead(router, "/endpoints/:key", handler.get)
	router.PUT("/endpoints/:key", handler.update)
	router.POST("/endpoints/:key/enable", handler.setEnabled(true))
	router.POST("/endpoints/:key/disable", handler.setEnabled(false))
	router.DELETE("/endpoints/:key", handler.delete)
	// Status pages (see api/admin_status_pages.go)
	registerAdminStatusPageRoutes(router, cfg.Security)
	// Global push keys (see api/admin_push_keys.go)
	registerAdminPushKeyRoutes(router, cfg.Security)
	// Backup and restore (fork, see api/admin_backup.go)
	registerAdminBackupRoutes(router, cfg)
}

// metadata handles GET and HEAD /api/v1/admin/metadata: it returns what the definition of a managed endpoint can use.
//
// Authentication: administrator (protected group, administrator permission).
// Responses: 200 with managedendpoint.Metadata (the configured alert types, the tunnels and the extra labels); the
// common responses of the administration (401, 403, 429).
func (h *adminHandler) metadata(c *echo.Context) error {
	return httpx.JSON(c, http.StatusOK, h.service.Metadata())
}

// list handles GET and HEAD /api/v1/admin/endpoints: it lists every endpoint, the ones of the configuration file
// (endpoints and external endpoints, read-only) and the managed ones, sorted by key.
//
// Authentication: administrator (protected group, administrator permission).
// Responses: 200 with an array of managedendpoint.Item; the common responses of the administration (401, 403, 429).
func (h *adminHandler) list(c *echo.Context) error {
	return httpx.JSON(c, http.StatusOK, h.service.List())
}

// get handles GET and HEAD /api/v1/admin/endpoints/:key: it returns an endpoint, managed or of the configuration file,
// with its definition (secrets masked) and its effective definition.
//
// Authentication: administrator (protected group, administrator permission).
// Request: the path parameter key is the key of the endpoint, unescaped once and lower-cased.
// Responses: 200 with managedendpoint.Detail and, for a managed endpoint, an ETag with its version in quotes, to be
// sent back in If-Match; 404 when no endpoint has the key; 500 on an unexpected error; the common responses of the
// administration (401, 403, 429).
func (h *adminHandler) get(c *echo.Context) error {
	detail, err := h.service.Get(adminKey(c))
	if err != nil {
		return adminServiceError(c, err)
	}
	return writeAdminDetail(c, http.StatusOK, detail)
}

// parse handles POST /api/v1/admin/endpoints/parse: it decodes the YAML or JSON definition in the body strictly and
// returns it as a JSON document with the YAML keys, without validating the endpoint, reading stored data or masking
// anything. The form of the administration uses it to convert a definition being edited.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: body with the definition of the endpoint, in YAML or JSON, with a matching Content-Type.
// Responses: 200 with {"json": <document>}; 400 when the definition is empty or cannot be decoded; 500 on an
// unexpected error; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminHandler) parse(c *echo.Context) error {
	document, err := h.service.ParseDefinition(httpx.Body(c))
	if err != nil {
		return adminServiceError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, map[string]any{"json": document})
}

// validate handles POST /api/v1/admin/endpoints/validate: it validates the definition in the body without persisting
// it.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: body with the definition of the endpoint, in YAML or JSON. The optional query parameter key, lower-cased,
// is the key of the managed endpoint being edited: its own key is then not a conflict and its masked secrets are
// restored from the stored definition.
// Responses: 200 with managedendpoint.Validation (the definition and the effective definition, secrets masked); 400
// when the definition is empty or invalid, uses a field, an alert provider, a provider override or an extra label that
// is not allowed; 409 when the key or the push token is already in use; 500 on an unexpected error; the common
// responses of the administration (401, 403, 413, 415, 429).
func (h *adminHandler) validate(c *echo.Context) error {
	validation, err := h.service.Validate(httpx.Body(c), strings.ToLower(httpx.Query(c, "key")))
	if err != nil {
		return adminServiceError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, validation)
}

// test handles POST /api/v1/admin/endpoints/test: it validates the definition in the body and evaluates it once,
// without persisting the result, sending alerts or publishing metrics. The timeout of the client is capped for the test.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: body with the definition of the endpoint, in YAML or JSON; optional query parameter key, as in validate.
// Responses: 200 with managedendpoint.TestResult, whether the evaluation succeeded or not; 400 when the definition is
// invalid (as in validate) or is a push endpoint, which cannot be tested; 409 when the key or the push token is already
// in use; 429 when too many tests are in progress; 500 on an unexpected error; the common responses of the
// administration (401, 403, 413, 415, 429).
func (h *adminHandler) test(c *echo.Context) error {
	result, err := h.service.Test(httpx.Body(c), strings.ToLower(httpx.Query(c, "key")))
	if err != nil {
		return adminServiceError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, result)
}

// create handles POST /api/v1/admin/endpoints: it creates a managed endpoint from the YAML or JSON definition in the
// body and starts monitoring it. A push endpoint without token gets a generated one.
//
// Authentication: administrator (protected group, administrator permission, request protection). The author of the
// change is the authenticated user.
// Request: body with the definition of the endpoint; If-Match is not used.
// Responses: 201 with the detail of the endpoint (managedendpoint.Detail) and an ETag with its version; 400 when the
// definition is empty or invalid; 409 when the key or the push token is already in use; 503 while a start or a
// configuration reload is in progress; 500 when the storage does not support managed endpoints or on an unexpected
// error; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminHandler) create(c *echo.Context) error {
	detail, err := h.service.Create(httpx.Body(c), h.security.RequestAuthor(c))
	if err != nil {
		return adminServiceError(c, err)
	}
	return writeAdminDetail(c, http.StatusCreated, detail)
}

// update handles PUT /api/v1/admin/endpoints/:key: it replaces the definition of a managed endpoint. A changed name or
// group renames the endpoint, and the managed status pages that select it by key are updated in the same transaction.
// The cached endpoint statuses are dropped.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: the path parameter key is unescaped once and lower-cased; the If-Match header is required and carries the
// current version of the endpoint, as "3" or W/"3" (the ETag of get); the body is the new definition, in YAML or JSON,
// in which a masked secret keeps its stored value.
// Responses: 200 with managedendpoint.Detail and an ETag with the new version; 400 when the definition is empty or
// invalid, or when it changes the type of the endpoint (push or not); 404 when no endpoint has the key; 409 when the
// endpoint is defined in the configuration file, when the new key or the push token is already in use, or when a
// managed status page was changed by another instance during a rename; 412 when If-Match is not a positive integer or
// is not the current version; 428 without If-Match; 503 while a start or a configuration reload is in progress; 500
// when the storage does not support managed endpoints, when the change could not be applied to the monitoring or on an
// unexpected error; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminHandler) update(c *echo.Context) error {
	expectedVersion, err := adminExpectedVersion(c)
	if err != nil {
		return adminServiceError(c, err)
	}
	detail, err := h.service.Update(adminKey(c), httpx.Body(c), expectedVersion, h.security.RequestAuthor(c))
	if err != nil {
		return adminServiceError(c, err)
	}
	// A renamed endpoint must not show up under its old key in the cached statuses anymore
	_ = cache.DeleteKeysByPattern("endpoint-status-*")
	return writeAdminDetail(c, http.StatusOK, detail)
}

// setEnabled returns the handler of POST /api/v1/admin/endpoints/:key/enable (enabled is true) and of POST
// /api/v1/admin/endpoints/:key/disable (enabled is false): it enables or disables a managed endpoint by rewriting the
// enabled field of its stored definition.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: the path parameter key is unescaped once and lower-cased; the If-Match header is required, as in update;
// the body is not used.
// Responses: 200 with managedendpoint.Detail and an ETag with the new version; 404 when no endpoint has the key; 409
// when the endpoint is defined in the configuration file; 412 when If-Match is invalid or is not the current version;
// 428 without If-Match; 503 while a start or a configuration reload is in progress; 400 or 409 when the stored
// definition does not validate anymore (as in update); 500 when the storage does not support managed endpoints, when
// the change could not be applied to the monitoring or on an unexpected error; the common responses of the
// administration (401, 403, 413, 415, 429).
func (h *adminHandler) setEnabled(enabled bool) echo.HandlerFunc {
	return func(c *echo.Context) error {
		expectedVersion, err := adminExpectedVersion(c)
		if err != nil {
			return adminServiceError(c, err)
		}
		detail, err := h.service.SetEnabled(adminKey(c), enabled, expectedVersion, h.security.RequestAuthor(c))
		if err != nil {
			return adminServiceError(c, err)
		}
		return writeAdminDetail(c, http.StatusOK, detail)
	}
}

// delete handles DELETE /api/v1/admin/endpoints/:key: it deletes a managed endpoint, stops its monitoring and deletes
// its data. The cached endpoint statuses are dropped.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: the path parameter key is unescaped once and lower-cased; the If-Match header is required, as in update.
// Responses: 200 with {"key": <key>, "triggeredAlerts": <number of alerts that were triggered, whose providers are not
// notified>}; 404 when no endpoint has the key; 409 when the endpoint is defined in the configuration file; 412 when
// If-Match is invalid or is not the current version; 428 without If-Match; 503 while a start or a configuration reload
// is in progress; 500 when the storage does not support managed endpoints, when the monitoring could not be stopped or
// on an unexpected error; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminHandler) delete(c *echo.Context) error {
	expectedVersion, err := adminExpectedVersion(c)
	if err != nil {
		return adminServiceError(c, err)
	}
	key := adminKey(c)
	triggeredAlerts, err := h.service.Delete(key, expectedVersion, h.security.RequestAuthor(c))
	if err != nil {
		return adminServiceError(c, err)
	}
	// The endpoint must not show up in the cached statuses anymore
	_ = cache.DeleteKeysByPattern("endpoint-status-*")
	return httpx.JSON(c, http.StatusOK, map[string]any{"key": key, "triggeredAlerts": triggeredAlerts})
}

// adminKey returns the key of the endpoint of the request: the path parameter key unescaped once (or as it was sent
// when it cannot be unescaped) and lower-cased.
func adminKey(c *echo.Context) string {
	key, err := url.PathUnescape(c.Param("key"))
	if err != nil {
		key = c.Param("key")
	}
	return strings.ToLower(key)
}

// adminExpectedVersion returns the version from the If-Match header, e.g. "3" or W/"3". A missing header is
// errAdminVersionRequired (428) and a value that is not a positive integer is a version mismatch (412).
func adminExpectedVersion(c *echo.Context) (int64, error) {
	value := strings.TrimSpace(httpx.Header(c, "If-Match"))
	if len(value) == 0 {
		return 0, errAdminVersionRequired
	}
	value = strings.Trim(strings.TrimPrefix(value, "W/"), `"`)
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("%w: invalid If-Match header", common.ErrManagedEndpointVersionMismatch)
	}
	return version, nil
}

// writeAdminDetail answers the detail of an endpoint as JSON with the given status, and sets the ETag header to its
// version in quotes when it has one (the endpoints of the configuration file have none).
func writeAdminDetail(c *echo.Context, status int, detail *managedendpoint.Detail) error {
	if detail.Version > 0 {
		httpx.SetHeader(c, "ETag", fmt.Sprintf(`"%d"`, detail.Version))
	}
	return httpx.JSON(c, status, detail)
}

// adminServiceError maps the errors of the administration of endpoints to their HTTP status and answers
// {"error": <text of the error>}: 404 not found; 409 read-only, key or push token in use, status page changed during a
// rename; 412 version mismatch; 428 missing If-Match; 429 too many tests; 503 start or reload in progress; 400 empty or
// invalid definition; 500 for anything else, which is logged. Unlike the other error mappers of the administration, the
// text of an unexpected error is sent to the client.
func adminServiceError(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, managedendpoint.ErrNotFound), errors.Is(err, common.ErrManagedEndpointNotFound):
		status = http.StatusNotFound
	case errors.Is(err, managedendpoint.ErrReadOnly), errors.Is(err, managedendpoint.ErrKeyConflict), errors.Is(err, common.ErrManagedEndpointAlreadyExists),
		errors.Is(err, common.ErrEndpointKeyInUse), errors.Is(err, common.ErrManagedStatusPageVersionMismatch),
		errors.Is(err, managedendpoint.ErrPushTokenInUse):
		// A managed status page changed by another instance during a rename is a conflict of the rename, not of the
		// version of the endpoint
		status = http.StatusConflict
	case errors.Is(err, common.ErrManagedEndpointVersionMismatch):
		status = http.StatusPreconditionFailed
	case errors.Is(err, errAdminVersionRequired):
		status = http.StatusPreconditionRequired
	case errors.Is(err, managedendpoint.ErrTooManyTests):
		status = http.StatusTooManyRequests
	case errors.Is(err, managedendpoint.ErrCycleInProgress):
		status = http.StatusServiceUnavailable
	case errors.Is(err, managedendpoint.ErrEmptyDefinition), errors.Is(err, managedendpoint.ErrInvalidDefinition),
		errors.Is(err, managedendpoint.ErrFieldNotAllowed), errors.Is(err, managedendpoint.ErrAlertProviderNotConfigured),
		errors.Is(err, managedendpoint.ErrInvalidAlertOverride), errors.Is(err, managedendpoint.ErrExtraLabelNotAllowed),
		errors.Is(err, managedendpoint.ErrPushNotTestable), errors.Is(err, managedendpoint.ErrTypeChanged):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		logr.Errorf("[api.adminServiceError] %s", err.Error())
	}
	return httpx.JSON(c, status, map[string]any{"error": err.Error()})
}
