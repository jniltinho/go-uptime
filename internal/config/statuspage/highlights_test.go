// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPage_FeaturedAndCharts(t *testing.T) {
	page := &Page{Slug: "infra", Title: "Infra", Featured: []string{" Core_API "}, Charts: []string{"CORE_API", "core_web"}}
	if err := page.ValidateAndSetDefaults(); err != nil {
		t.Fatalf("expected a page with only featured endpoints to be valid, got %v", err)
	}
	if strings.Join(page.Featured, ",") != "core_api" || strings.Join(page.Charts, ",") != "core_api,core_web" {
		t.Errorf("expected normalized keys, got featured=%q charts=%q", page.Featured, page.Charts)
	}
	keys := func(count int) []string {
		values := make([]string, count)
		for i := range values {
			values[i] = fmt.Sprintf("core_endpoint-%d", i)
		}
		return values
	}
	scenarios := []struct {
		name        string
		page        Page
		expectedErr error
	}{
		{name: "too-many-featured", page: Page{Slug: "infra", Title: "t", Featured: keys(MaximumFeatured + 1)}, expectedErr: ErrInvalidFeatured},
		{name: "duplicate-featured", page: Page{Slug: "infra", Title: "t", Featured: []string{"core_api", " CORE_API"}}, expectedErr: ErrInvalidFeatured},
		{name: "too-many-charts", page: Page{Slug: "infra", Title: "t", Groups: []string{"core"}, Charts: keys(MaximumCharts + 1)}, expectedErr: ErrInvalidCharts},
		{name: "charts-are-not-a-selection", page: Page{Slug: "infra", Title: "t", Charts: []string{"core_api"}}, expectedErr: ErrEmptySelection},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if err := scenario.page.ValidateAndSetDefaults(); !errors.Is(err, scenario.expectedErr) {
				t.Errorf("expected error %v, got %v", scenario.expectedErr, err)
			}
		})
	}
}
