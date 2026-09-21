package adminbackup

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// legacyBackupPassword is the password of testdata/backup-v6.3.0.enc.json
const legacyBackupPassword = "fixture-backup-password-123"

// The files of testdata were downloaded from the published image jniltinho/gatus:v6.3.0, before the project was renamed:
// one managed endpoint, one managed status page and one push key. They must never be regenerated with a newer version.
func readLegacyFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecode_BackupOfV6(t *testing.T) {
	data := readLegacyFixture(t, "backup-v6.3.0.json")
	if !strings.Contains(string(data), `"format": "gatus-admin-backup"`) {
		t.Fatal("expected the fixture to have the format of v6")
	}
	file, err := Decode(data)
	if err != nil {
		t.Fatalf("expected a backup of v6 to be read, got %v", err)
	}
	if file.Format != Format || len(file.Endpoints) != 1 || len(file.StatusPages) != 1 || len(file.PushKeys) != 1 {
		t.Errorf("expected the current format and the three items, got %q with %d, %d and %d", file.Format, len(file.Endpoints), len(file.StatusPages), len(file.PushKeys))
	}
	plaintext, encrypted, err := Unwrap(data, "")
	if err != nil || encrypted || string(plaintext) != string(data) {
		t.Errorf("expected a plain file of v6 to be unwrapped as it is, got encrypted=%v err=%v", encrypted, err)
	}
	if _, _, err = Unwrap(data, legacyBackupPassword); !errors.Is(err, ErrNotEncrypted) {
		t.Errorf("expected a password to be refused for a plain file of v6, got %v", err)
	}
}

// The header of an encrypted file is part of what the seal authenticates, its format included: the legacy format must
// reach the opening as it was read
func TestUnwrap_EncryptedBackupOfV6(t *testing.T) {
	data := readLegacyFixture(t, "backup-v6.3.0.enc.json")
	if !strings.Contains(string(data), `"format": "gatus-admin-backup-encrypted"`) {
		t.Fatal("expected the fixture to have the encrypted format of v6")
	}
	plaintext, encrypted, err := Unwrap(data, legacyBackupPassword)
	if err != nil || !encrypted {
		t.Fatalf("expected an encrypted backup of v6 to be opened, got encrypted=%v err=%v", encrypted, err)
	}
	file, err := Decode(plaintext)
	if err != nil || len(file.Endpoints) != 1 || file.Endpoints[0].Key != "web_site" {
		t.Fatalf("expected the plaintext of v6 to be read, got %+v (err=%v)", file, err)
	}
	if _, _, err = Unwrap(data, "another-password-1234567"); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("expected the wrong password to be refused, got %v", err)
	}
	// Rewriting the format of the header breaks the seal: that is what keeps the legacy format as it was read
	tampered := strings.Replace(string(data), LegacyEncryptedFormat, EncryptedFormat, 1)
	if _, _, err = Unwrap([]byte(tampered), legacyBackupPassword); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("expected a header with another format not to authenticate, got %v", err)
	}
}

func TestDecode_VersionFieldOfV6(t *testing.T) {
	legacy := `{"format":"gatus-admin-backup","version":1,"createdAt":"2026-09-01T00:00:00Z","createdBy":"admin","gatusVersion":"v6.3.0","endpoints":[],"statusPages":[],"pushKeys":[]}`
	file, err := Decode([]byte(legacy))
	if err != nil {
		t.Fatalf("expected gatusVersion to be accepted, got %v", err)
	}
	if file.AppVersion != "v6.3.0" || file.LegacyGatusVersion != "" {
		t.Errorf("expected the version in AppVersion only, got %q and %q", file.AppVersion, file.LegacyGatusVersion)
	}
	encoded, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "gatus") || !strings.Contains(string(encoded), `"appVersion":"v6.3.0"`) || !strings.Contains(string(encoded), `"format":"go-uptime-admin-backup"`) {
		t.Errorf("expected only the current names to be written, got %s", encoded)
	}
	// The strict decoding still refuses what it does not know
	if _, err = Decode([]byte(strings.Replace(legacy, `"gatusVersion"`, `"someVersion"`, 1))); !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected an unknown field to be refused, got %v", err)
	}
	if _, err = Decode([]byte(strings.Replace(legacy, "gatus-admin-backup", "other-admin-backup", 1))); !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected an unknown format to be refused, got %v", err)
	}
}

func TestEncrypt_WritesTheCurrentFormat(t *testing.T) {
	sealed, err := Encrypt([]byte(`{"format":"go-uptime-admin-backup"}`), legacyBackupPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sealed), `"format": "go-uptime-admin-backup-encrypted"`) || strings.Contains(string(sealed), "gatus") {
		t.Errorf("expected the current encrypted format, got %s", sealed)
	}
}
