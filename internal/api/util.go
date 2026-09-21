package api

import (
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"strconv"

	"github.com/labstack/echo/v5"
)

const (
	// DefaultPage is the default page to use if none is specified or an invalid value is provided
	DefaultPage = 1

	// DefaultPageSize is the default page size to use if none is specified or an invalid value is provided
	DefaultPageSize = 50
)

// extractPageAndPageSizeFromRequest reads the query parameters page and pageSize. A missing, invalid or lower than 1
// page is DefaultPage, and a missing, invalid or lower than 1 pageSize is DefaultPageSize. The page size is only capped
// at maximumNumberOfResults on page 1; on the other pages it is used as it was sent.
func extractPageAndPageSizeFromRequest(c *echo.Context, maximumNumberOfResults int) (page, pageSize int) {
	var err error
	if pageParameter := httpx.Query(c, "page"); len(pageParameter) == 0 {
		page = DefaultPage
	} else {
		page, err = strconv.Atoi(pageParameter)
		if err != nil {
			page = DefaultPage
		}
		if page < 1 {
			page = DefaultPage
		}
	}
	if pageSizeParameter := httpx.Query(c, "pageSize"); len(pageSizeParameter) == 0 {
		pageSize = DefaultPageSize
	} else {
		pageSize, err = strconv.Atoi(pageSizeParameter)
		if err != nil {
			pageSize = DefaultPageSize
		}
	}
	if page == 1 && pageSize > maximumNumberOfResults {
		// If the page is 1 and the page size is greater than the maximum number of results, return
		// no more than the maximum number of results
		pageSize = maximumNumberOfResults
	} else if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	return
}
