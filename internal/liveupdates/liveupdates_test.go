// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package liveupdates

import (
	"errors"
	"sync"
	"testing"
)

func TestPublishAndSubscribe(t *testing.T) {
	Open()
	notifications, initial, cancel, err := Subscribe("jobs_publish")
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	Publish("jobs_publish")
	Publish("jobs_publish")
	select {
	case <-notifications:
	default:
		t.Fatal("expected a notification")
	}
	select {
	case <-notifications:
		t.Fatal("expected the notifications to be merged")
	default:
	}
	if current := Sequence("jobs_publish"); current <= initial {
		t.Errorf("expected the sequence to grow from %d, got %d", initial, current)
	}
	Publish("other_key")
	select {
	case <-notifications:
		t.Error("expected no notification for another key")
	default:
	}
}

func TestSequencesOnlyGrow(t *testing.T) {
	Open()
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 50; j++ {
				Publish("jobs_concurrent")
			}
		}()
	}
	previous := Sequence("jobs_concurrent")
	Publish("jobs_ordered")
	first := Sequence("jobs_ordered")
	Publish("jobs_ordered")
	if second := Sequence("jobs_ordered"); second <= first {
		t.Errorf("expected %d > %d", second, first)
	}
	wait.Wait()
	if Sequence("jobs_concurrent") < previous {
		t.Error("expected the sequence not to decrease")
	}
}

func TestCloseAndOpen(t *testing.T) {
	Open()
	notifications, _, cancel, err := Subscribe("jobs_close")
	if err != nil {
		t.Fatal(err)
	}
	Publish("jobs_close")
	before := Sequence("jobs_close")
	Close()
	if !IsClosed() {
		t.Error("expected the live updates to be closed")
	}
	<-notifications
	if _, open := <-notifications; open {
		t.Error("expected the channel to be closed")
	}
	cancel()
	if _, _, _, err := Subscribe("jobs_close"); !errors.Is(err, ErrClosed) {
		t.Errorf("expected ErrClosed, got %v", err)
	}
	Open()
	if Sequence("jobs_close") != before {
		t.Error("expected the sequence to be kept across Close and Open")
	}
	_, _, cancel, err = Subscribe("jobs_close")
	if err != nil {
		t.Fatalf("expected a subscription after Open, got %v", err)
	}
	cancel()
	cancel()
}

func TestForget(t *testing.T) {
	Open()
	Publish("jobs_forget")
	Publish("jobs_kept")
	ForgetExcept([]string{"jobs_kept"})
	if Sequence("jobs_forget") != 0 || Sequence("jobs_kept") == 0 {
		t.Error("expected only the keys not in the list to be forgotten")
	}
	Forget("jobs_kept")
	if Sequence("jobs_kept") != 0 {
		t.Error("expected the key to be forgotten")
	}
}

func TestIsEventsPath(t *testing.T) {
	scenarios := map[string]bool{
		"/api/v1/endpoints/jobs_backup/events":                       true,
		"/api/v1/endpoints/jobs_backup/events?lastEventId=3":         true,
		"http://status.example.com/api/v1/endpoints/core_api/events": true,
		"/api/v1/status-pages/infra/endpoints/core_api/events":       true,
		"/api/v1/endpoints/statuses/events-export":                   false,
		"/api/v1/endpoints//events":                                  false,
		"/api/v1/endpoints/jobs_backup/statuses":                     false,
		"/api/v1/endpoints/a/b/events":                               false,
		"/api/v1/status-pages/infra/endpoints/core_api/events/x":     false,
		"http://status.example.com":                                  false,
		"":                                                           false,
	}
	for uri, expected := range scenarios {
		if IsEventsPath(uri) != expected {
			t.Errorf("IsEventsPath(%q): expected %v", uri, expected)
		}
	}
}
