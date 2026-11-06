// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package ui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	static "github.com/jniltinho/go-uptime/v7/web"
)

// ThemesPath is the path, in the embedded files, of the registry of the themes written by the build of the frontend
// from web/app/src/themes/themes.json (fork)
const ThemesPath = static.RootPath + "/themes.json"

// ThemeInfo is a theme of the registry (fork)
type ThemeInfo struct {
	// ID is the value of the theme cookie, of ui.default-theme and of the data-theme attribute of <html>
	ID string `json:"id"`

	// Label is the name of the theme in the theme selector
	Label string `json:"label"`

	// Scheme is light or dark, the value of the data-scheme attribute of <html>, which the dark: variant follows
	Scheme string `json:"scheme"`

	// ClassName is the class of <html> of the 7.x series ("dark", "theme-bio" or empty), kept for the custom CSS
	// written against it
	ClassName string `json:"className"`

	// ThemeColor is the theme-color of the browser for the theme
	ThemeColor string `json:"themeColor"`
}

var (
	themesOnce sync.Once
	themes     []ThemeInfo
	themesByID map[string]ThemeInfo
)

// fallbackThemes is the registry used when the embedded one is missing or invalid, which only happens with a broken
// build: the three themes of v7.3.0, so that the server still starts
var fallbackThemes = []ThemeInfo{
	{ID: ThemeLight, Label: "Light", Scheme: "light", ClassName: "", ThemeColor: "#f7f9fb"},
	{ID: ThemeDark, Label: "Dark", Scheme: "dark", ClassName: "dark", ThemeColor: "#030712"},
	{ID: ThemeBio, Label: "Bio", Scheme: "light", ClassName: "theme-bio", ThemeColor: "#f2f8fa"},
}

// Themes returns the themes of the registry, in the order of the theme selector
func Themes() []ThemeInfo {
	loadThemes()
	return append([]ThemeInfo(nil), themes...)
}

func loadThemes() {
	themesOnce.Do(func() {
		themes = fallbackThemes
		if data, err := fs.ReadFile(static.FileSystem, ThemesPath); err == nil {
			var registry []ThemeInfo
			if json.Unmarshal(data, &registry) == nil && validRegistry(registry) {
				themes = registry
			}
		}
		themesByID = make(map[string]ThemeInfo, len(themes))
		for _, theme := range themes {
			themesByID[theme.ID] = theme
		}
	})
}

// validRegistry returns whether every theme has an id, a scheme and a theme colour, the ids are unique and the light
// theme, the fallback of an unknown value, exists
func validRegistry(registry []ThemeInfo) bool {
	seen := make(map[string]bool, len(registry))
	for _, theme := range registry {
		if theme.ID == "" || seen[theme.ID] || (theme.Scheme != "light" && theme.Scheme != "dark") || theme.ThemeColor == "" {
			return false
		}
		seen[theme.ID] = true
	}
	return seen[ThemeLight]
}

// themeInfo returns the theme with the given id, or the light theme for a value that is not a theme
func themeInfo(id string) ThemeInfo {
	loadThemes()
	if theme, ok := themesByID[id]; ok {
		return theme
	}
	return themesByID[ThemeLight]
}

// themeIDs returns the ids of the themes, joined for an error message
func themeIDs() string {
	loadThemes()
	ids := make([]string, 0, len(themes))
	for _, theme := range themes {
		ids = append(ids, theme.ID)
	}
	return strings.Join(ids, ", ")
}

// invalidDefaultTheme returns ErrInvalidDefaultTheme with the value and the accepted themes
func invalidDefaultTheme(value string) error {
	return fmt.Errorf("%w: %q is not one of %s", ErrInvalidDefaultTheme, value, themeIDs())
}
