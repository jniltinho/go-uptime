package sql

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"
)

// The messages and origins of the results must be read back, cut without breaking UTF-8 characters, and deleted in
// cascade when old results are removed, on every database (fork)
func TestConformance_EndpointResultMessages(t *testing.T) {
	stores := newConformanceStores(t, 3, 50)
	ep := &endpoint.Endpoint{Name: "backup", Group: "push-messages"}
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	// 2 bytes per character: the cut must not split the last one
	longMessage := strings.Repeat("é", endpoint.MaximumResultMessageLength)
	results := []*endpoint.Result{
		{Success: true, Timestamp: start, Message: "Backup OK", Origin: endpoint.ResultOriginPush},
		{Success: false, Timestamp: start.Add(time.Minute), Errors: []string{"Falha no backup: disco cheio"}, Message: "Falha no backup: disco cheio", Origin: endpoint.ResultOriginPush},
		{Success: true, Timestamp: start.Add(2 * time.Minute), HTTPStatus: 200},
		{Success: true, Timestamp: start.Add(3 * time.Minute), Message: longMessage, Origin: endpoint.ResultOriginPush},
	}
	for _, conformance := range stores {
		// Older results, so that the store removes the oldest ones: it only does above the maximum plus a threshold of 10
		for i := 0; i < 10; i++ {
			older := &endpoint.Result{Success: true, Timestamp: start.Add(-time.Duration(10-i) * time.Minute), Message: "older push", Origin: endpoint.ResultOriginPush}
			if err := conformance.store.InsertEndpointResult(ep, older); err != nil {
				t.Fatalf("%s: failed to insert result: %v", conformance.name, err)
			}
		}
		for _, result := range results {
			if err := conformance.store.InsertEndpointResult(ep, result); err != nil {
				t.Fatalf("%s: failed to insert result: %v", conformance.name, err)
			}
		}
	}
	fingerprint := func(t *testing.T, store *Store) string {
		status, err := store.GetEndpointStatusByKey(ep.Key(), paging.NewEndpointStatusParams().WithResults(1, 10))
		if err != nil {
			t.Fatalf("failed to get the status: %v", err)
		}
		var lines []string
		for _, result := range status.Results {
			runes := []rune(result.Message)
			if len(runes) > 12 {
				runes = runes[:12]
			}
			lines = append(lines, fmt.Sprintf("%s success=%v origin=%q bytes=%d valid=%v prefix=%q", result.Timestamp.UTC().Format(time.RFC3339), result.Success, result.Origin, len(result.Message), utf8.ValidString(result.Message), string(runes)))
		}
		var rows int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM endpoint_result_messages").Scan(&rows); err != nil {
			t.Fatalf("failed to count the messages: %v", err)
		}
		return strings.Join(lines, "\n") + fmt.Sprintf("\nmessages=%d", rows)
	}
	compareWithSQLite(t, stores, fingerprint)

	status, err := stores[0].store.GetEndpointStatusByKey(ep.Key(), paging.NewEndpointStatusParams().WithResults(1, 10))
	if err != nil || len(status.Results) != 3 {
		t.Fatalf("expected the 3 latest results, got %v (err=%v)", status, err)
	}
	failure, check, long := status.Results[0], status.Results[1], status.Results[2]
	if failure.Message != "Falha no backup: disco cheio" || failure.Origin != endpoint.ResultOriginPush || failure.Success {
		t.Errorf("expected the pushed failure with its message, got %+v", failure)
	}
	if check.Message != "" || check.Origin != "" {
		t.Errorf("expected no message nor origin for a check of Gatus, got %+v", check)
	}
	if len(long.Message) > endpoint.MaximumResultMessageLength || !utf8.ValidString(long.Message) || !strings.HasPrefix(longMessage, long.Message) {
		t.Errorf("expected the long message to be cut at a character boundary, got %d bytes", len(long.Message))
	}
	var rows int
	if err := stores[0].store.db.QueryRow("SELECT COUNT(*) FROM endpoint_result_messages").Scan(&rows); err != nil || rows != 2 {
		t.Errorf("expected the message of the removed result to be deleted in cascade, got %d rows (err=%v)", rows, err)
	}
}

func TestTruncateResultMessage(t *testing.T) {
	if message := endpoint.TruncateResultMessage("OK"); message != "OK" {
		t.Errorf("expected a short message to be kept, got %q", message)
	}
	message := endpoint.TruncateResultMessage(strings.Repeat("a", endpoint.MaximumResultMessageLength-1) + "é")
	if len(message) != endpoint.MaximumResultMessageLength-1 || !utf8.ValidString(message) {
		t.Errorf("expected the cut before the 2-byte character, got %d bytes", len(message))
	}
}
