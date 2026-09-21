package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/adminbackup"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

const (
	// adminRestoreMaximumBodySize is the maximum size of the body of the restore routes: an encrypted backup of
	// adminbackup.MaximumPlaintextBytes with the options, below the body limit of 4 MiB of the server
	adminRestoreMaximumBodySize = 3584 * 1024

	// adminRestorePreviewPath and adminRestorePath are the full paths of the restore routes, exempted from the body limit
	// of adminRequestProtection
	adminRestorePreviewPath = "/api/v1/admin/restore/preview"
	adminRestorePath        = "/api/v1/admin/restore"

	// restorePasswordWindow, restorePasswordMaximumFailures and restorePasswordMaximumKeys limit the wrong passwords of
	// the restores per client, independently of the login
	restorePasswordWindow          = 15 * time.Minute
	restorePasswordMaximumFailures = 10
	restorePasswordMaximumKeys     = 10000

	// derivationRetryAfterSeconds is the Retry-After, in seconds, of the 429 answered while too many key derivations of
	// encrypted backups are in progress
	derivationRetryAfterSeconds = "5"
)

// restorePasswordLimiter counts the wrong passwords of the restores per client
var restorePasswordLimiter = security.NewFailureLimiter(restorePasswordWindow, restorePasswordMaximumFailures, restorePasswordMaximumKeys)

// isAdminRestorePath returns whether path is one of the restore routes, whose body can exceed adminMaximumBodySize.
func isAdminRestorePath(path string) bool {
	return path == adminRestorePreviewPath || path == adminRestorePath
}

// adminBackupHandler holds the handlers of the backup and of the restore of the administration.
type adminBackupHandler struct {
	security *security.Config
	restorer *adminbackup.Restorer
}

// backupRequest is the optional JSON body of POST /api/v1/admin/backup. Unknown fields are refused.
type backupRequest struct {
	// Password encrypts the backup when it is not empty; its length is checked by adminbackup.CheckPassword
	Password string `json:"password"`
}

// restoreRequest is the JSON body of POST /api/v1/admin/restore/preview and POST /api/v1/admin/restore. Unknown fields
// are refused. File is the backup file as it was downloaded, embedded as a JSON value: it is required and must not be
// null. Password decrypts an encrypted backup, and must be empty for a backup that is not encrypted. Overwrite updates
// the items that already exist with a different definition, instead of skipping them. DisableEndpoints creates and
// updates the endpoints disabled. Fingerprint is the fingerprint returned by the preview: it is ignored by the preview
// and required by the restore.
type restoreRequest struct {
	File             json.RawMessage `json:"file"`
	Password         string          `json:"password"`
	Overwrite        bool            `json:"overwrite"`
	DisableEndpoints bool            `json:"disableEndpoints"`
	Fingerprint      string          `json:"fingerprint"`
}

// registerAdminBackupRoutes registers the backup and restore routes of the administration (fork). The router must
// already require authentication, administrator permission and request protection; the restore routes are exempted
// from its body limit and check their own.
func registerAdminBackupRoutes(router httpx.Router, cfg *config.Config) {
	handler := &adminBackupHandler{security: cfg.Security, restorer: &adminbackup.Restorer{Endpoints: managedendpoint.NewService(cfg), StatusPages: statuspage.NewService()}}
	clientIP := clientIPMiddleware(cfg.StatusPages.TrustedProxyPrefixes())
	router.POST("/backup", handler.backup, requireJSON)
	router.POST("/restore/preview", handler.preview, requireJSON, clientIP)
	router.POST("/restore", handler.apply, requireJSON, clientIP)
}

// requireJSON rejects with 415 the requests whose content type is not application/json, even without body, and sets
// Cache-Control: no-store on every response of the route
func requireJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		httpx.SetHeader(c, echo.HeaderCacheControl, "no-store")
		mediaType, _, err := mime.ParseMediaType(httpx.Header(c, echo.HeaderContentType))
		if err != nil || mediaType != echo.MIMEApplicationJSON {
			return adminError(c, http.StatusUnsupportedMediaType, "content type must be application/json")
		}
		return next(c)
	}
}

// backup handles POST /api/v1/admin/backup: it returns a backup file with the endpoints, status pages and push keys
// registered through the administration, encrypted when a password is given.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: Content-Type application/json is required, even without body; the body is optional and is a backupRequest
// ({"password": "..."}).
// Responses: 200 with the backup file as application/json, Content-Disposition: attachment with its file name
// (go-uptime-backup-<date>-<time>.json, or .enc.json when encrypted) and Cache-Control: no-store; 400 when the body is not
// a valid backupRequest or the password has an invalid length; 415 when the Content-Type is not application/json; 422
// when the backup exceeds the limits of size or of items; 429 with Retry-After: 5 when too many encrypted backups are
// in progress; 501 when the storage does not support the administration; 503 while a start or a configuration reload is
// in progress or when the registered items could not be loaded from the storage; 500 on an unexpected error, without
// its text; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminBackupHandler) backup(c *echo.Context) error {
	var request backupRequest
	if body := bytes.TrimSpace(httpx.Body(c)); len(body) > 0 {
		if err := decodeAdminJSON(body, &request); err != nil {
			return adminError(c, http.StatusBadRequest, "invalid request: "+err.Error())
		}
	}
	backup, err := adminbackup.Build(h.security.RequestAuthor(c), request.Password)
	if err != nil {
		return backupError(c, err)
	}
	httpx.SetHeader(c, echo.HeaderContentDisposition, `attachment; filename="`+backup.Filename+`"`)
	httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
	return httpx.Send(c, http.StatusOK, backup.Body)
}

// preview handles POST /api/v1/admin/restore/preview: it returns what the restore of a backup file would do, without
// any effect.
//
// Authentication: administrator (protected group, administrator permission, request protection, with a body limit of
// its own).
// Request: Content-Type application/json is required; the body is a restoreRequest with file and, when the backup is
// encrypted, password; overwrite and disableEndpoints are the options of the restore; fingerprint is ignored.
// Responses: 200 with adminbackup.Plan (summary, notices, items and the fingerprint to send to the restore) and
// Cache-Control: no-store; 400 when the body is not a valid restoreRequest, the file is missing or invalid, the password
// is wrong, missing for an encrypted file or given for a file that is not encrypted; 413 when the body exceeds 3584 KiB;
// 415 when the Content-Type is not application/json; 422 when the backup exceeds the limits of size or of items; 429
// with Retry-After after too many wrong passwords from the client (10 in 15 minutes) or, with Retry-After: 5, when too
// many key derivations are in progress; 503 when the registered items could not be loaded from the storage; 500 on an
// unexpected error, without its text; the common responses of the administration (401, 403, 415, 429).
func (h *adminBackupHandler) preview(c *echo.Context) error {
	request, plaintext, sent, err := h.readRestore(c)
	if sent {
		return err
	}
	plan, err := h.restorer.Plan(plaintext, adminbackup.Options{Overwrite: request.Overwrite, DisableEndpoints: request.DisableEndpoints})
	if err != nil {
		return backupError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, plan)
}

// apply handles POST /api/v1/admin/restore: it applies the restore of a backup file that was previewed. The cached
// endpoint statuses are dropped.
//
// Authentication: administrator (protected group, administrator permission, request protection, with a body limit of
// its own). The author of the changes is the authenticated user.
// Request: Content-Type application/json is required; the body is a restoreRequest with the same file, password and
// options of the preview, and the fingerprint the preview returned.
// Responses: 200 with adminbackup.Result (the result of each item and their count) and Cache-Control: no-store, even
// when items failed or were skipped because a configuration reload started; 400 as in preview, and when the fingerprint
// is missing; 409 when the backup, the options or the registered items changed since the preview; 413, 415, 422 and 429
// as in preview; 503 when the registered items could not be loaded from the storage; 500 on an unexpected error,
// without its text; the common responses of the administration (401, 403, 415, 429).
func (h *adminBackupHandler) apply(c *echo.Context) error {
	request, plaintext, sent, err := h.readRestore(c)
	if sent {
		return err
	}
	if len(request.Fingerprint) == 0 {
		return adminError(c, http.StatusBadRequest, "the fingerprint of the preview is required")
	}
	result, err := h.restorer.Apply(plaintext, adminbackup.Options{Overwrite: request.Overwrite, DisableEndpoints: request.DisableEndpoints}, request.Fingerprint, h.security.RequestAuthor(c))
	if err != nil {
		return backupError(c, err)
	}
	// The statuses of the endpoints are cached by the API, like after the changes of api/admin.go
	_ = cache.DeleteKeysByPattern("endpoint-status-*")
	return httpx.JSON(c, http.StatusOK, result)
}

// readRestore reads the body of a restore and returns the plaintext of its backup file. When sent is true, the error
// response is already set and the handler must return err.
func (h *adminBackupHandler) readRestore(c *echo.Context) (request *restoreRequest, plaintext []byte, sent bool, err error) {
	body := httpx.Body(c)
	if len(body) > adminRestoreMaximumBodySize {
		return nil, nil, true, adminError(c, http.StatusRequestEntityTooLarge, "request body is too large")
	}
	request = &restoreRequest{}
	if err := decodeAdminJSON(body, request); err != nil {
		return nil, nil, true, adminError(c, http.StatusBadRequest, "invalid request: "+err.Error())
	}
	if len(bytes.TrimSpace(request.File)) == 0 || bytes.Equal(bytes.TrimSpace(request.File), []byte("null")) {
		return nil, nil, true, adminError(c, http.StatusBadRequest, "the backup file is required")
	}
	clientIP, _ := c.Get(security.LocalsClientIP).(netip.Addr)
	now := time.Now()
	if blocked, retryAfter := restorePasswordLimiter.Blocked(clientIP, now); blocked {
		httpx.SetHeader(c, echo.HeaderRetryAfter, strconv.Itoa(int(retryAfter.Seconds())))
		return nil, nil, true, adminError(c, http.StatusTooManyRequests, "too many wrong passwords, try again later")
	}
	plaintext, _, err = adminbackup.Unwrap(request.File, request.Password)
	if err != nil {
		if errors.Is(err, adminbackup.ErrInvalidPassword) {
			restorePasswordLimiter.Failure(clientIP, now)
			logr.Warnf("[api.readRestore] Wrong password for an encrypted backup from %s", clientIP)
		}
		return nil, nil, true, backupError(c, err)
	}
	return request, plaintext, false, nil
}

// decodeAdminJSON decodes a JSON object, rejecting unknown fields and trailing data
func decodeAdminJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("unexpected data after the JSON object")
	}
	return nil
}

// backupError maps the errors of the backup and of the restore to their HTTP status and answers {"error": "..."}: 429
// with Retry-After when too many key derivations are in progress; 422 when the backup is too large; 409 when the
// fingerprint does not match; 503 during a start or reload, or when the registries are unavailable; 501 when the storage
// does not support the administration; 400 for an invalid file or password; 500 for anything else, which is logged and
// answered without its text.
func backupError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, adminbackup.ErrBusy):
		httpx.SetHeader(c, echo.HeaderRetryAfter, derivationRetryAfterSeconds)
		return adminError(c, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, adminbackup.ErrTooLarge):
		return adminError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, adminbackup.ErrFingerprintMismatch):
		return adminError(c, http.StatusConflict, err.Error())
	case errors.Is(err, adminbackup.ErrCycleInProgress), errors.Is(err, adminbackup.ErrUnavailable):
		return adminError(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, adminbackup.ErrStorageNotSupported):
		return adminError(c, http.StatusNotImplemented, err.Error())
	case errors.Is(err, adminbackup.ErrInvalidFile), errors.Is(err, adminbackup.ErrInvalidPassword), errors.Is(err, adminbackup.ErrPasswordRequired),
		errors.Is(err, adminbackup.ErrNotEncrypted), errors.Is(err, adminbackup.ErrPasswordLength):
		return adminError(c, http.StatusBadRequest, err.Error())
	default:
		logr.Errorf("[api.backupError] Backup or restore failed: %s", err.Error())
		return adminError(c, http.StatusInternalServerError, "the backup or the restore failed")
	}
}
