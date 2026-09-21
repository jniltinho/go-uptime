package adminbackup

import (
	"errors"
	"sort"
	"time"

	"github.com/TwiN/logr"
	"github.com/jniltinho/go-uptime/v7/internal/lifecycle"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/pushkey"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
)

var (
	// ErrCycleInProgress is returned when a start or configuration reload is in progress
	ErrCycleInProgress = errors.New("a start or configuration reload is in progress, try again later")

	// ErrUnavailable is returned when what was registered through the administration could not be loaded
	ErrUnavailable = errors.New("the managed endpoints, status pages or push keys could not be loaded from the storage")

	// ErrStorageNotSupported is returned when the storage does not support the administration
	ErrStorageNotSupported = errors.New("the storage does not support the administration")
)

// Backup is a backup file ready to be downloaded. It is not encoded in JSON itself: POST /api/v1/admin/backup sends
// Body as the response, with Filename in the Content-Disposition header.
type Backup struct {
	// Body is the content of the file: a File encoded in indented JSON or, when Encrypted, the envelope that seals it.
	Body []byte

	// Filename is the suggested name of the file, gatus-backup-YYYYMMDD-HHMMSS.json with the UTC time of the backup, or
	// the same name ending in .enc.json when Encrypted.
	Filename string

	// Encrypted is whether Body was encrypted with a password.
	Encrypted bool

	// Endpoints, StatusPages and PushKeys are the numbers of items of each type in the backup.
	Endpoints   int
	StatusPages int
	PushKeys    int
}

// isAnyRegistryUnavailable returns whether one of the registries of the administration could not be loaded
func isAnyRegistryUnavailable() bool {
	return managedendpoint.IsManagedUnavailable() || statuspage.IsManagedUnavailable() || pushkey.IsManagedUnavailable()
}

// Build reads what was registered through the administration and returns the backup file, encrypted with password
// when it is not empty. The file is assembled during a runtime change, so that a reload cannot replace the storage in
// the meantime, and encrypted after it, so that the key derivation never delays a reload.
func Build(author, password string) (*Backup, error) {
	if len(password) > 0 {
		if err := CheckPassword(password); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	file, err := readRegistered(author, now)
	if err != nil {
		return nil, err
	}
	body, err := Encode(file)
	if err != nil {
		return nil, err
	}
	if len(body) > MaximumPlaintextBytes {
		return nil, ErrTooLarge
	}
	backup := &Backup{Body: body, Filename: "gatus-backup-" + now.Format("20060102-150405") + ".json", Endpoints: len(file.Endpoints), StatusPages: len(file.StatusPages), PushKeys: len(file.PushKeys)}
	if len(password) > 0 {
		if backup.Body, err = Encrypt(body, password); err != nil {
			return nil, err
		}
		backup.Encrypted = true
		backup.Filename = "gatus-backup-" + now.Format("20060102-150405") + ".enc.json"
	}
	logr.Infof("[adminbackup.Build] Backup with %d endpoints, %d status pages and %d push keys (encrypted=%v) downloaded by %s", backup.Endpoints, backup.StatusPages, backup.PushKeys, backup.Encrypted, auditAuthor(author))
	return backup, nil
}

func readRegistered(author string, now time.Time) (*File, error) {
	end, ok := lifecycle.TryBeginChange()
	if !ok {
		return nil, ErrCycleInProgress
	}
	defer end()
	if isAnyRegistryUnavailable() {
		return nil, ErrUnavailable
	}
	managedEndpointStore, endpointsOK := store.GetManagedEndpointStore()
	managedStatusPageStore, statusPagesOK := store.GetManagedStatusPageStore()
	pushKeyStore, pushKeysOK := store.GetPushKeyStore()
	if !endpointsOK || !statusPagesOK || !pushKeysOK {
		return nil, ErrStorageNotSupported
	}
	file := &File{Format: Format, Version: Version, CreatedAt: now, CreatedBy: author, GatusVersion: gatusVersion(), Endpoints: []Endpoint{}, StatusPages: []StatusPage{}, PushKeys: []PushKey{}}
	storedEndpoints, err := managedEndpointStore.ListManagedEndpoints()
	if err != nil {
		return nil, err
	}
	storedStatusPages, err := managedStatusPageStore.ListManagedStatusPages()
	if err != nil {
		return nil, err
	}
	storedPushKeys, err := pushKeyStore.ListPushKeys()
	if err != nil {
		return nil, err
	}
	if len(storedEndpoints) > MaximumEndpoints || len(storedStatusPages) > MaximumStatusPages || len(storedPushKeys) > MaximumPushKeys {
		return nil, ErrTooLarge
	}
	for _, stored := range storedEndpoints {
		file.Endpoints = append(file.Endpoints, Endpoint{Key: stored.Key, Definition: stored.Definition})
	}
	for _, stored := range storedStatusPages {
		file.StatusPages = append(file.StatusPages, StatusPage{Slug: stored.Slug, Definition: stored.Definition})
	}
	for _, stored := range storedPushKeys {
		file.PushKeys = append(file.PushKeys, PushKey{Name: stored.Name, TokenHash: stored.TokenHash, Hint: stored.Hint, CreatedAt: stored.CreatedAt.UTC(), CreatedBy: stored.CreatedBy})
	}
	sort.Slice(file.Endpoints, func(i, j int) bool { return file.Endpoints[i].Key < file.Endpoints[j].Key })
	sort.Slice(file.StatusPages, func(i, j int) bool { return file.StatusPages[i].Slug < file.StatusPages[j].Slug })
	sort.Slice(file.PushKeys, func(i, j int) bool { return file.PushKeys[i].Name < file.PushKeys[j].Name })
	return file, nil
}

func auditAuthor(author string) string {
	if len(author) == 0 {
		return "unknown"
	}
	return author
}
