// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package httpx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func serve(handler echo.HandlerFunc, request *http.Request, middleware ...echo.MiddlewareFunc) *httptest.ResponseRecorder {
	e := echo.New()
	e.Use(middleware...)
	e.Any("/*", handler)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	return recorder
}

// TestHeaderIsNotTheStore is the trap of the migration: the same call reads a header in Fiber and the store in Echo
func TestHeaderIsNotTheStore(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("Origin", "https://evil.example")
	serve(func(c *echo.Context) error {
		if c.Get("Origin") != nil {
			t.Error("expected the store of echo not to hold the headers")
		}
		if Header(c, "Origin") != "https://evil.example" {
			t.Errorf("expected the header of the request, got %q", Header(c, "Origin"))
		}
		return nil
	}, request)
}

func TestPathIsThePathOfTheRequest(t *testing.T) {
	e := echo.New()
	e.GET("/status/:slug", func(c *echo.Context) error {
		if c.Path() != "/status/:slug" {
			t.Errorf("expected echo to return the registered route, got %q", c.Path())
		}
		if Path(c) != "/status/clients" {
			t.Errorf("expected the path of the request, got %q", Path(c))
		}
		return nil
	})
	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/status/clients", nil))
}

func TestHeaderValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Add("X-Forwarded-For", "203.0.113.10")
	request.Header.Add("X-Forwarded-For", "198.51.100.20")
	serve(func(c *echo.Context) error {
		if values := HeaderValues(c, "X-Forwarded-For"); len(values) != 2 {
			t.Errorf("expected the two lines of the header, got %v", values)
		}
		return nil
	}, request)
}

func TestRemoteIP(t *testing.T) {
	scenarios := map[string]string{
		"203.0.113.10:4242":        "203.0.113.10",
		"[2001:db8::1]:4242":       "2001:db8::1",
		"[::ffff:203.0.113.10]:80": "203.0.113.10",
		"not-an-address":           "invalid IP",
	}
	for remoteAddr, expected := range scenarios {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = remoteAddr
		// A forged header must never be the address of the client
		request.Header.Set("X-Forwarded-For", "10.0.0.1")
		request.Header.Set("X-Real-Ip", "10.0.0.1")
		if actual := RemoteIPOf(request).String(); actual != expected {
			t.Errorf("%s: expected %s, got %s", remoteAddr, expected, actual)
		}
	}
}

func TestIsTLSIgnoresForwardedHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Ssl", "on")
	serve(func(c *echo.Context) error {
		if c.Scheme() != "https" {
			t.Error("expected echo to trust the forwarded headers, which is why Scheme is never used")
		}
		if IsTLS(c) {
			t.Error("expected a plain connection not to be TLS, whatever the headers say")
		}
		return nil
	}, request)
}

func TestJSONHasNoLineBreak(t *testing.T) {
	recorder := serve(func(c *echo.Context) error {
		return JSON(c, http.StatusCreated, map[string]string{"error": "x"})
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusCreated || recorder.Body.String() != `{"error":"x"}` || recorder.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unexpected answer: %d %q %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"))
	}
}

func TestSend(t *testing.T) {
	recorder := serve(func(c *echo.Context) error { return SendStatus(c, http.StatusNotFound) }, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound || recorder.Body.String() != "Not Found" || recorder.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("unexpected answer: %d %q %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"))
	}
	recorder = serve(func(c *echo.Context) error { return SendStatus(c, http.StatusNoContent) }, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 || len(recorder.Header().Get("Content-Type")) != 0 {
		t.Errorf("expected a 204 without body nor type, got %d %q %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"))
	}
	recorder = serve(func(c *echo.Context) error {
		SetHeader(c, "Content-Type", "image/svg+xml")
		return SendString(c, http.StatusOK, "<svg/>")
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("expected the content type that was set to be kept, got %q", recorder.Header().Get("Content-Type"))
	}
}

func TestVary(t *testing.T) {
	recorder := serve(func(c *echo.Context) error {
		Vary(c, "Accept-Encoding")
		Vary(c, "Authorization")
		Vary(c, "accept-encoding")
		return NoContent(c, http.StatusNoContent)
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if vary := recorder.Header().Get("Vary"); vary != "Accept-Encoding, Authorization" {
		t.Errorf("expected each field once, got %q", vary)
	}
}

// chunked hides the length of the body, as a client that streams it does
type chunked struct{ io.Reader }

func TestBufferBody(t *testing.T) {
	var handled int
	handler := func(c *echo.Context) error {
		handled++
		// A middleware already read the body: the handler still gets all of it, from Body and from the request
		fromRequest, _ := io.ReadAll(c.Request().Body)
		if !bytes.Equal(fromRequest, Body(c)) {
			t.Error("expected the request to hold the body read ahead")
		}
		return SendString(c, http.StatusOK, string(Body(c)))
	}
	reader := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			_ = Body(c)
			return next(c)
		}
	}
	t.Run("small", func(t *testing.T) {
		recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello")), BufferBody, reader)
		if recorder.Code != http.StatusOK || recorder.Body.String() != "hello" {
			t.Errorf("expected the body to reach the handler, got %d %q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("without a body", func(t *testing.T) {
		if recorder := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil), BufferBody); recorder.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", recorder.Code)
		}
	})
	t.Run("exactly the limit", func(t *testing.T) {
		recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", MaximumBodySize))), BufferBody)
		if recorder.Code != http.StatusOK {
			t.Errorf("expected the limit itself to be accepted, got %d", recorder.Code)
		}
	})
	for name, body := range map[string]io.Reader{
		"above the limit with a known length": strings.NewReader(strings.Repeat("a", MaximumBodySize+1)),
		// Without Content-Length, echo's BodyLimit only notices while the body is read: a handler that never reads it
		// would run
		"above the limit without a known length": chunked{strings.NewReader(strings.Repeat("a", MaximumBodySize+1))},
	} {
		t.Run(name, func(t *testing.T) {
			before := handled
			request := httptest.NewRequest(http.MethodPost, "/", body)
			recorder := serve(handler, request, BufferBody)
			if recorder.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("expected 413, got %d", recorder.Code)
			}
			if handled != before {
				t.Error("expected the handler not to run")
			}
		})
	}
	t.Run("without the middleware the body is read when it is asked", func(t *testing.T) {
		recorder := serve(func(c *echo.Context) error {
			first, second := Body(c), Body(c)
			return SendString(c, http.StatusOK, string(first)+"|"+string(second))
		}, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello")))
		if recorder.Body.String() != "hello|hello" {
			t.Errorf("expected the body twice, got %q", recorder.Body.String())
		}
	})
	t.Run("the limit of a route", func(t *testing.T) {
		serve(func(c *echo.Context) error {
			if _, err := BodyWithin(c, 4); err != ErrBodyTooLarge {
				t.Errorf("expected ErrBodyTooLarge, got %v", err)
			}
			if body, err := BodyWithin(c, 5); err != nil || string(body) != "hello" {
				t.Errorf("expected the body, got %q and %v", body, err)
			}
			return nil
		}, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello")), BufferBody)
	})
}

func TestGetAndHead(t *testing.T) {
	e := echo.New()
	var calls int
	GetAndHead(e, "/health", func(c *echo.Context) error {
		calls++
		return SendString(c, http.StatusOK, "up")
	})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(method, "/health", nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", method, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/health", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", recorder.Code)
	}
	if calls != 2 {
		t.Errorf("expected the handler to run for GET and HEAD only, got %d calls", calls)
	}
}

// TestNormalizePath covers the two things echo's own RemoveTrailingSlash leaves behind: a parameter must always reach
// its handler escaped, whatever the escaping of the request was, and a trailing slash must not break an escaped path
func TestNormalizePath(t *testing.T) {
	e := echo.New()
	e.Pre(NormalizePath)
	e.GET("/k/:key/badge", func(c *echo.Context) error {
		return SendString(c, http.StatusOK, c.Param("key")+" "+Path(c))
	})
	e.GET("/", func(c *echo.Context) error { return SendString(c, http.StatusOK, "root") })
	scenarios := map[string]string{
		// Canonical escaping: net/url leaves RawPath empty, and echo would hand the handler "core_100%"
		"/k/core_100%25/badge": "core_100%25 /k/core_100%25/badge",
		// An escaped escape must not be unescaped by the router: the handler would unescape it again into core_api
		"/k/core_%2561pi/badge": "core_%2561pi /k/core_%2561pi/badge",
		"/k/%63ore_api/badge":   "%63ore_api /k/%63ore_api/badge",
		"/k/core%2Fapi/badge":   "core%2Fapi /k/core%2Fapi/badge",
		"/k/plain/badge":        "plain /k/plain/badge",
		// A trailing slash is ignored, with an escaped path too
		"/k/%63ore_api/badge/":   "%63ore_api /k/%63ore_api/badge",
		"/k/core_100%25/badge/":  "core_100%25 /k/core_100%25/badge",
		"/k/plain/badge//":       "plain /k/plain/badge",
		"/k/plain/badge/?a=1%2F": "plain /k/plain/badge",
		"/":                      "root",
	}
	for target, expected := range scenarios {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != expected {
			t.Errorf("%s: expected %q, got %d %q", target, expected, recorder.Code, recorder.Body.String())
		}
	}
	// An escaped slash is not a separator
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/k/a%2Fb%2Fbadge", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("expected an escaped slash not to split the path, got %d", recorder.Code)
	}
}

// TestQuery covers the pairs net/url drops: for the push a missing status means "up", so dropping ?status=down;x would
// record a success
func TestQuery(t *testing.T) {
	scenarios := []struct {
		query, name, first, last string
		present                  bool
	}{
		{query: "status=down;maintenance&msg=disk;full", name: "status", first: "down;maintenance", last: "down;maintenance", present: true},
		{query: "status=down;maintenance&msg=disk;full", name: "msg", first: "disk;full", last: "disk;full", present: true},
		{query: "ping=%zz&status=down", name: "ping", first: "%zz", last: "%zz", present: true},
		{query: "ping=%zz&status=down", name: "status", first: "down", last: "down", present: true},
		{query: "msg=Disk%20full+now", name: "msg", first: "Disk full now", last: "Disk full now", present: true},
		{query: "msg=100%25", name: "msg", first: "100%", last: "100%", present: true},
		{query: "msg=50%", name: "msg", first: "50%", last: "50%", present: true},
		{query: "success=true&success=invalid", name: "success", first: "true", last: "invalid", present: true},
		{query: "msg=", name: "msg", first: "", last: "", present: true},
		{query: "flag", name: "flag", first: "", last: "", present: true},
		{query: "a=1", name: "missing", first: "", last: "", present: false},
		{query: "", name: "missing", first: "", last: "", present: false},
		{query: "m%73g=x", name: "msg", first: "x", last: "x", present: true},
	}
	for _, scenario := range scenarios {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.URL.RawQuery = scenario.query
		c := echo.New().NewContext(request, httptest.NewRecorder())
		if first := Query(c, scenario.name); first != scenario.first {
			t.Errorf("%q %s: expected the first value %q, got %q", scenario.query, scenario.name, scenario.first, first)
		}
		if last, present := QueryLast(c, scenario.name); last != scenario.last || present != scenario.present {
			t.Errorf("%q %s: expected the last value %q (present=%v), got %q (present=%v)", scenario.query, scenario.name, scenario.last, scenario.present, last, present)
		}
	}
}
