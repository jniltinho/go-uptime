// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"errors"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func TestStore_PushKeys(t *testing.T) {
	errApply := errors.New("failed to apply")
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			// The PostgreSQL database is shared between runs
			if _, err := store.db.Exec("DELETE FROM push_keys WHERE name LIKE 'push-test-%'"); err != nil {
				t.Fatalf("failed to clean push keys: %v", err)
			}
			hash := strings.Repeat("a", 64)
			created := &common.PushKey{Name: "push-test-akamai", TokenHash: hash, Hint: "End5", CreatedBy: "ops@example.com"}
			if err := store.CreatePushKey(created, nil); err != nil {
				t.Fatalf("failed to create push key: %v", err)
			}
			if created.ID == 0 || created.CreatedAt.IsZero() {
				t.Errorf("expected the id and the creation time to be set, got %+v", created)
			}
			if err := store.CreatePushKey(&common.PushKey{Name: "push-test-akamai", TokenHash: strings.Repeat("b", 64), Hint: "abcd"}, nil); !errors.Is(err, common.ErrPushKeyAlreadyExists) {
				t.Errorf("expected ErrPushKeyAlreadyExists for the same name, got %v", err)
			}
			if err := store.CreatePushKey(&common.PushKey{Name: "push-test-other", TokenHash: hash, Hint: "abcd"}, nil); !errors.Is(err, common.ErrPushKeyAlreadyExists) {
				t.Errorf("expected ErrPushKeyAlreadyExists for the same token, got %v", err)
			}
			if err := store.CreatePushKey(&common.PushKey{Name: "push-test-rolled-back", TokenHash: strings.Repeat("c", 64), Hint: "abcd"}, func() error { return errApply }); !errors.Is(err, errApply) {
				t.Errorf("expected the apply error, got %v", err)
			}
			pushKeys, err := store.ListPushKeys()
			if err != nil {
				t.Fatalf("failed to list push keys: %v", err)
			}
			var names []string
			for _, pushKey := range pushKeys {
				if strings.HasPrefix(pushKey.Name, "push-test-") {
					names = append(names, pushKey.Name)
				}
			}
			if len(names) != 1 || names[0] != "push-test-akamai" {
				t.Fatalf("expected only the created push key, got %v", names)
			}
			for _, pushKey := range pushKeys {
				if pushKey.ID == created.ID && (pushKey.TokenHash != hash || pushKey.Hint != "End5" || pushKey.CreatedBy != "ops@example.com" || !pushKey.CreatedAt.Equal(created.CreatedAt)) {
					t.Errorf("unexpected push key: %+v", pushKey)
				}
			}
			if err := store.DeletePushKey(created.ID, func() error { return errApply }); !errors.Is(err, errApply) {
				t.Errorf("expected the apply error, got %v", err)
			}
			if err := store.DeletePushKey(created.ID, nil); err != nil {
				t.Fatalf("failed to delete push key: %v", err)
			}
			if err := store.DeletePushKey(created.ID, nil); !errors.Is(err, common.ErrPushKeyNotFound) {
				t.Errorf("expected ErrPushKeyNotFound when deleting twice, got %v", err)
			}
		})
	}
}
