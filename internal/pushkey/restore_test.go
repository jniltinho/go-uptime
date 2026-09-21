// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package pushkey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	pushconfig "github.com/jniltinho/go-uptime/v7/internal/config/push"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
)

func noEndpointTokens(fn func(isEndpointTokenHash func([sha256.Size]byte) bool) error) error {
	return fn(func([sha256.Size]byte) bool { return false })
}

func TestRestore(t *testing.T) {
	setupPushKeyTest(t)
	const token = "restored-token-0123456789abcdef"
	hash := pushconfig.HashToken(token)
	tokenHash := hex.EncodeToString(hash[:])
	restored, err := Restore("akamai", tokenHash, "cdef", "admin", noEndpointTokens)
	if err != nil || restored.Name != "akamai" || restored.Hint != "cdef" || restored.Origin != OriginAdmin {
		t.Fatalf("unexpected restored key: %+v %v", restored, err)
	}
	if name, ok := Lookup(token); !ok || name != "akamai" {
		t.Error("expected the restored key to accept its original token")
	}
	if existing, ok := FindByName("akamai", hash); !ok || !existing.SameHash {
		t.Error("expected the restored key to be found with the same hash")
	}
	if _, err := Restore("akamai", strings.Repeat("a", 64), "", "admin", noEndpointTokens); !errors.Is(err, ErrNameInUse) {
		t.Errorf("expected ErrNameInUse, got %v", err)
	}
	if _, err := Restore("other", tokenHash, "", "admin", noEndpointTokens); !errors.Is(err, ErrHashInUse) {
		t.Errorf("expected ErrHashInUse for the hash of another key, got %v", err)
	}
	yamlHash := pushconfig.HashToken("yaml-key-token-12345")
	if _, err := Restore("yaml-copy", hex.EncodeToString(yamlHash[:]), "", "admin", noEndpointTokens); !errors.Is(err, ErrHashInUse) {
		t.Errorf("expected ErrHashInUse for the hash of a key of the configuration file, got %v", err)
	}
	endpointHash := strings.Repeat("b", 64)
	endpointGuard := func(fn func(isEndpointTokenHash func([sha256.Size]byte) bool) error) error {
		return fn(func(candidate [sha256.Size]byte) bool { return hex.EncodeToString(candidate[:]) == endpointHash })
	}
	if _, err := Restore("endpoint", endpointHash, "", "admin", endpointGuard); !errors.Is(err, ErrHashInUse) {
		t.Errorf("expected ErrHashInUse for the hash of the token of an endpoint, got %v", err)
	}
	for name, input := range map[string][3]string{
		"uppercase-hash": {"upper", strings.ToUpper(tokenHash), ""},
		"short-hash":     {"short", "abc", ""},
		"spaces":         {" spaced ", strings.Repeat("c", 64), ""},
		"long-hint":      {"hint", strings.Repeat("d", 64), "abcde"},
		"invalid-hint":   {"hint", strings.Repeat("d", 64), "a b"},
	} {
		if _, err := Restore(input[0], input[1], input[2], "admin", noEndpointTokens); !errors.Is(err, ErrInvalidRestoredKey) {
			t.Errorf("%s: expected ErrInvalidRestoredKey, got %v", name, err)
		}
	}
	lifecycle.BeginCycle()
	_, err = Restore("during-reload", strings.Repeat("e", 64), "", "admin", noEndpointTokens)
	lifecycle.EndCycle()
	if !errors.Is(err, ErrCycleInProgress) {
		t.Errorf("expected ErrCycleInProgress during a reload, got %v", err)
	}
}
