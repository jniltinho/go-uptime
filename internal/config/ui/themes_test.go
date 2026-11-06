// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestThemesAreTheRegistryOfTheFrontend makes sure the registry embedded by the build (static/themes.json) is the one
// of web/app/src/themes/themes.json: a theme added to the frontend without a build would not be known by the server.
func TestThemesAreTheRegistryOfTheFrontend(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "app", "src", "themes", "themes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var source []ThemeInfo
	if err = json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(Themes(), source) {
		t.Errorf("expected the embedded registry to be the one of the frontend (run make frontend-build):\nembedded %+v\nsource   %+v", Themes(), source)
	}
	if !validRegistry(source) {
		t.Error("expected the registry of the frontend to be valid: unique ids, light or dark schemes, theme colours and a light theme")
	}
}

func TestDefaultThemeMustBeInTheRegistry(t *testing.T) {
	for _, theme := range Themes() {
		cfg := GetDefaultConfig()
		cfg.DefaultTheme = theme.ID
		if err := cfg.ValidateAndSetDefaults(); err != nil {
			t.Errorf("%s: expected a theme of the registry to be valid, got %v", theme.ID, err)
		}
	}
	cfg := GetDefaultConfig()
	cfg.DefaultTheme = "grafana"
	if err := cfg.ValidateAndSetDefaults(); !errors.Is(err, ErrInvalidDefaultTheme) {
		t.Errorf("expected ErrInvalidDefaultTheme for a theme that is not in the registry, got %v", err)
	}
	if ThemeScheme("unknown") != "light" || ThemeColor("unknown") != ThemeColor(ThemeLight) {
		t.Error("expected a value that is not a theme to fall back on the light theme")
	}
}
