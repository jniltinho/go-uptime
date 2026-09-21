// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
)

const validReloadTestConfig = `
endpoints:
  - name: website
    url: https://twin.sh/health
    conditions:
      - "[STATUS] == 200"
`

func TestLoadUpdatedConfiguration(t *testing.T) {
	errInvalidConfig := errors.New("invalid configuration")
	scenarios := []struct {
		name                    string
		skipInvalidConfigUpdate bool
		loadErr                 error
		expectApplied           bool
		expectPanic             bool
	}{
		{
			name:          "valid-configuration-is-applied",
			expectApplied: true,
		},
		{
			name:                    "valid-configuration-is-applied-with-skip-invalid-config-update",
			skipInvalidConfigUpdate: true,
			expectApplied:           true,
		},
		{
			name:                    "invalid-configuration-is-skipped-with-skip-invalid-config-update",
			skipInvalidConfigUpdate: true,
			loadErr:                 errInvalidConfig,
		},
		{
			name:        "invalid-configuration-panics-without-skip-invalid-config-update",
			loadErr:     errInvalidConfig,
			expectPanic: true,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			current := &config.Config{SkipInvalidConfigUpdate: scenario.skipInvalidConfigUpdate}
			newConfig := &config.Config{}
			loadCalls := 0
			load := func() (*config.Config, error) {
				loadCalls++
				if scenario.loadErr != nil {
					return nil, scenario.loadErr
				}
				return newConfig, nil
			}
			defer func() {
				recovered := recover()
				if scenario.expectPanic && recovered == nil {
					t.Fatal("expected a panic")
				}
				if !scenario.expectPanic && recovered != nil {
					t.Fatalf("unexpected panic: %v", recovered)
				}
				if loadCalls != 1 {
					t.Errorf("expected the configuration to be loaded once, got %d", loadCalls)
				}
			}()
			updatedConfig, applied := loadUpdatedConfiguration(current, load)
			if applied != scenario.expectApplied {
				t.Errorf("expected applied=%v, got %v", scenario.expectApplied, applied)
			}
			if scenario.expectApplied && updatedConfig != newConfig {
				t.Error("expected the loaded configuration to be returned")
			}
			if !scenario.expectApplied && updatedConfig != nil {
				t.Error("expected no configuration to be returned")
			}
		})
	}
}

func TestLoadUpdatedConfiguration_InvalidFileIsNotProcessedAgain(t *testing.T) {
	t.Parallel()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(validReloadTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := config.LoadConfiguration(configPath)
	if err != nil {
		t.Fatalf("failed to load initial configuration: %v", err)
	}
	current.SkipInvalidConfigUpdate = true
	// The modification check has a granularity of one second
	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(configPath, []byte("endpoints: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !current.HasLoadedConfigurationBeenModified() {
		t.Fatal("expected the configuration file to be detected as modified")
	}
	time.Sleep(1100 * time.Millisecond)
	updatedConfig, applied := loadUpdatedConfiguration(current, func() (*config.Config, error) {
		return config.LoadConfiguration(configPath)
	})
	if applied || updatedConfig != nil {
		t.Fatal("expected the invalid configuration not to be applied")
	}
	if current.HasLoadedConfigurationBeenModified() {
		t.Error("expected the invalid modification to be marked as processed")
	}
}
