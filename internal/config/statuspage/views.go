// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// The views a status page can offer (fork)
const (
	// ViewList is the layout of the groups with one row per endpoint, the only one of a page without views
	ViewList = "list"

	// ViewGrouped is the layout of the groups split by environment, with filters and counters
	ViewGrouped = "grouped"

	// ViewMatrix is the table of the groups by application
	ViewMatrix = "matrix"
)

// Limits of the views block of a page (fork)
const (
	// DefaultGroupLabel is the noun used for a group by the views when group-label is not set
	DefaultGroupLabel = "Group"

	// MaximumGroupLabelLength is the maximum number of characters of group-label
	MaximumGroupLabelLength = 30

	// MaximumEnvironments is the maximum number of environments of a page
	MaximumEnvironments = 10

	// MaximumEnvironmentIDLength is the maximum length of the id of an environment
	MaximumEnvironmentIDLength = 20

	// MaximumEnvironmentNameLength is the maximum number of characters of the name of an environment
	MaximumEnvironmentNameLength = 40

	// MaximumEnvironmentShortLength is the maximum number of characters of the short name of an environment
	MaximumEnvironmentShortLength = 8

	// MaximumApplications is the maximum number of applications listed by a page
	MaximumApplications = 50

	// MaximumApplicationLength is the maximum number of characters of an application
	MaximumApplicationLength = 40

	// MaximumGroupDescriptionLength is the maximum number of characters of the description of a group
	MaximumGroupDescriptionLength = 100

	// MaximumGroupTags is the maximum number of tags of a group
	MaximumGroupTags = 5

	// MaximumGroupTagLength is the maximum number of characters of a tag of a group
	MaximumGroupTagLength = 20
)

var (
	// ErrInvalidViews is returned when the views block of a page is invalid (fork)
	ErrInvalidViews = errors.New("invalid views")

	environmentIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,18}[a-z0-9])?$`)

	knownViews = []string{ViewList, ViewGrouped, ViewMatrix}
)

// Views is the views block of a page (fork): which layouts the public page offers and how the endpoints are placed in
// them. An omitted, null or empty block offers only the list. The defaults are never written back into the struct: the
// Effective* methods compute them, so that the definition marshalled again is what the administrator wrote.
type Views struct {
	// Enabled are the views the page offers, among "list", "grouped" and "matrix", without duplicates. Omitted means
	// ["list"]; an explicit empty list is invalid.
	Enabled []string `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// Default is the view a visitor sees first. It must be enabled; omitted means the first enabled view.
	Default string `yaml:"default,omitempty" json:"default,omitempty"`

	// GroupLabel is the noun the views use for a group, for example "Client", with 1 to 30 characters. Omitted means
	// "Group".
	GroupLabel string `yaml:"group-label,omitempty" json:"group-label,omitempty"`

	// Environments are the environments of the page, in display order, at most 10. The first one is the primary
	// environment: the production of the incident counters and of the uptime of the matrix.
	Environments []*Environment `yaml:"environments,omitempty" json:"environments,omitempty"`

	// Applications is the display order of the applications, at most 50 of 1 to 40 characters. Applications that are
	// not listed come after the listed ones, alphabetically.
	Applications []string `yaml:"applications,omitempty" json:"applications,omitempty"`

	// Groups holds, by group name, the description and the tags the views show for a group, at most 50 groups
	Groups map[string]*GroupDetails `yaml:"groups,omitempty" json:"groups,omitempty"`

	// Endpoints classifies the endpoints by key (the lowercase <group>_<name> key) into an application and an
	// environment. An endpoint without entry uses its name as application and has no environment.
	Endpoints map[string]*EndpointClassification `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
}

// Environment is an environment of the views of a page (fork)
type Environment struct {
	// ID identifies the environment in the classification of the endpoints: 1 to 20 lowercase letters, digits or
	// hyphens, neither starting nor ending with a hyphen.
	ID string `yaml:"id" json:"id"`

	// Name is the name shown for the environment, with at most 40 characters. Omitted means the id.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Short is the short name shown on the chips of the environment, with at most 8 characters. Omitted means the id in
	// uppercase, cut to 8 characters.
	Short string `yaml:"short,omitempty" json:"short,omitempty"`
}

// GroupDetails is what the views show about a group besides its name (fork)
type GroupDetails struct {
	// Description is shown below the name of the group, with at most 100 characters
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Tags are shown next to the name of the group, at most 5 of 1 to 20 characters
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// EndpointClassification places an endpoint in the views of a page (fork). At least one of its fields is set.
type EndpointClassification struct {
	// Application is the application of the endpoint, with at most 40 characters. Omitted means the name of the
	// endpoint.
	Application string `yaml:"application,omitempty" json:"application,omitempty"`

	// Environment is the id of one of the environments of the page. Omitted means no environment.
	Environment string `yaml:"environment,omitempty" json:"environment,omitempty"`
}

// UnmarshalYAML decodes the block strictly, in the configuration file too: a misspelled field of the views (an
// environment with "colour", a classification with "env") is an error instead of being ignored.
func (v *Views) UnmarshalYAML(node *yaml.Node) error {
	type plain Views
	encoded, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(encoded))
	decoder.KnownFields(true)
	var decoded plain
	if err = decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidViews, err)
	}
	*v = Views(decoded)
	return nil
}

// EffectiveEnabled returns the views the page offers. It is safe to call on a nil Views.
func (v *Views) EffectiveEnabled() []string {
	if v == nil || v.Enabled == nil {
		return []string{ViewList}
	}
	return slices.Clone(v.Enabled)
}

// EffectiveDefault returns the view a visitor sees first. It is safe to call on a nil Views.
func (v *Views) EffectiveDefault() string {
	if v != nil && v.Default != "" {
		return v.Default
	}
	return v.EffectiveEnabled()[0]
}

// EffectiveGroupLabel returns the noun the views use for a group. It is safe to call on a nil Views.
func (v *Views) EffectiveGroupLabel() string {
	if v == nil || v.GroupLabel == "" {
		return DefaultGroupLabel
	}
	return v.GroupLabel
}

// DisplayName returns the name of the environment, or its id when it has none
func (e *Environment) DisplayName() string {
	if e.Name != "" {
		return e.Name
	}
	return e.ID
}

// ShortName returns the short name of the environment, or its id in uppercase cut to MaximumEnvironmentShortLength
// characters
func (e *Environment) ShortName() string {
	if e.Short != "" {
		return e.Short
	}
	short := strings.ToUpper(e.ID)
	if len(short) > MaximumEnvironmentShortLength {
		short = short[:MaximumEnvironmentShortLength]
	}
	return short
}

// Classification returns the classification of the endpoint with the given key, or nil. It is safe to call on a nil
// Views.
func (v *Views) Classification(key string) *EndpointClassification {
	if v == nil {
		return nil
	}
	return v.Endpoints[key]
}

// GroupDetailsOf returns the details of the group with the given name, or nil. It is safe to call on a nil Views.
func (v *Views) GroupDetailsOf(group string) *GroupDetails {
	if v == nil {
		return nil
	}
	return v.Groups[group]
}

// validate validates the block and normalizes its texts: trimmed, the keys of the endpoints in lowercase. It returns
// an error wrapping ErrInvalidViews.
func (v *Views) validate() error {
	if v == nil {
		return nil
	}
	if err := v.validateViews(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidViews, err)
	}
	environments, err := v.validateEnvironments()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidViews, err)
	}
	applications, err := normalizeList(v.Applications, MaximumApplications, MaximumApplicationLength, strings.TrimSpace)
	if err != nil {
		return fmt.Errorf("%w: applications: %w", ErrInvalidViews, err)
	}
	groups, err := v.validateGroups()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidViews, err)
	}
	endpoints, err := v.validateEndpoints(environments)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidViews, err)
	}
	if v.Applications != nil {
		v.Applications = applications
	}
	v.Groups, v.Endpoints = groups, endpoints
	return nil
}

func (v *Views) validateViews() error {
	if v.Enabled != nil && len(v.Enabled) == 0 {
		return errors.New("enabled must list at least one view")
	}
	seen := make(map[string]struct{}, len(v.Enabled))
	for i, view := range v.Enabled {
		view = strings.TrimSpace(view)
		if !slices.Contains(knownViews, view) {
			return fmt.Errorf("unknown view %q (accepted: %s)", view, strings.Join(knownViews, ", "))
		}
		if _, duplicate := seen[view]; duplicate {
			return fmt.Errorf("duplicate view %q", view)
		}
		seen[view] = struct{}{}
		v.Enabled[i] = view
	}
	v.Default = strings.TrimSpace(v.Default)
	if v.Default != "" && !slices.Contains(v.EffectiveEnabled(), v.Default) {
		return fmt.Errorf("default view %q is not enabled", v.Default)
	}
	v.GroupLabel = strings.TrimSpace(v.GroupLabel)
	if utf8.RuneCountInString(v.GroupLabel) > MaximumGroupLabelLength {
		return fmt.Errorf("group-label must have at most %d characters", MaximumGroupLabelLength)
	}
	return nil
}

// validateEnvironments returns the set of the ids of the environments
func (v *Views) validateEnvironments() (map[string]struct{}, error) {
	if len(v.Environments) > MaximumEnvironments {
		return nil, fmt.Errorf("at most %d environments are allowed", MaximumEnvironments)
	}
	ids := make(map[string]struct{}, len(v.Environments))
	for _, environment := range v.Environments {
		if environment == nil {
			return nil, errors.New("empty environment")
		}
		environment.ID = strings.TrimSpace(environment.ID)
		if !environmentIDPattern.MatchString(environment.ID) {
			return nil, fmt.Errorf("environment id %q must have 1 to %d lowercase letters, digits or hyphens, and must not start or end with a hyphen", environment.ID, MaximumEnvironmentIDLength)
		}
		if _, duplicate := ids[environment.ID]; duplicate {
			return nil, fmt.Errorf("duplicate environment %q", environment.ID)
		}
		ids[environment.ID] = struct{}{}
		environment.Name = strings.TrimSpace(environment.Name)
		if utf8.RuneCountInString(environment.Name) > MaximumEnvironmentNameLength {
			return nil, fmt.Errorf("the name of environment %q must have at most %d characters", environment.ID, MaximumEnvironmentNameLength)
		}
		environment.Short = strings.TrimSpace(environment.Short)
		if utf8.RuneCountInString(environment.Short) > MaximumEnvironmentShortLength {
			return nil, fmt.Errorf("the short name of environment %q must have at most %d characters", environment.ID, MaximumEnvironmentShortLength)
		}
	}
	return ids, nil
}

func (v *Views) validateGroups() (map[string]*GroupDetails, error) {
	if v.Groups == nil {
		return nil, nil
	}
	if len(v.Groups) > MaximumGroups {
		return nil, fmt.Errorf("at most %d groups are allowed", MaximumGroups)
	}
	groups := make(map[string]*GroupDetails, len(v.Groups))
	for _, name := range sortedKeys(v.Groups) {
		details := v.Groups[name]
		normalized := NormalizeGroup(name)
		if length := utf8.RuneCountInString(normalized); length == 0 || length > MaximumGroupLength {
			return nil, fmt.Errorf("group names must have 1 to %d characters", MaximumGroupLength)
		}
		if _, duplicate := groups[normalized]; duplicate {
			return nil, fmt.Errorf("duplicate group %q", normalized)
		}
		if details == nil {
			return nil, fmt.Errorf("group %q has no description and no tags", normalized)
		}
		details.Description = strings.TrimSpace(details.Description)
		if utf8.RuneCountInString(details.Description) > MaximumGroupDescriptionLength {
			return nil, fmt.Errorf("the description of group %q must have at most %d characters", normalized, MaximumGroupDescriptionLength)
		}
		if details.Tags != nil {
			tags, err := normalizeList(details.Tags, MaximumGroupTags, MaximumGroupTagLength, strings.TrimSpace)
			if err != nil {
				return nil, fmt.Errorf("tags of group %q: %w", normalized, err)
			}
			details.Tags = tags
		}
		if details.Description == "" && len(details.Tags) == 0 {
			return nil, fmt.Errorf("group %q has no description and no tags", normalized)
		}
		groups[normalized] = details
	}
	return groups, nil
}

func (v *Views) validateEndpoints(environments map[string]struct{}) (map[string]*EndpointClassification, error) {
	if v.Endpoints == nil {
		return nil, nil
	}
	if len(v.Endpoints) > MaximumEndpointKeys {
		return nil, fmt.Errorf("at most %d endpoints are allowed", MaximumEndpointKeys)
	}
	endpoints := make(map[string]*EndpointClassification, len(v.Endpoints))
	for _, rawKey := range sortedKeys(v.Endpoints) {
		classification := v.Endpoints[rawKey]
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if length := utf8.RuneCountInString(key); length == 0 || length > MaximumEndpointKeyLength {
			return nil, fmt.Errorf("endpoint keys must have 1 to %d characters", MaximumEndpointKeyLength)
		}
		if _, duplicate := endpoints[key]; duplicate {
			return nil, fmt.Errorf("duplicate endpoint %q", key)
		}
		if classification == nil {
			return nil, fmt.Errorf("endpoint %q has no application and no environment", key)
		}
		classification.Application = strings.TrimSpace(classification.Application)
		classification.Environment = strings.TrimSpace(classification.Environment)
		if classification.Application == "" && classification.Environment == "" {
			return nil, fmt.Errorf("endpoint %q has no application and no environment", key)
		}
		if utf8.RuneCountInString(classification.Application) > MaximumApplicationLength {
			return nil, fmt.Errorf("the application of endpoint %q must have at most %d characters", key, MaximumApplicationLength)
		}
		if classification.Environment != "" {
			if _, declared := environments[classification.Environment]; !declared {
				return nil, fmt.Errorf("endpoint %q uses environment %q, which is not declared in environments", key, classification.Environment)
			}
		}
		endpoints[key] = classification
	}
	return endpoints, nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
