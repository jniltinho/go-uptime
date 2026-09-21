// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/ui"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/labstack/echo/v5"
)

func TestSinglePageApplication(t *testing.T) {
	defer store.Get().Clear()
	defer cache.Clear()
	cfg := &config.Config{
		Metrics: true,
		Endpoints: []*endpoint.Endpoint{
			{
				Name:  "frontend",
				Group: "core",
			},
			{
				Name:  "backend",
				Group: "core",
			},
		},
		UI: &ui.Config{
			Title: "example-title",
		},
	}
	watchdog.UpdateEndpointStatus(cfg.Endpoints[0], &endpoint.Result{Success: true, Duration: time.Millisecond, Timestamp: time.Now()})
	watchdog.UpdateEndpointStatus(cfg.Endpoints[1], &endpoint.Result{Success: false, Duration: time.Second, Timestamp: time.Now()})
	api := New(cfg)
	router := api.Router()
	type Scenario struct {
		Name              string
		Path              string
		Gzip              bool
		CookieDarkMode    bool
		UIDarkMode        bool
		ExpectedCode      int
		ExpectedDarkTheme bool
	}
	scenarios := []Scenario{
		{
			Name:              "frontend-home",
			Path:              "/",
			CookieDarkMode:    true,
			UIDarkMode:        false,
			ExpectedDarkTheme: true,
			ExpectedCode:      200,
		},
		{
			Name:              "frontend-endpoint-light",
			Path:              "/endpoints/core_frontend",
			CookieDarkMode:    false,
			UIDarkMode:        false,
			ExpectedDarkTheme: false,
			ExpectedCode:      200,
		},
		{
			Name:              "frontend-endpoint-dark",
			Path:              "/endpoints/core_frontend",
			CookieDarkMode:    false,
			UIDarkMode:        true,
			ExpectedDarkTheme: true,
			ExpectedCode:      200,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			cfg.UI.DarkMode = &scenario.UIDarkMode
			request := httptest.NewRequest("GET", scenario.Path, http.NoBody)
			if scenario.Gzip {
				request.Header.Set("Accept-Encoding", "gzip")
			}
			if scenario.CookieDarkMode {
				request.Header.Set("Cookie", "theme=dark")
			}
			response, err := testHTTP(router, request)
			if err != nil {
				return
			}
			defer response.Body.Close()
			if response.StatusCode != scenario.ExpectedCode {
				t.Errorf("%s %s should have returned %d, but returned %d instead", request.Method, request.URL, scenario.ExpectedCode, response.StatusCode)
			}
			body, _ := io.ReadAll(response.Body)
			strBody := string(body)
			if !strings.Contains(strBody, cfg.UI.Title) {
				t.Errorf("%s %s should have contained the title", request.Method, request.URL)
			}
			if scenario.ExpectedDarkTheme && !strings.Contains(strBody, "class=\"dark\"") {
				t.Errorf("%s %s should have responded with dark mode headers", request.Method, request.URL)
			}
			if !scenario.ExpectedDarkTheme && strings.Contains(strBody, "class=\"dark\"") {
				t.Errorf("%s %s should not have responded with dark mode headers", request.Method, request.URL)
			}
		})
	}
}

// The theme of the HTML follows a valid theme cookie or ui.dark-mode, dark by default, on the dashboard and on the
// public status pages (fork). data-default-theme carries the identifier of the default theme, "light" and no longer an
// empty string for the light one; TestSPATheme covers ui.default-theme and the bio theme with the shared table.
func TestSinglePageApplication_DefaultTheme(t *testing.T) {
	for _, path := range []string{"/", "/login", "/status/services"} {
		for name, scenario := range map[string]struct {
			darkMode      *bool
			cookie        string
			expectedClass string
			expectedDflt  string
			expectedColor string
		}{
			"default":            {darkMode: nil, cookie: "", expectedClass: "dark", expectedDflt: "dark", expectedColor: "#030712"},
			"light-config":       {darkMode: boolPointer(false), cookie: "", expectedClass: "", expectedDflt: "light", expectedColor: "#f7f9fb"},
			"light-cookie":       {darkMode: nil, cookie: "theme=light", expectedClass: "", expectedDflt: "dark", expectedColor: "#f7f9fb"},
			"dark-cookie":        {darkMode: boolPointer(false), cookie: "theme=dark", expectedClass: "dark", expectedDflt: "light", expectedColor: "#030712"},
			"invalid-cookie":     {darkMode: nil, cookie: "theme=foo", expectedClass: "dark", expectedDflt: "dark", expectedColor: "#030712"},
			"invalid-cookie-off": {darkMode: boolPointer(false), cookie: "theme=foo", expectedClass: "", expectedDflt: "light", expectedColor: "#f7f9fb"},
		} {
			t.Run(path+"/"+name, func(t *testing.T) {
				uiConfig := ui.GetDefaultConfig()
				uiConfig.DarkMode = scenario.darkMode
				request := httptest.NewRequest(http.MethodGet, path, http.NoBody)
				if len(scenario.cookie) > 0 {
					request.Header.Set("Cookie", scenario.cookie)
				}
				response, err := testHTTP(spaTestApp(path, uiConfig), request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, _ := io.ReadAll(response.Body)
				html := string(body)
				if !strings.Contains(html, `<html lang="en" class="`+scenario.expectedClass+`" data-default-theme="`+scenario.expectedDflt+`">`) {
					t.Errorf("unexpected html element for class=%q default=%q: %s", scenario.expectedClass, scenario.expectedDflt, html[:min(len(html), 200)])
				}
				if !strings.Contains(html, `<meta name="theme-color" content="`+scenario.expectedColor+`"`) {
					t.Errorf("expected the theme color %s", scenario.expectedColor)
				}
			})
		}
	}
}

func boolPointer(value bool) *bool {
	return &value
}

// spaTestApp serves the single page application at path, like the dashboard or like the public status pages
func spaTestApp(path string, uiConfig *ui.Config) *echo.Echo {
	app := echo.New()
	if strings.HasPrefix(path, "/status/") {
		app.GET(path, renderSPA(uiConfig, setPublicHeaders))
	} else {
		app.GET(path, SinglePageApplication(uiConfig))
	}
	return app
}
