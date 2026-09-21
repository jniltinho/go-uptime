// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package statuspage publishes the public status pages defined in the configuration file and through the
// administration API
package statuspage

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"gopkg.in/yaml.v3"
)

var (
	// ErrEmptyDefinition is returned when a status page definition is empty
	ErrEmptyDefinition = errors.New("status page definition is empty")

	// ErrInvalidDefinition is returned when a status page definition cannot be decoded
	ErrInvalidDefinition = errors.New("invalid status page definition")
)

// Parse decodes a status page definition in YAML or JSON strictly (unknown fields and multiple documents are rejected)
// and validates it
func Parse(definition []byte) (*pageconfig.Page, error) {
	if len(bytes.TrimSpace(definition)) == 0 {
		return nil, ErrEmptyDefinition
	}
	decoder := yaml.NewDecoder(bytes.NewReader(definition))
	decoder.KnownFields(true)
	var page pageconfig.Page
	if err := decoder.Decode(&page); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrEmptyDefinition
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	var extraDocument yaml.Node
	if err := decoder.Decode(&extraDocument); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: a single document is expected", ErrInvalidDefinition)
	}
	if err := page.ValidateAndSetDefaults(); err != nil {
		return nil, err
	}
	return &page, nil
}
