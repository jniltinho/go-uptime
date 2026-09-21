// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"

	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/security"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// createPushKeyRequest is the body of POST /api/v1/admin/push-keys
type createPushKeyRequest struct {
	// Name is the name of the push key: trimmed, required, unique among the push keys and limited in length (see
	// pushconfig.MaximumKeyNameLength)
	Name string `json:"name"`
}

// registerAdminPushKeyRoutes registers the routes of the administration of the global push keys (fork). The router must
// already require authentication, administrator permission and request protection.
func registerAdminPushKeyRoutes(router httpx.Router, securityConfig *security.Config) {
	// GET and HEAD /api/v1/admin/push-keys: lists the global push keys, of the configuration file and created through
	// the administration, never with their token.
	//
	// Authentication: administrator (protected group, administrator permission).
	// Responses: 200 with pushkey.Listing (keys, each a pushkey.Key with a hint of its token, and managedUnavailable,
	// which is true when the keys of the administration could not be loaded); the common responses of the administration
	// (401, 403, 429), see registerAdminRoutes.
	httpx.GetAndHead(router, "/push-keys", func(c *echo.Context) error {
		return httpx.JSON(c, http.StatusOK, pushkey.List())
	})
	// POST /api/v1/admin/push-keys: creates a global push key with a generated token.
	//
	// Authentication: administrator (protected group, administrator permission, request protection). The author of the
	// change is the authenticated user.
	// Request: Content-Type application/json is required; the body is a createPushKeyRequest ({"name": "..."}).
	// Responses: 201 with pushkey.Created (the key and its token, which is only available in this response) and
	// Cache-Control: no-store; 400 when the Content-Type is not application/json, the body is not valid JSON or the name
	// is empty or too long; 409 when a push key already has the name; 501 when the storage does not support push keys
	// managed through the administration; 503 while a start or a configuration reload is in progress; 500 on an unexpected
	// error, without its text; the common responses of the administration (401, 403, 413, 415, 429).
	router.POST("/push-keys", func(c *echo.Context) error {
		var request createPushKeyRequest
		// Only JSON, as before: the body was already read ahead, see httpx.BufferBody
		mediaType, _, _ := mime.ParseMediaType(httpx.Header(c, echo.HeaderContentType))
		if err := json.Unmarshal(httpx.Body(c), &request); err != nil || mediaType != echo.MIMEApplicationJSON {
			return httpx.JSON(c, http.StatusBadRequest, map[string]any{"error": "invalid body: a JSON object with the name of the key is expected"})
		}
		created, err := pushkey.Create(request.Name, securityConfig.RequestAuthor(c))
		if err != nil {
			return adminPushKeyError(c, err)
		}
		// The token is only available in this response
		httpx.SetHeader(c, echo.HeaderCacheControl, "no-store")
		return httpx.JSON(c, http.StatusCreated, created)
	})
	// DELETE /api/v1/admin/push-keys/:id: deletes a push key created through the administration. If-Match is not used.
	//
	// Authentication: administrator (protected group, administrator permission, request protection).
	// Request: the path parameter id is the positive integer id of the push key (pushkey.Key.ID); the keys of the
	// configuration file have no id and cannot be deleted.
	// Responses: 200 with {"id": <id>}; 404 when id is not a positive integer or no managed push key has it; 501 when the
	// storage does not support push keys managed through the administration; 503 while a start or a configuration reload
	// is in progress; 500 on an unexpected error, without its text; the common responses of the administration (401, 403,
	// 413, 415, 429).
	router.DELETE("/push-keys/:id", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			return adminPushKeyError(c, pushkey.ErrNotFound)
		}
		if err := pushkey.Delete(id, securityConfig.RequestAuthor(c)); err != nil {
			return adminPushKeyError(c, err)
		}
		return httpx.JSON(c, http.StatusOK, map[string]any{"id": id})
	})
}

// adminPushKeyError maps the errors of the administration of the push keys to their HTTP status and answers
// {"error": "..."}: 501 storage not supported; 404 not found; 409 name in use; 503 start or reload in progress; 400
// invalid name; 500 for anything else. Unexpected errors are logged and answered without their text.
func adminPushKeyError(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, pushkey.ErrStorageNotSupported):
		status = http.StatusNotImplemented
	case errors.Is(err, pushkey.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, pushkey.ErrNameInUse):
		status = http.StatusConflict
	case errors.Is(err, pushkey.ErrCycleInProgress):
		status = http.StatusServiceUnavailable
	case errors.Is(err, pushconfig.ErrInvalidKeyName):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		logr.Errorf("[api.adminPushKeyError] %s", err.Error())
		return httpx.JSON(c, status, map[string]any{"error": "internal error"})
	}
	return httpx.JSON(c, status, map[string]any{"error": err.Error()})
}
