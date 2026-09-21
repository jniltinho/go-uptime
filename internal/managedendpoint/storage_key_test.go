// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package managedendpoint

import (
	"errors"
	"strings"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/storage"
)

func TestPrepare_MySQLKeyLength(t *testing.T) {
	definition := []byte("name: " + strings.Repeat("a", 800) + "\ngroup: web\nurl: https://example.org\nconditions: [\"[STATUS] == 200\"]\n")
	ctx := newTestContext()
	ctx.Config.Storage = &storage.Config{Type: storage.TypeMySQL}
	if _, err := Prepare(definition, ctx); !errors.Is(err, ErrInvalidDefinition) || !strings.Contains(err.Error(), "at most 768 characters") {
		t.Errorf("expected an invalid definition citing the limit of the mysql storage, got %v", err)
	}
	ctx.Config.Storage = &storage.Config{Type: storage.TypePostgres}
	if _, err := Prepare(definition, ctx); err != nil {
		t.Errorf("expected no key length limit with postgres, got %v", err)
	}
}
