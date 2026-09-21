package client

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint/dns"
	"github.com/jniltinho/go-uptime/v7/internal/pattern"
	"github.com/jniltinho/go-uptime/v7/internal/test"

	ping "github.com/prometheus-community/pro-bing"
	"golang.org/x/net/icmp"
)

func isIgnorableNetworkTestError(err error) bool {
	if err == nil {
		return false
	}
	errString := err.Error()
	return strings.Contains(errString, "no such host") ||
		strings.Contains(errString, "connection reset by peer") ||
		strings.Contains(errString, "i/o timeout") ||
		strings.Contains(errString, "server misbehaving")
}

func TestGetHTTPClient(t *testing.T) {
	cfg := &Config{
		Insecure:       false,
		IgnoreRedirect: false,
		Timeout:        0,
		DNSResolver:    "tcp://1.1.1.1:53",
		OAuth2Config: &OAuth2Config{
			ClientID:     "00000000-0000-0000-0000-000000000000",
			ClientSecret: "secretsauce",
			TokenURL:     "https://token-server.local/token",
			Scopes:       []string{"https://application.local/.default"},
		},
	}
	err := cfg.ValidateAndSetDefaults()
	if err != nil {
		t.Errorf("expected error to be nil, but got: `%s`", err)
	}
	if GetHTTPClient(cfg) == nil {
		t.Error("expected client to not be nil")
	}
	if GetHTTPClient(nil) == nil {
		t.Error("expected client to not be nil")
	}
}

func TestRdapQuery(t *testing.T) {
	t.Parallel()
	if _, err := rdapQuery("1.1.1.1"); err == nil {
		t.Error("expected an error due to the invalid domain type")
	}
	if _, err := rdapQuery("eurid.eu"); err == nil {
		t.Error("expected an error as there is no RDAP support currently in .eu")
	}
	if response, err := rdapQuery("example.com"); err != nil {
		t.Fatal("expected no error, got", err.Error())
	} else if response.ExpirationDate.Unix() <= 0 {
		t.Error("expected to have a valid expiry date, got", response.ExpirationDate.Unix())
	}
	// .net domain: rdapQuery must either return a valid expiration or an error,
	// but never silently return a zero-value expiration date (the bug in #1570)
	if response, err := rdapQuery("google.net"); err != nil {
		t.Logf("rdapQuery returned error for .net domain (fallback to WHOIS expected): %s", err)
	} else if response.ExpirationDate.IsZero() {
		t.Error("rdapQuery returned a zero expiration date without an error for .net domain")
	}
}

func TestGetDomainExpiration(t *testing.T) {
	t.Parallel()
	if domainExpiration, err := GetDomainExpiration("gatus.io"); err != nil {
		t.Fatalf("expected error to be nil, but got: `%s`", err)
	} else if domainExpiration <= 0 {
		t.Error("expected domain expiration to be higher than 0")
	}
	if domainExpiration, err := GetDomainExpiration("gatus.io"); err != nil {
		t.Errorf("expected error to be nil, but got: `%s`", err)
	} else if domainExpiration <= 0 {
		t.Error("expected domain expiration to be higher than 0")
	}
	// Hack to pretend like the domain is expiring in 1 hour, which should trigger a refresh
	whoisExpirationDateCache.SetWithTTL("gatus.io", time.Now().Add(time.Hour), 25*time.Hour)
	if domainExpiration, err := GetDomainExpiration("gatus.io"); err != nil {
		t.Errorf("expected error to be nil, but got: `%s`", err)
	} else if domainExpiration <= 0 {
		t.Error("expected domain expiration to be higher than 0")
	}
	// Make sure the refresh works when the ttl is <24 hours
	whoisExpirationDateCache.SetWithTTL("gatus.io", time.Now().Add(35*time.Hour), 23*time.Hour)
	if domainExpiration, err := GetDomainExpiration("gatus.io"); err != nil {
		t.Errorf("expected error to be nil, but got: `%s`", err)
	} else if domainExpiration <= 0 {
		t.Error("expected domain expiration to be higher than 0")
	}
	// .net domain: should succeed via RDAP or WHOIS fallback (#1570)
	if domainExpiration, err := GetDomainExpiration("google.net"); err != nil {
		t.Errorf("expected error to be nil for .net domain, but got: `%s`", err)
	} else if domainExpiration <= 0 {
		t.Error("expected domain expiration to be higher than 0 for .net domain")
	}
}

// fakePinger answers what a test says, without touching the network
type fakePinger struct {
	err        error
	statistics *ping.Statistics
}

func (pinger *fakePinger) Run() error                   { return pinger.err }
func (pinger *fakePinger) Statistics() *ping.Statistics { return pinger.statistics }

// TestPing covers the rules of Ping with a pinger that does not touch the network: whether ICMP is allowed to the user
// running the tests must not decide whether they pass
func TestPing(t *testing.T) {
	defer InjectPinger(nil)
	timeout := 500 * time.Millisecond
	scenarios := []struct {
		name            string
		pinger          *fakePinger
		expectedSuccess bool
		expectedRTT     time.Duration
	}{
		{name: "answered", pinger: &fakePinger{statistics: &ping.Statistics{PacketsSent: 1, PacketsRecv: 1, MaxRtt: 12 * time.Millisecond}}, expectedSuccess: true, expectedRTT: 12 * time.Millisecond},
		{name: "every packet lost, so the timeout is the round-trip time", pinger: &fakePinger{statistics: &ping.Statistics{PacketsSent: 1, PacketLoss: 100}}, expectedRTT: timeout},
		{name: "the pinger failed, e.g. an address that does not resolve or a socket that is not permitted", pinger: &fakePinger{err: errors.New("socket: permission denied")}},
		{name: "without statistics", pinger: &fakePinger{}, expectedSuccess: true},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var askedAddress string
			var askedConfig *Config
			InjectPinger(func(address string, config *Config) Pinger {
				askedAddress, askedConfig = address, config
				return scenario.pinger
			})
			config := &Config{Timeout: timeout, Network: "ip4"}
			success, rtt := Ping("192.0.2.10", config)
			if success != scenario.expectedSuccess || rtt != scenario.expectedRTT {
				t.Errorf("expected success=%v rtt=%s, got success=%v rtt=%s", scenario.expectedSuccess, scenario.expectedRTT, success, rtt)
			}
			if askedAddress != "192.0.2.10" || askedConfig != config {
				t.Errorf("expected the pinger to be created for the address and the configuration, got %q and %+v", askedAddress, askedConfig)
			}
		})
	}
}

// TestNewICMPPinger covers how the real pinger is configured, without running it
func TestNewICMPPinger(t *testing.T) {
	pinger := newICMPPinger("127.0.0.1", &Config{Timeout: 750 * time.Millisecond, Network: "ip4"})
	if pinger.Count != 1 || pinger.Timeout != 750*time.Millisecond || pinger.Privileged() != ShouldRunPingerAsPrivileged() {
		t.Errorf("expected a single echo within the timeout, got count=%d timeout=%s privileged=%v", pinger.Count, pinger.Timeout, pinger.Privileged())
	}
}

// requireICMP skips the test when this user cannot open the socket the pinger needs. It is the same socket, so a
// machine that can ping never skips, and a regression of Ping itself still fails wherever ICMP is allowed (the CI runs
// the tests with sudo for this reason).
func requireICMP(t *testing.T) {
	t.Helper()
	network := "udp4"
	if ShouldRunPingerAsPrivileged() {
		network = "ip4:icmp"
	}
	connection, err := icmp.ListenPacket(network, "0.0.0.0")
	if err != nil {
		t.Skipf("ICMP is not available to this user (%v): on Linux, run the tests as root or widen net.ipv4.ping_group_range", err)
	}
	_ = connection.Close()
}

// TestPing_RealICMP sends real ICMP echoes. It needs a raw socket (root) or a ping group that includes the user
// (net.ipv4.ping_group_range), which a test machine often does not have — WSL does not, by default. It is skipped only
// in that case, and the rules of Ping are covered by TestPing, which does not depend on the network.
func TestPing_RealICMP(t *testing.T) {
	requireICMP(t)
	if success, rtt := Ping("127.0.0.1", &Config{Timeout: 500 * time.Millisecond}); !success {
		t.Error("expected true")
		if rtt == 0 {
			t.Error("Round-trip time returned on success should've higher than 0")
		}
	}
	if success, rtt := Ping("256.256.256.256", &Config{Timeout: 500 * time.Millisecond}); success {
		t.Error("expected false, because the IP is invalid")
		if rtt != 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
	if success, rtt := Ping("192.168.152.153", &Config{Timeout: 500 * time.Millisecond}); success {
		t.Error("expected false, because the IP is valid but the host should be unreachable")
		if rtt != 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
	// Can't perform integration tests (e.g. pinging public targets by single-stacked hostname) here,
	// because ICMP is blocked in the network of GitHub-hosted runners.
	if success, rtt := Ping("127.0.0.1", &Config{Timeout: 500 * time.Millisecond, Network: "ip"}); !success {
		t.Error("expected true")
		if rtt == 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
	if success, rtt := Ping("::1", &Config{Timeout: 500 * time.Millisecond, Network: "ip"}); !success {
		t.Error("expected true")
		if rtt == 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
	if success, rtt := Ping("::1", &Config{Timeout: 500 * time.Millisecond, Network: "ip4"}); success {
		t.Error("expected false, because the IP isn't an IPv4 address")
		if rtt != 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
	if success, rtt := Ping("127.0.0.1", &Config{Timeout: 500 * time.Millisecond, Network: "ip6"}); success {
		t.Error("expected false, because the IP isn't an IPv6 address")
		if rtt != 0 {
			t.Error("Round-trip time returned on failure should've been 0")
		}
	}
}

func TestShouldRunPingerAsPrivileged(t *testing.T) {
	// Don't run in parallel since we're testing system-dependent behavior
	if runtime.GOOS == "windows" {
		result := ShouldRunPingerAsPrivileged()
		if !result {
			t.Error("On Windows, ShouldRunPingerAsPrivileged() should return true")
		}
		return
	}

	// Non-Windows tests
	result := ShouldRunPingerAsPrivileged()
	isRoot := os.Geteuid() == 0

	// Test cases based on current environment
	if isRoot {
		if !result {
			t.Error("When running as root, ShouldRunPingerAsPrivileged() should return true")
		}
	} else {
		// When not root, the result depends on raw socket creation
		// We can at least verify the function runs without panic
		t.Logf("Non-root privileged result: %v", result)
	}
}

func TestCanPerformStartTLS(t *testing.T) {
	type args struct {
		address     string
		insecure    bool
		dnsresolver string
	}
	tests := []struct {
		name          string
		args          args
		wantConnected bool
		wantErr       bool
	}{
		{
			name: "invalid address",
			args: args{
				address: "test",
			},
			wantConnected: false,
			wantErr:       true,
		},
		{
			name: "error dial",
			args: args{
				address: "test:1234",
			},
			wantConnected: false,
			wantErr:       true,
		},
		{
			name: "valid starttls",
			args: args{
				address: "smtp.gmail.com:587",
			},
			wantConnected: true,
			wantErr:       false,
		},
		{
			name: "dns resolver",
			args: args{
				address:     "smtp.gmail.com:587",
				dnsresolver: "tcp://1.1.1.1:53",
			},
			wantConnected: true,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connected, _, err := CanPerformStartTLS(tt.args.address, &Config{Insecure: tt.args.insecure, Timeout: 5 * time.Second, DNSResolver: tt.args.dnsresolver})
			if !tt.wantErr && isIgnorableNetworkTestError(err) {
				t.Skipf("skipping due to transient network error: %v", err)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("CanPerformStartTLS() err=%v, wantErr=%v", err, tt.wantErr)
				return
			}
			if connected != tt.wantConnected {
				t.Errorf("CanPerformStartTLS() connected=%v, wantConnected=%v", connected, tt.wantConnected)
			}
		})
	}
}

func TestCanPerformTLS(t *testing.T) {
	type args struct {
		address  string
		insecure bool
	}
	tests := []struct {
		name          string
		args          args
		wantConnected bool
		wantErr       bool
	}{
		{
			name: "invalid address",
			args: args{
				address: "test",
			},
			wantConnected: false,
			wantErr:       true,
		},
		{
			name: "error dial",
			args: args{
				address: "test:1234",
			},
			wantConnected: false,
			wantErr:       true,
		},
		{
			name: "valid tls",
			args: args{
				address: "smtp.gmail.com:465",
			},
			wantConnected: true,
			wantErr:       false,
		},
		{
			name: "bad cert with insecure true",
			args: args{
				address:  "expired.badssl.com:443",
				insecure: true,
			},
			wantConnected: true,
			wantErr:       false,
		},
		{
			name: "bad cert with insecure false",
			args: args{
				address:  "expired.badssl.com:443",
				insecure: false,
			},
			wantConnected: false,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connected, _, _, err := CanPerformTLS(tt.args.address, "", &Config{Insecure: tt.args.insecure, Timeout: 5 * time.Second})
			if !tt.wantErr && isIgnorableNetworkTestError(err) {
				t.Skipf("skipping due to transient network error: %v", err)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("CanPerformTLS() err=%v, wantErr=%v", err, tt.wantErr)
				return
			}
			if connected != tt.wantConnected {
				t.Errorf("CanPerformTLS() connected=%v, wantConnected=%v", connected, tt.wantConnected)
			}
		})
	}
}

func TestCanCreateConnection(t *testing.T) {
	t.Parallel()
	connected, _ := CanCreateNetworkConnection("tcp", "127.0.0.1", "", &Config{Timeout: 5 * time.Second})
	if connected {
		t.Error("should've failed, because there's no port in the address")
	}
	connected, _ = CanCreateNetworkConnection("tcp", "1.1.1.1:53", "", &Config{Timeout: 5 * time.Second})
	if !connected {
		t.Error("should've succeeded, because that IP should always™ be up")
	}
}

// This test checks if a HTTP client configured with `configureOAuth2()` automatically
// performs a Client Credentials OAuth2 flow and adds the obtained token as a `Authorization`
// header to all outgoing HTTP calls.
func TestHttpClientProvidesOAuth2BearerToken(t *testing.T) {
	defer InjectHTTPClient(nil)
	oAuth2Config := &OAuth2Config{
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "secretsauce",
		TokenURL:     "https://token-server.local/token",
		Scopes:       []string{"https://application.local/.default"},
	}
	mockHttpClient := &http.Client{
		Transport: test.MockRoundTripper(func(r *http.Request) *http.Response {
			// if the mock HTTP client tries to get a token from the `token-server`
			// we provide the expected token response
			if r.Host == "token-server.local" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewReader(
						[]byte(
							`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"secret-token"}`,
						),
					)),
				}
			}
			// to verify the headers were sent as expected, we echo them back in the
			// `X-Org-Authorization` header and check if the token value matches our
			// mocked `token-server` response
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: map[string][]string{
					"X-Org-Authorization": {r.Header.Get("Authorization")},
				},
				Body: http.NoBody,
			}
		}),
	}
	mockHttpClientWithOAuth := configureOAuth2(mockHttpClient, *oAuth2Config)
	InjectHTTPClient(mockHttpClientWithOAuth)
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8282", http.NoBody)
	if err != nil {
		t.Error("expected no error, got", err.Error())
	}
	response, err := mockHttpClientWithOAuth.Do(request)
	if err != nil {
		t.Error("expected no error, got", err.Error())
	}
	if response.Header == nil {
		t.Error("expected response headers, but got nil")
	}
	// the mock response echos the Authorization header used in the request back
	// to us as `X-Org-Authorization` header, we check here if the value matches
	// our expected token `secret-token`
	if response.Header.Get("X-Org-Authorization") != "Bearer secret-token" {
		t.Error("expected `secret-token` as Bearer token in the mocked response header `X-Org-Authorization`, but got", response.Header.Get("X-Org-Authorization"))
	}
}

func TestQueryWebSocket(t *testing.T) {
	t.Parallel()
	_, _, err := QueryWebSocket("", "body", nil, &Config{Timeout: 2 * time.Second})
	if err == nil {
		t.Error("expected an error due to the address being invalid")
	}
	_, _, err = QueryWebSocket("ws://example.org", "body", nil, &Config{Timeout: 2 * time.Second})
	if err == nil {
		t.Error("expected an error due to the target not being websocket-friendly")
	}
}

func TestTlsRenegotiation(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name           string
		cfg            TLSConfig
		expectedConfig tls.RenegotiationSupport
	}{
		{
			name:           "default",
			cfg:            TLSConfig{CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedConfig: tls.RenegotiateNever,
		},
		{
			name:           "never",
			cfg:            TLSConfig{RenegotiationSupport: "never", CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedConfig: tls.RenegotiateNever,
		},
		{
			name:           "once",
			cfg:            TLSConfig{RenegotiationSupport: "once", CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedConfig: tls.RenegotiateOnceAsClient,
		},
		{
			name:           "freely",
			cfg:            TLSConfig{RenegotiationSupport: "freely", CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedConfig: tls.RenegotiateFreelyAsClient,
		},
		{
			name:           "not-valid-and-broken",
			cfg:            TLSConfig{RenegotiationSupport: "invalid", CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedConfig: tls.RenegotiateNever,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			tls := &tls.Config{}
			tlsConfig := configureTLS(tls, scenario.cfg)
			if tlsConfig.Renegotiation != scenario.expectedConfig {
				t.Errorf("expected tls renegotiation to be %v, but got %v", scenario.expectedConfig, tls.Renegotiation)
			}
		})
	}
}

func TestQueryDNS(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name            string
		inputDNS        dns.Config
		inputURL        string
		expectedDNSCode string
		expectedBody    string
		isErrExpected   bool
	}{
		{
			name: "test Config with type A",
			inputDNS: dns.Config{
				QueryType: "A",
				QueryName: "example.com.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    "__IPV4__",
		},
		{
			name: "test Config with type AAAA",
			inputDNS: dns.Config{
				QueryType: "AAAA",
				QueryName: "example.com.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    "__IPV6__",
		},
		{
			name: "test Config with type CNAME",
			inputDNS: dns.Config{
				QueryType: "CNAME",
				QueryName: "en.wikipedia.org.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    "dyna.wikimedia.org.",
		},
		{
			name: "test Config with type MX",
			inputDNS: dns.Config{
				QueryType: "MX",
				QueryName: "example.com.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    ".",
		},
		{
			name: "test Config with type NS",
			inputDNS: dns.Config{
				QueryType: "NS",
				QueryName: "example.com.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    "*.ns.cloudflare.com.",
		},
		{
			name: "test Config with type PTR",
			inputDNS: dns.Config{
				QueryType: "PTR",
				QueryName: "8.8.8.8.in-addr.arpa.",
			},
			inputURL:        "8.8.8.8",
			expectedDNSCode: "NOERROR",
			expectedBody:    "dns.google.",
		},
		{
			name: "test Config with type PTR and forward IP / no in-addr",
			inputDNS: dns.Config{
				QueryType: "PTR",
				QueryName: "1.0.0.1",
			},
			inputURL:        "1.1.1.1",
			expectedDNSCode: "NOERROR",
			expectedBody:    "one.one.one.one.",
		},
		{
			name: "test Config with type TXT",
			inputDNS: dns.Config{
				QueryType: "TXT",
				QueryName: "example.com.",
			},
			inputURL:        "1.1.1.1",
			expectedDNSCode: "NOERROR",
			expectedBody:    "*v=spf1*",
		},
		{
			name: "test Config with fake type and retrieve error",
			inputDNS: dns.Config{
				QueryType: "B",
				QueryName: "example",
			},
			inputURL:      "8.8.8.8",
			isErrExpected: true,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, dnsRCode, body, err := QueryDNS(scenario.inputDNS.QueryType, scenario.inputDNS.QueryName, scenario.inputURL)
			if scenario.isErrExpected && err == nil {
				t.Errorf("there should be an error")
			}
			if dnsRCode != scenario.expectedDNSCode {
				t.Errorf("expected DNSRCode to be %s, got %s", scenario.expectedDNSCode, dnsRCode)
			}
			if scenario.inputDNS.QueryType == "NS" || scenario.inputDNS.QueryType == "TXT" {
				// Some record types can have multiple valid answers, so wildcard matching is used in those scenarios
				if !pattern.Match(scenario.expectedBody, string(body)) {
					t.Errorf("got %s, expected result %s,", string(body), scenario.expectedBody)
				}
			} else {
				if string(body) != scenario.expectedBody {
					// little hack to validate arbitrary ipv4/ipv6
					switch scenario.expectedBody {
					case "__IPV4__":
						if addr, err := netip.ParseAddr(string(body)); err != nil {
							t.Errorf("got %s, expected result %s", string(body), scenario.expectedBody)
						} else if !addr.Is4() {
							t.Errorf("got %s, expected valid IPv4", string(body))
						}
					case "__IPV6__":
						if addr, err := netip.ParseAddr(string(body)); err != nil {
							t.Errorf("got %s, expected result %s", string(body), scenario.expectedBody)
						} else if !addr.Is6() {
							t.Errorf("got %s, expected valid IPv6", string(body))
						}
					default:
						t.Errorf("got %s, expected result %s", string(body), scenario.expectedBody)
					}
				}
			}
		})
		time.Sleep(10 * time.Millisecond)
	}
}

// startSSHBannerServer starts a listener on the loopback that writes a banner and closes the connection, like an SSH
// server does before the handshake, and returns its address in the host:port format taken by CheckSSHBanner.
//
// Fork: the test used to dial a public SSH server (tty.sdf.org), so an outage of that server failed the build.
func startSSHBannerServer(t *testing.T, banner string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on the loopback: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte(banner))
			_ = conn.Close()
		}
	}()
	return listener.Addr().String()
}

// closedLoopbackAddress returns an address on the loopback where nothing is listening
func closedLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on the loopback: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close the listener: %v", err)
	}
	return address
}

func TestCheckSSHBanner(t *testing.T) {
	t.Parallel()
	cfg := &Config{Timeout: 3}
	t.Run("no-auth-ssh", func(t *testing.T) {
		connected, status, err := CheckSSHBanner(startSSHBannerServer(t, "SSH-2.0-GoUptime_Test\r\n"), cfg)
		if err != nil {
			t.Errorf("Expected: error != nil, got: %v ", err)
		}
		if connected == false {
			t.Errorf("Expected: connected == true, got: %v", connected)
		}
		if status != 0 {
			t.Errorf("Expected: 0, got: %v", status)
		}
	})
	t.Run("invalid-address", func(t *testing.T) {
		connected, status, err := CheckSSHBanner(closedLoopbackAddress(t), cfg)
		if err == nil {
			t.Errorf("Expected: error, got: %v ", err)
		}
		if connected != false {
			t.Errorf("Expected: connected == false, got: %v", connected)
		}
		if status != 1 {
			t.Errorf("Expected: 1, got: %v", status)
		}
	})
}
