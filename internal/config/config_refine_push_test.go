package config

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// The configuration file accepts heartbeat retries on external endpoints and show-messages on status pages (fork)
func TestParseAndValidateConfigBytesWithRetriesAndShowMessages(t *testing.T) {
	cfg, err := parseAndValidateConfigBytes([]byte(`
endpoints:
  - name: website
    url: https://twin.sh/health
    conditions:
      - "[STATUS] == 200"
external-endpoints:
  - name: backup
    group: jobs
    token: backup-token
    heartbeat:
      interval: 1m
      retries: 2
status-pages:
  pages:
    - slug: jobs
      title: Jobs
      groups: [jobs]
      show-messages: true
`))
	if err != nil {
		t.Fatalf("expected a valid configuration, got %v", err)
	}
	if cfg.ExternalEndpoints[0].Heartbeat.Retries != 2 || !cfg.StatusPages.Pages[0].ShowMessages {
		t.Errorf("expected retries and show-messages, got %+v %+v", cfg.ExternalEndpoints[0].Heartbeat, cfg.StatusPages.Pages[0])
	}
	_, err = parseAndValidateConfigBytes([]byte(`
endpoints:
  - name: website
    url: https://twin.sh/health
    conditions:
      - "[STATUS] == 200"
external-endpoints:
  - name: backup
    group: jobs
    token: backup-token
    heartbeat:
      retries: 2
`))
	if !errors.Is(err, endpoint.ErrExternalEndpointHeartbeatRetriesWithoutInterval) {
		t.Errorf("expected retries without interval to be invalid, got %v", err)
	}
}
