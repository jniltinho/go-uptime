package api

import (
	"bytes"
	"errors"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

// timeFormat is the format of the timestamps of the X axis of the chart for the durations 24h and 7d.
const timeFormat = "3:04PM"

// Styles of the response time chart: grey grid and axis on a transparent background, so that it fits any theme.
var (
	gridStyle = chart.Style{
		StrokeColor: drawing.Color{R: 119, G: 119, B: 119, A: 40},
		StrokeWidth: 1.0,
	}
	axisStyle = chart.Style{
		FontColor: drawing.Color{R: 119, G: 119, B: 119, A: 255},
	}
	transparentStyle = chart.Style{
		FillColor: drawing.Color{R: 255, G: 255, B: 255, A: 0},
	}
)

// ResponseTimeChart handles GET and HEAD /api/v1/endpoints/:key/response-times/:duration/chart.svg: the hourly average
// response time of an endpoint over the duration, rendered as an SVG line chart of 1280x300.
//
// Authentication: none (public group).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; the path parameter duration is one of 24h, 7d or 30d (1h is not supported here).
// Responses: 200 with the chart as image/svg+xml, Cache-Control: no-cache, no-store and Expires: 0; 204 without body
// when the endpoint has no response time in the period; 400 when the duration is not supported, the key cannot be
// unescaped or the time range is invalid; 404 when no endpoint has the key; 500 on an error of the storage or of the
// rendering. Errors are text/plain, and the 500 carries the text of the error.
func ResponseTimeChart(c *echo.Context) error {
	duration := c.Param("duration")
	chartTimestampFormatter := chart.TimeValueFormatterWithFormat(timeFormat)
	var from time.Time
	switch duration {
	case "30d":
		from = time.Now().Truncate(time.Hour).Add(-30 * 24 * time.Hour)
		chartTimestampFormatter = chart.TimeDateValueFormatter
	case "7d":
		from = time.Now().Truncate(time.Hour).Add(-7 * 24 * time.Hour)
	case "24h":
		from = time.Now().Truncate(time.Hour).Add(-24 * time.Hour)
	default:
		return httpx.SendString(c, 400, "Durations supported: 30d, 7d, 24h")
	}
	key, err := url.QueryUnescape(c.Param("key"))
	if err != nil {
		return httpx.SendString(c, 400, "invalid key encoding")
	}
	hourlyAverageResponseTime, err := store.Get().GetHourlyAverageResponseTimeByKey(key, from, time.Now())
	if err != nil {
		if errors.Is(err, common.ErrEndpointNotFound) {
			return httpx.SendString(c, 404, err.Error())
		} else if errors.Is(err, common.ErrInvalidTimeRange) {
			return httpx.SendString(c, 400, err.Error())
		}
		return httpx.SendString(c, 500, err.Error())
	}
	if len(hourlyAverageResponseTime) == 0 {
		return httpx.SendString(c, 204, "")
	}
	series := chart.TimeSeries{
		Name: "Average response time per hour",
		Style: chart.Style{
			StrokeWidth: 1.5,
			DotWidth:    2.0,
		},
	}
	keys := make([]int, 0, len(hourlyAverageResponseTime))
	earliestTimestamp := int64(0)
	for hourlyTimestamp := range hourlyAverageResponseTime {
		keys = append(keys, int(hourlyTimestamp))
		if earliestTimestamp == 0 || hourlyTimestamp < earliestTimestamp {
			earliestTimestamp = hourlyTimestamp
		}
	}
	for earliestTimestamp > from.Unix() {
		earliestTimestamp -= int64(time.Hour.Seconds())
		keys = append(keys, int(earliestTimestamp))
	}
	sort.Ints(keys)
	var maxAverageResponseTime float64
	for _, key := range keys {
		averageResponseTime := float64(hourlyAverageResponseTime[int64(key)])
		if maxAverageResponseTime < averageResponseTime {
			maxAverageResponseTime = averageResponseTime
		}
		series.XValues = append(series.XValues, time.Unix(int64(key), 0))
		series.YValues = append(series.YValues, averageResponseTime)
	}
	graph := chart.Chart{
		Canvas:     transparentStyle,
		Background: transparentStyle,
		Width:      1280,
		Height:     300,
		XAxis: chart.XAxis{
			ValueFormatter: chartTimestampFormatter,
			GridMajorStyle: gridStyle,
			GridMinorStyle: gridStyle,
			Style:          axisStyle,
			NameStyle:      axisStyle,
		},
		YAxis: chart.YAxis{
			Name:           "Average response time",
			GridMajorStyle: gridStyle,
			GridMinorStyle: gridStyle,
			Style:          axisStyle,
			NameStyle:      axisStyle,
			Range: &chart.ContinuousRange{
				Min: 0,
				Max: math.Ceil(maxAverageResponseTime * 1.25),
			},
		},
		Series: []chart.Series{series},
	}
	// Rendered into a buffer: once the first byte is written the answer cannot become an error anymore
	var rendered bytes.Buffer
	if err := graph.Render(chart.SVG, &rendered); err != nil {
		logr.Errorf("[api.ResponseTimeChart] Failed to render response time chart: %s", err.Error())
		return httpx.SendString(c, 500, err.Error())
	}
	httpx.SetHeader(c, "Content-Type", "image/svg+xml")
	httpx.SetHeader(c, "Cache-Control", "no-cache, no-store")
	httpx.SetHeader(c, "Expires", "0")
	return httpx.Send(c, http.StatusOK, rendered.Bytes())
}

// ResponseTimeHistory handles GET and HEAD /api/v1/endpoints/:key/response-times/:duration/history: the hourly average
// response time of an endpoint over the duration, as JSON series.
//
// Authentication: none (public group).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; the path parameter duration is one of 24h, 7d or 30d.
// Responses: 200 with {"timestamps": [...], "values": [...]}, two arrays of the same length: the start of each hour in
// Unix milliseconds, in ascending order, and the average response time of that hour in milliseconds. The hours between
// the start of the period and the first hour with data are filled with 0, an hour without data after it is absent, and
// both arrays are empty when there is no data at all. 400 when the duration is not supported, the key cannot be
// unescaped or the time range is invalid; 404 when no endpoint has the key; 500 on an error of the storage. Errors are
// text/plain, and the 500 carries the text of the error.
func ResponseTimeHistory(c *echo.Context) error {
	duration := c.Param("duration")
	var from time.Time
	switch duration {
	case "30d":
		from = time.Now().Truncate(time.Hour).Add(-30 * 24 * time.Hour)
	case "7d":
		from = time.Now().Truncate(time.Hour).Add(-7 * 24 * time.Hour)
	case "24h":
		from = time.Now().Truncate(time.Hour).Add(-24 * time.Hour)
	default:
		return httpx.SendString(c, 400, "Durations supported: 30d, 7d, 24h")
	}
	endpointKey, err := url.QueryUnescape(c.Param("key"))
	if err != nil {
		return httpx.SendString(c, 400, "invalid key encoding")
	}
	hourlyAverageResponseTime, err := store.Get().GetHourlyAverageResponseTimeByKey(endpointKey, from, time.Now())
	if err != nil {
		if errors.Is(err, common.ErrEndpointNotFound) {
			return httpx.SendString(c, 404, err.Error())
		}
		if errors.Is(err, common.ErrInvalidTimeRange) {
			return httpx.SendString(c, 400, err.Error())
		}
		return httpx.SendString(c, 500, err.Error())
	}
	if len(hourlyAverageResponseTime) == 0 {
		return httpx.JSON(c, 200, map[string]interface{}{
			"timestamps": []int64{},
			"values":     []int{},
		})
	}
	hourlyTimestamps := make([]int, 0, len(hourlyAverageResponseTime))
	earliestTimestamp := int64(0)
	for hourlyTimestamp := range hourlyAverageResponseTime {
		hourlyTimestamps = append(hourlyTimestamps, int(hourlyTimestamp))
		if earliestTimestamp == 0 || hourlyTimestamp < earliestTimestamp {
			earliestTimestamp = hourlyTimestamp
		}
	}
	for earliestTimestamp > from.Unix() {
		earliestTimestamp -= int64(time.Hour.Seconds())
		hourlyTimestamps = append(hourlyTimestamps, int(earliestTimestamp))
	}
	sort.Ints(hourlyTimestamps)
	timestamps := make([]int64, 0, len(hourlyTimestamps))
	values := make([]int, 0, len(hourlyTimestamps))
	for _, hourlyTimestamp := range hourlyTimestamps {
		timestamp := int64(hourlyTimestamp)
		averageResponseTime := hourlyAverageResponseTime[timestamp]
		timestamps = append(timestamps, timestamp*1000)
		values = append(values, averageResponseTime)
	}
	return httpx.JSON(c, http.StatusOK, map[string]interface{}{
		"timestamps": timestamps,
		"values":     values,
	})
}
