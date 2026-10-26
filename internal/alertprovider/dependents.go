// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"sync/atomic"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
)

// Dependents is what depends on the managed providers: the endpoints managed through the administration. This package
// cannot import them (they import it to validate their alerts), so they register themselves through RegisterDependents.
//
// Lock order of a provider change: lifecycle change, Dependents.Lock, the mutex of this package, the transaction.
// Endpoint writes take the lock of Dependents, so they are serialized with provider changes and an endpoint can never
// attach a provider that is being deleted.
type Dependents interface {
	// Lock takes the lock of the dependents' changes and returns the function that releases it
	Lock() (unlock func())

	// Users returns the keys of every managed endpoint whose stored definition has an alert of alertType, enabled or
	// not, valid or not, sorted. It must not take the lock of Lock.
	Users(alertType alert.Type) []string

	// Plan prepares again, against alertingConfig, every managed endpoint whose stored definition has an alert of
	// alertType, without changing anything. It returns a *DependentsError when an endpoint that was valid would become
	// invalid. It is called with the lock of Lock held.
	Plan(alertingConfig *alerting.Config, alertType alert.Type) (DependentsPlan, error)
}

// DependentsPlan applies a prepared change to the dependents
type DependentsPlan interface {
	// Keys returns the keys of the endpoints the plan restarts, sorted
	Keys() []string

	// Stop stops the affected endpoints, waiting for their in-flight executions
	Stop()

	// Start publishes and starts the affected endpoints prepared by the plan, after the configuration was published, and
	// returns the problems found
	Start() []string
}

var dependents atomic.Pointer[Dependents]

// RegisterDependents registers the dependents of the managed providers. It is called once, from an init function.
func RegisterDependents(registered Dependents) {
	dependents.Store(&registered)
}

func getDependents() Dependents {
	if registered := dependents.Load(); registered != nil {
		return *registered
	}
	return nil
}
