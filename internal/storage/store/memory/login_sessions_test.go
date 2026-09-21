package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func TestStore_LoginSessions(t *testing.T) {
	store, _ := NewStore(10, 10)
	now := time.Now()
	active := &common.LoginSession{TokenHash: "active", Username: "admin", CredentialFingerprint: "fingerprint", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	expired := &common.LoginSession{TokenHash: "expired", Username: "admin", CredentialFingerprint: "fingerprint", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now}
	for _, session := range []*common.LoginSession{active, expired} {
		if err := store.CreateLoginSession(session); err != nil {
			t.Fatalf("failed to create login session: %v", err)
		}
	}
	if session, err := store.GetLoginSession("active"); err != nil || *session != *active {
		t.Errorf("expected %+v, got %+v %v", active, session, err)
	}
	if _, err := store.GetLoginSession("missing"); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected ErrLoginSessionNotFound, got %v", err)
	}
	if deleted, err := store.DeleteExpiredLoginSessions(now); err != nil || deleted != 1 {
		t.Errorf("expected the session expiring now to be deleted, got deleted=%d err=%v", deleted, err)
	}
	if _, err := store.GetLoginSession("expired"); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected the expired session to be gone, got %v", err)
	}
	if err := store.DeleteLoginSession("active"); err != nil {
		t.Fatalf("failed to delete login session: %v", err)
	}
	if _, err := store.GetLoginSession("active"); !errors.Is(err, common.ErrLoginSessionNotFound) {
		t.Errorf("expected the deleted session to be gone, got %v", err)
	}
}
