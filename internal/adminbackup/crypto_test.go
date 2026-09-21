package adminbackup

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testPassword = "correct horse battery"

func TestEncryptDecrypt(t *testing.T) {
	sealed, err := Encrypt([]byte(validBackup), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "abcdefgh12345678") || strings.Contains(string(sealed), "jobs_backup") {
		t.Fatal("the encrypted backup must not contain the definitions in plain text")
	}
	plaintext, err := Decrypt(sealed, testPassword)
	if err != nil || string(plaintext) != validBackup {
		t.Fatalf("expected the original backup, got %v", err)
	}
	if _, err := Decrypt(sealed, "wrong password!"); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword with a wrong password, got %v", err)
	}
	if _, err := Decrypt(sealed, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Errorf("expected ErrPasswordRequired without password, got %v", err)
	}
	if _, err := Encrypt([]byte(validBackup), "short"); !errors.Is(err, ErrPasswordLength) {
		t.Errorf("expected ErrPasswordLength, got %v", err)
	}
	var fields map[string]any
	_ = json.Unmarshal(sealed, &fields)
	// A changed salt or data is authenticated: the same error as a wrong password
	for _, change := range []func(map[string]any){
		func(m map[string]any) {
			m["kdf"].(map[string]any)["salt"] = base64.StdEncoding.EncodeToString(make([]byte, 16))
		},
		func(m map[string]any) {
			data, _ := base64.StdEncoding.DecodeString(m["data"].(string))
			data[0] ^= 1
			m["data"] = base64.StdEncoding.EncodeToString(data)
		},
	} {
		changed := cloneFields(fields)
		change(changed)
		encoded, _ := json.Marshal(changed)
		if _, err := Decrypt(encoded, testPassword); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("expected ErrInvalidPassword for a changed envelope, got %v", err)
		}
	}
	// Forged parameters and malformed envelopes are rejected without deriving a key
	for name, change := range map[string]func(map[string]any){
		"memory":      func(m map[string]any) { m["kdf"].(map[string]any)["memoryKiB"] = 262144 },
		"threads":     func(m map[string]any) { m["kdf"].(map[string]any)["threads"] = 4 },
		"kdf-name":    func(m map[string]any) { m["kdf"].(map[string]any)["name"] = "scrypt" },
		"cipher-name": func(m map[string]any) { m["cipher"].(map[string]any)["name"] = "aes-128-gcm" },
		"short-nonce": func(m map[string]any) {
			m["cipher"].(map[string]any)["nonce"] = base64.StdEncoding.EncodeToString(make([]byte, 8))
		},
		"unknown-field": func(m map[string]any) { m["extra"] = true },
	} {
		changed := cloneFields(fields)
		change(changed)
		encoded, _ := json.Marshal(changed)
		if _, err := Decrypt(encoded, testPassword); !errors.Is(err, ErrInvalidFile) {
			t.Errorf("%s: expected ErrInvalidFile, got %v", name, err)
		}
	}
}

func cloneFields(fields map[string]any) map[string]any {
	encoded, _ := json.Marshal(fields)
	var cloned map[string]any
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

// The additional authenticated data is the JSON encoding of the header, with the fields in this order
func TestEnvelopeHeaderEncoding(t *testing.T) {
	header := envelopeHeader{Format: EncryptedFormat, Version: 1, KDF: envelopeKDF{Name: "argon2id", Time: 2, MemoryKiB: 19456, Threads: 1, Salt: "AAAAAAAAAAAAAAAAAAAAAA=="}, Cipher: envelopeCipher{Name: "aes-256-gcm", Nonce: "AAAAAAAAAAAAAAAA"}}
	encoded, _ := json.Marshal(header)
	expected := `{"format":"go-uptime-admin-backup-encrypted","version":1,"kdf":{"name":"argon2id","time":2,"memoryKiB":19456,"threads":1,"salt":"AAAAAAAAAAAAAAAAAAAAAA=="},"cipher":{"name":"aes-256-gcm","nonce":"AAAAAAAAAAAAAAAA"}}`
	if string(encoded) != expected {
		t.Errorf("unexpected header encoding:\n%s\n%s", encoded, expected)
	}
}

func TestUnwrap(t *testing.T) {
	if plaintext, encrypted, err := Unwrap([]byte(validBackup), ""); err != nil || encrypted || string(plaintext) != validBackup {
		t.Errorf("expected the plain backup, got %v %v", encrypted, err)
	}
	if _, _, err := Unwrap([]byte(validBackup), testPassword); !errors.Is(err, ErrNotEncrypted) {
		t.Errorf("expected ErrNotEncrypted, got %v", err)
	}
	sealed, _ := Encrypt([]byte(validBackup), testPassword)
	if plaintext, encrypted, err := Unwrap(sealed, testPassword); err != nil || !encrypted || string(plaintext) != validBackup {
		t.Errorf("expected the decrypted backup, got %v %v", encrypted, err)
	}
	if _, _, err := Unwrap([]byte(`{"format":"other"}`), ""); !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected ErrInvalidFile, got %v", err)
	}
}

func FuzzDecrypt(f *testing.F) {
	sealed, _ := Encrypt([]byte(validBackup), testPassword)
	f.Add(sealed)
	f.Add([]byte(`{"format":"go-uptime-admin-backup-encrypted"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if _, err := Decrypt(data, testPassword); err != nil && !errors.Is(err, ErrInvalidFile) && !errors.Is(err, ErrInvalidPassword) && !errors.Is(err, ErrBusy) {
			t.Errorf("unexpected error type: %v", err)
		}
	})
}
