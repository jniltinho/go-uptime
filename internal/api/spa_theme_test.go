// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/ui"
)

// themeCases is web/app/src/utils/theme.cases.json, the table shared with the inline script of index.html and with
// theme.js: the three implementations of the theme rule follow the same cases.
type themeCases struct {
	Themes map[string]struct {
		Class      string `json:"class"`
		ThemeColor string `json:"themeColor"`
		Scheme     string `json:"scheme"`
	} `json:"themes"`
	Cases []struct {
		Cookie      string  `json:"cookie"`
		Default     *string `json:"default"`
		Theme       string  `json:"theme"`
		Class       string  `json:"class"`
		ThemeColor  string  `json:"themeColor"`
		BrowserOnly bool    `json:"browserOnly"`
	} `json:"cases"`
}

func loadThemeCases(t *testing.T) themeCases {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "app", "src", "utils", "theme.cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases themeCases
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Cases) == 0 {
		t.Fatal("expected cases in theme.cases.json")
	}
	return cases
}

func TestThemeTable(t *testing.T) {
	cases := loadThemeCases(t)
	// Fork: the themes of the server come from the registry embedded by the build (static/themes.json)
	if len(cases.Themes) != len(ui.Themes()) {
		t.Fatalf("expected the %d themes of the registry, got %d", len(ui.Themes()), len(cases.Themes))
	}
	for theme, expected := range cases.Themes {
		if !ui.IsTheme(theme) || ui.ThemeClass(theme) != expected.Class || ui.ThemeColor(theme) != expected.ThemeColor || ui.ThemeScheme(theme) != expected.Scheme {
			t.Errorf("%s: expected class %q, theme-color %q and scheme %q, got %q, %q and %q", theme, expected.Class, expected.ThemeColor, expected.Scheme, ui.ThemeClass(theme), ui.ThemeColor(theme), ui.ThemeScheme(theme))
		}
	}
}

// TestSPATheme renders the two HTML pages of the interface for every case the server can see: the HTML already has the
// class and the theme-color of the theme, only one theme class at a time, and the headers say that it varies with the
// cookie.
func TestSPATheme(t *testing.T) {
	cases := loadThemeCases(t)
	for _, item := range cases.Cases {
		if item.BrowserOnly || item.Default == nil {
			continue
		}
		uiConfig := ui.GetDefaultConfig()
		uiConfig.DefaultTheme = *item.Default
		if err := uiConfig.ValidateAndSetDefaults(); err != nil {
			t.Fatal(err)
		}
		api := New(&config.Config{UI: uiConfig})
		router := api.Router()
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			request := httptest.NewRequest(method, "/", http.NoBody)
			if len(item.Cookie) > 0 {
				request.AddCookie(&http.Cookie{Name: "theme", Value: item.Cookie})
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("cookie=%q default=%q: expected 200, got %d", item.Cookie, *item.Default, recorder.Code)
			}
			if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-cache" {
				t.Errorf("%s: expected Cache-Control: no-cache, got %q", method, cacheControl)
			}
			if vary := strings.Join(recorder.Header().Values("Vary"), ", "); !strings.Contains(vary, "Cookie") {
				t.Errorf("%s: expected Vary to include Cookie, got %q", method, vary)
			}
			if method == http.MethodHead {
				continue
			}
			body := recorder.Body.String()
			htmlElement := `<html lang="en" class="` + item.Class + `" data-theme="` + item.Theme + `" data-scheme="` + cases.Themes[item.Theme].Scheme + `" data-default-theme="` + *item.Default + `">`
			if !strings.Contains(body, htmlElement) {
				t.Errorf("cookie=%q default=%q: expected the class %q and the default theme in <html>, got %s", item.Cookie, *item.Default, item.Class, body[:min(len(body), 160)])
			}
			if !strings.Contains(body, `<meta name="theme-color" content="`+item.ThemeColor+`"`) {
				t.Errorf("cookie=%q default=%q: expected the theme-color %s", item.Cookie, *item.Default, item.ThemeColor)
			}
		}
	}
}
