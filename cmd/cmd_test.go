package cmd

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/security"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const validConfiguration = "endpoints:\n  - name: website\n    url: https://example.org\n    conditions:\n      - \"[STATUS] == 200\"\n"

// execute runs the command line with the given arguments and standard input, and returns what it printed. The flags of
// the commands are global, so they are reset before every run: a flag changed by a test would stay changed.
func execute(t *testing.T, stdin string, arguments ...string) (string, error) {
	t.Helper()
	resetFlags(rootCmd)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetArgs(arguments)
	err := rootCmd.Execute()
	return output.String(), err
}

func resetFlags(command *cobra.Command) {
	reset := func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	command.Flags().VisitAll(reset)
	command.PersistentFlags().VisitAll(reset)
	for _, child := range command.Commands() {
		resetFlags(child)
	}
}

func writeConfiguration(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHelpDoesNotStartTheServer(t *testing.T) {
	output, err := execute(t, "", "--help")
	if err != nil {
		t.Fatalf("expected the help to work, got %v", err)
	}
	for _, command := range []string{"serve", "version", "config", "password", "healthcheck"} {
		if !strings.Contains(output, command) {
			t.Errorf("expected the help to list %s, got %s", command, output)
		}
	}
}

func TestVersion(t *testing.T) {
	previous := Version
	Version = "6.0.0"
	defer func() { Version = previous }()
	// Without any configuration: the command does not depend on it
	t.Setenv(ConfigPathEnvVar, filepath.Join(t.TempDir(), "missing.yaml"))
	output, err := execute(t, "", "version")
	if err != nil || !strings.Contains(output, "6.0.0") {
		t.Errorf("expected the version, got %q and %v", output, err)
	}
}

func TestResolveConfigPath(t *testing.T) {
	fromFlag := writeConfiguration(t, validConfiguration)
	fromEnvironment := writeConfiguration(t, validConfiguration)
	scenarios := []struct {
		name          string
		arguments     []string
		environment   map[string]string
		expected      string
		expectedError bool
	}{
		{name: "the flag wins over the environment", arguments: []string{"--config", fromFlag}, environment: map[string]string{ConfigPathEnvVar: fromEnvironment}, expected: fromFlag},
		{name: "only the environment", environment: map[string]string{ConfigPathEnvVar: fromEnvironment}, expected: fromEnvironment},
		{name: "the deprecated variable", environment: map[string]string{LegacyConfigFileEnvVar: fromEnvironment}, expected: fromEnvironment},
		{name: "the current variable wins over the deprecated one", environment: map[string]string{ConfigPathEnvVar: fromEnvironment, LegacyConfigFileEnvVar: fromFlag}, expected: fromEnvironment},
		{name: "the name that the variable had in Gatus", environment: map[string]string{LegacyConfigPathEnvVar: fromEnvironment}, expected: fromEnvironment},
		{name: "the new name wins over the one of Gatus", environment: map[string]string{ConfigPathEnvVar: fromEnvironment, LegacyConfigPathEnvVar: fromFlag}, expected: fromEnvironment},
		{name: "the one of Go Uptime wins over the deprecated one", environment: map[string]string{LegacyConfigPathEnvVar: fromEnvironment, LegacyConfigFileEnvVar: fromFlag}, expected: fromEnvironment},
		{name: "an empty new name does not hide the one of Gatus", environment: map[string]string{ConfigPathEnvVar: "", LegacyConfigPathEnvVar: fromEnvironment}, expected: fromEnvironment},
		{name: "empty names do not hide the deprecated one", environment: map[string]string{ConfigPathEnvVar: "", LegacyConfigPathEnvVar: "", LegacyConfigFileEnvVar: fromEnvironment}, expected: fromEnvironment},
		{name: "the flag wins over every variable", arguments: []string{"--config", fromFlag}, environment: map[string]string{ConfigPathEnvVar: fromEnvironment, LegacyConfigPathEnvVar: fromEnvironment, LegacyConfigFileEnvVar: fromEnvironment}, expected: fromFlag},
		{name: "an explicit empty flag is still refused", arguments: []string{"--config", ""}, environment: map[string]string{ConfigPathEnvVar: fromEnvironment}, expectedError: true},
		{name: "nothing, so the default paths of the load", expected: ""},
		{name: "a path typed by the operator that does not exist", arguments: []string{"--config", filepath.Join(t.TempDir(), "typo.yaml")}, expectedError: true},
		{name: "a missing path of the environment is left to the load, as before", environment: map[string]string{ConfigPathEnvVar: "/missing.yaml"}, expected: "/missing.yaml"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			clearConfigurationEnvironment(t)
			for name, value := range scenario.environment {
				t.Setenv(name, value)
			}
			resetFlags(rootCmd)
			if err := rootCmd.ParseFlags(scenario.arguments); err != nil {
				t.Fatal(err)
			}
			actual, err := resolveConfigPath(rootCmd)
			if (err != nil) != scenario.expectedError {
				t.Fatalf("expected error=%v, got %v", scenario.expectedError, err)
			}
			if actual != scenario.expected {
				t.Errorf("expected %q, got %q", scenario.expected, actual)
			}
		})
	}
}

// clearConfigurationEnvironment unsets, for the test, every variable that the commands read, under both names, and
// forgets the legacy variables already warned about
func clearConfigurationEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{ConfigPathEnvVar, LogLevelEnvVar, DelayStartEnvVar, LegacyConfigPathEnvVar, LegacyConfigFileEnvVar, LegacyLogLevelEnvVar, LegacyDelayStartEnvVar} {
		t.Setenv(name, "")
	}
	warnedLegacyVariables.Range(func(key, _ any) bool {
		warnedLegacyVariables.Delete(key)
		return true
	})
}

func TestLookupEnvironment(t *testing.T) {
	scenarios := []struct {
		name        string
		environment map[string]string
		expected    string
	}{
		{name: "only the new name", environment: map[string]string{LogLevelEnvVar: "DEBUG"}, expected: "DEBUG"},
		{name: "only the name of Gatus", environment: map[string]string{LegacyLogLevelEnvVar: "WARN"}, expected: "WARN"},
		{name: "both: the new one wins and the old one is ignored", environment: map[string]string{LogLevelEnvVar: "DEBUG", LegacyLogLevelEnvVar: "WARN"}, expected: "DEBUG"},
		{name: "an empty new name counts as not set", environment: map[string]string{LogLevelEnvVar: "", LegacyLogLevelEnvVar: "WARN"}, expected: "WARN"},
		{name: "neither", expected: ""},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			clearConfigurationEnvironment(t)
			for name, value := range scenario.environment {
				t.Setenv(name, value)
			}
			if actual := lookupEnvironment(LogLevelEnvVar, LegacyLogLevelEnvVar); actual != scenario.expected {
				t.Errorf("expected %q, got %q", scenario.expected, actual)
			}
		})
	}
	t.Run("a legacy variable is warned about once", func(t *testing.T) {
		clearConfigurationEnvironment(t)
		t.Setenv(LegacyDelayStartEnvVar, "3")
		for i := 0; i < 3; i++ {
			if actual := lookupEnvironment(DelayStartEnvVar, LegacyDelayStartEnvVar); actual != "3" {
				t.Fatalf("expected the value of the legacy variable, got %q", actual)
			}
		}
		if _, warned := warnedLegacyVariables.Load(LegacyDelayStartEnvVar); !warned {
			t.Error("expected the legacy variable to be recorded as warned")
		}
		if _, warned := warnedLegacyVariables.Load(DelayStartEnvVar); warned {
			t.Error("expected the new variable never to be warned about")
		}
	})
}

func TestResolveLogLevel(t *testing.T) {
	clearConfigurationEnvironment(t)
	t.Setenv(LogLevelEnvVar, "DEBUG")
	resetFlags(rootCmd)
	if level := resolveLogLevel(rootCmd); level != "DEBUG" {
		t.Errorf("expected the environment, got %q", level)
	}
	if err := rootCmd.ParseFlags([]string{"--log-level", "WARN"}); err != nil {
		t.Fatal(err)
	}
	if level := resolveLogLevel(rootCmd); level != "WARN" {
		t.Errorf("expected the flag to win, got %q", level)
	}
}

// TestReloadUsesTheResolvedPath makes sure a reload loads the file the server was started with, and not the one of the
// environment
func TestReloadUsesTheResolvedPath(t *testing.T) {
	started := writeConfiguration(t, validConfiguration)
	t.Setenv(ConfigPathEnvVar, writeConfiguration(t, "endpoints: []\n"))
	previous := serveConfigPath
	serveConfigPath = started
	defer func() { serveConfigPath = previous }()
	cfg, err := loadConfiguration()
	if err != nil {
		t.Fatalf("expected the configuration of the flag to be loaded, got %v", err)
	}
	if len(cfg.Endpoints) != 1 {
		t.Errorf("expected the endpoint of the configuration the server started with, got %d", len(cfg.Endpoints))
	}
}

func TestConfigValidate(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv(ConfigPathEnvVar, "")
	t.Setenv(LegacyConfigFileEnvVar, "")
	t.Run("valid", func(t *testing.T) {
		output, err := execute(t, "", "config", "validate", "--config", writeConfiguration(t, validConfiguration))
		if err != nil || !strings.Contains(output, "The configuration is valid") {
			t.Errorf("expected a valid configuration, got %q and %v", output, err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		_, err := execute(t, "", "config", "validate", "--config", writeConfiguration(t, "endpoints:\n  - name: website\n    url: https://example.org\n"))
		if err == nil {
			t.Error("expected an endpoint without conditions to be refused")
		}
	})
	t.Run("explicit-path-that-does-not-exist", func(t *testing.T) {
		// A default configuration exists: it must not be validated in place of the path that was typed
		if err := os.MkdirAll(filepath.Join(directory, "config"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, config.DefaultConfigurationFilePath), []byte(validConfiguration), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := execute(t, "", "config", "validate", "--config", filepath.Join(directory, "typo.yaml")); err == nil {
			t.Error("expected the missing path to be an error, not the default configuration to be validated")
		}
	})
	t.Run("flag-before-the-command", func(t *testing.T) {
		output, err := execute(t, "", "--config", writeConfiguration(t, validConfiguration), "config", "validate")
		if err != nil || !strings.Contains(output, "The configuration is valid") {
			t.Errorf("expected the flag to work before the command, got %q and %v", output, err)
		}
	})
	t.Run("a-document-that-makes-the-load-panic", func(t *testing.T) {
		_, err := execute(t, "", "config", "validate", "--config", writeConfiguration(t, "endpoints: [null]\n"))
		if err == nil {
			t.Error("expected an error and not a panic")
		}
	})
	t.Run("the-log-level-is-restored", func(t *testing.T) {
		logr.SetThreshold(logr.LevelInfo)
		if _, err := execute(t, "", "config", "validate", "--config", writeConfiguration(t, validConfiguration)); err != nil {
			t.Fatal(err)
		}
		if logr.GetThreshold() != logr.LevelInfo {
			t.Errorf("expected the log level to be restored, got %s", logr.GetThreshold())
		}
	})
	t.Run("opens-no-storage", func(t *testing.T) {
		database := filepath.Join(t.TempDir(), "go-uptime.db")
		withStorage := validConfiguration + "storage:\n  type: sqlite\n  path: " + database + "\n"
		if _, err := execute(t, "", "config", "validate", "--config", writeConfiguration(t, withStorage)); err != nil {
			t.Fatalf("expected a valid configuration, got %v", err)
		}
		if _, err := os.Stat(database); !os.IsNotExist(err) {
			t.Errorf("expected no database to be created, got %v", err)
		}
	})
}

func TestPasswordHash(t *testing.T) {
	scenarios := []struct {
		name     string
		stdin    string
		password string
	}{
		{name: "without a line break", stdin: "a-good-password", password: "a-good-password"},
		{name: "with LF, as echo sends", stdin: "a-good-password\n", password: "a-good-password"},
		{name: "with CRLF", stdin: "a-good-password\r\n", password: "a-good-password"},
		{name: "spaces are part of the password", stdin: "  spaced password  \n", password: "  spaced password  "},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			output, err := execute(t, scenario.stdin, "password", "hash")
			if err != nil {
				t.Fatal(err)
			}
			hash := strings.TrimSpace(output)
			if !security.CheckCredentials("admin", hash, "admin", scenario.password) {
				t.Errorf("expected the hash %q to validate with %q", hash, scenario.password)
			}
			if strings.Contains(output, scenario.password) {
				t.Errorf("expected the password not to be printed, got %q", output)
			}
		})
	}
	t.Run("empty", func(t *testing.T) {
		if _, err := execute(t, "\n", "password", "hash"); err == nil {
			t.Error("expected an empty password to be refused")
		}
	})
	t.Run("longer-than-bcrypt-accepts", func(t *testing.T) {
		if _, err := execute(t, strings.Repeat("a", 73), "password", "hash"); err == nil {
			t.Error("expected a password above 72 bytes to be refused")
		}
	})
	t.Run("as-an-argument", func(t *testing.T) {
		output, err := execute(t, "", "password", "hash", "a-good-password")
		if err == nil {
			t.Fatal("expected the password as an argument to be refused")
		}
		// cobra.NoArgs would answer `unknown command "a-good-password"`: the refusal must not repeat the password
		if strings.Contains(err.Error(), "a-good-password") || strings.Contains(output, "a-good-password") {
			t.Errorf("expected the refusal not to repeat the password, got %q and %q", err.Error(), output)
		}
	})
	t.Run("a-carriage-return-that-is-not-the-line-break", func(t *testing.T) {
		output, err := execute(t, "abc\r\r\n", "password", "hash")
		if err != nil {
			t.Fatal(err)
		}
		if !security.CheckCredentials("admin", strings.TrimSpace(output), "admin", "abc\r") {
			t.Error("expected only the line break to be dropped")
		}
	})
	t.Run("exactly-the-limit-of-bcrypt", func(t *testing.T) {
		if _, err := execute(t, strings.Repeat("a", 72), "password", "hash"); err != nil {
			t.Errorf("expected 72 bytes to be accepted, got %v", err)
		}
		// 37 characters of 2 bytes are 74 bytes: the limit is in bytes
		if _, err := execute(t, strings.Repeat("é", 37), "password", "hash"); err == nil {
			t.Error("expected 74 bytes in 37 characters to be refused")
		}
	})
}

func TestIsLoopbackTarget(t *testing.T) {
	scenarios := map[string]bool{
		"https://127.0.0.1:8443/health":     true,
		"https://[::1]:8443/health":         true,
		"https://localhost:8443/health":     true,
		"https://status.example.org/health": false,
		"https://192.0.2.10:8443/health":    false,
		"https://127.0.0.1.example.org/":    false,
		"https://localhost.example.org/":    false,
		"://not a url":                      false,
	}
	for target, expected := range scenarios {
		if actual := isLoopbackTarget(target); actual != expected {
			t.Errorf("%s: expected %v, got %v", target, expected, actual)
		}
	}
}

func TestCommandLineErrors(t *testing.T) {
	for _, arguments := range [][]string{{"unknown-command"}, {"serve", "extra-argument"}, {"config"}} {
		_, err := execute(t, "", arguments...)
		if arguments[0] == "config" {
			// A group of commands shows its help
			if err != nil {
				t.Errorf("%v: expected the help, got %v", arguments, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%v: expected an error", arguments)
		}
	}
}

func TestHealthcheckURL(t *testing.T) {
	scenarios := []struct {
		address  string
		port     int
		hasTLS   bool
		expected string
	}{
		{address: "0.0.0.0", port: 8080, expected: "http://127.0.0.1:8080/health"},
		// The loopback of the same family: a server bound to :: with bindv6only does not answer on 127.0.0.1
		{address: "::", port: 8080, expected: "http://[::1]:8080/health"},
		{address: "[::]", port: 8080, expected: "http://[::1]:8080/health"},
		// The server accepts the address with brackets, which must not be doubled
		{address: "[::1]", port: 8080, expected: "http://[::1]:8080/health"},
		{address: "", port: 9090, expected: "http://127.0.0.1:9090/health"},
		{address: "127.0.0.1", port: 9090, hasTLS: true, expected: "https://127.0.0.1:9090/health"},
		{address: "192.0.2.10", port: 8080, expected: "http://192.0.2.10:8080/health"},
		{address: "::1", port: 8080, expected: "http://[::1]:8080/health"},
	}
	for _, scenario := range scenarios {
		if actual := healthcheckURL(scenario.address, scenario.port, scenario.hasTLS); actual != scenario.expected {
			t.Errorf("%s:%d tls=%v: expected %s, got %s", scenario.address, scenario.port, scenario.hasTLS, scenario.expected, actual)
		}
	}
}

func TestHealthcheck(t *testing.T) {
	healthy := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}
	t.Run("healthy-from-the-port-of-the-configuration", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(healthy))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		// A wildcard address in the configuration, as the default is: the check must reach the loopback
		configuration := validConfiguration + "web:\n  address: 0.0.0.0\n  port: " + parsed.Port() + "\n"
		output, err := execute(t, "", "healthcheck", "--config", writeConfiguration(t, configuration))
		if err != nil || !strings.Contains(output, "healthy") {
			t.Errorf("expected the server to be healthy, got %q and %v", output, err)
		}
	})
	t.Run("https-with-a-self-signed-certificate", func(t *testing.T) {
		server := httptest.NewUnstartedServer(http.HandlerFunc(healthy))
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		server.StartTLS()
		defer server.Close()
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err != nil {
			t.Errorf("expected the self-signed certificate of the loopback to be accepted, got %v", err)
		}
	})
	t.Run("url-does-not-read-the-configuration", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(healthy))
		defer server.Close()
		t.Setenv(ConfigPathEnvVar, writeConfiguration(t, "this is: [not valid"))
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err != nil {
			t.Errorf("expected --url not to depend on the configuration, got %v", err)
		}
	})
	t.Run("the-proxy-of-the-environment-is-not-used", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(healthy))
		defer server.Close()
		// A proxy that refuses every connection: the check would fail if it went through it
		t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
		t.Setenv("http_proxy", "http://127.0.0.1:1")
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err != nil {
			t.Errorf("expected the proxy of the environment to be ignored, got %v", err)
		}
	})
	t.Run("the-start-delay-does-not-apply", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(healthy))
		defer server.Close()
		t.Setenv(DelayStartEnvVar, "30")
		started := time.Now()
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Errorf("expected the check not to wait for the start delay, took %s", elapsed)
		}
	})
	t.Run("unhealthy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
		defer server.Close()
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err == nil {
			t.Error("expected a 503 to be unhealthy")
		}
	})
	t.Run("redirect-is-not-healthy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
				return
			}
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()
		if _, err := execute(t, "", "healthcheck", "--url", server.URL+"/health"); err == nil {
			t.Error("expected a redirect not to be followed")
		}
	})
	t.Run("server-down", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(healthy))
		parsed, _ := url.Parse(server.URL)
		port, _ := strconv.Atoi(parsed.Port())
		server.Close()
		if _, err := execute(t, "", "healthcheck", "--url", healthcheckURL("127.0.0.1", port, false)); err == nil {
			t.Error("expected a closed port to be unhealthy")
		}
	})
}
