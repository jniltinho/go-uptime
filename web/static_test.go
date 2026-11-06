// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package static

import (
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestEmbed(t *testing.T) {
	scenarios := []struct {
		path                  string
		shouldExist           bool
		expectedContainString string
	}{
		{
			path:                  "index.html",
			shouldExist:           true,
			expectedContainString: "</body>",
		},
		{
			path:                  "favicon.ico",
			shouldExist:           true,
			expectedContainString: "", // not checking because it's an image
		},
		{
			path:        "img/logo.svg",
			shouldExist: false,
		},
		{
			path:                  "css/app.css",
			shouldExist:           true,
			expectedContainString: "background-color",
		},
		{
			path:                  "js/app.js",
			shouldExist:           true,
			expectedContainString: "function",
		},
		{
			path:                  "js/chunk-vendors.js",
			shouldExist:           true,
			expectedContainString: "function",
		},
		{
			// Fork: the Inter served by Go Uptime itself, see AGENTS.md
			path:                  "fonts/inter-4-1-latin.woff2",
			shouldExist:           true,
			expectedContainString: "wOF2",
		},
		{
			path:                  "fonts/inter-4-1-latin-ext.woff2",
			shouldExist:           true,
			expectedContainString: "wOF2",
		},
		{
			// The SIL Open Font License has to be distributed with the font
			path:                  "fonts/OFL.txt",
			shouldExist:           true,
			expectedContainString: "SIL Open Font License",
		},
		{
			path:        "file-that-does-not-exist.html",
			shouldExist: false,
		},
	}
	staticFileSystem, err := fs.Sub(FileSystem, RootPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range scenarios {
		t.Run(scenario.path, func(t *testing.T) {
			content, err := fs.ReadFile(staticFileSystem, scenario.path)
			if !scenario.shouldExist {
				if err == nil {
					t.Errorf("%s should not have existed", scenario.path)
				}
			} else {
				if err != nil {
					t.Errorf("opening %s should not have returned an error, got %s", scenario.path, err.Error())
				}
				if len(content) == 0 {
					t.Errorf("%s should have existed in the static FileSystem, but was empty", scenario.path)
				}
				if !strings.Contains(string(content), scenario.expectedContainString) {
					t.Errorf("%s should have contained %s, but did not", scenario.path, scenario.expectedContainString)
				}
			}
		})
	}
}

// TestIndexKeepsTheGoTemplatePlaceholders makes sure the build of the frontend copies every placeholder of the Go
// template of web/app/index.html into the embedded index.html: Vite parses the HTML, and a placeholder it rewrote or
// dropped (a favicon link resolved as an asset, for example) would only show up as a broken page at runtime.
func TestIndexKeepsTheGoTemplatePlaceholders(t *testing.T) {
	placeholder := regexp.MustCompile(`\{\{[^}]*\}\}`)
	source, err := os.ReadFile("app/index.html")
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := fs.ReadFile(FileSystem, IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := placeholder.FindAllString(string(source), -1)
	got := placeholder.FindAllString(string(embedded), -1)
	slices.Sort(expected)
	slices.Sort(got)
	if len(expected) == 0 || !slices.Equal(expected, got) {
		t.Errorf("expected the placeholders of app/index.html in the embedded index.html:\nexpected %v\ngot      %v", expected, got)
	}
	for _, link := range []string{`href="/css/custom.css"`, `href="/fonts/inter-4-1-latin.woff2"`} {
		if !strings.Contains(string(embedded), link) {
			t.Errorf("expected the embedded index.html to keep the runtime link %s", link)
		}
	}
}
