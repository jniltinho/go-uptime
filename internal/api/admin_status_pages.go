package api

import (
	"fmt"
	"net/http"

	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"

	"github.com/labstack/echo/v5"
)

// adminStatusPageHandler holds the handlers of the administration of the status pages, under
// /api/v1/admin/status-pages.
type adminStatusPageHandler struct {
	service  *statuspage.Service
	security *security.Config
}

// registerAdminStatusPageRoutes registers the routes of the administration of the status pages on the administration
// router, which already requires authentication, administrator permission and request protection. The fixed segments
// are registered before /:slug, and the slugs matching them are reserved.
func registerAdminStatusPageRoutes(router httpx.Router, securityConfig *security.Config) {
	handler := &adminStatusPageHandler{service: statuspage.NewService(), security: securityConfig}
	httpx.GetAndHead(router, "/status-pages", handler.list)
	httpx.GetAndHead(router, "/status-pages/options", handler.options)
	httpx.GetAndHead(router, "/status-pages/exposure", handler.exposure)
	router.POST("/status-pages/validate", handler.validate)
	router.POST("/status-pages", handler.create)
	httpx.GetAndHead(router, "/status-pages/:slug", handler.get)
	router.PUT("/status-pages/:slug", handler.update)
	router.POST("/status-pages/:slug/enable", handler.setEnabled(true))
	router.POST("/status-pages/:slug/disable", handler.setEnabled(false))
	router.DELETE("/status-pages/:slug", handler.delete)
	httpx.GetAndHead(router, "/status-pages/:slug/preview", handler.preview)
}

// list handles GET and HEAD /api/v1/admin/status-pages: it lists every status page, of the configuration file
// (read-only) and managed, published or not.
//
// Authentication: administrator (protected group, administrator permission).
// Responses: 200 with statuspage.Listing (the pages, whether the publication is enabled, whether the managed pages
// could not be loaded and the warning of a shared rate limit); the common responses of the administration (401, 403,
// 429), see registerAdminRoutes.
func (h *adminStatusPageHandler) list(c *echo.Context) error {
	return httpx.JSON(c, http.StatusOK, h.service.List())
}

// options handles GET and HEAD /api/v1/admin/status-pages/options: it lists the groups and the endpoints a status page
// can select.
//
// Authentication: administrator (protected group, administrator permission).
// Responses: 200 with statuspage.Options; the common responses of the administration (401, 403, 429).
func (h *adminStatusPageHandler) options(c *echo.Context) error {
	return httpx.JSON(c, http.StatusOK, h.service.Options())
}

// exposure handles GET and HEAD /api/v1/admin/status-pages/exposure: it lists the status pages on which an endpoint
// with the given group or key would appear, so that the form of an endpoint can warn before publishing it.
//
// Authentication: administrator (protected group, administrator permission).
// Request: query parameters group (normalized like the group of a page) and key (trimmed and lower-cased); at least one
// of them is required. A page that selects the group is reported with the reason "group", otherwise a page that selects
// or features the key with the reason "key".
// Responses: 200 with statuspage.Exposure; 400 when both group and key are empty; the common responses of the
// administration (401, 403, 429).
func (h *adminStatusPageHandler) exposure(c *echo.Context) error {
	exposure, err := h.service.Exposure(httpx.Query(c, "group"), httpx.Query(c, "key"))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, exposure)
}

// validate handles POST /api/v1/admin/status-pages/validate: it validates the definition in the body without
// persisting it.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: body with the definition of the status page, in YAML or JSON. The optional query parameter slug is the slug
// of the managed status page being edited: its own slug is then not in use, and a password that is not sent keeps the
// stored one.
// Responses: 200 with statuspage.Validation (the normalized definition with the hash of the credential masked, the
// warnings of the groups and keys without match and the number of endpoints selected); 400 when the definition is
// empty or invalid (slug, title, description, selection, charts, login), or when it changes the slug; 409 when the slug
// is already in use; 500 on an unexpected error, without its text; the common responses of the administration (401,
// 403, 413, 415, 429).
func (h *adminStatusPageHandler) validate(c *echo.Context) error {
	validation, err := h.service.Validate(httpx.Body(c), httpx.Query(c, "slug"))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, validation)
}

// create handles POST /api/v1/admin/status-pages: it creates a managed status page from the YAML or JSON definition in
// the body. Without enabled, the page is created disabled.
//
// Authentication: administrator (protected group, administrator permission, request protection). The author of the
// change is the authenticated user.
// Request: body with the definition of the status page; If-Match is not used.
// Responses: 201 with statuspage.Detail and an ETag with its version in quotes; 400 when the definition is empty or
// invalid; 409 when the slug is already in use; 501 when the storage does not support managed status pages; 503 while a
// start or a configuration reload is in progress; 500 on an unexpected error, without its text; the common responses of
// the administration (401, 403, 413, 415, 429).
func (h *adminStatusPageHandler) create(c *echo.Context) error {
	detail, err := h.service.Create(httpx.Body(c), h.security.RequestAuthor(c))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	return writeAdminStatusPageDetail(c, http.StatusCreated, detail)
}

// get handles GET and HEAD /api/v1/admin/status-pages/:slug: it returns a status page, managed or of the configuration
// file, with its definition.
//
// Authentication: administrator (protected group, administrator permission).
// Request: the path parameter slug is the slug of the page, used as it was sent; options, exposure and validate are
// fixed segments registered before it, and reserved slugs.
// Responses: 200 with statuspage.Detail and, for a managed page, an ETag with its version in quotes, to be sent back in
// If-Match; 404 when no page has the slug; 500 on an unexpected error, without its text; the common responses of the
// administration (401, 403, 429).
func (h *adminStatusPageHandler) get(c *echo.Context) error {
	detail, err := h.service.Get(c.Param("slug"))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	return writeAdminStatusPageDetail(c, http.StatusOK, detail)
}

// update handles PUT /api/v1/admin/status-pages/:slug: it replaces the definition of a managed status page. The slug
// cannot be changed.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: path parameter slug; the If-Match header is required and carries the current version of the page, as "3" or
// W/"3" (the ETag of get); the body is the new definition, in YAML or JSON, in which a password that is not sent keeps
// the stored one.
// Responses: 200 with statuspage.Detail and an ETag with the new version; 400 when the definition is empty or invalid,
// or when it changes the slug; 404 when no page has the slug; 409 when the page is defined in the configuration file;
// 412 when If-Match is not a positive integer or is not the current version; 428 without If-Match; 501 when the storage
// does not support managed status pages; 503 while a start or a configuration reload is in progress; 500 on an
// unexpected error, without its text; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminStatusPageHandler) update(c *echo.Context) error {
	expectedVersion, err := adminExpectedVersion(c)
	if err != nil {
		return adminStatusPageError(c, err)
	}
	detail, err := h.service.Update(c.Param("slug"), httpx.Body(c), expectedVersion, h.security.RequestAuthor(c))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	return writeAdminStatusPageDetail(c, http.StatusOK, detail)
}

// setEnabled returns the handler of POST /api/v1/admin/status-pages/:slug/enable (enabled is true) and of POST
// /api/v1/admin/status-pages/:slug/disable (enabled is false): it enables or disables a managed status page.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: path parameter slug; the If-Match header is required, as in update; the body is not used.
// Responses: 200 with statuspage.Detail and an ETag with the new version; 404, 409, 412, 428, 501, 503 and 500 as in
// update; 400 when the stored definition does not validate anymore; the common responses of the administration (401,
// 403, 413, 415, 429).
func (h *adminStatusPageHandler) setEnabled(enabled bool) echo.HandlerFunc {
	return func(c *echo.Context) error {
		expectedVersion, err := adminExpectedVersion(c)
		if err != nil {
			return adminStatusPageError(c, err)
		}
		detail, err := h.service.SetEnabled(c.Param("slug"), enabled, expectedVersion, h.security.RequestAuthor(c))
		if err != nil {
			return adminStatusPageError(c, err)
		}
		return writeAdminStatusPageDetail(c, http.StatusOK, detail)
	}
}

// delete handles DELETE /api/v1/admin/status-pages/:slug: it deletes a managed status page and unpublishes it.
//
// Authentication: administrator (protected group, administrator permission, request protection).
// Request: path parameter slug; the If-Match header is required, as in update.
// Responses: 200 with {"slug": <slug>}; 404 when no page has the slug; 409 when the page is defined in the
// configuration file; 412 when If-Match is invalid or is not the current version; 428 without If-Match; 501 when the
// storage does not support managed status pages; 503 while a start or a configuration reload is in progress; 500 on an
// unexpected error, without its text; the common responses of the administration (401, 403, 413, 415, 429).
func (h *adminStatusPageHandler) delete(c *echo.Context) error {
	expectedVersion, err := adminExpectedVersion(c)
	if err != nil {
		return adminStatusPageError(c, err)
	}
	slug := c.Param("slug")
	if err := h.service.Delete(slug, expectedVersion, h.security.RequestAuthor(c)); err != nil {
		return adminStatusPageError(c, err)
	}
	return httpx.JSON(c, http.StatusOK, map[string]any{"slug": slug})
}

// preview handles GET and HEAD /api/v1/admin/status-pages/:slug/preview: it returns the public payload of any status
// page, including the disabled ones, the ones in conflict and the ones of the configuration file, without cache and
// without rate limit. One preview is assembled at a time.
//
// Authentication: administrator (protected group, administrator permission).
// Request: path parameter slug.
// Responses: 200 with the same JSON body as GET /api/v1/status-pages/:slug (statuspage.Payload) and Cache-Control:
// no-store; 400 when the stored definition of the page is invalid; 404 when no page has the slug; 503 when another
// preview is in progress or the payload could not be assembled; 500 on an unexpected error, without its text; the
// common responses of the administration (401, 403, 429).
func (h *adminStatusPageHandler) preview(c *echo.Context) error {
	body, err := h.service.Preview(c.Param("slug"))
	if err != nil {
		return adminStatusPageError(c, err)
	}
	httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
	httpx.SetHeader(c, echo.HeaderCacheControl, "no-store")
	return httpx.Send(c, http.StatusOK, body)
}

// writeAdminStatusPageDetail answers the detail of a status page as JSON with the given status, and sets the ETag
// header to its version in quotes when it has one (the pages of the configuration file have none).
func writeAdminStatusPageDetail(c *echo.Context, status int, detail *statuspage.Detail) error {
	if detail.Version > 0 {
		httpx.SetHeader(c, "ETag", fmt.Sprintf(`"%d"`, detail.Version))
	}
	return httpx.JSON(c, status, detail)
}
