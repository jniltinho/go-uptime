package sql

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

// Renaming a managed endpoint must move the same history, uptime and events on every database (fork)
func TestConformance_RenameManagedEndpoint(t *testing.T) {
	stores := newConformanceStores(t, 100, 50)
	original := &endpoint.Endpoint{Name: "before", Group: "rename"}
	renamed := &endpoint.Endpoint{Name: "After", Group: "renamed"}
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, conformance := range stores {
		store := conformance.store
		// Clear does not delete managed endpoints, and the PostgreSQL database is shared between runs
		for _, key := range []string{original.Key(), renamed.Key()} {
			if _, err := store.db.Exec("DELETE FROM managed_endpoints WHERE endpoint_key = $1", key); err != nil {
				t.Fatalf("%s: failed to clean managed endpoints: %v", conformance.name, err)
			}
		}
		for i := 0; i < 6; i++ {
			if err := store.InsertEndpointResult(original, conformanceResult(start.Add(time.Duration(i)*time.Minute), i)); err != nil {
				t.Fatalf("%s: failed to insert result: %v", conformance.name, err)
			}
		}
		if err := store.CreateManagedEndpoint(&common.ManagedEndpoint{Key: original.Key(), Definition: "name: before\n"}, nil); err != nil {
			t.Fatalf("%s: failed to create managed endpoint: %v", conformance.name, err)
		}
		rename := &common.ManagedEndpointRename{OldKey: original.Key(), Name: renamed.Name, Group: renamed.Group, MoveHistory: true}
		if err := store.RenameManagedEndpoint(&common.ManagedEndpoint{Key: renamed.Key(), Definition: "name: After\n"}, 1, rename, nil); err != nil {
			t.Fatalf("%s: failed to rename managed endpoint: %v", conformance.name, err)
		}
	}
	compareWithSQLite(t, stores, func(t *testing.T, store *Store) string {
		uptime, err := store.GetUptimeByKey(renamed.Key(), start.Add(-time.Hour), start.Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to get the uptime of the renamed endpoint: %v", err)
		}
		_, oldKeyErr := store.GetEndpointStatusByKey(original.Key(), paging.NewEndpointStatusParams())
		// Fork: the buckets of the response time chart follow the endpoint
		buckets := bucketsFingerprint(t, store, renamed.Key(), start.Add(-24*time.Hour), start.Add(24*time.Hour))
		if !strings.Contains(buckets, "bucket 60 ") {
			t.Errorf("expected the buckets to follow the renamed endpoint, got %q", buckets)
		}
		return fmt.Sprintf("%s\nuptime=%.4f\nold key: %v\n%s", endpointFingerprint(t, store, renamed.Key()), uptime, oldKeyErr, buckets)
	})
}
