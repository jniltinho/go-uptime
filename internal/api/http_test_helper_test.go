// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/labstack/echo/v5"
)

// testHTTP serves a request with the router, in memory, and returns the answer. It replaces the Test method Fiber had.
func testHTTP(router *echo.Echo, request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
