package config

import (
	"errors"
	"fmt"

	"github.com/jniltinho/go-uptime/v7/internal/config/push"
)

var (
	// ErrPushEndpointNotFound is returned when push.endpoints lists a key that is not an endpoint of the configuration file
	ErrPushEndpointNotFound = errors.New("push.endpoints[].key must be the key of an endpoint of the configuration file")
)

// ValidatePushConfig validates the push monitoring configuration (fork): the global keys, the endpoints that receive
// push and the uniqueness of the push tokens, including the tokens of the external endpoints.
//
// It must run after ValidateEndpointsConfig.
func ValidatePushConfig(config *Config) error {
	if config.Push == nil {
		return nil
	}
	if err := config.Push.ValidateAndSetDefaults(); err != nil {
		return err
	}
	for _, pushEndpoint := range config.Push.Endpoints {
		if config.GetEndpointByKey(pushEndpoint.Key) == nil {
			return fmt.Errorf("%w: %s", ErrPushEndpointNotFound, pushEndpoint.Key)
		}
	}
	externalTokens := make(map[string]struct{}, len(config.ExternalEndpoints))
	for _, externalEndpoint := range config.ExternalEndpoints {
		externalTokens[externalEndpoint.Token] = struct{}{}
	}
	for _, token := range config.Push.Tokens() {
		if _, used := externalTokens[token]; used {
			return fmt.Errorf("%w: a token of push is also the token of an external endpoint", push.ErrDuplicateToken)
		}
	}
	return nil
}
