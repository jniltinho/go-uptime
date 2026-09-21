package test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// docs/manager-gatus.py is the address that the script had up to v6, and it is downloaded by that address. Whoever
// downloads it gets one file, so it cannot be a shortcut to the new one: it stays, during the 7.x series, as an identical
// copy of docs/manager-go-uptime.py.
func TestManagerScriptCopiesAreIdentical(t *testing.T) {
	root := filepath.Join("..", "..")
	current, err := os.ReadFile(filepath.Join(root, "docs", "manager-go-uptime.py"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(filepath.Join(root, "docs", "manager-gatus.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, legacy) {
		t.Error("docs/manager-gatus.py differs from docs/manager-go-uptime.py: copy the new one over the old one (cp docs/manager-go-uptime.py docs/manager-gatus.py)")
	}
}
