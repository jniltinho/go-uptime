// Package adminbackup backs up and restores what was registered through the administration (fork): the managed
// endpoints, the managed status pages and the push keys created through the administration.
package adminbackup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"time"
)

const (
	// Format identifies a backup file
	Format = "go-uptime-admin-backup"

	// LegacyFormat identifies a backup file written while the project was called Gatus, up to v6. It is still read, so
	// that a backup made before the change of name can be restored, and never written.
	LegacyFormat = "gatus-admin-backup"

	// Version is the version of the backup file written by this version of Go Uptime, and the highest one it reads
	Version = 1

	// MaximumPlaintextBytes is the maximum size of a backup file without encryption
	MaximumPlaintextBytes = 2 << 20

	// MaximumEndpoints, MaximumStatusPages and MaximumPushKeys are the maximum numbers of items of a backup
	MaximumEndpoints   = 1000
	MaximumStatusPages = 200
	MaximumPushKeys    = 500
)

var (
	// ErrInvalidFile is returned when a backup file cannot be read
	ErrInvalidFile = errors.New("invalid backup file")

	// ErrTooLarge is returned when what was registered through the administration exceeds the limits of a backup
	ErrTooLarge = fmt.Errorf("the backup exceeds %d bytes or the limits of %d endpoints, %d status pages and %d push keys", MaximumPlaintextBytes, MaximumEndpoints, MaximumStatusPages, MaximumPushKeys)
)

// File is a backup file without encryption: the body of the response of POST /api/v1/admin/backup when no password is
// sent, the plaintext sealed in the envelope when one is, and the file property of the body of
// POST /api/v1/admin/restore/preview and POST /api/v1/admin/restore. It is decoded strictly: an unknown field is
// refused, and the encoded file cannot exceed MaximumPlaintextBytes (2 MiB). Nothing in it is encrypted or masked: the
// definitions carry the secrets of the endpoints in clear, which is why a backup should be downloaded with a password.
type File struct {
	// Format identifies a backup file. It is "go-uptime-admin-backup", or "gatus-admin-backup" in a file written up to
	// v6, which is still read; any other value is refused. Once decoded it is always the current one.
	Format string `json:"format"`

	// Version is the version of the format of the file. It is 1, and a restore refuses a value below 1 or above the
	// version written by the running Go Uptime.
	Version int `json:"version"`

	// CreatedAt is when the backup was made, as an RFC 3339 timestamp in UTC. It is only informative.
	CreatedAt time.Time `json:"createdAt"`

	// CreatedBy is the identity of the user who downloaded the backup: the user name with basic authentication or the
	// OIDC subject. It is empty when unknown and only informative.
	CreatedBy string `json:"createdBy"`

	// AppVersion is the version of the module that wrote the file, e.g. "v7.0.0". It is omitted when the binary does not
	// know its version, such as a development build, and only informative.
	AppVersion string `json:"appVersion,omitempty"`

	// LegacyGatusVersion is the name that AppVersion had in the files written up to v6. It is only read: once decoded,
	// its value is in AppVersion and it is empty, so that it is never written.
	LegacyGatusVersion string `json:"gatusVersion,omitempty"`

	// Endpoints are the managed endpoints, ordered by key. The list is required, may be empty but not null, holds at most
	// MaximumEndpoints (1000) items and no two items with the same key.
	Endpoints []Endpoint `json:"endpoints"`

	// StatusPages are the managed status pages, ordered by slug. The list is required, may be empty but not null, holds
	// at most MaximumStatusPages (200) items and no two items with the same slug.
	StatusPages []StatusPage `json:"statusPages"`

	// PushKeys are the push keys created through the administration, ordered by name; those of the configuration file
	// are not part of a backup. The list is required, may be empty but not null, holds at most MaximumPushKeys (500)
	// items and no two items with the same name or the same token hash.
	PushKeys []PushKey `json:"pushKeys"`
}

// Endpoint is a managed endpoint of a backup, with its complete stored definition. It is an element of File.Endpoints.
type Endpoint struct {
	// Key is the key of the endpoint in the group_name form, e.g. "core_my-api". It is required and must be the key
	// computed from the name and the group of Definition, or the restore skips the item.
	Key string `json:"key"`

	// Definition is the definition of the endpoint as stored, a YAML document without default values. Its secrets
	// (headers, passwords, push token, provider overrides of the alerts) are in clear, not masked: a restore skips a
	// definition that carries the "********" mask of the administration API, and a push endpoint without token.
	Definition string `json:"definition"`
}

// StatusPage is a managed status page of a backup, with its complete stored definition. It is an element of
// File.StatusPages.
type StatusPage struct {
	// Slug identifies the status page, as in its public URL. It is required and must be the slug of Definition, or the
	// restore skips the item.
	Slug string `json:"slug"`

	// Definition is the definition of the status page as stored, a YAML document without default values. It includes the
	// hash of the password of a page with its own login, never the password. When that hash is masked, the restore keeps
	// the hash stored for the same slug at the destination and skips the page if there is none.
	Definition string `json:"definition"`
}

// PushKey is a push key created through the administration, without its token. CreatedAt and CreatedBy are only
// informative: a restored push key is created by the author of the restore. It is an element of File.PushKeys. The
// token itself is never stored, so it is neither in a backup nor recoverable from it; the restored key accepts the same
// token as the original.
type PushKey struct {
	// Name is the unique name of the push key, of 1 to 64 characters without leading or trailing spaces. It is required.
	Name string `json:"name"`

	// TokenHash is the SHA-256 hash of the token of the push key, as 64 lowercase hexadecimal characters. It is required.
	TokenHash string `json:"tokenHash"`

	// Hint is the last 4 characters of the token, shown to recognize the key: at most 4 letters, digits, - or _. It is
	// empty when the token is too short to have a hint.
	Hint string `json:"hint"`

	// CreatedAt is when the push key was created at the origin, as an RFC 3339 timestamp in UTC. It is only informative.
	CreatedAt time.Time `json:"createdAt"`

	// CreatedBy is the identity of the user who created the push key at the origin. It is only informative.
	CreatedBy string `json:"createdBy"`
}

func invalidFile(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidFile, fmt.Sprintf(format, args...))
}

// decodeStrict decodes a single JSON value, rejecting unknown fields and trailing data
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// Decode reads a backup file without encryption: unknown fields, missing lists, duplicated items, an unknown format, a
// higher version and a file above the limits are rejected
func Decode(data []byte) (*File, error) {
	if len(data) > MaximumPlaintextBytes {
		return nil, invalidFile("the file exceeds %d bytes", MaximumPlaintextBytes)
	}
	var file File
	if err := decodeStrict(data, &file); err != nil {
		return nil, invalidFile("%s", err.Error())
	}
	if file.Format != Format && file.Format != LegacyFormat {
		return nil, invalidFile("unknown format %q", file.Format)
	}
	// A file of v6 becomes a file of today: whatever is encoded from it has the current format and field
	file.Format = Format
	if len(file.AppVersion) == 0 {
		file.AppVersion = file.LegacyGatusVersion
	}
	file.LegacyGatusVersion = ""
	if file.Version < 1 || file.Version > Version {
		return nil, invalidFile("unsupported version %d", file.Version)
	}
	if file.Endpoints == nil || file.StatusPages == nil || file.PushKeys == nil {
		return nil, invalidFile("the lists endpoints, statusPages and pushKeys are required")
	}
	if len(file.Endpoints) > MaximumEndpoints || len(file.StatusPages) > MaximumStatusPages || len(file.PushKeys) > MaximumPushKeys {
		return nil, invalidFile("the file exceeds the limits of %d endpoints, %d status pages and %d push keys", MaximumEndpoints, MaximumStatusPages, MaximumPushKeys)
	}
	if err := checkDuplicates(&file); err != nil {
		return nil, err
	}
	return &file, nil
}

func checkDuplicates(file *File) error {
	seen := make(map[string]bool)
	check := func(kind, value string) error {
		if len(value) == 0 {
			return invalidFile("a %s without identifier", kind)
		}
		if seen[kind+"\x00"+value] {
			return invalidFile("duplicated %s %q", kind, value)
		}
		seen[kind+"\x00"+value] = true
		return nil
	}
	for _, item := range file.Endpoints {
		if err := check("endpoint", item.Key); err != nil {
			return err
		}
	}
	for _, item := range file.StatusPages {
		if err := check("status page", item.Slug); err != nil {
			return err
		}
	}
	for _, item := range file.PushKeys {
		if err := check("push key", item.Name); err != nil {
			return err
		}
		if err := check("push key hash", item.TokenHash); err != nil {
			return err
		}
	}
	return nil
}

// Encode writes a backup file, indented to be readable
func Encode(file *File) ([]byte, error) {
	return json.MarshalIndent(file, "", "  ")
}

// appVersion returns the version of the module of the binary, if known
func appVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return ""
}
