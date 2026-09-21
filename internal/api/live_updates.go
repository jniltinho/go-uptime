package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/liveupdates"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

const (
	// maximumEventStreams is the maximum number of event streams open at the same time, and maximumEventStreamsPerIP
	// the maximum per IP address of client (fork)
	maximumEventStreams      = 500
	maximumEventStreamsPerIP = 10

	// eventStreamRetryAfterSeconds is the Retry-After, in seconds, of the 429 of an event stream, and
	// eventStreamUnavailableBody the body of the 503 answered while the live updates are closed
	eventStreamRetryAfterSeconds = "30"
	eventStreamUnavailableBody   = `{"error":"live updates temporarily unavailable"}`
)

// eventStreamTestHook is called at the beginning of every event stream when set by the tests
var eventStreamTestHook func()

// eventStreams limits the event streams open in total and per IP address of client
var eventStreams = &eventStreamLimiter{perIP: make(map[netip.Addr]int)}

// eventStreamLimiter counts the event streams open, in total and per IP address of client.
type eventStreamLimiter struct {
	mutex sync.Mutex
	total int
	perIP map[netip.Addr]int
}

// acquire reserves a slot for a new event stream of the client, and returns false when a limit is reached
func (limiter *eventStreamLimiter) acquire(clientIP netip.Addr) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.total >= maximumEventStreams || limiter.perIP[clientIP] >= maximumEventStreamsPerIP {
		return false
	}
	limiter.total++
	limiter.perIP[clientIP]++
	return true
}

// release frees the slot of an event stream of the client
func (limiter *eventStreamLimiter) release(clientIP netip.Addr) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.total--
	if limiter.perIP[clientIP] <= 1 {
		delete(limiter.perIP, clientIP)
	} else {
		limiter.perIP[clientIP]--
	}
}

// endpointEventsHandler streams the notifications of the new results of an endpoint of the dashboard. The endpoint must
// be known in memory (configuration file, external endpoints or managed endpoints in any state), so that an endpoint
// without results yet can be watched without reading the storage.
//
// It returns the handler of GET and HEAD /api/v1/endpoints/:key/events, a server-sent event stream that is never
// compressed.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased. The Last-Event-ID header or, without it, the query parameter lastEventId is the last sequence the client
// saw (an unsigned integer; missing or invalid is 0).
// Responses: 200 with Content-Type: text/event-stream, Cache-Control: no-cache, no-store, no-transform and
// X-Accel-Buffering: no. The stream starts with "retry: 3000" and the current sequence as the id, sends at once an event
// "result" when the client missed one, then an event "result" (id: the sequence, data: {}) per new result and a comment
// ": ping" at every ping interval, until the client leaves, the live updates are closed or the maximum duration of a
// stream is reached. A HEAD answers 200 with the same headers, without opening a stream or taking a slot. 401 without a
// valid authentication (and 429 with security.basic while the client is blocked); 404 with
// {"error": "endpoint not found"} when the key cannot be unescaped or is not a known endpoint; 429 with Retry-After: 30
// and {"error": "too many requests"} when 500 streams are open or 10 from the IP address of the client; 503 with
// {"error": "live updates temporarily unavailable"} while the live updates are closed. The 429 and the 503 have
// Cache-Control: no-store.
func endpointEventsHandler(cfg *config.Config) echo.HandlerFunc {
	trustedProxies := cfg.StatusPages.TrustedProxyPrefixes()
	return func(c *echo.Context) error {
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil || (cfg.GetEndpointByKey(key) == nil && cfg.GetExternalEndpointByKey(key) == nil && managedendpoint.Get(key) == nil) {
			return httpx.JSON(c, http.StatusNotFound, map[string]any{"error": "endpoint not found"})
		}
		return streamEndpointEvents(c, key, eventStreamClientIP(c, trustedProxies), func(status int, body string) error {
			httpx.SetHeader(c, echo.HeaderCacheControl, "no-store")
			httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
			return httpx.SendString(c, status, body)
		})
	}
}

// statusPageEndpointEventsHandler streams the notifications of the new results of an endpoint of a published status
// page. Like the details of the endpoint, a page that is not published or does not show the endpoint gets the identical
// 404 of the status pages, before any limit and without reading the storage.
//
// It returns the handler of GET and HEAD /api/v1/status-pages/:slug/endpoints/:key/events, a server-sent event stream
// that is never compressed. Only registered when status-pages.enabled is true.
//
// Authentication: none, or HTTP Basic with the login of the page when the page requires one (statusPageAuth).
// Request: path parameters slug and key (the key is unescaped once with url.QueryUnescape and not lower-cased);
// Last-Event-ID header or lastEventId query parameter, as in endpointEventsHandler.
// Responses: 200 with the same stream and headers as endpointEventsHandler, plus the headers of the public status pages
// (X-Robots-Tag, X-Content-Type-Options, Referrer-Policy, Vary: Accept-Encoding); for a page with a login,
// Cache-Control is private, no-cache, no-store, no-transform and Vary has Authorization. A HEAD answers 200 with the
// headers only. 401 with WWW-Authenticate: Basic when the page requires a login and the credential is missing or wrong;
// 404 with {"error": "status page not found"} when the page is not published or does not show the endpoint; 429 with
// Retry-After when the client exceeded the rate limit of the 404s, failed the login of the page too many times, or
// when 500 streams are open or 10 from its IP address (Retry-After: 30); 503 while the live updates are closed.
func statusPageEndpointEventsHandler(notFound echo.HandlerFunc, trustedProxies []netip.Prefix) echo.HandlerFunc {
	return func(c *echo.Context) error {
		published, captured := publishedStatusPage(c)
		if !captured {
			return notFound(c)
		}
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil || !statuspage.IsEndpointShownOf(published, key) {
			return notFound(c)
		}
		setPublicAPIHeaders(c)
		if published.Page.RequiresLogin() {
			httpx.Vary(c, echo.HeaderAuthorization)
			c.Set(localsProtectedEventStream, true)
		}
		return streamEndpointEvents(c, key, eventStreamClientIP(c, trustedProxies), func(status int, body string) error {
			return sendStatusPageError(c, status, body)
		})
	}
}

// eventStreamClientIP returns the IP address of the client of an event stream, resolved with
// status-pages.trusted-proxies, and lets the status pages warn about a reverse proxy that is not trusted.
func eventStreamClientIP(c *echo.Context, trustedProxies []netip.Prefix) netip.Addr {
	remoteIP := httpx.RemoteIP(c)
	forwardedFor := httpx.HeaderValues(c, echo.HeaderXForwardedFor)
	statuspage.ObserveConnection(remoteIP, len(forwardedFor) > 0, trustedProxies)
	return statuspage.ClientIP(remoteIP, forwardedFor, trustedProxies)
}

// streamEndpointEvents answers HEAD with the headers only, refuses the stream while the live updates are closed (503) or
// when a limit is reached (429), and otherwise streams the notifications of the endpoint until the client leaves, the
// live updates are closed or the maximum duration of a stream is reached
func streamEndpointEvents(c *echo.Context, key string, clientIP netip.Addr, sendError func(status int, body string) error) error {
	if c.Request().Method == http.MethodHead {
		setEventStreamHeaders(c)
		return httpx.SendStatus(c, http.StatusOK)
	}
	notifications, currentSequence, cancel, err := liveupdates.Subscribe(key)
	if errors.Is(err, liveupdates.ErrClosed) {
		return sendError(http.StatusServiceUnavailable, eventStreamUnavailableBody)
	}
	if !eventStreams.acquire(clientIP) {
		cancel()
		httpx.SetHeader(c, echo.HeaderRetryAfter, eventStreamRetryAfterSeconds)
		return sendError(http.StatusTooManyRequests, statusPageTooManyRequestsBody)
	}
	lastEventID := parseLastEventID(httpx.Header(c, "Last-Event-ID"), httpx.Query(c, "lastEventId"))
	// The handler is the loop of the stream: there is no writer goroutine anymore, so the slot and the subscription are
	// released on every way out of it, a panic included
	defer func() {
		if recovered := recover(); recovered != nil {
			logr.Errorf("[api.streamEndpointEvents] Recovered from a panic in the event stream of key=%s: %v", key, recovered)
		}
		cancel()
		eventStreams.release(clientIP)
	}()
	stream := newEventStream(c)
	// Before the first byte: the write timeout of the server is for ordinary answers, and would cut the stream after a few
	// seconds. The cut would look like a client that left, because it also ends the context of the request.
	stream.extendWriteDeadline()
	setEventStreamHeaders(c)
	c.Response().WriteHeader(http.StatusOK)
	if eventStreamTestHook != nil {
		eventStreamTestHook()
	}
	writeEndpointEvents(c.Request().Context(), stream, key, notifications, currentSequence, lastEventID)
	return nil
}

// eventStream writes the events to the client and flushes each one, so that none waits in a buffer
type eventStream struct {
	writer     io.Writer
	controller *http.ResponseController
}

// newEventStream returns the event stream of the response of the request, with the controller that flushes it.
func newEventStream(c *echo.Context) *eventStream {
	return &eventStream{writer: c.Response(), controller: http.NewResponseController(c.Response())}
}

// extendWriteDeadline gives the stream a write deadline longer than its maximum duration. A writer without deadlines,
// like the recorder of a test, is left as it is.
func (stream *eventStream) extendWriteDeadline() {
	if err := stream.controller.SetWriteDeadline(time.Now().Add(liveupdates.StreamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		logr.Debugf("[api.eventStream] Failed to extend the write deadline of an event stream: %s", err.Error())
	}
}

// send writes and flushes, and returns whether the client is still there
func (stream *eventStream) send(text string) bool {
	if _, err := io.WriteString(stream.writer, text); err != nil {
		return false
	}
	return stream.controller.Flush() == nil
}

// setEventStreamHeaders sets the headers of an event stream: Content-Type: text/event-stream, a Cache-Control that
// forbids caching and transforming (private for a status page with a login) and X-Accel-Buffering: no, so that nginx
// does not buffer the events.
func setEventStreamHeaders(c *echo.Context) {
	httpx.SetHeader(c, echo.HeaderContentType, "text/event-stream")
	// Fork: the stream of a page that requires a login is private, so that no shared cache keeps it
	if protected, _ := c.Get(localsProtectedEventStream).(bool); protected {
		httpx.SetHeader(c, echo.HeaderCacheControl, "private, no-cache, no-store, no-transform")
	} else {
		httpx.SetHeader(c, echo.HeaderCacheControl, "no-cache, no-store, no-transform")
	}
	httpx.SetHeader(c, "X-Accel-Buffering", "no")
}

// parseLastEventID returns the sequence of the Last-Event-ID header, or of the lastEventId parameter of a new
// EventSource, and 0 when it is missing or invalid
func parseLastEventID(header, parameter string) uint64 {
	value := header
	if len(value) == 0 {
		value = parameter
	}
	lastEventID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return lastEventID
}

// writeEndpointEvents writes the event stream: the retry delay and the current sequence, a result event right away when
// the client missed one, then a result event per notification and a comment every PingInterval
func writeEndpointEvents(ctx context.Context, stream *eventStream, key string, notifications <-chan struct{}, currentSequence, lastEventID uint64) {
	if !stream.send(fmt.Sprintf("retry: 3000\nid: %d\n\n", currentSequence)) {
		return
	}
	if lastEventID < currentSequence && !writeResultEvent(stream, currentSequence) {
		return
	}
	ping := time.NewTicker(liveupdates.PingInterval)
	defer ping.Stop()
	maximumDuration := time.NewTimer(liveupdates.MaximumStreamDuration)
	defer maximumDuration.Stop()
	for {
		select {
		case _, open := <-notifications:
			if !open || !writeResultEvent(stream, liveupdates.Sequence(key)) {
				return
			}
		case <-ping.C:
			if !stream.send(": ping\n\n") {
				return
			}
		case <-maximumDuration.C:
			return
		case <-ctx.Done():
			// The client left, or the server is shutting down
			return
		}
	}
}

// writeResultEvent writes a result event and returns whether the client is still connected
func writeResultEvent(stream *eventStream, sequence uint64) bool {
	return stream.send(fmt.Sprintf("event: result\nid: %d\ndata: {}\n\n", sequence))
}
