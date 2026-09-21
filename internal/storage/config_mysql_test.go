package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestConfig_ValidateAndSetDefaults_MySQL(t *testing.T) {
	scenarios := []struct {
		name        string
		path        string
		expectedErr error
	}{
		{name: "valid", path: "go_uptime:secret@tcp(mariadb:3306)/go_uptime"},
		{name: "valid-with-parameters", path: "go_uptime:secret@tcp(127.0.0.1:3306)/go_uptime?tls=preferred&timeout=5s"},
		{name: "missing-path", path: "", expectedErr: ErrSQLStorageRequiresPath},
		{name: "malformed", path: "go_uptime:secret@tcp(mariadb:3306", expectedErr: ErrMySQLStorageInvalidPath},
		{name: "unknown-parameter-value", path: "go_uptime:secret@tcp(mariadb:3306)/go_uptime?parseTime=maybe", expectedErr: ErrMySQLStorageInvalidPath},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &Config{Type: TypeMySQL, Path: scenario.path}
			err := cfg.ValidateAndSetDefaults()
			if !errors.Is(err, scenario.expectedErr) {
				t.Fatalf("expected %v, got %v", scenario.expectedErr, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Errorf("expected the error not to contain the password, got %q", err.Error())
			}
			if err == nil && (cfg.MaximumNumberOfResults != DefaultMaximumNumberOfResults || cfg.MaximumNumberOfEvents != DefaultMaximumNumberOfEvents) {
				t.Errorf("expected the default limits, got %d results and %d events", cfg.MaximumNumberOfResults, cfg.MaximumNumberOfEvents)
			}
		})
	}
}
