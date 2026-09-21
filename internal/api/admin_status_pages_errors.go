package api

import (
	"errors"
	"net/http"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// adminStatusPageError maps the errors of the administration of the status pages to their HTTP status and answers
// {"error": "..."}: 501 storage not supported; 404 page not found; 409 read-only page or slug in use; 412 version
// mismatch; 428 missing If-Match; 503 start or reload in progress, or page unavailable; 400 empty or invalid definition,
// changed slug, missing exposure query or invalid login of the page; 500 for anything else. Unexpected errors are
// logged and answered without their text.
func adminStatusPageError(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, statuspage.ErrStorageNotSupported):
		status = http.StatusNotImplemented
	case errors.Is(err, statuspage.ErrPageNotFound), errors.Is(err, common.ErrManagedStatusPageNotFound):
		status = http.StatusNotFound
	case errors.Is(err, statuspage.ErrReadOnly), errors.Is(err, statuspage.ErrSlugInUse), errors.Is(err, common.ErrManagedStatusPageAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, common.ErrManagedStatusPageVersionMismatch), errors.Is(err, common.ErrManagedEndpointVersionMismatch):
		status = http.StatusPreconditionFailed
	case errors.Is(err, errAdminVersionRequired):
		status = http.StatusPreconditionRequired
	case errors.Is(err, statuspage.ErrCycleInProgress), errors.Is(err, statuspage.ErrPageUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, statuspage.ErrEmptyDefinition), errors.Is(err, statuspage.ErrInvalidDefinition),
		errors.Is(err, statuspage.ErrSlugChanged), errors.Is(err, statuspage.ErrExposureQueryRequired),
		errors.Is(err, pageconfig.ErrInvalidSlug), errors.Is(err, pageconfig.ErrReservedSlug),
		errors.Is(err, pageconfig.ErrInvalidTitle), errors.Is(err, pageconfig.ErrDescriptionTooLong),
		errors.Is(err, pageconfig.ErrEmptySelection), errors.Is(err, pageconfig.ErrInvalidGroups),
		errors.Is(err, pageconfig.ErrInvalidEndpoints), errors.Is(err, pageconfig.ErrInvalidFeatured),
		errors.Is(err, pageconfig.ErrInvalidCharts),
		errors.Is(err, statuspage.ErrAuthPasswordTooShort), errors.Is(err, statuspage.ErrAuthPasswordTooLong),
		errors.Is(err, statuspage.ErrAuthPasswordRequired), errors.Is(err, pageconfig.ErrInvalidAuthUsername),
		errors.Is(err, pageconfig.ErrInvalidAuthPasswordHash):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		logr.Errorf("[api.adminStatusPageError] %s", err.Error())
		return httpx.JSON(c, status, map[string]any{"error": "internal error"})
	}
	return httpx.JSON(c, status, map[string]any{"error": err.Error()})
}
