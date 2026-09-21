// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package pushkey

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

func setupPushKeyTest(t *testing.T) *config.Config {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeSQLite, Path: filepath.Join(t.TempDir(), "go-uptime.db"), MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() { store.Get().Close() })
	cfg := &config.Config{Push: &pushconfig.Config{Keys: []*pushconfig.Key{{Name: "yaml-key", Token: "yaml-key-token-12345"}}}}
	if err := cfg.Push.ValidateAndSetDefaults(); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	return cfg
}

func TestPushKeys_Lifecycle(t *testing.T) {
	cfg := setupPushKeyTest(t)
	if name, ok := Lookup("yaml-key-token-12345"); !ok || name != "yaml-key" {
		t.Errorf("expected the key of the configuration file to be accepted, got %q (ok=%v)", name, ok)
	}
	created, err := Create(" akamai ", "ops@example.com")
	if err != nil {
		t.Fatalf("failed to create push key: %v", err)
	}
	if created.Name != "akamai" || created.ID == 0 || created.Origin != OriginAdmin || len(created.Token) != pushconfig.GeneratedTokenLength || created.Hint != created.Token[len(created.Token)-4:] || created.CreatedBy != "ops@example.com" {
		t.Fatalf("unexpected created push key: %+v", created)
	}
	if name, ok := Lookup(created.Token); !ok || name != "akamai" {
		t.Errorf("expected the created key to be accepted immediately, got %q (ok=%v)", name, ok)
	}
	if _, err := Create("akamai", "ops@example.com"); !errors.Is(err, ErrNameInUse) {
		t.Errorf("expected ErrNameInUse for the same name, got %v", err)
	}
	if _, err := Create("yaml-key", "ops@example.com"); !errors.Is(err, ErrNameInUse) {
		t.Errorf("expected ErrNameInUse for the name of a key of the configuration file, got %v", err)
	}
	if _, err := Create(" ", "ops@example.com"); !errors.Is(err, pushconfig.ErrInvalidKeyName) {
		t.Errorf("expected ErrInvalidKeyName, got %v", err)
	}
	listing := List()
	if len(listing.Keys) != 2 || listing.Keys[0].Origin != OriginConfig || listing.Keys[1].Name != "akamai" || listing.Keys[1].Hint != created.Hint {
		t.Fatalf("expected the key of the configuration file then the created key, got %+v", listing.Keys)
	}
	// The created key survives a reload, only by its hash
	Load(cfg)
	if name, ok := Lookup(created.Token); !ok || name != "akamai" {
		t.Errorf("expected the created key to be accepted after a reload, got %q (ok=%v)", name, ok)
	}
	if err := Delete(created.ID, "ops@example.com"); err != nil {
		t.Fatalf("failed to delete push key: %v", err)
	}
	if _, ok := Lookup(created.Token); ok {
		t.Error("expected the deleted key to be rejected immediately")
	}
	if _, ok := Lookup("yaml-key-token-12345"); !ok {
		t.Error("expected the key of the configuration file to keep being accepted")
	}
	if err := Delete(created.ID, "ops@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound when deleting twice, got %v", err)
	}
}

func TestPushKeys_MemoryStorageAndCycle(t *testing.T) {
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	Load(&config.Config{})
	if _, err := Create("akamai", "ops@example.com"); !errors.Is(err, ErrStorageNotSupported) {
		t.Errorf("expected ErrStorageNotSupported with the memory storage, got %v", err)
	}
	lifecycle.BeginCycle()
	_, err := Create("akamai", "ops@example.com")
	lifecycle.EndCycle()
	if !errors.Is(err, ErrCycleInProgress) {
		t.Errorf("expected ErrCycleInProgress during a cycle, got %v", err)
	}
}
