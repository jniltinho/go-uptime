// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package ui

import (
	"errors"
	"strconv"
	"testing"
)

func TestConfig_ValidateAndSetDefaults(t *testing.T) {
	t.Run("empty-config", func(t *testing.T) {
		cfg := &Config{
			Title:               "",
			Description:         "",
			DashboardHeading:    "",
			DashboardSubheading: "",
			Header:              "",
			Logo:                "",
			Link:                "",
		}
		if err := cfg.ValidateAndSetDefaults(); err != nil {
			t.Error("expected no error, got", err.Error())
		}
		if cfg.Title != defaultTitle {
			t.Errorf("expected title to be %s, got %s", defaultTitle, cfg.Title)
		}
		if cfg.Description != defaultDescription {
			t.Errorf("expected description to be %s, got %s", defaultDescription, cfg.Description)
		}
		if cfg.DashboardHeading != defaultDashboardHeading {
			t.Errorf("expected DashboardHeading to be %s, got %s", defaultDashboardHeading, cfg.DashboardHeading)
		}
		if cfg.DashboardSubheading != defaultDashboardSubheading {
			t.Errorf("expected DashboardSubheading to be %s, got %s", defaultDashboardSubheading, cfg.DashboardSubheading)
		}
		if cfg.Header != defaultHeader {
			t.Errorf("expected header to be %s, got %s", defaultHeader, cfg.Header)
		}
		if cfg.DefaultSortBy != defaultSortBy {
			t.Errorf("expected defaultSortBy to be %s, got %s", defaultSortBy, cfg.DefaultSortBy)
		}
		if cfg.DefaultFilterBy != defaultFilterBy {
			t.Errorf("expected defaultFilterBy to be %s, got %s", defaultFilterBy, cfg.DefaultFilterBy)
		}
		if cfg.Favicon.Default != defaultFavicon {
			t.Errorf("expected favicon to be %s, got %s", defaultFavicon, cfg.Favicon.Default)
		}
		if cfg.Favicon.Size16x16 != defaultFavicon16 {
			t.Errorf("expected favicon to be %s, got %s", defaultFavicon16, cfg.Favicon.Size16x16)
		}
		if cfg.Favicon.Size32x32 != defaultFavicon32 {
			t.Errorf("expected favicon to be %s, got %s", defaultFavicon32, cfg.Favicon.Size32x32)
		}
		if cfg.LoginSubtitle != defaultLoginSubtitle {
			t.Errorf("expected LoginSubtitle to be %s, got %s", defaultLoginSubtitle, cfg.LoginSubtitle)
		}
	})
	t.Run("custom-values", func(t *testing.T) {
		cfg := &Config{
			Title:               "Custom Title",
			Description:         "Custom Description",
			DashboardHeading:    "Production Status",
			DashboardSubheading: "Monitor all production endpoints",
			Header:              "My Company",
			Logo:                "https://example.com/logo.png",
			Link:                "https://example.com",
			DefaultSortBy:       "health",
			DefaultFilterBy:     "failing",
			LoginSubtitle:       "Welcome",
		}
		if err := cfg.ValidateAndSetDefaults(); err != nil {
			t.Error("expected no error, got", err.Error())
		}
		if cfg.Title != "Custom Title" {
			t.Errorf("expected title to be preserved, got %s", cfg.Title)
		}
		if cfg.Description != "Custom Description" {
			t.Errorf("expected description to be preserved, got %s", cfg.Description)
		}
		if cfg.DashboardHeading != "Production Status" {
			t.Errorf("expected DashboardHeading to be preserved, got %s", cfg.DashboardHeading)
		}
		if cfg.DashboardSubheading != "Monitor all production endpoints" {
			t.Errorf("expected DashboardSubheading to be preserved, got %s", cfg.DashboardSubheading)
		}
		if cfg.Header != "My Company" {
			t.Errorf("expected header to be preserved, got %s", cfg.Header)
		}
		if cfg.Logo != "https://example.com/logo.png" {
			t.Errorf("expected logo to be preserved, got %s", cfg.Logo)
		}
		if cfg.Link != "https://example.com" {
			t.Errorf("expected link to be preserved, got %s", cfg.Link)
		}
		if cfg.DefaultSortBy != "health" {
			t.Errorf("expected defaultSortBy to be preserved, got %s", cfg.DefaultSortBy)
		}
		if cfg.DefaultFilterBy != "failing" {
			t.Errorf("expected defaultFilterBy to be preserved, got %s", cfg.DefaultFilterBy)
		}
		if cfg.LoginSubtitle != "Welcome" {
			t.Errorf("expected LoginSubtitle to be preserved, got %s", cfg.LoginSubtitle)
		}
	})
	t.Run("partial-custom-values", func(t *testing.T) {
		cfg := &Config{
			Title:               "Custom Title",
			DashboardHeading:    "My Dashboard",
			Header:              "",
			DashboardSubheading: "",
		}
		if err := cfg.ValidateAndSetDefaults(); err != nil {
			t.Error("expected no error, got", err.Error())
		}
		if cfg.Title != "Custom Title" {
			t.Errorf("expected custom title to be preserved, got %s", cfg.Title)
		}
		if cfg.DashboardHeading != "My Dashboard" {
			t.Errorf("expected custom DashboardHeading to be preserved, got %s", cfg.DashboardHeading)
		}
		if cfg.DashboardSubheading != defaultDashboardSubheading {
			t.Errorf("expected DashboardSubheading to use default, got %s", cfg.DashboardSubheading)
		}
		if cfg.Header != defaultHeader {
			t.Errorf("expected header to use default, got %s", cfg.Header)
		}
		if cfg.Description != defaultDescription {
			t.Errorf("expected description to use default, got %s", cfg.Description)
		}
		if cfg.LoginSubtitle != defaultLoginSubtitle {
			t.Errorf("expected LoginSubtitle to use default, got %s", cfg.LoginSubtitle)
		}
	})
}

func TestButton_Validate(t *testing.T) {
	scenarios := []struct {
		Name, Link    string
		ExpectedError error
	}{
		{
			Name:          "",
			Link:          "",
			ExpectedError: ErrButtonValidationFailed,
		},
		{
			Name:          "",
			Link:          "link",
			ExpectedError: ErrButtonValidationFailed,
		},
		{
			Name:          "name",
			Link:          "",
			ExpectedError: ErrButtonValidationFailed,
		},
		{
			Name:          "name",
			Link:          "link",
			ExpectedError: nil,
		},
	}
	for i, scenario := range scenarios {
		t.Run(strconv.Itoa(i)+"_"+scenario.Name+"_"+scenario.Link, func(t *testing.T) {
			button := &Button{
				Name: scenario.Name,
				Link: scenario.Link,
			}
			if err := button.Validate(); err != scenario.ExpectedError {
				t.Errorf("expected error %v, got %v", scenario.ExpectedError, err)
			}
		})
	}
}

func TestGetDefaultConfig(t *testing.T) {
	defaultConfig := GetDefaultConfig()
	if defaultConfig.Title != defaultTitle {
		t.Error("expected GetDefaultConfig() to return defaultTitle, got", defaultConfig.Title)
	}
	if defaultConfig.DashboardHeading != defaultDashboardHeading {
		t.Error("expected GetDefaultConfig() to return defaultDashboardHeading, got", defaultConfig.DashboardHeading)
	}
	if defaultConfig.DashboardSubheading != defaultDashboardSubheading {
		t.Error("expected GetDefaultConfig() to return defaultDashboardSubheading, got", defaultConfig.DashboardSubheading)
	}
	if defaultConfig.Logo != defaultLogo {
		t.Error("expected GetDefaultConfig() to return defaultLogo, got", defaultConfig.Logo)
	}
	if defaultConfig.DefaultSortBy != defaultSortBy {
		t.Error("expected GetDefaultConfig() to return defaultSortBy, got", defaultConfig.DefaultSortBy)
	}
	if defaultConfig.DefaultFilterBy != defaultFilterBy {
		t.Error("expected GetDefaultConfig() to return defaultFilterBy, got", defaultConfig.DefaultFilterBy)
	}
	if defaultConfig.LoginSubtitle != defaultLoginSubtitle {
		t.Error("expected GetDefaultConfig() to return defaultLoginSubtitle, got", defaultConfig.LoginSubtitle)
	}
}

func TestConfig_ValidateAndSetDefaults_DefaultSortBy(t *testing.T) {
	scenarios := []struct {
		Name          string
		DefaultSortBy string
		ExpectedError error
		ExpectedValue string
	}{
		{
			Name:          "EmptyDefaultSortBy",
			DefaultSortBy: "",
			ExpectedError: nil,
			ExpectedValue: defaultSortBy,
		},
		{
			Name:          "ValidDefaultSortBy_name",
			DefaultSortBy: "name",
			ExpectedError: nil,
			ExpectedValue: "name",
		},
		{
			Name:          "ValidDefaultSortBy_group",
			DefaultSortBy: "group",
			ExpectedError: nil,
			ExpectedValue: "group",
		},
		{
			Name:          "ValidDefaultSortBy_health",
			DefaultSortBy: "health",
			ExpectedError: nil,
			ExpectedValue: "health",
		},
		{
			Name:          "InvalidDefaultSortBy",
			DefaultSortBy: "invalid",
			ExpectedError: ErrInvalidDefaultSortBy,
			ExpectedValue: "invalid",
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			cfg := &Config{DefaultSortBy: scenario.DefaultSortBy}
			err := cfg.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.ExpectedError) {
				t.Errorf("expected error %v, got %v", scenario.ExpectedError, err)
			}
			if cfg.DefaultSortBy != scenario.ExpectedValue {
				t.Errorf("expected DefaultSortBy to be %s, got %s", scenario.ExpectedValue, cfg.DefaultSortBy)
			}
		})
	}
}

func TestConfig_ValidateAndSetDefaults_DefaultFilterBy(t *testing.T) {
	scenarios := []struct {
		Name            string
		DefaultFilterBy string
		ExpectedError   error
		ExpectedValue   string
	}{
		{
			Name:            "EmptyDefaultFilterBy",
			DefaultFilterBy: "",
			ExpectedError:   nil,
			ExpectedValue:   defaultFilterBy,
		},
		{
			Name:            "ValidDefaultFilterBy_none",
			DefaultFilterBy: "none",
			ExpectedError:   nil,
			ExpectedValue:   "none",
		},
		{
			Name:            "ValidDefaultFilterBy_failing",
			DefaultFilterBy: "failing",
			ExpectedError:   nil,
			ExpectedValue:   "failing",
		},
		{
			Name:            "ValidDefaultFilterBy_unstable",
			DefaultFilterBy: "unstable",
			ExpectedError:   nil,
			ExpectedValue:   "unstable",
		},
		{
			Name:            "InvalidDefaultFilterBy",
			DefaultFilterBy: "invalid",
			ExpectedError:   ErrInvalidDefaultFilterBy,
			ExpectedValue:   "invalid",
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			cfg := &Config{DefaultFilterBy: scenario.DefaultFilterBy}
			err := cfg.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.ExpectedError) {
				t.Errorf("expected error %v, got %v", scenario.ExpectedError, err)
			}
			if cfg.DefaultFilterBy != scenario.ExpectedValue {
				t.Errorf("expected DefaultFilterBy to be %s, got %s", scenario.ExpectedValue, cfg.DefaultFilterBy)
			}
		})
	}
}

// TestConfig_DefaultTheme covers default-theme: the three themes, an invalid value, its precedence over dark-mode and
// dark-mode alone, which keeps deciding between dark and light as before.
func TestConfig_DefaultTheme(t *testing.T) {
	enabled, disabled := true, false
	for name, scenario := range map[string]struct {
		defaultTheme string
		darkMode     *bool
		expected     string
		expectedErr  error
	}{
		"nothing-set":           {expected: ThemeDark},
		"dark-mode-off":         {darkMode: &disabled, expected: ThemeLight},
		"dark-mode-on":          {darkMode: &enabled, expected: ThemeDark},
		"bio":                   {defaultTheme: ThemeBio, expected: ThemeBio},
		"light":                 {defaultTheme: ThemeLight, expected: ThemeLight},
		"dark":                  {defaultTheme: ThemeDark, expected: ThemeDark},
		"light-beats-dark-mode": {defaultTheme: ThemeLight, darkMode: &enabled, expected: ThemeLight},
		"bio-beats-dark-mode":   {defaultTheme: ThemeBio, darkMode: &disabled, expected: ThemeBio},
		"invalid":               {defaultTheme: "blue", expectedErr: ErrInvalidDefaultTheme},
		"class-is-not-a-theme":  {defaultTheme: "theme-bio", expectedErr: ErrInvalidDefaultTheme},
		"case-sensitive":        {defaultTheme: "Bio", expectedErr: ErrInvalidDefaultTheme},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := GetDefaultConfig()
			cfg.DefaultTheme, cfg.DarkMode = scenario.defaultTheme, scenario.darkMode
			err := cfg.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.expectedErr) {
				t.Fatalf("expected error %v, got %v", scenario.expectedErr, err)
			}
			if err == nil && cfg.Theme() != scenario.expected {
				t.Errorf("expected the default theme %s, got %s", scenario.expected, cfg.Theme())
			}
		})
	}
}

// TestThemeClassAndColor keeps the classes mutually exclusive by construction: one class per theme, none for light,
// and a value that is not a theme falls back to the light theme.
func TestThemeClassAndColor(t *testing.T) {
	classes := map[string]bool{}
	for _, theme := range []string{ThemeDark, ThemeLight, ThemeBio} {
		if !IsTheme(theme) {
			t.Errorf("expected %s to be a theme", theme)
		}
		if class := ThemeClass(theme); len(class) > 0 {
			if classes[class] {
				t.Errorf("class %s is used by two themes", class)
			}
			classes[class] = true
		}
	}
	if ThemeClass(ThemeLight) != "" || ThemeClass("blue") != "" || ThemeColor("blue") != ThemeColor(ThemeLight) || IsTheme("") {
		t.Error("expected the light theme to have no class, and a value that is not a theme to fall back to it")
	}
}

// TestConfig_Logo covers the default logo: an empty ui.logo means the logo embedded in the binary, "none" means no logo
// at all, and a logo of the user is kept.
func TestConfig_Logo(t *testing.T) {
	for name, scenario := range map[string]struct {
		logo     string
		expected string
	}{
		"not-set":          {logo: "", expected: "/logo-192x192.png"},
		"only-spaces":      {logo: "   ", expected: "/logo-192x192.png"},
		"spaces-around":    {logo: " /logo-512x512.png ", expected: "/logo-512x512.png"},
		"none":             {logo: "none", expected: ""},
		"none-uppercase":   {logo: " None ", expected: ""},
		"url-of-the-user":  {logo: "https://example.org/logo.svg", expected: "https://example.org/logo.svg"},
		"path-of-the-user": {logo: "/logo-512x512.png", expected: "/logo-512x512.png"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{Logo: scenario.logo}
			if err := cfg.ValidateAndSetDefaults(); err != nil {
				t.Fatal(err)
			}
			if cfg.Logo != scenario.expected {
				t.Errorf("expected the logo %q, got %q", scenario.expected, cfg.Logo)
			}
		})
	}
	if GetDefaultConfig().Logo != "/logo-192x192.png" {
		t.Errorf("expected the default configuration to have the embedded logo, got %q", GetDefaultConfig().Logo)
	}
}
