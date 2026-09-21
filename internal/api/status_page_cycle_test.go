// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
)

// The routes of the status pages are created on every start and reload: they must not leave goroutines behind
func TestStatusPage_NoGoroutineLeftAcrossCycles(t *testing.T) {
	newStatusPageTestApp(t, nil, statusPagesTestConfig(true, 5))
	t.Cleanup(func() { statuspage.ConfigureLimiter(0) })
	statusPages := statusPagesTestConfig(true, 5)
	if err := statusPages.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	baseline := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		app := New(&config.Config{StatusPages: statusPages}).Router()
		doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/missing")
		doStatusPageRequest(t, app, http.MethodGet, "/api/v1/status-pages/infra")
	}
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	if goroutines := runtime.NumGoroutine(); goroutines > baseline+2 {
		t.Errorf("expected no goroutine to be left behind by 20 cycles, got %d goroutines (baseline %d)", goroutines, baseline)
	}
}
