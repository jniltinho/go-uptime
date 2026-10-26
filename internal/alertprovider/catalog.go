// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

// docsJSON describes each provider for the administration: name, category and help of its fields. It was written
// from the parameter tables of the Alerting section of the README of the original Gatus.
//
//go:embed docs.json
var docsJSON []byte

// Field kinds of TypeInfo.Fields
const (
	KindString   = "string"
	KindInteger  = "integer"
	KindBoolean  = "boolean"
	KindList     = "list"
	KindMap      = "map"
	KindDuration = "duration"
)

// TypeInfo describes an alert type for the forms of the administration. It is an item of the types of Listing.
type TypeInfo struct {
	// Type is the alert type, the key of the provider in the alerting section, e.g. "teams-workflows"
	Type string `json:"type"`

	// Name is the readable name of the provider, e.g. "Microsoft Teams Workflows"
	Name string `json:"name"`

	// Category groups the providers in the picker: "Chat", "Email", "SMS", "Push", "Incidents", "Observability",
	// "Issues" or "Automation"
	Category string `json:"category"`

	// Untested is whether the original Gatus warns that the provider has not been validated by its community
	Untested bool `json:"untested,omitempty"`

	// Deprecated is whether the provider is deprecated in favour of another one
	Deprecated bool `json:"deprecated,omitempty"`

	// SupportsOverrides is whether the provider accepts group overrides
	SupportsOverrides bool `json:"supportsOverrides"`

	// Fields are the fields of the default configuration that the form shows, in the order of the provider's struct.
	// The client section, the default alert and the overrides are not among them: the form edits the default alert
	// with fields of its own, and the rest is only edited in YAML.
	Fields []Field `json:"fields"`
}

// Field is a field of the configuration of a provider
type Field struct {
	// Key is the YAML key of the field, e.g. "webhook-url"
	Key string `json:"key"`

	// Kind is how the value is edited: "string", "integer", "boolean", "list" (of strings), "map" (of strings) or
	// "duration" (Go duration, e.g. "30s")
	Kind string `json:"kind"`

	// Secret is whether the value is a credential: masked in every answer and edited as a password
	Secret bool `json:"secret"`

	// Required is whether the provider refuses an empty value
	Required bool `json:"required,omitempty"`

	// Default is the value used when the field is empty, as text; omitted when there is none
	Default string `json:"default,omitempty"`

	// Help is a short English description of the field
	Help string `json:"help,omitempty"`
}

type providerDoc struct {
	Name       string              `json:"name"`
	Category   string              `json:"category"`
	Untested   bool                `json:"untested"`
	Deprecated bool                `json:"deprecated"`
	Fields     map[string]fieldDoc `json:"fields"`
}

type fieldDoc struct {
	Help     string `json:"help"`
	Required bool   `json:"required"`
	Default  string `json:"default"`
}

var (
	catalogOnce sync.Once
	catalog     []*TypeInfo
)

// Catalog returns the description of every alert type, in the order of Types
func Catalog() []*TypeInfo {
	catalogOnce.Do(func() {
		docs := map[string]providerDoc{}
		_ = json.Unmarshal(docsJSON, &docs)
		for _, alertType := range Types() {
			catalog = append(catalog, newTypeInfo(alertType, docs[string(alertType)]))
		}
	})
	return catalog
}

// typeInfo returns the description of alertType
func typeInfo(alertType alert.Type) *TypeInfo {
	for _, info := range Catalog() {
		if info.Type == string(alertType) {
			return info
		}
	}
	return &TypeInfo{Type: string(alertType), Name: string(alertType), Fields: []Field{}}
}

func newTypeInfo(alertType alert.Type, doc providerDoc) *TypeInfo {
	info := &TypeInfo{Type: string(alertType), Name: doc.Name, Category: doc.Category, Untested: doc.Untested, Deprecated: doc.Deprecated, Fields: []Field{}}
	if len(info.Name) == 0 {
		info.Name = string(alertType)
	}
	structType, _ := providerType(alertType)
	if overrides, ok := structType.FieldByName("Overrides"); ok && overrides.Type.Kind() == reflect.Slice {
		info.SupportsOverrides = true
	}
	defaultConfig, ok := structType.FieldByName("DefaultConfig")
	if !ok {
		return info
	}
	configType := defaultConfig.Type
	for i := 0; i < configType.NumField(); i++ {
		structField := configType.Field(i)
		key := yamlName(structField)
		kind := kindOf(structField.Type)
		if len(key) == 0 || key == "client" || len(kind) == 0 {
			continue
		}
		field := Field{Key: key, Kind: kind, Secret: isSecretPath(alertType, key)}
		if fieldDoc, ok := doc.Fields[key]; ok {
			field.Required, field.Default, field.Help = fieldDoc.Required, fieldDoc.Default, fieldDoc.Help
		}
		info.Fields = append(info.Fields, field)
	}
	return info
}

// kindOf returns how a field of a provider configuration is edited, or "" when only YAML can edit it
func kindOf(fieldType reflect.Type) string {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch {
	case fieldType == durationType:
		return KindDuration
	case fieldType.Kind() == reflect.String:
		return KindString
	case fieldType.Kind() == reflect.Bool:
		return KindBoolean
	case strings.HasPrefix(fieldType.Kind().String(), "int") || strings.HasPrefix(fieldType.Kind().String(), "uint"):
		return KindInteger
	case fieldType.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.String:
		return KindList
	case fieldType.Kind() == reflect.Map && fieldType.Elem().Kind() == reflect.String:
		return KindMap
	}
	return ""
}
