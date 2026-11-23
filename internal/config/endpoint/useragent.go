// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

import (
	"errors"
	"fmt"
	"strings"
)

// UserAgentPreset is the value of the user-agent field of an endpoint: the name of the User-Agent string that its HTTP
// and WebSocket checks send. The empty value means no preset (the User-Agent header, or else DefaultUserAgent).
type UserAgentPreset string

// The accepted values of the user-agent field of an endpoint.
const (
	UserAgentGoUptime UserAgentPreset = "go-uptime"
	UserAgentChrome   UserAgentPreset = "chrome"
	UserAgentFirefox  UserAgentPreset = "firefox"
	UserAgentEdge     UserAgentPreset = "edge"
)

// userAgentPresets maps each preset to the string it sends: the desktop Windows agent of the stable release of each
// browser, in the reduced format of Chrome and Edge (minor, build and patch are 0).
//
// Last refreshed on 2026-11-23 from the release pages of each browser: Chrome 158, Edge 158, Firefox 160. Refreshing
// them is a change of the major versions below, checked by TestUserAgentPresetFormats.
var userAgentPresets = map[UserAgentPreset]string{
	UserAgentGoUptime: DefaultUserAgent,
	UserAgentChrome:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/158.0.0.0 Safari/537.36",
	UserAgentFirefox:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:160.0) Gecko/20100101 Firefox/160.0",
	UserAgentEdge:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/158.0.0.0 Safari/537.36 Edg/158.0.0.0",
}

var (
	// ErrEndpointWithInvalidUserAgent is returned when the user-agent field of an endpoint is not one of the presets.
	ErrEndpointWithInvalidUserAgent = errors.New("invalid user-agent: accepted values are go-uptime, chrome, firefox and edge")

	// ErrEndpointUserAgentNotSupported is returned when the user-agent field is set on an endpoint whose check does not
	// send the endpoint headers (anything but HTTP, GraphQL and WebSocket).
	ErrEndpointUserAgentNotSupported = errors.New("user-agent is only supported on HTTP(S), GraphQL and WebSocket endpoints")

	// ErrEndpointUserAgentConflict is returned when an endpoint sets both the user-agent field and a User-Agent header.
	ErrEndpointUserAgentConflict = errors.New("user-agent cannot be set together with a User-Agent header: keep only one of them")
)

// validateUserAgent checks the user-agent field against the presets, the type of the endpoint and its headers. It
// returns nil when the field is empty.
func (e *Endpoint) validateUserAgent() error {
	if e.UserAgent == "" {
		return nil
	}
	if _, ok := userAgentPresets[e.UserAgent]; !ok {
		return fmt.Errorf("%w (got %q)", ErrEndpointWithInvalidUserAgent, string(e.UserAgent))
	}
	if endpointType := e.Type(); endpointType != TypeHTTP && endpointType != TypeWS {
		return fmt.Errorf("%w (endpoint type is %s)", ErrEndpointUserAgentNotSupported, strings.ToLower(string(endpointType)))
	}
	if hasHeader(e.Headers, UserAgentHeader) {
		return ErrEndpointUserAgentConflict
	}
	return nil
}

// userAgent returns the User-Agent string of the preset of the endpoint, or "" when the endpoint has no preset (or an
// unknown one, which the validation refuses).
func (e *Endpoint) userAgent() string {
	return userAgentPresets[e.UserAgent]
}
