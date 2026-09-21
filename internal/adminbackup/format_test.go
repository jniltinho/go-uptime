// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package adminbackup

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const validBackup = `{"format":"go-uptime-admin-backup","version":1,"createdAt":"2026-09-16T18:00:00Z","createdBy":"admin",
"endpoints":[{"key":"jobs_backup","definition":"type: push\nname: backup\ngroup: jobs\ntoken: abcdefgh12345678\n"}],
"statusPages":[{"slug":"jobs","definition":"slug: jobs\ntitle: Jobs\n"}],
"pushKeys":[{"name":"akamai","tokenHash":"` + "0000000000000000000000000000000000000000000000000000000000000000" + `","hint":"Ab12","createdAt":"2026-09-16T18:00:00Z","createdBy":"admin"}]}`

func TestDecode(t *testing.T) {
	file, err := Decode([]byte(validBackup))
	if err != nil || len(file.Endpoints) != 1 || len(file.StatusPages) != 1 || len(file.PushKeys) != 1 {
		t.Fatalf("expected a valid backup, got %v %v", file, err)
	}
	scenarios := map[string]string{
		"unknown-format":   strings.Replace(validBackup, `"go-uptime-admin-backup"`, `"other"`, 1),
		"higher-version":   strings.Replace(validBackup, `"version":1`, `"version":2`, 1),
		"unknown-field":    strings.Replace(validBackup, `"createdBy":"admin",`, `"createdBy":"admin","extra":true,`, 1),
		"unknown-item-key": strings.Replace(validBackup, `"slug":"jobs",`, `"slug":"jobs","enabled":true,`, 1),
		"missing-list":     strings.Replace(validBackup, `"statusPages":[{"slug":"jobs","definition":"slug: jobs\ntitle: Jobs\n"}],`, ``, 1),
		"duplicated-item":  strings.Replace(validBackup, `"statusPages":[{"slug":"jobs","definition":"slug: jobs\ntitle: Jobs\n"}]`, `"statusPages":[{"slug":"jobs","definition":"a"},{"slug":"jobs","definition":"b"}]`, 1),
		"trailing-data":    validBackup + `{}`,
		"not-json":         `slug: jobs`,
		"too-large":        validBackup[:len(validBackup)-1] + strings.Repeat(" ", MaximumPlaintextBytes) + "}",
	}
	for name, data := range scenarios {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(data)); !errors.Is(err, ErrInvalidFile) {
				t.Errorf("expected ErrInvalidFile, got %v", err)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	file, _ := Decode([]byte(validBackup))
	encoded, err := Encode(file)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || decoded.Endpoints[0].Definition != file.Endpoints[0].Definition || decoded.PushKeys[0].Hint != "Ab12" {
		t.Errorf("expected the same backup after encoding, got %v %v", decoded, err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte(validBackup))
	f.Add([]byte(`{"format":"go-uptime-admin-backup","version":1,"endpoints":[],"statusPages":[],"pushKeys":[]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := Decode(data)
		if err == nil && (file.Format != Format || file.Endpoints == nil) {
			t.Errorf("decoded an invalid backup: %v", file)
		}
		if err != nil && !errors.Is(err, ErrInvalidFile) {
			t.Errorf("unexpected error type: %v", err)
		}
		_ = bytes.TrimSpace(data)
	})
}
