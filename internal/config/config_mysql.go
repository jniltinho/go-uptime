package config

import (
	"fmt"
	"unicode/utf8"

	"github.com/jniltinho/go-uptime/v7/internal/config/key"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

// ValidateStorageKeysConfig rejects, with the mysql storage, the endpoints, external endpoints, suites and endpoints of
// suites whose key is longer than storage.MySQLMaximumKeyLength characters, which MySQL and MariaDB could not index
// (fork). With the other storage types, keys have no length limit.
//
// It must run after ValidateStorageConfig, ValidateEndpointsConfig and ValidateSuitesConfig.
func ValidateStorageKeysConfig(config *Config) error {
	for _, ep := range config.Endpoints {
		if err := CheckStorageKeyLength(config.Storage, "endpoint", ep.Key()); err != nil {
			return err
		}
	}
	for _, ee := range config.ExternalEndpoints {
		if err := CheckStorageKeyLength(config.Storage, "external endpoint", ee.Key()); err != nil {
			return err
		}
	}
	for _, su := range config.Suites {
		if err := CheckStorageKeyLength(config.Storage, "suite", su.Key()); err != nil {
			return err
		}
		for _, ep := range su.Endpoints {
			if err := CheckStorageKeyLength(config.Storage, "endpoint of suite "+su.Name, key.ConvertGroupAndNameToKey(su.Group, ep.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckStorageKeyLength returns an error when the storage is mysql and entityKey has more than
// storage.MySQLMaximumKeyLength characters. kind describes what the key identifies, for the error message.
func CheckStorageKeyLength(storageConfig *storage.Config, kind, entityKey string) error {
	if storageConfig == nil || storageConfig.Type != storage.TypeMySQL {
		return nil
	}
	length := utf8.RuneCountInString(entityKey)
	if length <= storage.MySQLMaximumKeyLength {
		return nil
	}
	prefix := []rune(entityKey)[:40]
	return fmt.Errorf("invalid %s %s...: its key has %d characters, but the mysql storage supports keys of at most %d characters", kind, string(prefix), length, storage.MySQLMaximumKeyLength)
}
