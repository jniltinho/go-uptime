// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWebConfiguration(t *testing.T) {
	t.Setenv("GO_UPTIME_TEST_WEB_PORT", "9191")
	scenarios := []struct {
		name            string
		content         string
		expectedAddress string
		expectedPort    int
		expectedTLS     bool
	}{
		{name: "without a web section", content: "endpoints: []\n", expectedAddress: "0.0.0.0", expectedPort: 8080},
		{name: "with a port", content: "web:\n  port: 9090\n", expectedAddress: "0.0.0.0", expectedPort: 9090},
		{name: "with a port from the environment", content: "web:\n  port: ${GO_UPTIME_TEST_WEB_PORT}\n", expectedAddress: "0.0.0.0", expectedPort: 9191},
		{
			// The certificate files do not exist: they must not be loaded to know where the server listens
			name:            "with tls",
			content:         "web:\n  address: 127.0.0.1\n  tls:\n    certificate-file: /missing/cert.pem\n    private-key-file: /missing/key.pem\n",
			expectedAddress: "127.0.0.1",
			expectedPort:    8080,
			expectedTLS:     true,
		},
		{
			// The rest of the configuration is not validated
			name:            "with an invalid endpoint elsewhere",
			content:         "web:\n  port: 9090\nendpoints:\n  - name: without-conditions\n    url: https://example.org\n",
			expectedAddress: "0.0.0.0",
			expectedPort:    9090,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(scenario.content), 0o600); err != nil {
				t.Fatal(err)
			}
			webConfig, err := LoadWebConfiguration(path)
			if err != nil {
				t.Fatal(err)
			}
			if webConfig.Address != scenario.expectedAddress || webConfig.Port != scenario.expectedPort || webConfig.HasTLS() != scenario.expectedTLS {
				t.Errorf("expected %s:%d tls=%v, got %s:%d tls=%v", scenario.expectedAddress, scenario.expectedPort, scenario.expectedTLS, webConfig.Address, webConfig.Port, webConfig.HasTLS())
			}
		})
	}
	t.Run("a port out of range, which the server refuses too", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		_ = os.WriteFile(path, []byte("web:\n  port: 70000\n"), 0o600)
		if _, err := LoadWebConfiguration(path); err == nil {
			t.Error("expected the port to be refused")
		}
	})
	t.Run("an empty file is no configuration, as for the server", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		_ = os.WriteFile(path, nil, 0o600)
		if _, err := LoadWebConfiguration(path); !errors.Is(err, ErrConfigFileNotFound) {
			t.Errorf("expected ErrConfigFileNotFound, got %v", err)
		}
	})
	t.Run("a directory of configuration files", func(t *testing.T) {
		directory := t.TempDir()
		_ = os.WriteFile(filepath.Join(directory, "endpoints.yaml"), []byte("endpoints: []\n"), 0o600)
		_ = os.WriteFile(filepath.Join(directory, "web.yaml"), []byte("web:\n  port: 7070\n"), 0o600)
		webConfig, err := LoadWebConfiguration(directory)
		if err != nil || webConfig.Port != 7070 {
			t.Errorf("expected the port of the merged files, got %+v and %v", webConfig, err)
		}
	})
}

func TestRequireConfigPath(t *testing.T) {
	if err := RequireConfigPath(filepath.Join(t.TempDir(), "typo.yaml")); !errors.Is(err, ErrConfigPathNotFound) {
		t.Errorf("expected ErrConfigPathNotFound, got %v", err)
	}
	if err := RequireConfigPath(t.TempDir()); err != nil {
		t.Errorf("expected an existing directory to be accepted, got %v", err)
	}
}
