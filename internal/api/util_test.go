package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jniltinho/go-uptime/v7/internal/storage"

	"github.com/labstack/echo/v5"
)

func TestExtractPageAndPageSizeFromRequest(t *testing.T) {
	type Scenario struct {
		Name                   string
		Page                   string
		PageSize               string
		ExpectedPage           int
		ExpectedPageSize       int
		MaximumNumberOfResults int
	}
	scenarios := []Scenario{
		{
			Page:                   "1",
			PageSize:               "20",
			ExpectedPage:           1,
			ExpectedPageSize:       20,
			MaximumNumberOfResults: 20,
		},
		{
			Page:                   "2",
			PageSize:               "10",
			ExpectedPage:           2,
			ExpectedPageSize:       10,
			MaximumNumberOfResults: 40,
		},
		{
			Page:                   "2",
			PageSize:               "10",
			ExpectedPage:           2,
			ExpectedPageSize:       10,
			MaximumNumberOfResults: 200,
		},
		{
			Page:                   "1",
			PageSize:               "999999",
			ExpectedPage:           1,
			ExpectedPageSize:       storage.DefaultMaximumNumberOfResults,
			MaximumNumberOfResults: 100,
		},
		{
			Page:                   "-1",
			PageSize:               "-1",
			ExpectedPage:           DefaultPage,
			ExpectedPageSize:       DefaultPageSize,
			MaximumNumberOfResults: 20,
		},
		{
			Page:                   "invalid",
			PageSize:               "invalid",
			ExpectedPage:           DefaultPage,
			ExpectedPageSize:       DefaultPageSize,
			MaximumNumberOfResults: 100,
		},
	}
	for _, scenario := range scenarios {
		t.Run("page-"+scenario.Page+"-pageSize-"+scenario.PageSize, func(t *testing.T) {
			request := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/statuses?page=%s&pageSize=%s", scenario.Page, scenario.PageSize), http.NoBody)
			c := echo.New().NewContext(request, httptest.NewRecorder())
			actualPage, actualPageSize := extractPageAndPageSizeFromRequest(c, scenario.MaximumNumberOfResults)
			if actualPage != scenario.ExpectedPage {
				t.Errorf("expected %d, got %d", scenario.ExpectedPage, actualPage)
			}
			if actualPageSize != scenario.ExpectedPageSize {
				t.Errorf("expected %d, got %d", scenario.ExpectedPageSize, actualPageSize)
			}
		})
	}
}
