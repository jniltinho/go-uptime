// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

import (
	"errors"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/alerting"
	"github.com/jniltinho/go-uptime/v7/internal/alerting/provider/slack"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// fakeStore lists fixed rows, or fails
type fakeStore struct {
	rows []*common.ManagedAlertProvider
	err  error
}

func (f *fakeStore) ListManagedAlertProviders() ([]*common.ManagedAlertProvider, error) {
	return f.rows, f.err
}
func (f *fakeStore) GetManagedAlertProvider(string) (*common.ManagedAlertProvider, error) {
	return nil, common.ErrManagedAlertProviderNotFound
}
func (f *fakeStore) CreateManagedAlertProvider(*common.ManagedAlertProvider, func() error) error {
	return nil
}
func (f *fakeStore) UpdateManagedAlertProvider(*common.ManagedAlertProvider, int64, func() error) error {
	return nil
}
func (f *fakeStore) DeleteManagedAlertProvider(string, int64, func() error) error { return nil }

// useFakeStore makes Load read from fake; the tests that use it must not run in parallel
func useFakeStore(t *testing.T, fake *fakeStore) {
	t.Helper()
	previous := getManagedAlertProviderStore
	getManagedAlertProviderStore = func() (store.ManagedAlertProviderStore, bool) { return fake, true }
	t.Cleanup(func() {
		getManagedAlertProviderStore = previous
		current.Store(nil)
	})
}

func itemOf(t *testing.T, alertType string) *Item {
	t.Helper()
	for _, item := range List().Providers {
		if item.Type == alertType {
			return item
		}
	}
	t.Fatalf("no item for %s", alertType)
	return nil
}

func TestLoad_States(t *testing.T) {
	useFakeStore(t, &fakeStore{rows: []*common.ManagedAlertProvider{
		{Type: "ntfy", Definition: "topic: ops\n", Version: 2},
		{Type: "telegram", Definition: "id: \"1\"\n", Version: 1},
		{Type: "slack", Definition: "webhook-url: https://hooks.example.com/web\n", Version: 1},
	}})
	cfg := &config.Config{Alerting: &alerting.Config{Slack: &slack.AlertProvider{DefaultConfig: slack.Config{WebhookURL: "https://hooks.example.com/file"}}}}
	Load(cfg)
	if ntfy := itemOf(t, "ntfy"); ntfy.Source != SourceWeb || ntfy.State != StateValid || ntfy.Version != 2 {
		t.Errorf("expected ntfy to be a valid web provider, got %+v", ntfy)
	}
	if telegram := itemOf(t, "telegram"); telegram.State != StateInvalid || len(telegram.Error) == 0 {
		t.Errorf("expected telegram without token to be invalid with its error, got %+v", telegram)
	}
	if slackItem := itemOf(t, "slack"); slackItem.Source != SourceYAML || !slackItem.Shadowed {
		t.Errorf("expected slack of the file to shadow the web one, got %+v", slackItem)
	}
	effective := EffectiveFor(cfg)
	if effective.Slack.DefaultConfig.WebhookURL != "https://hooks.example.com/file" || effective.Ntfy == nil || effective.Telegram != nil {
		t.Errorf("expected the file's slack, the managed ntfy and no telegram, got %+v", effective)
	}
	if cfg.Alerting.Ntfy != nil {
		t.Error("the alerting section of the configuration must not be changed")
	}
	if other := (&config.Config{}); EffectiveFor(other) != nil {
		t.Error("a configuration of another cycle must not see the managed providers")
	}
}

func TestLoad_UnavailableIsNotEmpty(t *testing.T) {
	useFakeStore(t, &fakeStore{err: errors.New("connection reset")})
	cfg := &config.Config{Alerting: &alerting.Config{Slack: &slack.AlertProvider{DefaultConfig: slack.Config{WebhookURL: "https://hooks.example.com/file"}}}}
	Load(cfg)
	listing := List()
	if !listing.ManagedUnavailable {
		t.Error("expected the listing to say the managed providers are unavailable")
	}
	if EffectiveFor(cfg).Slack == nil {
		t.Error("expected the providers of the file to keep working")
	}
}
