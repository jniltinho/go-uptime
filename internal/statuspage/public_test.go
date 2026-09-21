package statuspage

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// countingReader counts the assemblies. When block is set, the first call signals entered and waits for block to be closed.
type countingReader struct {
	calls   atomic.Int32
	err     error
	delay   time.Duration
	block   chan struct{}
	entered chan struct{}
	once    sync.Once
}

func (reader *countingReader) GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error) {
	reader.calls.Add(1)
	if reader.block != nil {
		first := false
		reader.once.Do(func() { first = true })
		if first {
			close(reader.entered)
			<-reader.block
		}
	}
	time.Sleep(reader.delay)
	if reader.err != nil {
		return nil, reader.err
	}
	return store.Get().(store.EndpointSummaryBatchReader).GetEndpointSummaries(keys, maximumResults, now)
}

func setupPublicPageTest(t *testing.T, reader *countingReader) *config.Config {
	t.Helper()
	if err := store.Initialize(&storage.Config{Type: storage.TypeMemory, MaximumNumberOfResults: 100, MaximumNumberOfEvents: 50}); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, testConfig)
	ep := cfg.Endpoints[0]
	if err := store.Get().InsertEndpointResult(ep, &endpoint.Result{Success: true, Timestamp: time.Now(), Duration: 42 * time.Millisecond, Hostname: "10.0.0.5", Errors: []string{"dial tcp 10.0.0.5:443"}}); err != nil {
		t.Fatal(err)
	}
	Load(cfg)
	previousReader := getSummaryReader
	getSummaryReader = func() (summaryReader, bool) { return reader, true }
	t.Cleanup(func() {
		getSummaryReader = previousReader
		publicCache.Clear()
	})
	return cfg
}

func TestPublicPage(t *testing.T) {
	reader := &countingReader{}
	setupPublicPageTest(t, reader)
	body, err := PublicPage("infra")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Slug != "infra" || len(payload.Groups) != 1 || payload.Groups[0].Endpoints[0].Name != "api" || payload.Groups[0].Endpoints[0].Status != StatusUp || payload.Status != StatusOperational {
		t.Errorf("unexpected payload: %s", body)
	}
	if _, err := PublicPage("infra"); err != nil || reader.calls.Load() != 1 {
		t.Errorf("expected the second request to be served from the cache, got %d assemblies (err=%v)", reader.calls.Load(), err)
	}
	for _, slug := range []string{"missing", "hidden", "Infra", ""} {
		if _, err := PublicPage(slug); !errors.Is(err, ErrPageNotFound) {
			t.Errorf("expected ErrPageNotFound for %q, got %v", slug, err)
		}
	}
	if reader.calls.Load() != 1 {
		t.Errorf("expected pages that are not published not to read the storage, got %d assemblies", reader.calls.Load())
	}
}

func TestPublicPage_ConcurrentRequestsAssembleOnce(t *testing.T) {
	reader := &countingReader{delay: 50 * time.Millisecond}
	setupPublicPageTest(t, reader)
	var waitGroup sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < 100; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if body, err := PublicPage("infra"); err != nil || len(body) == 0 {
				failures.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	if failures.Load() != 0 || reader.calls.Load() != 1 {
		t.Errorf("expected 100 successful requests and 1 assembly, got %d failures and %d assemblies", failures.Load(), reader.calls.Load())
	}
	if len(assemblySemaphore) != 0 {
		t.Errorf("expected every slot to be released, got %d", len(assemblySemaphore))
	}
}

func TestPublicPage_NewRevisionDuringAssembly(t *testing.T) {
	reader := &countingReader{block: make(chan struct{}), entered: make(chan struct{})}
	cfg := setupPublicPageTest(t, reader)
	firstDone := make(chan error, 1)
	go func() {
		_, err := PublicPage("infra")
		firstDone <- err
	}()
	<-reader.entered
	Load(cfg) // new revision while the first assembly is blocked
	if _, err := PublicPage("infra"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader.calls.Load() != 2 {
		t.Errorf("expected the request after the new revision not to join the previous assembly, got %d assemblies", reader.calls.Load())
	}
	close(reader.block)
	if err := <-firstDone; err != nil {
		t.Errorf("unexpected error for the first request: %v", err)
	}
}

func TestPublicPage_UnavailableIsCached(t *testing.T) {
	reader := &countingReader{err: errors.New("database is locked")}
	setupPublicPageTest(t, reader)
	for i := 0; i < 50; i++ {
		if _, err := PublicPage("infra"); !errors.Is(err, ErrPageUnavailable) {
			t.Fatalf("expected ErrPageUnavailable, got %v", err)
		}
	}
	if reader.calls.Load() != 1 {
		t.Errorf("expected a failing storage to be read once, got %d assemblies", reader.calls.Load())
	}
}

func TestPublicPage_SemaphoreTimeout(t *testing.T) {
	reader := &countingReader{}
	setupPublicPageTest(t, reader)
	previousTimeout := semaphoreTimeout
	semaphoreTimeout = 50 * time.Millisecond
	t.Cleanup(func() { semaphoreTimeout = previousTimeout })
	for i := 0; i < maximumConcurrentAssemblies; i++ {
		assemblySemaphore <- struct{}{}
	}
	_, err := PublicPage("infra")
	for i := 0; i < maximumConcurrentAssemblies; i++ {
		<-assemblySemaphore
	}
	if !errors.Is(err, ErrPageUnavailable) || reader.calls.Load() != 0 {
		t.Fatalf("expected ErrPageUnavailable without assembly, got err=%v assemblies=%d", err, reader.calls.Load())
	}
	if _, err := PublicPage("infra"); err != nil || reader.calls.Load() != 1 {
		t.Errorf("expected the timeout not to be cached, got err=%v assemblies=%d", err, reader.calls.Load())
	}
}

func TestPublicPage_ConcurrentLoads(t *testing.T) {
	reader := &countingReader{}
	cfg := setupPublicPageTest(t, reader)
	var waitGroup sync.WaitGroup
	stop := make(chan struct{})
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for {
			select {
			case <-stop:
				return
			default:
				Load(cfg)
			}
		}
	}()
	var readers sync.WaitGroup
	for i := 0; i < 100; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 5; j++ {
				if _, err := PublicPage("infra"); err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
			}
		}()
	}
	readers.Wait()
	close(stop)
	waitGroup.Wait()
}
