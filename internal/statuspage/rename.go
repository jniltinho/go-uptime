// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"slices"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"gopkg.in/yaml.v3"
)

func init() {
	managedendpoint.RegisterKeyRenameParticipant(keyRenameParticipant{})
}

// keyRenameParticipant updates the managed status pages that select a renamed managed endpoint by key, and lists the
// status pages of the configuration file that do
type keyRenameParticipant struct{}

// PrepareKeyRename holds mutex from the moment it is called until the returned plan is committed or discarded, so that
// no managed status page changes in between
func (keyRenameParticipant) PrepareKeyRename(oldKey, newKey, author string) (*managedendpoint.KeyRenamePlan, error) {
	mutex.Lock()
	released := false
	release := func() {
		if !released {
			released = true
			mutex.Unlock()
		}
	}
	plan := &managedendpoint.KeyRenamePlan{Discard: release}
	if snap := current.Load(); snap != nil {
		for _, state := range sortedStates(snap.configStates) {
			if state.Page != nil && (slices.Contains(state.Page.Endpoints, oldKey) || slices.Contains(state.Page.Featured, oldKey)) {
				plan.AffectedConfigStatusPages = append(plan.AffectedConfigStatusPages, managedendpoint.AffectedStatusPage{Slug: state.Slug, Title: state.Page.Title})
			}
		}
		for _, state := range sortedStates(snap.managedStates) {
			// Pages in conflict are updated too: their definition is kept and may be published again later
			page, err := Parse([]byte(state.Stored.Definition))
			if err != nil {
				continue
			}
			endpoints, endpointsChanged := replaceEndpointKey(page.Endpoints, oldKey, newKey)
			featured, featuredChanged := replaceEndpointKey(page.Featured, oldKey, newKey)
			if !endpointsChanged && !featuredChanged {
				continue
			}
			page.Endpoints, page.Featured = endpoints, featured
			definition, err := yaml.Marshal(page)
			if err != nil {
				release()
				return nil, err
			}
			plan.StatusPages = append(plan.StatusPages, &common.ManagedStatusPageUpdate{
				StatusPage:      &common.ManagedStatusPage{Slug: state.Slug, Definition: string(definition), CreatedAt: state.Stored.CreatedAt, UpdatedBy: author},
				ExpectedVersion: state.Stored.Version,
			})
		}
	}
	plan.Commit = func() {
		defer release()
		if len(plan.StatusPages) == 0 {
			return
		}
		next := cloneCurrentSnapshot()
		for _, update := range plan.StatusPages {
			next.managedStates[update.StatusPage.Slug] = newManagedState(update.StatusPage, next.configStates)
		}
		publish(next)
		for _, update := range plan.StatusPages {
			_ = publicCache.DeleteKeysByPattern(update.StatusPage.Slug + "|*")
			logr.Infof("[statuspage.PrepareKeyRename] Managed status page with slug=%s updated by %s: endpoint key=%s renamed to key=%s", update.StatusPage.Slug, auditAuthor(author), oldKey, newKey)
		}
	}
	return plan, nil
}

// replaceEndpointKey returns keys with oldKey replaced by newKey, without duplicating newKey, and whether oldKey was
// found
func replaceEndpointKey(keys []string, oldKey, newKey string) ([]string, bool) {
	if !slices.Contains(keys, oldKey) {
		return keys, false
	}
	replaced := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == oldKey {
			key = newKey
		}
		if !slices.Contains(replaced, key) {
			replaced = append(replaced, key)
		}
	}
	return replaced, true
}
