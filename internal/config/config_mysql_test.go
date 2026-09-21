// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

import (
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

func TestValidateStorageKeysConfig(t *testing.T) {
	longName := strings.Repeat("a", 800)
	accentedName := strings.Repeat("é", 700) // 1400 bytes, but 700 characters
	scenarios := []struct {
		name          string
		storageType   storage.Type
		config        *Config
		expectedError string
	}{
		{name: "mysql-long-endpoint", storageType: storage.TypeMySQL, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: longName, Group: "core"}}}, expectedError: "invalid endpoint"},
		{name: "mysql-long-external-endpoint", storageType: storage.TypeMySQL, config: &Config{ExternalEndpoints: []*endpoint.ExternalEndpoint{{Name: longName, Group: "core"}}}, expectedError: "invalid external endpoint"},
		{name: "mysql-long-suite", storageType: storage.TypeMySQL, config: &Config{Suites: []*suite.Suite{{Name: longName, Group: "flows"}}}, expectedError: "invalid suite"},
		{name: "mysql-long-endpoint-of-suite", storageType: storage.TypeMySQL, config: &Config{Suites: []*suite.Suite{{Name: "checkout", Group: "flows", Endpoints: []*endpoint.Endpoint{{Name: longName}}}}}, expectedError: "invalid endpoint of suite checkout"},
		{name: "mysql-accented-key-within-the-limit", storageType: storage.TypeMySQL, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: accentedName, Group: "core"}}}},
		{name: "mysql-short-keys", storageType: storage.TypeMySQL, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: "api", Group: "core"}}}},
		{name: "postgres-long-endpoint", storageType: storage.TypePostgres, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: longName, Group: "core"}}}},
		{name: "sqlite-long-endpoint", storageType: storage.TypeSQLite, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: longName, Group: "core"}}}},
		{name: "memory-long-endpoint", storageType: storage.TypeMemory, config: &Config{Endpoints: []*endpoint.Endpoint{{Name: longName, Group: "core"}}}},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			scenario.config.Storage = &storage.Config{Type: scenario.storageType}
			err := ValidateStorageKeysConfig(scenario.config)
			if len(scenario.expectedError) == 0 {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), scenario.expectedError) || !strings.Contains(err.Error(), "at most 768 characters") {
				t.Fatalf("expected an error containing %q and the limit, got %v", scenario.expectedError, err)
			}
			if strings.Contains(err.Error(), longName) {
				t.Errorf("expected the error not to include the whole key, got %d characters", len(err.Error()))
			}
		})
	}
	if err := ValidateStorageKeysConfig(&Config{Endpoints: []*endpoint.Endpoint{{Name: longName}}}); err != nil {
		t.Errorf("expected no limit without storage configuration, got %v", err)
	}
}
