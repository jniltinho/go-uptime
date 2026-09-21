package controller

import (
	"crypto/tls"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/web"
)

func TestHandle(t *testing.T) {
	cfg := &config.Config{
		Web: &web.Config{
			Address: "0.0.0.0",
			Port:    rand.Intn(65534),
		},
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
	}
	_ = os.Setenv("ROUTER_TEST", "true")
	_ = os.Setenv("ENVIRONMENT", "dev")
	defer os.Clearenv()
	Handle(cfg)
	defer Shutdown()
	request := httptest.NewRequest("GET", "/health", http.NoBody)
	if server == nil {
		t.Fatal("server should've been set (but because we set ROUTER_TEST, it shouldn't have been started)")
	}
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Error("expected GET /health to return status code 200")
	}
}

func TestHandleTLS(t *testing.T) {
	scenarios := []struct {
		name               string
		tls                *web.TLSConfig
		expectedStatusCode int
	}{
		{
			name:               "good-tls-config",
			tls:                &web.TLSConfig{CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"},
			expectedStatusCode: 200,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &config.Config{
				Web: &web.Config{Address: "0.0.0.0", Port: rand.Intn(65534), TLS: scenario.tls},
				Endpoints: []*endpoint.Endpoint{
					{Name: "frontend", Group: "core"},
					{Name: "backend", Group: "core"},
				},
			}
			if err := cfg.Web.ValidateAndSetDefaults(); err != nil {
				t.Error("expected no error from web (TLS) validation, got", err)
			}
			_ = os.Setenv("ROUTER_TEST", "true")
			_ = os.Setenv("ENVIRONMENT", "dev")
			defer os.Clearenv()
			Handle(cfg)
			defer Shutdown()
			request := httptest.NewRequest("GET", "/health", http.NoBody)
			if server == nil {
				t.Fatal("server should've been set (but because we set ROUTER_TEST, it shouldn't have been started)")
			}
			recorder := httptest.NewRecorder()
			server.Handler.ServeHTTP(recorder, request)
			if recorder.Code != scenario.expectedStatusCode {
				t.Errorf("%s %s should have returned %d, but returned %d instead", request.Method, request.URL, scenario.expectedStatusCode, recorder.Code)
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	// Pretend that we called controller.Handle(), which initializes the server variable
	server = &http.Server{}
	Shutdown()
	if server != nil {
		t.Error("server should've been shut down")
	}
	// Without a server, Shutdown does nothing
	Shutdown()
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealth(t *testing.T, base string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if response, err := http.Get(base + "/health"); err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the server at %s did not answer /health", base)
}

// TestHandle_ReloadOnTheSamePort is what a reload of the configuration does: the server is stopped and another one
// listens on the same address. http.Server returns http.ErrServerClosed when it is shut down, which must not be fatal,
// and Shutdown must only return once the address is free.
func TestHandle_ReloadOnTheSamePort(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Web:       &web.Config{Address: "127.0.0.1", Port: port, ReadBufferSize: web.DefaultReadBufferSize},
		Endpoints: []*endpoint.Endpoint{{Name: "frontend", Group: "core"}},
	}
	base := "http://" + cfg.Web.SocketAddress()
	for cycle := 1; cycle <= 3; cycle++ {
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			Handle(cfg)
		}()
		waitForHealth(t, base)
		Shutdown()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatalf("cycle %d: Handle did not return after Shutdown", cycle)
		}
		if _, err := http.Get(base + "/health"); err == nil {
			t.Fatalf("cycle %d: expected the server to have stopped listening", cycle)
		}
	}
}

// TestHandle_HeaderLimit checks that web.read-buffer-size still caps the headers of a request. net/http counts them with
// a margin of its own, so the sizes are far from the limit on purpose.
func TestHandle_HeaderLimit(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Web:       &web.Config{Address: "127.0.0.1", Port: port, ReadBufferSize: web.DefaultReadBufferSize},
		Endpoints: []*endpoint.Endpoint{{Name: "frontend", Group: "core"}},
	}
	base := "http://" + cfg.Web.SocketAddress()
	go Handle(cfg)
	defer Shutdown()
	waitForHealth(t, base)
	for name, scenario := range map[string]struct {
		size     int
		expected int
	}{
		"well below the limit": {size: 1024, expected: http.StatusOK},
		"well above the limit": {size: 64 * 1024, expected: http.StatusRequestHeaderFieldsTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodGet, base+"/health", http.NoBody)
			request.Header.Set("X-Padding", strings.Repeat("a", scenario.size))
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != scenario.expected {
				t.Errorf("expected %d, got %d", scenario.expected, response.StatusCode)
			}
		})
	}
}

// TestHandle_TLS opens a real TLS connection, which TestHandleTLS never does: with ROUTER_TEST the server does not listen.
// net/http negotiates HTTP/2 over TLS by itself, which the fasthttp of Fiber never spoke.
func TestHandle_TLS(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Web:       &web.Config{Address: "127.0.0.1", Port: port, ReadBufferSize: web.DefaultReadBufferSize, TLS: &web.TLSConfig{CertificateFile: "../testdata/cert.pem", PrivateKeyFile: "../testdata/cert.key"}},
		Endpoints: []*endpoint.Endpoint{{Name: "frontend", Group: "core"}},
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		Handle(cfg)
	}()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}, Timeout: 5 * time.Second} //nolint:gosec // self-signed test certificate
	var response *http.Response
	var err error
	for i := 0; i < 100; i++ {
		if response, err = client.Get("https://" + cfg.Web.SocketAddress() + "/health"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected the TLS server to answer, got %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.TLS == nil {
		t.Errorf("expected 200 over TLS, got %d (tls=%v)", response.StatusCode, response.TLS != nil)
	}
	if response.ProtoMajor != 2 {
		t.Errorf("expected HTTP/2 to be negotiated over TLS, got %s", response.Proto)
	}
	Shutdown()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Handle did not return after Shutdown")
	}
}

// TestShutdown_ClosesAConnectionThatDoesNotEnd is a reload with an open stream that ignores the shutdown: past the
// deadline the connection is closed, because http.Server.Shutdown alone waits for it forever
func TestShutdown_ClosesAConnectionThatDoesNotEnd(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	stuck := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		<-release
	})}
	done := make(chan struct{})
	mutex.Lock()
	server, stopped = stuck, done
	mutex.Unlock()
	go func() {
		defer close(done)
		_ = stuck.Serve(listener)
	}()
	response, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	previous := shutdownTimeout
	shutdownTimeout = 300 * time.Millisecond
	defer func() { shutdownTimeout = previous }()
	started := time.Now()
	Shutdown()
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("expected Shutdown to give up on the open connection after its deadline, took %s", elapsed)
	}
	if _, err = http.Get("http://" + listener.Addr().String() + "/"); err == nil {
		t.Error("expected the server to have stopped listening")
	}
}
