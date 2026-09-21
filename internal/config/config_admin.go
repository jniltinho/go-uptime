package config

import (
	"errors"
	"strings"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

var (
	// ErrAdminRequiresSecurity is returned when the administration is enabled without security.basic or security.oidc
	ErrAdminRequiresSecurity = errors.New("admin requires security.basic or security.oidc to be configured")

	// ErrAdminRequiresPersistentStorage is returned when the administration is enabled with the memory storage
	ErrAdminRequiresPersistentStorage = errors.New("admin requires storage.type to be sqlite, postgres or mysql")

	// ErrAdminRequiresAllowedSubjects is returned when the administration is enabled with OIDC and no allowed subject
	ErrAdminRequiresAllowedSubjects = errors.New("admin.allowed-subjects is required when security.oidc is configured")
)

// ValidateAdminConfig validates the administration configuration and its prerequisites.
//
// It must run after ValidateSecurityConfig and ValidateStorageConfig.
func ValidateAdminConfig(config *Config) error {
	if !config.Admin.IsEnabled() {
		return nil
	}
	if err := config.Admin.ValidateAndSetDefaults(); err != nil {
		return err
	}
	if config.Security == nil || (config.Security.Basic == nil && config.Security.OIDC == nil) {
		return ErrAdminRequiresSecurity
	}
	if config.Storage == nil || (config.Storage.Type != storage.TypeSQLite && config.Storage.Type != storage.TypePostgres && config.Storage.Type != storage.TypeMySQL) {
		return ErrAdminRequiresPersistentStorage
	}
	if oidc := config.Security.OIDC; oidc != nil {
		if len(config.Admin.AllowedSubjects) == 0 {
			return ErrAdminRequiresAllowedSubjects
		}
		if len(oidc.AllowedSubjects) > 0 {
			for _, subject := range config.Admin.AllowedSubjects {
				if !containsIgnoringCase(oidc.AllowedSubjects, subject) {
					logr.Warnf("[config.ValidateAdminConfig] admin.allowed-subjects contains %s, which is not in security.oidc.allowed-subjects and will not be able to log in", subject)
				}
			}
		}
	}
	if config.Storage.Type == storage.TypePostgres || config.Storage.Type == storage.TypeMySQL {
		logr.Warn("[config.ValidateAdminConfig] With multiple Go Uptime instances sharing the same PostgreSQL, MySQL or MariaDB database, endpoint changes made through the administration only apply to the other instances after they restart or reload their configuration")
	}
	return nil
}

func containsIgnoringCase(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// ResolveTunnelForClientConfig resolves the SSH tunnel referenced by clientConfig, if any, against the tunneling
// configuration. It is used for endpoints that are not part of the configuration file.
func ResolveTunnelForClientConfig(config *Config, clientConfig *client.Config) error {
	return resolveTunnelForClientConfig(config, clientConfig)
}
