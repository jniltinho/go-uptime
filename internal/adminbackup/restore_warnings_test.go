package adminbackup

import (
	"reflect"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
)

func TestDescribeWarnings(t *testing.T) {
	described := describeWarnings([]statuspage.Warning{
		{Type: "group", Value: "missing"},
		{Type: statuspage.WarningTypeTruncated, Value: "400"},
	})
	expected := []string{
		`group "missing" selects nothing`,
		"selects more endpoints than status-pages.maximum-endpoints-per-page (400): only the first 400 are shown",
	}
	if !reflect.DeepEqual(described, expected) {
		t.Errorf("expected %q, got %q", expected, described)
	}
	if described = describeWarnings(nil); described == nil || len(described) != 0 {
		t.Errorf("expected an empty list, never nil, got %#v", described)
	}
}
