// Series of the response time chart in the format of the monitor page of Uptime Kuma (fork)
//
// Ports of getChartDatapointsFromHeartbeatList and getChartDatapointsFromStats of src/components/PingChart.vue of
// Uptime Kuma 2.x, fed by the routes /api/v1/endpoints/{key}/response-time-chart and
// /api/v1/status-pages/{slug}/endpoints/{key}/response-time-chart. The abscissas are timestamps in milliseconds.

import { preferenceKey, readPreference, writePreference } from './storage.js'

export const RECENT_PERIOD = 'recent'

export const CHART_PERIODS = Object.freeze([RECENT_PERIOD, '3h', '6h', '24h', '1w'])

// Options of the period selector of the "Response Time Trend" cards
export const CHART_PERIOD_OPTIONS = Object.freeze([
  { value: 'recent', label: 'Recent' },
  { value: '3h', label: '3h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '1w', label: '1w' }
])

// The preference of the period, and its key in the browser (see storage.js, which also migrates the key of Gatus)
export const CHART_PERIOD_PREFERENCE = 'response-time-chart-period'
export const CHART_PERIOD_STORAGE_KEY = preferenceKey(CHART_PERIOD_PREFERENCE)

export const CHART_COLORS = Object.freeze({
  line: '#5CDD8B',
  recentFill: '#5CDD8B38',
  bucketFill: '#5CDD8B06',
  minLine: '#3CBD6B38',
  maxLine: '#7CBD6B38',
  down: 'rgba(220,53,69,0.41)',
  pending: 'rgba(245,182,23,0.41)',
  none: '#00000000'
})

// Fork: colors of the legend samples, the opaque tone of each series (the minimum and the maximum lines are drawn
// translucent, which over the card is almost the same green as the average)
export const CHART_LEGEND_COLORS = Object.freeze({
  line: '#5CDD8B',
  minLine: '#3CBD6B',
  maxLine: '#7CBD6B',
  down: '#DC3545',
  pending: '#F5B617'
})

// Dash patterns of the lines, repeated by the sample of each legend item
export const CHART_LINE_DASHES = Object.freeze({
  average: [],
  minimum: [6, 4],
  maximum: [2, 3]
})

const SECOND_MS = 1000
const MINUTE_MS = 60 * SECOND_MS
const HOUR_MS = 60 * MINUTE_MS

export const isChartPeriod = (value) => CHART_PERIODS.includes(value)

const validInterval = (intervalSeconds) => (Number.isFinite(intervalSeconds) && intervalSeconds > 0 ? intervalSeconds : null)

// recentDatasets returns the series of the Recent period from the results in ascending order: the line has the
// duration of the Up results of at least 1 ms, and each Down or Pending result is a red or yellow column of height 1.
// With an interval, a gap longer than 10 intervals gets null points at previous + interval and current - interval.
export const recentDatasets = (results, intervalSeconds) => {
  const interval = validInterval(intervalSeconds)
  const line = []
  const bars = []
  const barColors = []
  let lastTime = null
  for (const result of results || []) {
    const time = Date.parse(result.timestamp)
    if (lastTime !== null && interval) {
      if (Math.abs(time - lastTime) > interval * SECOND_MS * 10) {
        for (const x of [lastTime + interval * SECOND_MS, time - interval * SECOND_MS]) {
          line.push({ x, y: null })
          bars.push({ x, y: null })
          barColors.push(CHART_COLORS.none)
        }
      }
    }
    line.push({ x: time, y: result.status === 'up' && result.durationMs > 0 ? result.durationMs : null })
    if (result.status === 'down') {
      bars.push({ x: time, y: 1 })
      barColors.push(CHART_COLORS.down)
    } else if (result.status === 'pending') {
      bars.push({ x: time, y: 1 })
      barColors.push(CHART_COLORS.pending)
    } else {
      bars.push({ x: time, y: 0 })
      barColors.push(CHART_COLORS.none)
    }
    lastTime = time
  }
  return { line, bars, barColors }
}

// bucketColor returns the color of the column of an aggregate: red without Up and with Down, yellow with Pending
// without Down or with Up and Down or Pending, and transparent without Down and Pending
export const bucketColor = (bucket) => {
  const failures = bucket.down + bucket.pending
  if (failures === 0) {
    return CHART_COLORS.none
  }
  if (bucket.up === 0 && bucket.down > 0) {
    return CHART_COLORS.down
  }
  return CHART_COLORS.pending
}

// averageBuckets joins the aggregates of a window, like getAverage of Uptime Kuma. The average is weighted by the Up
// count of the aggregates with an average, and the aggregates without minimum or maximum are left out of them.
const averageBuckets = (buckets) => {
  let up = 0
  let down = 0
  let pending = 0
  let totalMs = 0
  let timedUp = 0
  let minMs = null
  let maxMs = null
  for (const bucket of buckets) {
    up += bucket.up
    down += bucket.down
    pending += bucket.pending
    if (bucket.avgMs > 0) {
      totalMs += bucket.avgMs * bucket.up
      timedUp += bucket.up
    }
    if (bucket.minMs !== null && bucket.minMs !== undefined) {
      minMs = minMs === null ? bucket.minMs : Math.min(minMs, bucket.minMs)
    }
    if (bucket.maxMs !== null && bucket.maxMs !== undefined) {
      maxMs = maxMs === null ? bucket.maxMs : Math.max(maxMs, bucket.maxMs)
    }
  }
  return {
    time: buckets[Math.floor(buckets.length / 2)].time,
    up,
    down,
    pending,
    avgMs: timedUp > 0 ? totalMs / timedUp : null,
    minMs,
    maxMs
  }
}

// bucketDatasets returns the series of the periods 3h, 6h, 24h and 1w from the aggregates in ascending order. Like
// Uptime Kuma, it walks from the newest to the oldest aggregate, joins the aggregates with Up in sliding windows of 4
// (up to 6h) or 12 (24h and 1w) aggregates advancing half of the window when there are more than twice the window,
// breaks the line on gaps only with an interval, and reverses the series at the end so that they are ascending.
export const bucketDatasets = (buckets, period, intervalSeconds) => {
  const interval = validInterval(intervalSeconds)
  const list = buckets || []
  const windowSize = period === '24h' || period === '1w' ? 12 : 4
  const gapThresholdMs = interval ? Math.max(period === '1w' ? 10 * HOUR_MS : 10 * MINUTE_MS, interval * SECOND_MS * 10) : null
  const line = []
  const min = []
  const max = []
  const bars = []
  const barColors = []

  const push = (point) => {
    const hasLine = point.up > 0 && point.avgMs > 0
    line.push({ x: point.time, y: hasLine ? point.avgMs : null })
    min.push({ x: point.time, y: hasLine && point.minMs !== undefined ? point.minMs : null })
    max.push({ x: point.time, y: hasLine && point.maxMs !== undefined ? point.maxMs : null })
    bars.push({ x: point.time, y: point.down + point.pending > 0 ? 1 : 0 })
    barColors.push(bucketColor(point))
  }

  let windowBuffer = []
  const flush = () => {
    if (windowBuffer.length > 0) {
      push(averageBuckets(windowBuffer))
      windowBuffer = []
    }
  }

  let lastTime = null
  for (let i = list.length - 1; i >= 0; i--) {
    const bucket = { ...list[i], up: list[i].up || 0, down: list[i].down || 0, pending: list[i].pending || 0 }
    if (bucket.up === 0 && bucket.down === 0 && bucket.pending === 0) {
      continue
    }
    bucket.time = Date.parse(bucket.timestamp)
    if (lastTime !== null && interval && Math.abs(bucket.time - lastTime) > gapThresholdMs) {
      flush()
      for (const x of [lastTime - interval * SECOND_MS, bucket.time + MINUTE_MS]) {
        line.push({ x, y: null })
        min.push({ x, y: null })
        max.push({ x, y: null })
        bars.push({ x, y: null })
        barColors.push(CHART_COLORS.none)
      }
    }
    if (bucket.up > 0 && list.length > windowSize * 2) {
      windowBuffer.push(bucket)
      if (windowBuffer.length === windowSize) {
        push(averageBuckets(windowBuffer))
        windowBuffer = windowBuffer.slice(Math.floor(windowSize / 2))
      }
    } else {
      flush()
      push(bucket)
    }
    lastTime = bucket.time
  }
  flush()

  return {
    line: line.reverse(),
    min: min.reverse(),
    max: max.reverse(),
    bars: bars.reverse(),
    barColors: barColors.reverse()
  }
}

// chartSummary counts the points with a value of the line and the red and yellow columns, for the data-* attributes
// read by the end-to-end tests
export const chartSummary = (series) => {
  const summary = { linePoints: 0, downColumns: 0, pendingColumns: 0 }
  if (!series) {
    return summary
  }
  summary.linePoints = series.line.filter((point) => point.y !== null && point.y !== undefined).length
  series.bars.forEach((bar, index) => {
    if (bar.y !== 1) {
      return
    }
    if (series.barColors[index] === CHART_COLORS.down) {
      summary.downColumns++
    } else if (series.barColors[index] === CHART_COLORS.pending) {
      summary.pendingColumns++
    }
  })
  return summary
}

// chartLegend returns the legend items of the period, in the order the series are drawn. The identifiers are the
// values of data-series read by the end-to-end tests.
export const chartLegend = (period, summary) => {
  const counts = summary || { downColumns: 0, pendingColumns: 0 }
  const items = period === RECENT_PERIOD
    ? [{ id: 'response-time', label: 'Response time', color: CHART_LEGEND_COLORS.line, dash: 'solid' }]
    : [
      { id: 'average', label: 'Average', color: CHART_LEGEND_COLORS.line, dash: 'solid' },
      { id: 'minimum', label: 'Minimum', color: CHART_LEGEND_COLORS.minLine, dash: 'dashed' },
      { id: 'maximum', label: 'Maximum', color: CHART_LEGEND_COLORS.maxLine, dash: 'dotted' }
    ]
  if (counts.downColumns > 0) {
    items.push({ id: 'down', label: 'Down', color: CHART_LEGEND_COLORS.down, column: true })
  }
  if (counts.pendingColumns > 0) {
    items.push({ id: 'pending', label: 'Pending', color: CHART_LEGEND_COLORS.pending, column: true })
  }
  return items
}

const browserStorage = () => (typeof window === 'undefined' ? null : window.localStorage)

// readStoredPeriod returns the period chosen last in the browser, or Recent when it is missing, invalid or unreadable
export const readStoredPeriod = (storage) => {
  try {
    const store = storage === undefined ? browserStorage() : storage
    const value = store ? readPreference(CHART_PERIOD_PREFERENCE, store) : null
    return isChartPeriod(value) ? value : RECENT_PERIOD
  } catch (error) {
    return RECENT_PERIOD
  }
}

// storePeriod remembers the chosen period in the browser, ignoring invalid periods and unavailable storage
export const storePeriod = (period, storage) => {
  if (!isChartPeriod(period)) {
    return
  }
  try {
    const store = storage === undefined ? browserStorage() : storage
    if (store) {
      writePreference(CHART_PERIOD_PREFERENCE, period, store)
    }
  } catch (error) {
    // The choice is only a convenience
  }
}
