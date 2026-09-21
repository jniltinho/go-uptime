// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"sort"
	"strings"

	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
)

// EndpointRef identifies an endpoint that can be published on a status page. Only fields that the watchdog never
// changes are copied, so that monitored endpoints are never read concurrently with their evaluation.
type EndpointRef struct {
	Key   string
	Name  string
	Group string
}

// Section is a group of endpoints shown on a status page. Endpoints without group are in a section with an empty name.
type Section struct {
	Group     string
	Endpoints []EndpointRef
}

// Selection is the ordered list of the featured endpoints and of the sections of a status page
type Selection struct {
	// Featured are the featured endpoints, in the order of the page. They are not repeated in Sections.
	Featured []EndpointRef

	Sections []Section

	// Truncated is whether endpoints were left out because the page selects more than the limit given to Select
	Truncated bool
}

// Keys returns the keys of the selected endpoints, in display order: the featured endpoints, then the sections
func (selection Selection) Keys() []string {
	keys := make([]string, 0, len(selection.Featured))
	for _, ref := range selection.Featured {
		keys = append(keys, ref.Key)
	}
	for _, section := range selection.Sections {
		for _, ref := range section.Endpoints {
			keys = append(keys, ref.Key)
		}
	}
	return keys
}

// Refs returns the selected endpoints, in display order: the featured endpoints, then the sections
func (selection Selection) Refs() []EndpointRef {
	refs := append([]EndpointRef{}, selection.Featured...)
	for _, section := range selection.Sections {
		refs = append(refs, section.Endpoints...)
	}
	return refs
}

// Select returns the endpoints of refs selected by the page, in display order:
//   - the featured endpoints, in the order of page.Featured, which are left out of the sections;
//   - sections follow the order of page.Groups, then the groups only reached through page.Endpoints in alphabetical
//     order, then the endpoints without group;
//   - endpoints are ordered by name, ignoring case, within each section;
//   - at most limit endpoints are kept, the featured ones first. The limit is the one of the snapshot the page was
//     read from (Published.MaximumEndpoints): it also decides which endpoints the routes of the page may serve, so
//     every caller must use the limit captured with the page, never the configuration.
func Select(page *pageconfig.Page, refs []EndpointRef, limit int) Selection {
	var selection Selection
	remaining := max(limit, 0)
	refsByKey := make(map[string]EndpointRef, len(refs))
	for _, ref := range refs {
		refsByKey[ref.Key] = ref
	}
	featuredKeys := make(map[string]struct{}, len(page.Featured))
	for _, key := range page.Featured {
		ref, exists := refsByKey[key]
		if !exists {
			continue
		}
		featuredKeys[key] = struct{}{}
		if remaining == 0 {
			selection.Truncated = true
			continue
		}
		selection.Featured = append(selection.Featured, ref)
		remaining--
	}
	groupOrder := make(map[string]int, len(page.Groups))
	for i, group := range page.Groups {
		groupOrder[group] = i
	}
	selectedKeys := make(map[string]struct{}, len(page.Endpoints))
	for _, key := range page.Endpoints {
		selectedKeys[key] = struct{}{}
	}
	endpointsByGroup := make(map[string][]EndpointRef)
	for _, ref := range refs {
		if _, featured := featuredKeys[ref.Key]; featured {
			continue
		}
		group := pageconfig.NormalizeGroup(ref.Group)
		_, byGroup := groupOrder[group]
		_, byKey := selectedKeys[ref.Key]
		if !byGroup && !byKey {
			continue
		}
		endpointsByGroup[group] = append(endpointsByGroup[group], ref)
	}
	groups := make([]string, 0, len(endpointsByGroup))
	for group := range endpointsByGroup {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groupLess(groups[i], groups[j], groupOrder)
	})
	for _, group := range groups {
		endpoints := endpointsByGroup[group]
		sort.Slice(endpoints, func(i, j int) bool {
			nameI, nameJ := strings.ToLower(endpoints[i].Name), strings.ToLower(endpoints[j].Name)
			if nameI != nameJ {
				return nameI < nameJ
			}
			return endpoints[i].Key < endpoints[j].Key
		})
		if remaining == 0 {
			selection.Truncated = true
			break
		}
		if len(endpoints) > remaining {
			endpoints = endpoints[:remaining]
			selection.Truncated = true
		}
		remaining -= len(endpoints)
		selection.Sections = append(selection.Sections, Section{Group: group, Endpoints: endpoints})
	}
	return selection
}

// groupLess orders the groups of page.Groups first, then the other named groups alphabetically, then the empty group
func groupLess(a, b string, groupOrder map[string]int) bool {
	orderA, selectedA := groupOrder[a]
	orderB, selectedB := groupOrder[b]
	switch {
	case selectedA && selectedB:
		return orderA < orderB
	case selectedA != selectedB:
		return selectedA
	case (a == "") != (b == ""):
		return b == ""
	case strings.ToLower(a) != strings.ToLower(b):
		return strings.ToLower(a) < strings.ToLower(b)
	default:
		return a < b
	}
}
