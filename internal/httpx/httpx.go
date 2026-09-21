// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package httpx holds what the handlers need from the HTTP framework, in one place. Go Uptime moved from Fiber (fasthttp)
// to Echo v5 (net/http), and several methods kept their name while changing their meaning: echo.Context.Get reads the
// store of the request and not a header, Path returns the registered route and not the path of the request, and the
// body can only be read once. Every handler goes through these functions, so that those rules live here and not in
// the head of whoever writes the next handler.
package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
)

const (
	// MaximumBodySize is the largest body the server accepts, on every route: the 4 MiB Fiber used to enforce by default,
	// which the limits of the administration (256 KB), of the restore (3.5 MiB) and of the login (4 KB) stay below
	MaximumBodySize = 4 << 20

	// bodyKey is the key of the store holding the body read ahead by BufferBody
	bodyKey = "go-uptime.httpx.body"

	mimeTextPlain = "text/plain; charset=utf-8"
	mimeJSON      = "application/json"
)

// ErrBodyTooLarge is returned by Body when the body is larger than the limit of the route
var ErrBodyTooLarge = errors.New("request body is too large")

// Header returns a header of the request. Never use echo.Context.Get for this: it reads the store of the request, so
// `c.Get("Origin")` is always nil and a check built on it lets everything through.
func Header(c *echo.Context, name string) string {
	return c.Request().Header.Get(name)
}

// HeaderValues returns every line of a header of the request. X-Forwarded-For may come in several lines, and
// http.Header.Get only returns the first one.
func HeaderValues(c *echo.Context, name string) []string {
	return c.Request().Header.Values(name)
}

// SetHeader sets a header of the response. It must be called before the body is written: net/http sends the headers
// with the first byte, and what is set afterwards never reaches the client.
func SetHeader(c *echo.Context, name, value string) {
	c.Response().Header().Set(name, value)
}

// Vary adds a field to the Vary header of the response, once
func Vary(c *echo.Context, field string) {
	header := c.Response().Header()
	for _, line := range header.Values(echo.HeaderVary) {
		for _, existing := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(existing), field) {
				return
			}
		}
	}
	if current := header.Get(echo.HeaderVary); len(current) > 0 {
		header.Set(echo.HeaderVary, current+", "+field)
		return
	}
	header.Set(echo.HeaderVary, field)
}

// Path returns the path of the request as it was sent, still escaped, which is what Fiber's Path returned and what the
// handlers unescape themselves. Never use echo.Context.Path for this: it returns the registered route, such as
// /status/:slug, whatever the request was.
func Path(c *echo.Context) string {
	return c.Request().URL.EscapedPath()
}

// IsTLS returns whether the connection itself uses TLS. Never use echo.Context.Scheme for this: it trusts
// X-Forwarded-Proto and X-Forwarded-Ssl from anyone.
func IsTLS(c *echo.Context) bool {
	return c.Request().TLS != nil
}

// RemoteIP returns the IP address of the connection, never of a header. Never use echo.Context.RealIP for this: what it
// returns depends on Echo.IPExtractor, which this project never sets, so that the rule does not rest on a default.
func RemoteIP(c *echo.Context) netip.Addr {
	return RemoteIPOf(c.Request())
}

// RemoteIPOf returns the IP address of the connection of a request
func RemoteIPOf(request *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return address.Unmap()
}

// Send writes the status and the body. The Content-Type must already be set, otherwise it is text/plain.
func Send(c *echo.Context, status int, body []byte) error {
	header := c.Response().Header()
	// An empty body has no type, as before
	if len(body) > 0 && len(header.Get(echo.HeaderContentType)) == 0 {
		header.Set(echo.HeaderContentType, mimeTextPlain)
	}
	c.Response().WriteHeader(status)
	_, err := c.Response().Write(body)
	return err
}

// SendString writes the status and a text
func SendString(c *echo.Context, status int, body string) error {
	return Send(c, status, []byte(body))
}

// SendStatus writes the status with its standard text as the body, as Fiber did, except for the statuses that cannot
// have a body (1xx, 204 and 304)
func SendStatus(c *echo.Context, status int) error {
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified {
		return NoContent(c, status)
	}
	return SendString(c, status, http.StatusText(status))
}

// NoContent writes the status without a body
func NoContent(c *echo.Context, status int) error {
	c.Response().WriteHeader(status)
	return nil
}

// JSON writes the status and the value as JSON. It does not use echo.Context.JSON, whose encoder adds a line break at
// the end of every body.
func JSON(c *echo.Context, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return JSONBlob(c, status, body)
}

// JSONBlob writes the status and a body that is already JSON
func JSONBlob(c *echo.Context, status int, body []byte) error {
	c.Response().Header().Set(echo.HeaderContentType, mimeJSON)
	c.Response().WriteHeader(status)
	_, err := c.Response().Write(body)
	return err
}

// BufferBody reads the body of every request ahead, up to MaximumBodySize, and answers 413 above it BEFORE the handler
// runs. echo's BodyLimit only notices the excess of a body without Content-Length while it is being read, so a handler
// that never reads the body — the push — would record its result with a chunked body of any size. The body read ahead
// is what Body returns, as many times as it is asked: a middleware that inspects the body does not leave the handler
// with an empty one.
func BufferBody(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		if request.Body == nil || request.Body == http.NoBody {
			return next(c)
		}
		if request.ContentLength > MaximumBodySize {
			return tooLarge(c)
		}
		body, err := readAhead(c)
		if err != nil {
			return SendString(c, http.StatusBadRequest, "invalid request body")
		}
		if len(body) > MaximumBodySize {
			return tooLarge(c)
		}
		return next(c)
	}
}

func tooLarge(c *echo.Context) error {
	// The rest of the body is not read: the connection is closed, as Fiber did
	c.Response().Header().Set("Connection", "close")
	return SendString(c, http.StatusRequestEntityTooLarge, http.StatusText(http.StatusRequestEntityTooLarge))
}

// readAhead reads the body once, up to one byte past MaximumBodySize so that the excess can be told, keeps it in the
// store of the request and puts it back in the request for whoever reads the request itself
func readAhead(c *echo.Context) ([]byte, error) {
	request := c.Request()
	if request.Body == nil || request.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaximumBodySize+1))
	if err != nil {
		return nil, err
	}
	_ = request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	c.Set(bodyKey, body)
	return body, nil
}

// Body returns the body of the request, as many times as it is asked. BufferBody has normally read it ahead; without
// that middleware it is read now, so that a check of the body never sees an empty one by accident.
func Body(c *echo.Context) []byte {
	if body, buffered := c.Get(bodyKey).([]byte); buffered {
		return body
	}
	body, _ := readAhead(c)
	return body
}

// BodyWithin returns the body of the request, or ErrBodyTooLarge when it is larger than the limit of the route
func BodyWithin(c *echo.Context, limit int) ([]byte, error) {
	body := Body(c)
	if len(body) > limit {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// Router is what *echo.Echo and *echo.Group have in common, for the functions that register routes on either
type Router interface {
	Use(middleware ...echo.MiddlewareFunc)
	Add(method, path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) echo.RouteInfo
	Any(path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) echo.RouteInfo
	POST(path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) echo.RouteInfo
	PUT(path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) echo.RouteInfo
	DELETE(path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) echo.RouteInfo
	Group(prefix string, middleware ...echo.MiddlewareFunc) *echo.Group
}

// GetAndHead registers a handler for GET and for HEAD. Fiber registered HEAD with every GET; Echo does not, and its
// RouterConfig.AutoHandleHEAD must stay off: it would run the handler of an event stream, which would reserve a slot and
// open a stream, for a HEAD. net/http drops the body of the answer to a HEAD by itself.
func GetAndHead(router Router, path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) {
	router.Add(http.MethodGet, path, handler, middleware...)
	router.Add(http.MethodHead, path, handler, middleware...)
}

// NormalizePath is a Pre middleware that makes the router always see the path as it was sent, and ignore a trailing
// slash as Fiber did. It replaces echo's RemoveTrailingSlash, and fixes two things that one leaves behind:
//
//   - net/url only keeps URL.RawPath when the escaping is not the canonical one, and echo's router matches the DECODED
//     path without it. A key would then reach its handler escaped or not depending on how it was written: core_100%25
//     arrives as "core_100%", which no longer unescapes, and core_%2561pi arrives as "core_%61pi", which the handler
//     unescapes again into the key of ANOTHER endpoint. With RawPath always set, every parameter is escaped, and every
//     handler unescapes it exactly once.
//   - RemoveTrailingSlash changes URL.Path and leaves URL.RawPath alone, so an escaped path with a trailing slash matched
//     nothing, and fell to the 404 of the protected group.
func NormalizePath(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		escaped := request.URL.EscapedPath()
		for len(escaped) > 1 && strings.HasSuffix(escaped, "/") {
			escaped = strings.TrimSuffix(escaped, "/")
		}
		if unescaped, err := url.PathUnescape(escaped); err == nil {
			request.URL.Path, request.URL.RawPath = unescaped, escaped
			request.RequestURI = escaped
			if len(request.URL.RawQuery) > 0 {
				request.RequestURI += "?" + request.URL.RawQuery
			}
		}
		return next(c)
	}
}

// Query returns the first value of a parameter of the query, read as fasthttp read it. net/url drops a whole pair when
// it has a ";" or an invalid escape, and for the push a missing status means "up": `?status=down;maintenance` would
// record a success. Here the pairs are only split on "&", a ";" is part of the value, "+" is a space and an invalid
// escape is kept as it is.
func Query(c *echo.Context, name string) string {
	values := queryValues(c.Request().URL.RawQuery, name)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// QueryLast returns the last value of a parameter of the query, and whether the parameter is present
func QueryLast(c *echo.Context, name string) (string, bool) {
	values := queryValues(c.Request().URL.RawQuery, name)
	if len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

func queryValues(rawQuery, name string) []string {
	var values []string
	for _, pair := range strings.Split(rawQuery, "&") {
		if len(pair) == 0 {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		if decodeQueryComponent(key) == name {
			values = append(values, decodeQueryComponent(value))
		}
	}
	return values
}

// decodeQueryComponent decodes "+" and the valid escapes, and keeps an invalid escape as it is
func decodeQueryComponent(component string) string {
	if !strings.ContainsAny(component, "%+") {
		return component
	}
	decoded := make([]byte, 0, len(component))
	for i := 0; i < len(component); i++ {
		switch {
		case component[i] == '+':
			decoded = append(decoded, ' ')
		case component[i] == '%' && i+2 < len(component) && isHex(component[i+1]) && isHex(component[i+2]):
			decoded = append(decoded, unhex(component[i+1])<<4|unhex(component[i+2]))
			i += 2
		default:
			decoded = append(decoded, component[i])
		}
	}
	return string(decoded)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}
