// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

// ViewsPayload is the views field of Payload (fork): the layouts the public page offers and the order of the
// environments and of the applications. Every value comes from the views block of the definition, or from its
// defaults; the classification of the endpoints by key is never published, only its result on each endpoint.
type ViewsPayload struct {
	// Enabled are the views the page offers, among "list", "grouped" and "matrix". It has at least one item and is
	// ["list"] for a page without views.
	Enabled []string `json:"enabled"`

	// Default is the view a visitor sees first, one of Enabled.
	Default string `json:"default"`

	// GroupLabel is the noun the views use for a group, for example "Client". It is "Group" by default.
	GroupLabel string `json:"groupLabel"`

	// Environments are the environments of the page in display order; the first one is the primary environment. It is
	// an empty array, never null, when the page declares none.
	Environments []EnvironmentPayload `json:"environments"`

	// Applications is the display order of the applications declared by the page. Applications of the endpoints that it
	// does not list come after them, alphabetically. It is an empty array, never null, when the page declares none.
	Applications []string `json:"applications"`

	// Groups are the description and the tags of the groups reached only through featured endpoints, which have no item
	// in the groups field of Payload to carry them. It is an empty array, never null, when there is none.
	Groups []GroupDetailsPayload `json:"groups"`
}

// GroupDetailsPayload is the description and the tags of a group of the page, an item of the groups field of
// ViewsPayload (fork)
type GroupDetailsPayload struct {
	// Name is the name of the group, as configured.
	Name string `json:"name"`

	// Description is the description of the group, omitted when it has none.
	Description string `json:"description,omitempty"`

	// Tags are the tags of the group, at most 5, omitted when it has none.
	Tags []string `json:"tags,omitempty"`
}

// EnvironmentPayload is an environment of the views of a page, an item of the environments field of ViewsPayload
// (fork)
type EnvironmentPayload struct {
	// ID identifies the environment in the environment field of the endpoints: 1 to 20 lowercase letters, digits or
	// hyphens.
	ID string `json:"id"`

	// Name is the name of the environment, the id when the page gives none.
	Name string `json:"name"`

	// Short is the short name of the environment, at most 8 characters.
	Short string `json:"short"`
}

// buildViewsPayload returns the views field of the payload of a page
func buildViewsPayload(page *pageconfig.Page) ViewsPayload {
	views := page.Views
	payload := ViewsPayload{
		Enabled:      views.EffectiveEnabled(),
		Default:      views.EffectiveDefault(),
		GroupLabel:   views.EffectiveGroupLabel(),
		Environments: []EnvironmentPayload{},
		Applications: []string{},
		Groups:       []GroupDetailsPayload{},
	}
	if views == nil {
		return payload
	}
	for _, environment := range views.Environments {
		payload.Environments = append(payload.Environments, EnvironmentPayload{ID: environment.ID, Name: environment.DisplayName(), Short: environment.ShortName()})
	}
	payload.Applications = append(payload.Applications, views.Applications...)
	return payload
}

// classify returns the application and the environment of an endpoint of a page: its classification, with its name as
// the application when the classification has none, and no environment without classification
func classify(page *pageconfig.Page, ref EndpointRef) (application, environment string) {
	application = ref.Name
	if classification := page.Views.Classification(ref.Key); classification != nil {
		if classification.Application != "" {
			application = classification.Application
		}
		environment = classification.Environment
	}
	return application, environment
}

// applyGroupDetails sets the description and the tags of the views block on a group of the payload
func applyGroupDetails(page *pageconfig.Page, group *GroupPayload) {
	if details := page.Views.GroupDetailsOf(group.Name); details != nil {
		group.Description = details.Description
		if len(details.Tags) > 0 {
			group.Tags = append([]string(nil), details.Tags...)
		}
	}
}

// featuredGroupDetails returns the details of the groups reached only through featured endpoints, in the order of the
// featured endpoints
func featuredGroupDetails(page *pageconfig.Page, selection Selection) []GroupDetailsPayload {
	details := []GroupDetailsPayload{}
	if page.Views == nil {
		return details
	}
	seen := make(map[string]struct{}, len(selection.Sections))
	for _, section := range selection.Sections {
		seen[section.Group] = struct{}{}
	}
	for _, ref := range selection.Featured {
		if _, done := seen[ref.Group]; done {
			continue
		}
		seen[ref.Group] = struct{}{}
		if group := page.Views.GroupDetailsOf(pageconfig.NormalizeGroup(ref.Group)); group != nil {
			details = append(details, GroupDetailsPayload{Name: ref.Group, Description: group.Description, Tags: append([]string(nil), group.Tags...)})
		}
	}
	return details
}
