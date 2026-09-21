package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strings"

	"github.com/TwiN/deepmerge"
	"github.com/jniltinho/go-uptime/v7/internal/config/web"
	"gopkg.in/yaml.v3"
)

// ErrConfigPathNotFound is returned when a configuration path given explicitly does not exist
var ErrConfigPathNotFound = errors.New("configuration path not found")

// LoadWebConfiguration reads only the web section of the configuration, with its default address and port, without
// validating the rest of the configuration, without loading the TLS certificates and without opening anything. It is
// what `gatus healthcheck` needs to know where the server listens.
func LoadWebConfiguration(configPath string) (*web.Config, error) {
	configBytes, err := readConfigurationBytes(configPath)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Web *web.Config `yaml:"web"`
	}
	if err = yaml.Unmarshal(expandEnvironmentVariables(configBytes), &parsed); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	webConfig := parsed.Web
	if webConfig == nil {
		webConfig = web.GetDefaultConfig()
	}
	if len(webConfig.Address) == 0 {
		webConfig.Address = web.DefaultAddress
	}
	if webConfig.Port == 0 {
		webConfig.Port = web.DefaultPort
	} else if webConfig.Port < 0 || webConfig.Port > math.MaxUint16 {
		// The same rule as web.Config.ValidateAndSetDefaults, which the server applies
		return nil, fmt.Errorf("invalid port: value should be between %d and %d", 0, math.MaxUint16)
	}
	return webConfig, nil
}

// RequireConfigPath returns ErrConfigPathNotFound when configPath does not exist. LoadConfiguration falls back on the
// default paths when the given one is missing, which is right for the environment but wrong for a path typed by the
// operator: `gatus config validate --config typo.yaml` would validate another file.
func RequireConfigPath(configPath string) error {
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("%w: %s", ErrConfigPathNotFound, configPath)
	}
	return nil
}

// readConfigurationBytes returns the merged YAML of configPath, or of the default paths when it is empty or missing,
// exactly as LoadConfiguration resolves them
func readConfigurationBytes(configPath string) ([]byte, error) {
	for _, candidate := range []string{configPath, DefaultConfigurationFilePath, DefaultFallbackConfigurationFilePath} {
		if len(candidate) == 0 {
			continue
		}
		fileInfo, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !fileInfo.IsDir() {
			configBytes, readErr := os.ReadFile(candidate)
			if readErr == nil && len(configBytes) == 0 {
				// Like LoadConfiguration, for which an empty file is no configuration
				return nil, ErrConfigFileNotFound
			}
			return configBytes, readErr
		}
		var configBytes []byte
		err = walkConfigDir(candidate, func(path string, _ fs.DirEntry, _ error) error {
			if strings.Contains(path, "..") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			configBytes, readErr = deepmerge.YAML(configBytes, data)
			return readErr
		})
		if err != nil {
			return nil, err
		}
		if len(configBytes) == 0 {
			return nil, ErrConfigFileNotFound
		}
		return configBytes, nil
	}
	return nil, ErrConfigFileNotFound
}

// expandEnvironmentVariables expands the environment variables of the configuration, keeping a literal "$" for "$$",
// like parseAndValidateConfigBytes
func expandEnvironmentVariables(yamlBytes []byte) []byte {
	expanded := strings.ReplaceAll(string(yamlBytes), "$$", "__GO_UPTIME_LITERAL_DOLLAR_SIGN__")
	expanded = os.ExpandEnv(expanded)
	return []byte(strings.ReplaceAll(expanded, "__GO_UPTIME_LITERAL_DOLLAR_SIGN__", "$"))
}
