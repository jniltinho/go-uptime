// Package ui models the ui section of the YAML configuration: the texts, logo, favicons, buttons, custom CSS, theme
// and default sorting and filtering of the dashboard. It validates the section, applies its defaults and makes sure
// that the index template renders with it.
package ui

import (
	"bytes"
	"errors"
	"html/template"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	static "github.com/jniltinho/go-uptime/v7/web"
)

const (
	defaultTitle               = "Health Dashboard | Status"
	defaultDescription         = "Automated status page that lets you monitor your applications and configure alerts to notify you if there's an issue"
	defaultHeader              = "Status"
	defaultDashboardHeading    = "Health Dashboard"
	defaultDashboardSubheading = "Monitor the health of your endpoints in real-time"
	// defaultLogo is the logo embedded in the binary (web/static), shown when ui.logo is not set (fork)
	defaultLogo = "/logo-192x192.png"
	// NoLogo is the value of ui.logo that shows no logo at all: an empty value means the default one
	NoLogo               = "none"
	defaultLink          = ""
	defaultFavicon       = "/favicon.ico"
	defaultFavicon16     = "/favicon-16x16.png"
	defaultFavicon32     = "/favicon-32x32.png"
	defaultCustomCSS     = ""
	defaultSortBy        = "name"
	defaultFilterBy      = "none"
	defaultLoginSubtitle = "System Monitoring Dashboard"
)

var (
	defaultDarkMode = true

	// ErrButtonValidationFailed is returned by Button.Validate when a button has no name or no link.
	ErrButtonValidationFailed = errors.New("invalid button configuration: missing required name or link")

	// ErrInvalidDefaultTheme is returned when default-theme is not one of the themes of the interface.
	ErrInvalidDefaultTheme = errors.New("invalid default-theme value: must be 'dark', 'light', or 'bio'")

	// ErrInvalidDefaultSortBy is returned by Config.ValidateAndSetDefaults when default-sort-by is set to something
	// other than name, group or health.
	ErrInvalidDefaultSortBy = errors.New("invalid default-sort-by value: must be 'name', 'group', or 'health'")

	// ErrInvalidDefaultFilterBy is returned by Config.ValidateAndSetDefaults when default-filter-by is set to
	// something other than none, failing or unstable.
	ErrInvalidDefaultFilterBy = errors.New("invalid default-filter-by value: must be 'none', 'failing', or 'unstable'")
)

// Config is the configuration for the UI of Gatus
type Config struct {
	Title               string   `yaml:"title,omitempty"`                // Title of the page
	Description         string   `yaml:"description,omitempty"`          // Meta description of the page
	DashboardHeading    string   `yaml:"dashboard-heading,omitempty"`    // Dashboard Title between header and endpoints
	DashboardSubheading string   `yaml:"dashboard-subheading,omitempty"` // Dashboard Description between header and endpoints
	Header              string   `yaml:"header,omitempty"`               // Header is the text at the top of the page
	Logo                string   `yaml:"logo,omitempty"`                 // Logo shown in the headers and on the login screen: a URL, empty for the embedded one, or "none" for no logo
	Link                string   `yaml:"link,omitempty"`                 // Link to open when clicking on the logo
	Favicon             Favicon  `yaml:"favicon,omitempty"`              // Favourite icon to display in web browser tab or address bar
	Buttons             []Button `yaml:"buttons,omitempty"`              // Buttons to display below the header
	CustomCSS           string   `yaml:"custom-css,omitempty"`           // Custom CSS to include in the page
	DarkMode            *bool    `yaml:"dark-mode,omitempty"`            // DarkMode is a flag to enable dark mode by default
	// DefaultTheme is the theme used without a valid theme cookie: dark, light or bio. When it is not set, dark-mode
	// decides between dark and light; when both are set, DefaultTheme wins (fork).
	DefaultTheme    string `yaml:"default-theme,omitempty"`
	DefaultSortBy   string `yaml:"default-sort-by,omitempty"`   // DefaultSortBy is the default sort option ('name', 'group', 'health')
	DefaultFilterBy string `yaml:"default-filter-by,omitempty"` // DefaultFilterBy is the default filter option ('none', 'failing', 'unstable')
	LoginSubtitle   string `yaml:"login-subtitle,omitempty"`    // LoginSubtitle is the subtitle displayed on the OIDC login page
	//////////////////////////////////////////////
	// Non-configurable - used for UI rendering //
	//////////////////////////////////////////////
	MaximumNumberOfResults int `yaml:"-"` // MaximumNumberOfResults to display on the page, it's not configurable because we're passing it from the storage config
}

// Identifiers of the themes of the interface: the values of the theme cookie and of default-theme. The same table
// lives in web/app/public/index.html (inline script) and in web/app/src/utils/theme.js; the three are tested with
// web/app/src/utils/theme.cases.json.
const (
	ThemeDark  = "dark"
	ThemeLight = "light"
	ThemeBio   = "bio"
)

// IsTheme returns whether the value is the identifier of a theme
func IsTheme(value string) bool {
	return value == ThemeDark || value == ThemeLight || value == ThemeBio
}

// ThemeClass returns the class of <html> of a theme: "dark", "theme-bio", or empty for the light theme and for a
// value that is not a theme
func ThemeClass(theme string) string {
	switch theme {
	case ThemeDark:
		return "dark"
	case ThemeBio:
		return "theme-bio"
	}
	return ""
}

// ThemeColor returns the theme-color of the browser for a theme, the one of the light theme for a value that is not
// a theme
func ThemeColor(theme string) string {
	switch theme {
	case ThemeDark:
		return "#030712"
	case ThemeBio:
		return "#f2f8fa"
	}
	return "#f7f9fb"
}

// Theme returns the identifier of the default theme: default-theme when it is set, otherwise the one of dark-mode,
// which is dark when it is not configured either.
func (cfg *Config) Theme() string {
	if IsTheme(cfg.DefaultTheme) {
		return cfg.DefaultTheme
	}
	if cfg.IsDarkMode() {
		return ThemeDark
	}
	return ThemeLight
}

// IsDarkMode returns whether the dark theme is the default one. It is true when dark-mode is not configured.
func (cfg *Config) IsDarkMode() bool {
	if cfg.DarkMode != nil {
		return *cfg.DarkMode
	}
	return defaultDarkMode
}

// Button is the configuration for a button on the UI
type Button struct {
	Name string `yaml:"name,omitempty"` // Name is the text to display on the button
	Link string `yaml:"link,omitempty"` // Link to open when the button is clicked.
}

// Validate validates the button configuration
func (btn *Button) Validate() error {
	if len(btn.Name) == 0 || len(btn.Link) == 0 {
		return ErrButtonValidationFailed
	}
	return nil
}

// Favicon is the configuration of the favourite icons of the UI. Each empty value defaults to the icon bundled with
// the application.
type Favicon struct {
	Default   string `yaml:"default,omitempty"`   // URL or path to default favourite icon.
	Size16x16 string `yaml:"size16x16,omitempty"` // URL or path to favourite icon for 16x16 size.
	Size32x32 string `yaml:"size32x32,omitempty"` // URL or path to favourite icon for 32x32 size.
}

// GetDefaultConfig returns a Config struct with the default values
func GetDefaultConfig() *Config {
	return &Config{
		Title:                  defaultTitle,
		Description:            defaultDescription,
		DashboardHeading:       defaultDashboardHeading,
		DashboardSubheading:    defaultDashboardSubheading,
		Header:                 defaultHeader,
		Logo:                   defaultLogo,
		Link:                   defaultLink,
		CustomCSS:              defaultCustomCSS,
		DarkMode:               &defaultDarkMode,
		DefaultSortBy:          defaultSortBy,
		DefaultFilterBy:        defaultFilterBy,
		LoginSubtitle:          defaultLoginSubtitle,
		MaximumNumberOfResults: storage.DefaultMaximumNumberOfResults,
		Favicon: Favicon{
			Default:   defaultFavicon,
			Size16x16: defaultFavicon16,
			Size32x32: defaultFavicon32,
		},
	}
}

// ValidateAndSetDefaults validates the UI configuration and sets the default values if necessary.
// Every empty text, icon and option gets its default. It returns ErrInvalidDefaultSortBy, ErrInvalidDefaultFilterBy,
// ErrButtonValidationFailed, or the error of the index template if it cannot be parsed and executed with the result.
func (cfg *Config) ValidateAndSetDefaults() error {
	if len(cfg.Title) == 0 {
		cfg.Title = defaultTitle
	}
	if len(cfg.Description) == 0 {
		cfg.Description = defaultDescription
	}
	if len(cfg.DashboardHeading) == 0 {
		cfg.DashboardHeading = defaultDashboardHeading
	}
	if len(cfg.DashboardSubheading) == 0 {
		cfg.DashboardSubheading = defaultDashboardSubheading
	}
	if len(cfg.Header) == 0 {
		cfg.Header = defaultHeader
	}
	// Spaces alone are no logo of the user: they would become an <img> without image
	cfg.Logo = strings.TrimSpace(cfg.Logo)
	if len(cfg.Logo) == 0 {
		cfg.Logo = defaultLogo
	} else if strings.EqualFold(cfg.Logo, NoLogo) {
		// The template and the frontend show a logo whenever there is one: "none" becomes no logo at all
		cfg.Logo = ""
	}
	if len(cfg.Link) == 0 {
		cfg.Link = defaultLink
	}
	if len(cfg.CustomCSS) == 0 {
		cfg.CustomCSS = defaultCustomCSS
	}
	// The presence of dark-mode is looked at before its default is applied: afterwards it is always set
	switch cfg.DefaultTheme {
	case "":
	case ThemeDark, ThemeLight, ThemeBio:
		if cfg.DarkMode != nil {
			logr.Warnf("[ui.ValidateAndSetDefaults] Both ui.default-theme and ui.dark-mode are set: ui.default-theme=%s is used", cfg.DefaultTheme)
		}
	default:
		return ErrInvalidDefaultTheme
	}
	if cfg.DarkMode == nil {
		cfg.DarkMode = &defaultDarkMode
	}
	if len(cfg.DefaultSortBy) == 0 {
		cfg.DefaultSortBy = defaultSortBy
	} else if cfg.DefaultSortBy != "name" && cfg.DefaultSortBy != "group" && cfg.DefaultSortBy != "health" {
		return ErrInvalidDefaultSortBy
	}
	if len(cfg.DefaultFilterBy) == 0 {
		cfg.DefaultFilterBy = defaultFilterBy
	} else if cfg.DefaultFilterBy != "none" && cfg.DefaultFilterBy != "failing" && cfg.DefaultFilterBy != "unstable" {
		return ErrInvalidDefaultFilterBy
	}
	if len(cfg.LoginSubtitle) == 0 {
		cfg.LoginSubtitle = defaultLoginSubtitle
	}
	if len(cfg.Favicon.Default) == 0 {
		cfg.Favicon.Default = defaultFavicon
	}
	if len(cfg.Favicon.Size16x16) == 0 {
		cfg.Favicon.Size16x16 = defaultFavicon16
	}
	if len(cfg.Favicon.Size32x32) == 0 {
		cfg.Favicon.Size32x32 = defaultFavicon32
	}
	for _, btn := range cfg.Buttons {
		if err := btn.Validate(); err != nil {
			return err
		}
	}
	// Validate that the template works
	t, err := template.ParseFS(static.FileSystem, static.IndexPath)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	return t.Execute(&buffer, NewViewData(cfg, ThemeDark))
}

// ViewData is the data handed to the index template when the HTML page of the application is rendered.
type ViewData struct {
	// UI is the UI configuration, from which the template takes the title, the texts, the icons and the custom CSS
	UI *Config
	// Theme is the class of <html> of the theme the page is rendered with, see ThemeClass: "dark", "theme-bio", or
	// empty for the light theme. The classes are mutually exclusive.
	Theme string
	// ThemeColor is the theme-color of the browser for the theme the page is rendered with, see ThemeColor
	ThemeColor string
	// DefaultTheme is the identifier of the default theme (dark, light or bio), used by the browser without a valid
	// theme cookie (fork)
	DefaultTheme string
}

// NewViewData returns the data of the index template for a theme, which must be one of the identifiers
func NewViewData(cfg *Config, theme string) ViewData {
	return ViewData{UI: cfg, Theme: ThemeClass(theme), ThemeColor: ThemeColor(theme), DefaultTheme: cfg.Theme()}
}
