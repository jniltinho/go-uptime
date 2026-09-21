// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The project was called Gatus up to v6. The old name stays only where AGENTS.md ("Origin, and names kept from Gatus")
// says so: to credit the project it derives from, to keep the installations of v6 working, in the identifiers that
// other systems read, and in the names of third parties. Anywhere else it is a leftover, and this test fails on it.

// allowedNameFiles are the files that may mention the old name anywhere: their subject is the old name itself.
var allowedNameFiles = map[string]string{
	"NOTICE":                                               "the origin",
	"README.md":                                            "the origin and the migration",
	"AGENTS.md":                                            "the list of the names that are kept",
	"docs/migrating-from-gatus.md":                         "the migration guide",
	"docs/manager-gatus.py":                                "the copy of the script at the address it had up to v6",
	"cmd/environment.go":                                   "the variables of v6, read as aliases",
	"cmd/cmd_test.go":                                      "the tests of the aliases",
	"test/e2e/cli.sh":                                      "the aliases on the real binary",
	"test/e2e/upgrade.sh":                                  "the upgrade from the image of v6",
	"Dockerfile":                                           "the /gatus symbolic link",
	"Dockerfile.release":                                   "the /gatus symbolic link",
	"web/app/public/manifest.json":                         "the id of an installed PWA",
	"web/app/src/utils/storage.js":                         "the prefix that the preferences are migrated from",
	"internal/adminbackup/legacy_format_test.go":           "the backups of v6",
	"internal/metrics/metrics_test.go":                     "the names of the metrics of v6, which metrics-namespace: gatus reproduces",
	"internal/metrics/namespace_test.go":                   "the prefix of the metrics of v6",
	"internal/config/metrics_namespace_test.go":            "the prefix of the metrics of v6",
	"internal/test/project_name_test.go":                   "this test",
	"internal/test/script_copies_test.go":                  "the copy of the script",
	".claude/skills/create-release/SKILL.md":               "the image that v7.0.0 upgrades from",
	"web/app/src/utils/storage.test.mjs":                   "the migration of the preferences",
	"web/app/src/utils/adminBackup.test.mjs":               "the backups of v6",
	"internal/adminbackup/testdata/backup-v6.3.0.json":     "a backup of v6",
	"internal/adminbackup/testdata/backup-v6.3.0.enc.json": "a backup of v6",
}

// allowedNameDirectories may mention the old name anywhere
var allowedNameDirectories = []string{
	"openspec/",    // the specs describe the compatibility, and the archived changes are history
	"third_party/", // modules of the original author
	"web/static/",  // generated from web/app
}

// allowedNameLine is a reason for a single line to mention the old name. With prefixes, the reason only holds for the
// files under them: a compatibility that lives in one place must not excuse a leftover somewhere else.
type allowedNameLine struct {
	pattern  *regexp.Regexp
	prefixes []string
}

var allowedNameLines = []allowedNameLine{
	// Credit and history
	{pattern: regexp.MustCompile(`TwiN|twinproduction|twin\.sh|twin/gatus`)},
	{pattern: regexp.MustCompile(`(?i)original Gatus|Gatus original|project, Gatus|Not in Gatus|called Gatus|from Gatus|up to v6|of v6\b|on v6\b|from v6\b|written for the original`)},
	{pattern: regexp.MustCompile(`fork\.\d|v5\.36\.0`), prefixes: []string{"docs/"}},
	// Compatibility with v6, each where it lives
	{pattern: regexp.MustCompile(`GATUS_(CONFIG_PATH|CONFIG_FILE|LOG_LEVEL|DELAY_START_SECONDS|URL|USERNAME|PASSWORD|\*|\{)|--gatus-url|manager-gatus\.py|migrating-from-gatus|/opt/gatus|/gatus\b`), prefixes: []string{"docs/", ".gitignore", "cmd/root.go"}},
	{pattern: regexp.MustCompile(`Legacy|legacy|LEGACY|gatus-admin-backup|gatusVersion`), prefixes: []string{"internal/adminbackup/", "web/app/src/utils/adminBackup.js", "docs/"}},
	{pattern: regexp.MustCompile(`LegacyMetricsNamespace|metrics-namespace|gatus_results_total|"gatus" keeps`), prefixes: []string{"internal/config/config.go", "internal/metrics/", "docs/"}},
	// Identifiers that live in other systems, and the tests and the documentation that pin them
	{pattern: regexp.MustCompile(`alert\(gatus\)|gatus_alert|GatusAlert|gatus-healthcheck|gatus:alert|source:gatus|/events/gatus/|gatus-%s|gatus-hc|domain=gatus`), prefixes: []string{"internal/alerting/", "docs/"}},
	{pattern: regexp.MustCompile(`(?i)(source|sourcetype|entity|alias|topic|service|monitoring.?tool|event.?type|eventid|externalid|external_id|event_id|X-S4-ExternalID|source_type_name)\W{0,12}"?\\?"?` + "`?" + `"?gatus`), prefixes: []string{"internal/alerting/", "docs/"}},
	{pattern: regexp.MustCompile("`gatus`|`gatus-`|`\"gatus\"`|`Gatus`|\\(gatus\\)|default: gatus|Defaults to gatus|defaults to \"Gatus\"|is always \"Gatus\"|\"gatus-|'gatus-|gatus-my-super-app|gatus-_|gatus-group_"), prefixes: []string{"internal/alerting/", "docs/"}},
	{pattern: regexp.MustCompile(`jniltinho/gatus:v6`), prefixes: []string{"test/e2e/", "docs/"}},
	// Names of third parties
	{pattern: regexp.MustCompile(`gatus-cli|gatus-sdk|gatus\.io|n8n-nodes-gatus|terraform-kubernetes-gatus|charts/gatus|notify/gatus/`)},
}

var oldProjectName = regexp.MustCompile(`(?i)gatus`)

func TestProjectName(t *testing.T) {
	root := filepath.Join("..", "..")
	output, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("the list of the files of the repository needs git: %v", err)
	}
	var leftovers []string
	for _, name := range strings.Split(string(bytes.TrimRight(output, "\x00")), "\x00") {
		if _, allowed := allowedNameFiles[name]; allowed || hasAnyPrefix(name, allowedNameDirectories) {
			continue
		}
		if oldProjectName.MatchString(name) {
			leftovers = append(leftovers, name+": the name of the file")
		}
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || isBinaryFile(name, content) {
			continue // removed in the working tree, or binary
		}
		scanner := bufio.NewScanner(bytes.NewReader(content))
		scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
		for number := 1; scanner.Scan(); number++ {
			line := scanner.Text()
			if oldProjectName.MatchString(line) && !matchesAny(name, line, allowedNameLines) {
				leftovers = append(leftovers, name+":"+itoa(number)+": "+strings.TrimSpace(truncate(line, 140)))
			}
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("the old name of the project appears outside the names kept on purpose (AGENTS.md, \"Origin, and names kept from Gatus\"). Write Go Uptime or go-uptime, or, if the name must stay, add the reason to this test:\n  %s", strings.Join(leftovers, "\n  "))
	}
}

// binaryFileSuffixes are the files that are never read as text
var binaryFileSuffixes = []string{".png", ".jpg", ".jpeg", ".gif", ".ico", ".woff", ".woff2", ".ttf", ".pyc", ".gz", ".db", ".pdf"}

// isBinaryFile tells a binary from a text file by its name first, and only then by its first bytes: a source file may
// have a NUL byte inside a regular expression (web/app/src/utils/adminBackup.js and redirect.js do), and skipping it as a
// binary would hide whatever else it says
func isBinaryFile(name string, content []byte) bool {
	for _, suffix := range binaryFileSuffixes {
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return true
		}
	}
	for _, suffix := range []string{".go", ".js", ".mjs", ".cjs", ".vue", ".css", ".html", ".md", ".yaml", ".yml", ".sh", ".py", ".json", ".svg", ".service", ".conf", ".mod", ".sum"} {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	return bytes.IndexByte(content[:min(len(content), 512)], 0) >= 0
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func matchesAny(name, line string, allowed []allowedNameLine) bool {
	for _, reason := range allowed {
		if (len(reason.prefixes) == 0 || hasAnyPrefix(name, reason.prefixes)) && reason.pattern.MatchString(line) {
			return true
		}
	}
	return false
}

func truncate(text string, length int) string {
	if len(text) <= length {
		return text
	}
	return text[:length] + "…"
}

func itoa(number int) string {
	digits := ""
	for ; number > 0; number /= 10 {
		digits = string(rune('0'+number%10)) + digits
	}
	return digits
}
