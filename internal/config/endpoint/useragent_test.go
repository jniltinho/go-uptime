package endpoint

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/dns"
)

func TestUserAgentPresetFormats(t *testing.T) {
	t.Parallel()
	formats := map[UserAgentPreset]*regexp.Regexp{
		UserAgentGoUptime: regexp.MustCompile(`^go-uptime/1\.0$`),
		UserAgentChrome:   regexp.MustCompile(`^Mozilla/5\.0 \(Windows NT 10\.0; Win64; x64\) AppleWebKit/537\.36 \(KHTML, like Gecko\) Chrome/\d{3}\.0\.0\.0 Safari/537\.36$`),
		UserAgentFirefox:  regexp.MustCompile(`^Mozilla/5\.0 \(Windows NT 10\.0; Win64; x64; rv:(\d{3})\.0\) Gecko/20100101 Firefox/(\d{3})\.0$`),
		UserAgentEdge:     regexp.MustCompile(`^Mozilla/5\.0 \(Windows NT 10\.0; Win64; x64\) AppleWebKit/537\.36 \(KHTML, like Gecko\) Chrome/(\d{3})\.0\.0\.0 Safari/537\.36 Edg/(\d{3})\.0\.0\.0$`),
	}
	if len(formats) != len(userAgentPresets) {
		t.Fatalf("expected a format for each of the %d presets", len(userAgentPresets))
	}
	for preset, format := range formats {
		value, ok := userAgentPresets[preset]
		if !ok || !format.MatchString(value) {
			t.Errorf("preset %s: %q does not match %s", preset, value, format)
			continue
		}
		// The two version numbers of Firefox and of Edge must be the same release
		if groups := format.FindStringSubmatch(value); len(groups) == 3 && groups[1] != groups[2] {
			t.Errorf("preset %s: versions %s and %s differ", preset, groups[1], groups[2])
		}
	}
}

func TestEndpoint_ValidateUserAgent(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name      string
		endpoint  Endpoint
		expectErr error
	}{
		{name: "no-preset", endpoint: Endpoint{URL: "https://example.org"}},
		{name: "chrome", endpoint: Endpoint{URL: "https://example.org", UserAgent: UserAgentChrome}},
		{name: "explicit-default", endpoint: Endpoint{URL: "https://example.org", UserAgent: UserAgentGoUptime}},
		{name: "graphql", endpoint: Endpoint{URL: "https://example.org/graphql", GraphQL: true, Body: "{ a }", UserAgent: UserAgentFirefox}},
		{name: "websocket", endpoint: Endpoint{URL: "wss://example.org/ws", UserAgent: UserAgentEdge}},
		{name: "unknown", endpoint: Endpoint{URL: "https://example.org", UserAgent: "opera"}, expectErr: ErrEndpointWithInvalidUserAgent},
		{name: "tcp", endpoint: Endpoint{URL: "tcp://example.org:443", UserAgent: UserAgentChrome}, expectErr: ErrEndpointUserAgentNotSupported},
		{name: "icmp", endpoint: Endpoint{URL: "icmp://example.org", UserAgent: UserAgentChrome}, expectErr: ErrEndpointUserAgentNotSupported},
		{name: "grpc", endpoint: Endpoint{URL: "grpcs://example.org:443", UserAgent: UserAgentChrome}, expectErr: ErrEndpointUserAgentNotSupported},
		{name: "tls", endpoint: Endpoint{URL: "tls://example.org:443", UserAgent: UserAgentChrome}, expectErr: ErrEndpointUserAgentNotSupported},
		{
			name:      "dns-block-wins-over-https-url",
			endpoint:  Endpoint{URL: "https://1.1.1.1", DNSConfig: &dns.Config{QueryName: "example.org", QueryType: "A"}, UserAgent: UserAgentChrome},
			expectErr: ErrEndpointUserAgentNotSupported,
		},
		{
			name:      "explicit-default-on-tcp",
			endpoint:  Endpoint{URL: "tcp://example.org:443", UserAgent: UserAgentGoUptime},
			expectErr: ErrEndpointUserAgentNotSupported,
		},
		{
			name:      "conflict-with-header",
			endpoint:  Endpoint{URL: "https://example.org", Headers: map[string]string{"User-Agent": "Mine/1.0"}, UserAgent: UserAgentChrome},
			expectErr: ErrEndpointUserAgentConflict,
		},
		{
			name:      "conflict-with-lowercase-header",
			endpoint:  Endpoint{URL: "https://example.org", Headers: map[string]string{"user-agent": "Mine/1.0"}, UserAgent: UserAgentGoUptime},
			expectErr: ErrEndpointUserAgentConflict,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			ep := scenario.endpoint
			ep.Name = "site"
			ep.Conditions = []Condition{"[STATUS] == 200"}
			err := ep.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.expectErr) {
				t.Fatalf("expected error %v, got %v", scenario.expectErr, err)
			}
		})
	}
}

func TestEndpoint_ValidateAndSetDefaultsWithPresetIsIdempotent(t *testing.T) {
	t.Parallel()
	ep := Endpoint{Name: "site", URL: "https://example.org", Conditions: []Condition{"[STATUS] == 200"}, UserAgent: UserAgentChrome}
	for i := range 2 {
		if err := ep.ValidateAndSetDefaults(); err != nil {
			t.Fatalf("validation %d: unexpected error: %v", i+1, err)
		}
	}
	if hasHeader(ep.Headers, UserAgentHeader) {
		t.Errorf("expected the preset not to be written into the headers, got %v", ep.Headers)
	}
}

func TestEndpoint_buildHTTPRequestWithUserAgentPreset(t *testing.T) {
	t.Parallel()
	for preset, expected := range userAgentPresets {
		t.Run(string(preset), func(t *testing.T) {
			t.Parallel()
			ep := Endpoint{Name: "site", URL: "https://example.org", Conditions: []Condition{"[STATUS] == 200"}, UserAgent: preset}
			if err := ep.ValidateAndSetDefaults(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			request := ep.buildHTTPRequest()
			if got := request.Header.Get(UserAgentHeader); got != expected {
				t.Errorf("expected %q, got %q", expected, got)
			}
			for _, name := range []string{"Accept-Language", "Sec-Ch-Ua", "Sec-Fetch-Mode"} {
				if request.Header.Get(name) != "" {
					t.Errorf("expected no %s header", name)
				}
			}
		})
	}
}

// userAgentRecorder is a test server that records the User-Agent of every request it receives.
type userAgentRecorder struct {
	mutex      sync.Mutex
	userAgents []string
}

func (r *userAgentRecorder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.mutex.Lock()
	r.userAgents = append(r.userAgents, request.UserAgent())
	r.mutex.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (r *userAgentRecorder) last() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.userAgents) == 0 {
		return ""
	}
	return r.userAgents[len(r.userAgents)-1]
}

func TestEndpoint_EvaluateHealthSendsUserAgentPreset(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name   string
		scheme string
	}{
		{name: "http", scheme: "http"},
		// The handshake fails (the recorder is not a WebSocket server), but the request reaches it with its headers
		{name: "websocket", scheme: "ws"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			recorder := &userAgentRecorder{}
			server := httptest.NewServer(recorder)
			defer server.Close()
			url := scenario.scheme + strings.TrimPrefix(server.URL, "http")
			ep := Endpoint{Name: "site", URL: url, Conditions: []Condition{"[CONNECTED] == true"}, UserAgent: UserAgentFirefox}
			if err := ep.ValidateAndSetDefaults(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			ep.EvaluateHealth()
			if got := recorder.last(); got != userAgentPresets[UserAgentFirefox] {
				t.Errorf("expected the Firefox agent, got %q", got)
			}
		})
	}
}
