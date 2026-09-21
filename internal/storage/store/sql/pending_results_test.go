// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

// Pending results do not create events, and the other results are compared with the last healthy or unhealthy event,
// on every database (fork)
func TestConformance_PendingResults(t *testing.T) {
	stores := newConformanceStores(t, 10, 50)
	start := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	up := func(offset int) *endpoint.Result {
		return &endpoint.Result{Success: true, Timestamp: start.Add(time.Duration(offset) * time.Second)}
	}
	down := func(offset int) *endpoint.Result {
		return &endpoint.Result{Success: false, Errors: []string{"down"}, Timestamp: start.Add(time.Duration(offset) * time.Second)}
	}
	pending := func(offset int) *endpoint.Result {
		return &endpoint.Result{Pending: true, Message: "Aguardando", Origin: endpoint.ResultOriginPush, Timestamp: start.Add(time.Duration(offset) * time.Second)}
	}
	manyPending := []*endpoint.Result{down(0)}
	for i := 1; i <= 25; i++ {
		manyPending = append(manyPending, pending(i))
	}
	manyPending = append(manyPending, down(26), up(27))
	sequences := map[string][]*endpoint.Result{
		"first-pending":     {pending(0), up(1)},
		"up-pending-down":   {up(0), pending(1), down(2)},
		"up-pending-up":     {up(0), pending(1), up(2)},
		"many-pending":      manyPending,
		"without-pending":   {up(0), down(1), down(2), up(3)},
		"only-pending":      {pending(0), pending(1)},
		"pending-then-down": {pending(0), down(1), pending(2), up(3)},
	}
	for _, conformance := range stores {
		for name, results := range sequences {
			ep := &endpoint.Endpoint{Name: name, Group: "pending"}
			for _, result := range results {
				if err := conformance.store.InsertEndpointResult(ep, result); err != nil {
					t.Fatalf("%s: failed to insert result: %v", conformance.name, err)
				}
			}
		}
	}
	events := func(t *testing.T, store *Store, name string) string {
		status, err := store.GetEndpointStatusByKey("pending_"+name, paging.NewEndpointStatusParams().WithResults(1, 50).WithEvents(1, 50))
		if err != nil {
			t.Fatalf("failed to get the status of %s: %v", name, err)
		}
		var types []string
		for _, event := range status.Events {
			types = append(types, string(event.Type))
		}
		var pendingResults int
		for _, result := range status.Results {
			if result.Pending {
				pendingResults++
				if result.Success || result.Message != "Aguardando" {
					t.Errorf("expected a pending result without success and with its message, got %+v", result)
				}
			}
		}
		return fmt.Sprintf("%s pending=%d", strings.Join(types, ","), pendingResults)
	}
	fingerprint := func(t *testing.T, store *Store) string {
		var lines []string
		for _, name := range []string{"first-pending", "up-pending-down", "up-pending-up", "many-pending", "without-pending", "only-pending", "pending-then-down"} {
			lines = append(lines, name+": "+events(t, store, name))
		}
		return strings.Join(lines, "\n")
	}
	compareWithSQLite(t, stores, fingerprint)
	expected := map[string]string{
		"first-pending":     "START,HEALTHY pending=1",
		"up-pending-down":   "START,HEALTHY,UNHEALTHY pending=1",
		"up-pending-up":     "START,HEALTHY pending=1",
		"many-pending":      "START,UNHEALTHY,HEALTHY pending=15",
		"without-pending":   "START,HEALTHY,UNHEALTHY,HEALTHY pending=0",
		"only-pending":      "START pending=2",
		"pending-then-down": "START,UNHEALTHY,HEALTHY pending=2",
	}
	for name, want := range expected {
		if got := events(t, stores[0].store, name); got != want {
			t.Errorf("%s: expected %q, got %q", name, want, got)
		}
	}
	// Summaries of the public status pages
	summaries, err := stores[0].store.GetEndpointSummaries([]string{"pending_only-pending"}, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if results := summaries["pending_only-pending"].Results; len(results) != 2 || !results[1].Pending || results[1].Message != "Aguardando" || results[1].Origin != endpoint.ResultOriginPush {
		t.Errorf("expected the pending mark, the message and the origin in the summaries, got %+v", results)
	}
}

// A database created by a previous version of the fork, without the pending column, gets it without losing the
// messages, and the schema can be created again (fork)
func TestStore_PendingColumnMigration(t *testing.T) {
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			ep := &endpoint.Endpoint{Name: "migration", Group: "pending-" + driver}
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Message: "before", Origin: endpoint.ResultOriginPush}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec("ALTER TABLE endpoint_result_messages DROP COLUMN pending"); err != nil {
				t.Fatalf("failed to drop the column: %v", err)
			}
			for i := 0; i < 2; i++ {
				if err := store.createEndpointResultMessagesSchema(); err != nil {
					t.Fatalf("failed to create the schema again: %v", err)
				}
			}
			if err := store.InsertEndpointResult(ep, &endpoint.Result{Timestamp: time.Now(), Pending: true}); err != nil {
				t.Fatal(err)
			}
			status, err := store.GetEndpointStatusByKey(ep.Key(), paging.NewEndpointStatusParams().WithResults(1, 10))
			if err != nil {
				t.Fatal(err)
			}
			last := len(status.Results) - 1
			if last < 1 || status.Results[last-1].Message != "before" || status.Results[last-1].Pending || !status.Results[last].Pending {
				t.Errorf("expected the old message kept and the new pending result, got %+v", status.Results)
			}
		})
	}
}
