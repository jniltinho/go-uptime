// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	// overridesPrefix starts the paths of the fields of a group override, which share the classification of the
	// fields of the default configuration
	overridesPrefix = "overrides[]."

	// defaultAlertField is the default alert of a provider, which holds no secret
	defaultAlertField = "default-alert"
)

var durationType = reflect.TypeOf(time.Duration(0))

// fieldPaths returns the YAML paths of every leaf field of a provider type, sorted. A field of a struct is joined with
// a dot, an element of a slice of structs with "[]." and the values of a map with ".*". Inline fields are flattened,
// like yaml.v3 does, and fields tagged "-" are skipped.
func fieldPaths(providerType reflect.Type) []string {
	paths := make(map[string]bool)
	walkFields(providerType, "", paths, map[reflect.Type]bool{})
	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	return sorted
}

func walkFields(fieldType reflect.Type, prefix string, paths map[string]bool, visiting map[reflect.Type]bool) {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch {
	case fieldType == durationType:
		paths[prefix] = true
	case fieldType.Kind() == reflect.Struct:
		if visiting[fieldType] {
			return
		}
		visiting[fieldType] = true
		defer delete(visiting, fieldType)
		for i := 0; i < fieldType.NumField(); i++ {
			field := fieldType.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("yaml")
			name, options, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if strings.Contains(options, "inline") {
				walkFields(field.Type, prefix, paths, visiting)
				continue
			}
			if len(name) == 0 {
				name = strings.ToLower(field.Name)
			}
			walkFields(field.Type, joinPath(prefix, name), paths, visiting)
		}
	case fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array:
		element := fieldType.Elem()
		for element.Kind() == reflect.Pointer {
			element = element.Elem()
		}
		if element.Kind() == reflect.Struct && element != durationType {
			walkFields(element, prefix+"[]", paths, visiting)
			return
		}
		paths[prefix] = true
	case fieldType.Kind() == reflect.Map:
		walkFields(fieldType.Elem(), joinPath(prefix, "*"), paths, visiting)
	default:
		if len(prefix) > 0 {
			paths[prefix] = true
		}
	}
}

func joinPath(prefix, name string) string {
	if len(prefix) == 0 {
		return name
	}
	if strings.HasSuffix(prefix, "[]") {
		return prefix + "." + name
	}
	return prefix + "." + name
}
