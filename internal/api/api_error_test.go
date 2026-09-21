// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// TestHTTPErrorHandler_NeverAnswersWithTheError makes sure a panic or an unexpected error never reaches the client:
// echo's Recover wraps a panic in an error whose text is the whole stack trace
func TestHTTPErrorHandler_NeverAnswersWithTheError(t *testing.T) {
	router := echo.NewWithConfig(echo.Config{HTTPErrorHandler: httpErrorHandler})
	router.Use(middleware.Recover())
	router.GET("/panic", func(*echo.Context) error { panic("secret value in a panic") })
	router.GET("/error", func(*echo.Context) error { return errors.New("dial tcp 10.0.0.5:5432: secret of the storage") })
	for _, path := range []string{"/panic", "/error"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		body := recorder.Body.String()
		if recorder.Code != http.StatusInternalServerError || body != "Internal Server Error" {
			t.Errorf("%s: expected a plain 500, got %d %q", path, recorder.Code, body)
		}
		for _, leak := range []string{"secret", "PANIC", "goroutine", ".go:", "10.0.0.5"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s: the answer leaks %q: %q", path, leak, body)
			}
		}
	}
	// The errors of the router keep the bodies clients already know
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if recorder.Code != http.StatusNotFound || recorder.Body.String() != "Cannot GET /missing" {
		t.Errorf("expected the 404 of the router, got %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/error", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", recorder.Code)
	}
}
