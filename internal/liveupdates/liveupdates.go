// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package liveupdates notifies, in real time, the pages that watch an endpoint that a new result was stored for it
// (fork). A notification carries no data: the pages fetch the endpoint again through the existing routes.
package liveupdates

import (
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	// ErrClosed is returned by Subscribe while the live updates are closed, e.g. while the configuration is reloaded
	ErrClosed = errors.New("live updates are closed")

	// PingInterval, MaximumStreamDuration and StreamWriteTimeout are the durations of the event streams. They are
	// variables so that the tests can shorten them.
	PingInterval          = 15 * time.Second
	MaximumStreamDuration = 5 * time.Minute
	StreamWriteTimeout    = 6 * time.Minute
)

var (
	mutex       sync.Mutex
	closed      bool
	sequence    uint64
	sequences   = make(map[string]uint64)
	subscribers = make(map[string]map[*subscription]struct{})
)

type subscription struct {
	notifications chan struct{}
}

// Publish records that a new result was stored for the endpoint with the given key and notifies its subscribers
// without blocking. A subscriber that was already notified and has not read the notification yet is not notified
// twice, because it always fetches the latest state. It must be called after the result is stored, with the lock of the
// results of the key held, so that the sequences follow the order of the results.
func Publish(key string) {
	mutex.Lock()
	defer mutex.Unlock()
	sequence++
	sequences[key] = sequence
	for subscriber := range subscribers[key] {
		select {
		case subscriber.notifications <- struct{}{}:
		default:
		}
	}
}

// Sequence returns the sequence of the last result published for the endpoint with the given key, or 0 without result
// since the process started. The sequences only grow while the process runs, including across reloads.
func Sequence(key string) uint64 {
	mutex.Lock()
	defer mutex.Unlock()
	return sequences[key]
}

// Subscribe returns the channel notified of the new results of the endpoint with the given key, the current sequence of
// the endpoint and the function that cancels the subscription. The channel is closed by Close. It returns ErrClosed
// while the live updates are closed.
func Subscribe(key string) (<-chan struct{}, uint64, func(), error) {
	mutex.Lock()
	defer mutex.Unlock()
	if closed {
		return nil, 0, nil, ErrClosed
	}
	subscriber := &subscription{notifications: make(chan struct{}, 1)}
	if subscribers[key] == nil {
		subscribers[key] = make(map[*subscription]struct{})
	}
	subscribers[key][subscriber] = struct{}{}
	cancel := func() {
		mutex.Lock()
		defer mutex.Unlock()
		// Close already removed the subscription and closed its channel
		if _, exists := subscribers[key][subscriber]; !exists {
			return
		}
		delete(subscribers[key], subscriber)
		if len(subscribers[key]) == 0 {
			delete(subscribers, key)
		}
	}
	return subscriber.notifications, sequences[key], cancel, nil
}

// Close closes every subscription and refuses the new ones until Open. The sequences are kept.
func Close() {
	mutex.Lock()
	defer mutex.Unlock()
	closed = true
	for key, keySubscribers := range subscribers {
		for subscriber := range keySubscribers {
			close(subscriber.notifications)
		}
		delete(subscribers, key)
	}
}

// Open accepts subscriptions again after Close
func Open() {
	mutex.Lock()
	defer mutex.Unlock()
	closed = false
}

// IsClosed returns whether the live updates are closed
func IsClosed() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return closed
}

// Forget forgets the sequence of the endpoint with the given key, when the key stops existing
func Forget(key string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(sequences, key)
}

// ForgetExcept forgets the sequences of every key that is not in keys
func ForgetExcept(keys []string) {
	existing := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		existing[key] = struct{}{}
	}
	mutex.Lock()
	defer mutex.Unlock()
	for key := range sequences {
		if _, exists := existing[key]; !exists {
			delete(sequences, key)
		}
	}
}

// IsEventsPath returns whether the request URI (absolute or not, with or without query) is the path of an event stream:
// /api/v1/endpoints/{key}/events or /api/v1/status-pages/{slug}/endpoints/{key}/events, with non-empty segments. It is
// used before routing, to give these requests a longer write timeout.
func IsEventsPath(requestURI string) bool {
	path := requestURI
	if index := strings.Index(path, "://"); index >= 0 {
		path = path[index+3:]
		if slash := strings.IndexByte(path, '/'); slash >= 0 {
			path = path[slash:]
		} else {
			return false
		}
	}
	if index := strings.IndexAny(path, "?#"); index >= 0 {
		path = path[:index]
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	segments := strings.Split(path[1:], "/")
	for _, segment := range segments {
		if len(segment) == 0 {
			return false
		}
	}
	switch len(segments) {
	case 5:
		return segments[0] == "api" && segments[1] == "v1" && segments[2] == "endpoints" && segments[4] == "events"
	case 7:
		return segments[0] == "api" && segments[1] == "v1" && segments[2] == "status-pages" && segments[4] == "endpoints" && segments[6] == "events"
	default:
		return false
	}
}
