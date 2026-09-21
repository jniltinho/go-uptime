package sql

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

func TestStore_LoginSessions(t *testing.T) {
	for driver, store := range managedEndpointTestStores(t) {
		t.Run(driver, func(t *testing.T) {
			// The PostgreSQL database is shared between runs
			if _, err := store.db.Exec("DELETE FROM login_sessions WHERE username LIKE 'login-test-%'"); err != nil {
				t.Fatalf("failed to clean login sessions: %v", err)
			}
			now := time.UnixMilli(time.Now().UnixMilli())
			active := &common.LoginSession{TokenHash: strings.Repeat("a", 64), Username: "login-test-admin", CredentialFingerprint: strings.Repeat("f", 64), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
			expired := &common.LoginSession{TokenHash: strings.Repeat("b", 64), Username: "login-test-admin", CredentialFingerprint: strings.Repeat("f", 64), CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
			for _, session := range []*common.LoginSession{active, expired} {
				if err := store.CreateLoginSession(session); err != nil {
					t.Fatalf("failed to create login session: %v", err)
				}
			}
			if err := store.CreateLoginSession(active); err == nil {
				t.Error("expected an error for a duplicated token hash")
			}
			session, err := store.GetLoginSession(active.TokenHash)
			if err != nil {
				t.Fatalf("failed to get login session: %v", err)
			}
			if session.TokenHash != active.TokenHash || session.Username != active.Username || session.CredentialFingerprint != active.CredentialFingerprint || !session.CreatedAt.Equal(active.CreatedAt) || !session.ExpiresAt.Equal(active.ExpiresAt) {
				t.Errorf("expected %+v, got %+v", active, session)
			}
			if _, err := store.GetLoginSession(strings.Repeat("c", 64)); !errors.Is(err, common.ErrLoginSessionNotFound) {
				t.Errorf("expected ErrLoginSessionNotFound, got %v", err)
			}
			if deleted, err := store.DeleteExpiredLoginSessions(now); err != nil || deleted < 1 {
				t.Errorf("expected the expired session to be deleted, got deleted=%d err=%v", deleted, err)
			}
			if _, err := store.GetLoginSession(expired.TokenHash); !errors.Is(err, common.ErrLoginSessionNotFound) {
				t.Errorf("expected the expired session to be gone, got %v", err)
			}
			if _, err := store.GetLoginSession(active.TokenHash); err != nil {
				t.Errorf("expected the active session to be kept, got %v", err)
			}
			if err := store.DeleteLoginSession(active.TokenHash); err != nil {
				t.Fatalf("failed to delete login session: %v", err)
			}
			if _, err := store.GetLoginSession(active.TokenHash); !errors.Is(err, common.ErrLoginSessionNotFound) {
				t.Errorf("expected the deleted session to be gone, got %v", err)
			}
			if err := store.DeleteLoginSession(active.TokenHash); err != nil {
				t.Errorf("expected no error when deleting a missing session, got %v", err)
			}
		})
	}
}
