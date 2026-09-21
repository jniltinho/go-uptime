// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package statuspage contains the configuration of the public status pages, i.e. the status-pages section of the
// YAML configuration. It validates and normalizes the pages (slug, title, selection of groups and endpoints, login)
// and the trusted proxies and rate limit that protect them. Page is also the JSON object exchanged with the
// administration API.
package statuspage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultRateLimit is the default number of costly requests per minute accepted from each client IP
	DefaultRateLimit = 120

	// MaximumSlugLength is the maximum length of a slug
	MaximumSlugLength = 64

	// MaximumTitleLength is the maximum number of characters of a title
	MaximumTitleLength = 100

	// MaximumDescriptionLength is the maximum number of characters of a description
	MaximumDescriptionLength = 1000

	// MaximumGroups is the maximum number of groups selected by a page
	MaximumGroups = 50

	// MaximumGroupLength is the maximum number of characters of a group name
	MaximumGroupLength = 200

	// MaximumEndpointKeys is the maximum number of endpoint keys that a definition may list one by one. It is a
	// structural limit of a definition and does not depend on the configuration: stored pages are validated without
	// it at hand, and a limit that could be lowered would take them down.
	MaximumEndpointKeys = 1000

	// DefaultMaximumEndpointsPerPage is how many endpoints a page shows when status-pages.maximum-endpoints-per-page
	// is not set
	DefaultMaximumEndpointsPerPage = 400

	// MinimumEndpointsPerPage and MaximumEndpointsPerPage bound status-pages.maximum-endpoints-per-page. The maximum is
	// MaximumEndpointKeys, so that the endpoints field of any valid definition can be shown whole by some configuration.
	MinimumEndpointsPerPage = 1
	MaximumEndpointsPerPage = MaximumEndpointKeys

	// MaximumEndpointKeyLength is the maximum length of an endpoint key
	MaximumEndpointKeyLength = 400

	// MaximumAuthUsernameLength is the maximum length of the username of the login of a page (fork)
	MaximumAuthUsernameLength = 100

	// MaximumFeatured is the maximum number of featured endpoints of a page
	MaximumFeatured = 10

	// MaximumCharts is the maximum number of keys of the deprecated charts of a page
	MaximumCharts = 10
)

var (
	// ErrInvalidSlug is returned when a slug does not match the allowed format
	ErrInvalidSlug = errors.New("slug must have 1 to 64 lowercase letters, digits or hyphens, and must not start or end with a hyphen")

	// ErrReservedSlug is returned when a slug is reserved by the administration routes
	ErrReservedSlug = errors.New("slug is reserved")

	// ErrDuplicateSlug is returned when two pages of the configuration file have the same slug
	ErrDuplicateSlug = errors.New("slug is used by more than one status page")

	// ErrInvalidTitle is returned when a title is empty or too long
	ErrInvalidTitle = fmt.Errorf("title must have 1 to %d characters", MaximumTitleLength)

	// ErrDescriptionTooLong is returned when a description is too long
	ErrDescriptionTooLong = fmt.Errorf("description must have at most %d characters", MaximumDescriptionLength)

	// ErrEmptySelection is returned when a page selects no group, no endpoint and no featured endpoint
	ErrEmptySelection = errors.New("status page must select at least one group, endpoint or featured endpoint")

	// ErrInvalidAuthUsername is returned when the username of the credential of a page is empty or too long (fork)
	ErrInvalidAuthUsername = fmt.Errorf("the username of the login of a status page must have 1 to %d characters", MaximumAuthUsernameLength)

	// ErrInvalidAuthPasswordHash is returned when the password of a page is not a bcrypt hash in base64 (fork)
	ErrInvalidAuthPasswordHash = errors.New("the password of the login of a status page must be a bcrypt hash encoded in base64")

	// ErrInvalidGroups is returned when the groups of a page are invalid
	ErrInvalidGroups = errors.New("invalid groups")

	// ErrInvalidEndpoints is returned when the endpoint keys of a page are invalid
	ErrInvalidEndpoints = errors.New("invalid endpoints")

	// ErrInvalidFeatured is returned when the featured endpoint keys of a page are invalid
	ErrInvalidFeatured = errors.New("invalid featured endpoints")

	// ErrInvalidCharts is returned when the endpoint keys of the charts of a page are invalid
	ErrInvalidCharts = errors.New("invalid charts")

	// ErrInvalidTrustedProxy is returned when an entry of trusted-proxies is neither an IP address nor a CIDR
	ErrInvalidTrustedProxy = errors.New("status-pages.trusted-proxies entries must be IP addresses or CIDRs")

	// ErrInvalidMaximumEndpointsPerPage is returned when maximum-endpoints-per-page is outside of its bounds
	ErrInvalidMaximumEndpointsPerPage = fmt.Errorf("status-pages.maximum-endpoints-per-page must be between %d and %d", MinimumEndpointsPerPage, MaximumEndpointsPerPage)

	// ErrInvalidRateLimit is returned when rate-limit is negative
	ErrInvalidRateLimit = errors.New("status-pages.rate-limit must not be negative")

	slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

	// reservedSlugs collide with the static segments of the administration routes (/api/v1/admin/status-pages/<segment>)
	reservedSlugs = map[string]struct{}{"exposure": {}, "new": {}, "options": {}, "preview": {}, "validate": {}}
)

// Config is the configuration of the public status pages
type Config struct {
	// Enabled is whether the public status pages are served. Defaults to true.
	Enabled *bool `yaml:"enabled,omitempty"`

	// TrustedProxies is the list of IP addresses or CIDRs of the reverse proxies whose X-Forwarded-For header is used
	// to identify the client IP
	TrustedProxies []string `yaml:"trusted-proxies,omitempty"`

	// RateLimit is the number of costly requests per minute accepted from each client IP. 0 disables the limit.
	// Defaults to DefaultRateLimit.
	RateLimit *int `yaml:"rate-limit,omitempty"`

	// MaximumEndpointsPerPage is how many endpoints a page shows, the featured ones first and then the sections in
	// display order. The endpoints beyond it are not shown and are not reachable through the routes of the page
	// either (details, chart, event stream, badges), so raising it publishes more endpoints. Defaults to
	// DefaultMaximumEndpointsPerPage; between MinimumEndpointsPerPage and MaximumEndpointsPerPage.
	MaximumEndpointsPerPage *int `yaml:"maximum-endpoints-per-page,omitempty"`

	// Pages is the list of status pages defined in the configuration file
	Pages []*Page `yaml:"pages,omitempty"`

	trustedProxyPrefixes []netip.Prefix
}

// IsEnabled returns whether the public status pages are served. It is safe to call on a nil Config.
func (c *Config) IsEnabled() bool {
	return c == nil || c.Enabled == nil || *c.Enabled
}

// GetRateLimit returns the number of costly requests per minute accepted from each client IP, 0 meaning unlimited.
// It is safe to call on a nil Config.
func (c *Config) GetRateLimit() int {
	if c == nil || c.RateLimit == nil {
		return DefaultRateLimit
	}
	return *c.RateLimit
}

// UnmarshalYAML decodes the section and refuses a maximum-endpoints-per-page that is not an integer: the YAML decoder
// would otherwise truncate 2.5 to 2 without a word, and the limit decides what a page publishes.
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	type plain Config
	if err := node.Decode((*plain)(c)); err != nil {
		return err
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if value := node.Content[i+1]; node.Content[i].Value == "maximum-endpoints-per-page" && value.Tag != "!!int" && value.Tag != "!!null" {
			return fmt.Errorf("%w: %q is not an integer", ErrInvalidMaximumEndpointsPerPage, value.Value)
		}
	}
	return nil
}

// GetMaximumEndpointsPerPage returns how many endpoints a page shows. It is safe to call on a nil Config.
func (c *Config) GetMaximumEndpointsPerPage() int {
	if c == nil || c.MaximumEndpointsPerPage == nil {
		return DefaultMaximumEndpointsPerPage
	}
	return *c.MaximumEndpointsPerPage
}

// TrustedProxyPrefixes returns the normalized trusted-proxies. It is safe to call on a nil Config.
func (c *Config) TrustedProxyPrefixes() []netip.Prefix {
	if c == nil {
		return nil
	}
	return c.trustedProxyPrefixes
}

// ValidateAndSetDefaults validates the configuration and normalizes its values: the trusted proxies are parsed into
// prefixes and every page is validated. It returns ErrInvalidRateLimit, ErrInvalidTrustedProxy, ErrDuplicateSlug or the
// wrapped error of the first invalid page.
func (c *Config) ValidateAndSetDefaults() error {
	if c.RateLimit != nil && *c.RateLimit < 0 {
		return ErrInvalidRateLimit
	}
	if c.MaximumEndpointsPerPage != nil && (*c.MaximumEndpointsPerPage < MinimumEndpointsPerPage || *c.MaximumEndpointsPerPage > MaximumEndpointsPerPage) {
		return ErrInvalidMaximumEndpointsPerPage
	}
	prefixes := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, trustedProxy := range c.TrustedProxies {
		prefix, err := parseTrustedProxy(trustedProxy)
		if err != nil {
			return err
		}
		prefixes = append(prefixes, prefix)
	}
	c.trustedProxyPrefixes = prefixes
	slugs := make(map[string]struct{}, len(c.Pages))
	for _, page := range c.Pages {
		if page == nil {
			return fmt.Errorf("%w: empty status page", ErrEmptySelection)
		}
		if err := page.ValidateAndSetDefaults(); err != nil {
			return fmt.Errorf("invalid status page %q: %w", page.Slug, err)
		}
		if _, exists := slugs[page.Slug]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateSlug, page.Slug)
		}
		slugs[page.Slug] = struct{}{}
	}
	return nil
}

// Page is the definition of a public status page: its address, its texts, the endpoints it shows and how it shows
// them. It is read from the configuration file and is also the JSON object that the administration API receives and
// returns for a managed page.
type Page struct {
	// Slug identifies the page in its public path (/status/<slug>). It cannot be changed. It has 1 to 64 lowercase
	// letters, digits or hyphens and neither starts nor ends with a hyphen.
	Slug string `yaml:"slug" json:"slug"`

	// Title is shown at the top of the page. It has 1 to MaximumTitleLength characters.
	Title string `yaml:"title" json:"title"`

	// Description is shown below the title, as plain text, with at most MaximumDescriptionLength characters. Omitted
	// when empty.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Groups selects every enabled endpoint whose group is in the list, including the ones created later. At least one
	// of Groups, Endpoints and Featured must not be empty.
	Groups []string `yaml:"groups,omitempty" json:"groups,omitempty"`

	// Endpoints selects endpoints by key (the lowercase <group>_<name> key of the status API)
	Endpoints []string `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`

	// Featured selects endpoints by key and shows them at the top of the page, with more details, instead of in their
	// group
	Featured []string `yaml:"featured,omitempty" json:"featured,omitempty"`

	// Charts is deprecated and ignored: every endpoint of a page has a public details page with its response time chart.
	// It is still accepted so that the definitions saved by v5.36.0-fork.2 stay valid.
	Charts []string `yaml:"charts,omitempty" json:"charts,omitempty"`

	// ShowCertificateExpiration shows below the name of each endpoint how many days are left until its TLS certificate
	// expires, like the "Show Certificate Expiry" option of the Uptime Kuma (fork). The same key is used in YAML and JSON,
	// because the definitions managed through the administration are decoded as YAML.
	ShowCertificateExpiration bool `yaml:"show-certificate-expiration,omitempty" json:"show-certificate-expiration,omitempty"`

	// ShowMessages publishes, on the details page of each endpoint, the table of checks of the dashboard with the message
	// and the origin of each result (fork). Only the messages of pushes and heartbeats and the HTTP status of the checks
	// are published, never their errors.
	ShowMessages bool `yaml:"show-messages,omitempty" json:"show-messages,omitempty"`

	// GroupsCollapsed makes the groups of the public page start collapsed. A group that is not operational is shown
	// expanded whatever this says, so that a problem never starts hidden, and the choice of a visitor for a group is
	// kept in their browser and wins over it. A null or omitted value means false.
	GroupsCollapsed bool `yaml:"groups-collapsed,omitempty" json:"groups-collapsed,omitempty"`

	// Auth requires a username and a password to view the page: with it, every route of the page answers 401 without
	// the credential of this page (fork). Without it, the page stays public.
	Auth *PageAuth `yaml:"auth,omitempty" json:"auth,omitempty"`

	// Enabled is whether the page is published. Pages of the configuration file default to true. A null or omitted
	// value means true.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// IsEnabled returns whether the page is published, defaulting to true
func (p *Page) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

// ValidateAndSetDefaults validates the page and normalizes its values: the title, the description and the group names
// are trimmed, and the endpoint keys are trimmed and converted to lowercase
func (p *Page) ValidateAndSetDefaults() error {
	p.Slug = strings.TrimSpace(p.Slug)
	if err := ValidateSlug(p.Slug); err != nil {
		return err
	}
	p.Title = strings.TrimSpace(p.Title)
	if length := utf8.RuneCountInString(p.Title); length == 0 || length > MaximumTitleLength {
		return ErrInvalidTitle
	}
	p.Description = strings.TrimSpace(p.Description)
	if utf8.RuneCountInString(p.Description) > MaximumDescriptionLength {
		return ErrDescriptionTooLong
	}
	groups, err := normalizeList(p.Groups, MaximumGroups, MaximumGroupLength, strings.TrimSpace)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidGroups, err)
	}
	normalizeKey := func(key string) string {
		return strings.ToLower(strings.TrimSpace(key))
	}
	endpoints, err := normalizeList(p.Endpoints, MaximumEndpointKeys, MaximumEndpointKeyLength, normalizeKey)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEndpoints, err)
	}
	featured, err := normalizeList(p.Featured, MaximumFeatured, MaximumEndpointKeyLength, normalizeKey)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFeatured, err)
	}
	charts, err := normalizeList(p.Charts, MaximumCharts, MaximumEndpointKeyLength, normalizeKey)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCharts, err)
	}
	if len(groups) == 0 && len(endpoints) == 0 && len(featured) == 0 {
		return ErrEmptySelection
	}
	if err = p.Auth.validate(); err != nil {
		return err
	}
	p.Groups, p.Endpoints, p.Featured, p.Charts = groups, endpoints, featured, charts
	return nil
}

// PageAuth is the credential required to view a page, in the same shape as security.basic (fork). The username is
// trimmed during validation, which returns ErrInvalidAuthUsername or ErrInvalidAuthPasswordHash.
type PageAuth struct {
	// Username is the username of the HTTP Basic credential of the page, with 1 to MaximumAuthUsernameLength characters
	Username string `yaml:"username" json:"username"`

	// PasswordBcryptHashBase64Encoded is the bcrypt hash of the password, encoded in base64 with the URL alphabet. The
	// password itself is never stored.
	PasswordBcryptHashBase64Encoded string `yaml:"password-bcrypt-base64" json:"password-bcrypt-base64"`
}

// RequiresLogin returns whether the page requires a credential to be viewed. It is safe to call on a nil Page.
func (p *Page) RequiresLogin() bool {
	return p != nil && p.Auth != nil
}

// validate returns an error when the credential of the page is incomplete or does not hold a bcrypt hash
func (a *PageAuth) validate() error {
	if a == nil {
		return nil
	}
	a.Username = strings.TrimSpace(a.Username)
	if length := utf8.RuneCountInString(a.Username); length == 0 || length > MaximumAuthUsernameLength {
		return ErrInvalidAuthUsername
	}
	hash, err := base64.URLEncoding.DecodeString(strings.TrimSpace(a.PasswordBcryptHashBase64Encoded))
	if err != nil {
		return ErrInvalidAuthPasswordHash
	}
	if _, err = bcrypt.Cost(hash); err != nil {
		return ErrInvalidAuthPasswordHash
	}
	return nil
}

// ValidateSlug returns an error if the slug does not match the allowed format or is reserved
func ValidateSlug(slug string) error {
	if len(slug) > MaximumSlugLength || !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	if _, reserved := reservedSlugs[slug]; reserved {
		return fmt.Errorf("%w: %s", ErrReservedSlug, slug)
	}
	return nil
}

// NormalizeGroup returns the group name as compared with the groups of a page
func NormalizeGroup(group string) string {
	return strings.TrimSpace(group)
}

func normalizeList(values []string, maximumItems, maximumLength int, normalize func(string) string) ([]string, error) {
	if len(values) > maximumItems {
		return nil, fmt.Errorf("at most %d entries are allowed", maximumItems)
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalize(value)
		if length := utf8.RuneCountInString(value); length == 0 || length > maximumLength {
			return nil, fmt.Errorf("entries must have 1 to %d characters", maximumLength)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("duplicate entry %q", value)
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func parseTrustedProxy(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("%w: %q", ErrInvalidTrustedProxy, value)
		}
		if prefix.Addr().Is4In6() {
			bits := prefix.Bits() - 96
			if bits < 0 {
				return netip.Prefix{}, fmt.Errorf("%w: %q", ErrInvalidTrustedProxy, value)
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), bits)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%w: %q", ErrInvalidTrustedProxy, value)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
