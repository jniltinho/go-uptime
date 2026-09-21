// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// licenseNoticeMarker is what the one-line notice of every file of the project contains. The Apache License 2.0, section
// 4(b), asks modified files to say that they were changed. Which files came from Gatus cannot be told reliably (git
// loses the files that were moved and heavily rewritten), so the rule covers every file of the project that can carry a
// comment, and NOTICE lists by path the ones that cannot. .github/license-notices.py puts the notice, with these rules.
const licenseNoticeMarker = "derived from Gatus by TwiN"

var (
	licenseNoticeExcludedPrefixes = []string{"third_party/", "openspec/", ".claude/commands/", "web/static/", "web/app/src/assets/fonts/", "web/app/public/fonts/"}
	licenseNoticeExcludedFiles    = map[string]bool{"LICENSE": true, "NOTICE": true, ".claude/skills/LICENSE.cc-skills-golang": true}
	licenseNoticeExcludedSuffixes = []string{".gitkeep", ".crt", ".key", ".pem"}
	licenseNoticeWithoutComment   = []string{".json", ".sum", ".nvmrc", ".lock"}
	// The skills of the project itself; the others are imported and must stay identical to their origin
	licenseNoticeOwnSkills = []string{".claude/skills/create-release/"}
)

func TestLicenseNotices(t *testing.T) {
	root := filepath.Join("..", "..")
	output, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("the list of the files of the repository needs git: %v", err)
	}
	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Split(string(bytes.TrimRight(output, "\x00")), "\x00") {
		if licenseNoticeExcluded(name) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
			continue // removed in the working tree, or binary
		}
		if hasAnySuffix(name, licenseNoticeWithoutComment) {
			if !bytes.Contains(notice, []byte("\n  "+name+"\n")) {
				t.Errorf("%s cannot carry a comment, so it must be listed by its path in NOTICE", name)
			}
			continue
		}
		lines := strings.SplitN(string(content), "\n", 13)
		head := strings.Join(lines[:min(12, len(lines))], "\n")
		if !strings.Contains(head, licenseNoticeMarker) {
			t.Errorf("%s has no license notice in its first lines: run python3 .github/license-notices.py .", name)
		}
	}
}

func licenseNoticeExcluded(name string) bool {
	if licenseNoticeExcludedFiles[name] || hasAnyPrefix(name, licenseNoticeExcludedPrefixes) || hasAnySuffix(name, licenseNoticeExcludedSuffixes) || strings.Contains(name, "/testdata/") {
		return true
	}
	return strings.HasPrefix(name, ".claude/skills/") && !hasAnyPrefix(name, licenseNoticeOwnSkills)
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
