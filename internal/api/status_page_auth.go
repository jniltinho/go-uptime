package api

import (
	"encoding/base64"
	"math"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"

	"github.com/labstack/echo/v5"
)

// Fork: login of a status page. The middleware runs before every route of a page and does, in this order:
//
//  1. resolves the slug once and keeps the published page in the locals, so that the route that answers uses exactly
//     the definition that was authorised;
//  2. answers the 404 of a page that does not exist, is not published or is not a page, exactly as before and without
//     WWW-Authenticate, so that nothing new is revealed;
//  3. challenges with 401 when the page requires a login, always sending WWW-Authenticate — including to requests that
//     look like they come from a browser, which is the opposite of what the login screen of security.basic needs.
//
// The 404 of a key that does not belong to the page stays in the route, after the challenge, so that the answer does
// not tell which endpoints a protected page has.
const (
	// localsPublishedStatusPage is the key of the locals holding the status page captured by the middleware
	localsPublishedStatusPage = "go-uptime.status-page"

	statusPageUnauthorizedBody = `{"error":"authentication required"}`

	// localsProtectedEventStream marks the event stream of a page that requires a login
	localsProtectedEventStream = "go-uptime.status-page-protected-stream"

	// localsPrivateBadge marks the badge of a page that requires a login, whose Cache-Control is private
	localsPrivateBadge = "go-uptime.status-page-private-badge"

	// localsProtectedPageHTML marks the HTML of a page that requires a login
	localsProtectedPageHTML = "go-uptime.status-page-protected-html"
)

// statusPageAuth returns the middleware of the routes of a status page
//
// It answers 404 (or 429 above the rate limit, see statusPageNotFound) when the slug is not a published page. For a
// page that requires a login, it reads Authorization: Basic and answers 401 with WWW-Authenticate: Basic
// realm="<slug>", charset="UTF-8" and {"error": "authentication required"} when the credential is missing or wrong, and
// 429 with Retry-After (seconds) and {"error": "too many requests"} after too many failures of the client.
func statusPageAuth(notFound echo.HandlerFunc, trustedProxies []netip.Prefix) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			published, ok := statuspage.Lookup(c.Param("slug"))
			if !ok {
				return notFound(c)
			}
			c.Set(localsPublishedStatusPage, published)
			if !published.Page.RequiresLogin() {
				return next(c)
			}
			username, password, hasCredential := basicCredentials(c)
			clientIP := statusPageClientIP(c, trustedProxies)
			result, retryAfter := statuspage.VerifyPageCredential(published.Page, published.Page.Slug, username, password, hasCredential, clientIP, time.Now())
			switch result {
			case statuspage.AuthTooManyFailures:
				httpx.SetHeader(c, echo.HeaderRetryAfter, strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				return sendStatusPageError(c, http.StatusTooManyRequests, statusPageTooManyRequestsBody)
			case statuspage.AuthUnauthorized:
				return sendStatusPageUnauthorized(c, published.Page.Slug)
			default:
				return next(c)
			}
		}
	}
}

// publishedStatusPage returns the page captured by the middleware, and false when the route ran without it
func publishedStatusPage(c *echo.Context) (statuspage.Published, bool) {
	published, ok := c.Get(localsPublishedStatusPage).(statuspage.Published)
	return published, ok
}

// sendStatusPageUnauthorized answers the challenge of a page. The realm is the slug of the published definition, which
// is validated, and never the parameter of the request.
func sendStatusPageUnauthorized(c *echo.Context, slug string) error {
	httpx.SetHeader(c, echo.HeaderWWWAuthenticate, `Basic realm="`+slug+`", charset="UTF-8"`)
	return sendStatusPageError(c, http.StatusUnauthorized, statusPageUnauthorizedBody)
}

// setProtectedPageCacheControl keeps the answers of a protected page out of any shared cache
func setProtectedPageCacheControl(c *echo.Context, protected bool, public string) {
	if protected {
		httpx.SetHeader(c, echo.HeaderCacheControl, "private, no-store")
		httpx.Vary(c, echo.HeaderAuthorization)
		return
	}
	httpx.SetHeader(c, echo.HeaderCacheControl, public)
}

// statusPageClientIP resolves the client of a request with status-pages.trusted-proxies
func statusPageClientIP(c *echo.Context, trustedProxies []netip.Prefix) netip.Addr {
	remoteIP := httpx.RemoteIP(c)
	return statuspage.ClientIP(remoteIP, httpx.HeaderValues(c, echo.HeaderXForwardedFor), trustedProxies)
}

// basicCredentials returns the username and the password of the Authorization: Basic header of the request
func basicCredentials(c *echo.Context) (string, string, bool) {
	scheme, encoded, found := strings.Cut(strings.TrimSpace(httpx.Header(c, echo.HeaderAuthorization)), " ")
	if !found || !strings.EqualFold(scheme, "basic") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}
	username, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}
	return username, password, true
}

// statusPageBadgeHandler serves a badge of an endpoint of a page, with the same rules of the other routes of the page:
// the key that does not belong to the page answers 404 only after the challenge of the middleware
//
// It returns the handler of GET and HEAD /api/v1/status-pages/:slug/endpoints/:key/health/badge.svg (badge is
// HealthBadge) and of GET and HEAD /api/v1/status-pages/:slug/endpoints/:key/response-times/:duration/badge.svg (badge
// is the handler of ResponseTimeBadge, duration being 1h, 24h, 7d or 30d).
//
// Authentication: none, or HTTP Basic with the login of the page when the page requires one (statusPageAuth).
// Request: path parameters slug and key; the key is unescaped once with url.QueryUnescape and not lower-cased.
// Responses: those of the badge handler (200 as image/svg+xml, 400, 404 and 500 as text/plain), with X-Robots-Tag,
// X-Content-Type-Options and Referrer-Policy, and with Cache-Control: private, no-store and Vary: Authorization for a
// page with a login; 401 and 429 from statusPageAuth; 404 with {"error": "status page not found"} (or 429 above the rate
// limit) when the page is not published, the key cannot be unescaped or the page does not show the endpoint.
func statusPageBadgeHandler(notFound echo.HandlerFunc, badge echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		published, captured := publishedStatusPage(c)
		if !captured {
			return notFound(c)
		}
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil || !statuspage.IsEndpointShownOf(published, key) {
			return notFound(c)
		}
		setPublicHeaders(c)
		// Said before the badge is written, never changed afterwards. The headers are set here for the answers the badge
		// handler gives without reaching setBadgeCacheControl — an invalid duration, a failure of the storage —, and the
		// mark is for the badge itself, which would otherwise overwrite them.
		if published.Page.RequiresLogin() {
			setProtectedPageCacheControl(c, true, "")
			c.Set(localsPrivateBadge, true)
		}
		if err = badge(c); err != nil {
			return err
		}
		return nil
	}
}

// statusPageHTMLAuth challenges the HTML routes of a page that requires a login. Every other path under /status/ keeps
// answering 200 with the HTML of the SPA, revealing nothing.
//
// For a published page that requires a login it answers, as JSON: 401 with WWW-Authenticate: Basic realm="<slug>",
// charset="UTF-8" when the credential of Authorization: Basic is missing or wrong, and 429 with Retry-After (seconds)
// after too many failures of the client.
func statusPageHTMLAuth(trustedProxies []netip.Prefix) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			slug := firstPathSegmentAfterStatus(httpx.Path(c))
			if len(slug) == 0 {
				return next(c)
			}
			published, ok := statuspage.Lookup(slug)
			if !ok || !published.Page.RequiresLogin() {
				return next(c)
			}
			username, password, hasCredential := basicCredentials(c)
			clientIP := statusPageClientIP(c, trustedProxies)
			result, retryAfter := statuspage.VerifyPageCredential(published.Page, published.Page.Slug, username, password, hasCredential, clientIP, time.Now())
			switch result {
			case statuspage.AuthTooManyFailures:
				httpx.SetHeader(c, echo.HeaderRetryAfter, strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				return sendStatusPageError(c, http.StatusTooManyRequests, statusPageTooManyRequestsBody)
			case statuspage.AuthUnauthorized:
				return sendStatusPageUnauthorized(c, published.Page.Slug)
			default:
				c.Set(localsProtectedPageHTML, true)
				return next(c)
			}
		}
	}
}

// firstPathSegmentAfterStatus returns the slug of /status/<slug> and of any path under it, unescaped, or an empty
// string when the path has no slug
func firstPathSegmentAfterStatus(path string) string {
	rest, found := strings.CutPrefix(path, "/status/")
	if !found {
		return ""
	}
	if index := strings.IndexByte(rest, '/'); index >= 0 {
		rest = rest[:index]
	}
	slug, err := url.QueryUnescape(rest)
	if err != nil {
		return ""
	}
	return slug
}
