// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  CHART_COLORS,
  CHART_PERIOD_STORAGE_KEY,
  CHART_LEGEND_COLORS,
  bucketColor,
  bucketDatasets,
  chartLegend,
  chartSummary,
  readStoredPeriod,
  recentDatasets,
  storePeriod
} from './responseTimeChart.js'

const SECOND = 1000
const MINUTE = 60 * SECOND
const HOUR = 60 * MINUTE
const BASE = Date.parse('2026-09-16T10:00:00Z')

const iso = (ms) => new Date(ms).toISOString()
const result = (offsetMs, status, durationMs = 0) => ({ timestamp: iso(BASE + offsetMs), status, durationMs })
const bucket = (offsetMs, fields) => ({ timestamp: iso(BASE + offsetMs), up: 0, down: 0, pending: 0, avgMs: null, minMs: null, maxMs: null, ...fields })
const ys = (points) => points.map((point) => point.y)
const xs = (points) => points.map((point) => point.x)

class MemoryStorage {
  constructor(values = {}) {
    this.values = { ...values }
  }

  getItem(key) {
    return key in this.values ? this.values[key] : null
  }

  setItem(key, value) {
    this.values[key] = String(value)
  }

  removeItem(key) {
    delete this.values[key]
  }
}

// Recent

test('recent: the line only has the duration of the Up results of at least 1 ms', () => {
  const series = recentDatasets([
    result(0, 'up', 40),
    result(MINUTE, 'up', 0),
    result(2 * MINUTE, 'down', 25),
    result(3 * MINUTE, 'pending', 30)
  ], 60)
  assert.deepEqual(ys(series.line), [40, null, null, null])
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 2 * MINUTE, BASE + 3 * MINUTE])
})

test('recent: Down is a red column, Pending a yellow column and Up has no column', () => {
  const series = recentDatasets([result(0, 'up', 40), result(MINUTE, 'down'), result(2 * MINUTE, 'pending')], 60)
  assert.deepEqual(ys(series.bars), [0, 1, 1])
  assert.deepEqual(series.barColors, [CHART_COLORS.none, CHART_COLORS.down, CHART_COLORS.pending])
  assert.deepEqual(chartSummary(series), { linePoints: 1, downColumns: 1, pendingColumns: 1 })
})

test('recent: a gap longer than 10 intervals gets null points at previous + interval and current - interval', () => {
  const series = recentDatasets([result(0, 'up', 10), result(11 * MINUTE, 'up', 20)], 60)
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 10 * MINUTE, BASE + 11 * MINUTE])
  assert.deepEqual(ys(series.line), [10, null, null, 20])
  assert.deepEqual(ys(series.bars), [0, null, null, 0])
  assert.equal(series.barColors.length, 4)
})

test('recent: a gap of exactly 10 intervals does not break the line', () => {
  const series = recentDatasets([result(0, 'up', 10), result(10 * MINUTE, 'up', 20)], 60)
  assert.equal(series.line.length, 2)
})

test('recent: without an interval, gaps do not break the line', () => {
  for (const interval of [null, undefined, 0]) {
    const series = recentDatasets([result(0, 'up', 10), result(5 * HOUR, 'up', 20)], interval)
    assert.deepEqual(ys(series.line), [10, 20])
  }
})

test('recent: without results the series are empty', () => {
  assert.deepEqual(recentDatasets([], 60), { line: [], bars: [], barColors: [] })
  assert.deepEqual(recentDatasets(undefined, null), { line: [], bars: [], barColors: [] })
})

// Aggregates

test('aggregates: colors of the columns', () => {
  assert.equal(bucketColor({ up: 0, down: 2, pending: 0 }), CHART_COLORS.down)
  assert.equal(bucketColor({ up: 0, down: 2, pending: 1 }), CHART_COLORS.down)
  assert.equal(bucketColor({ up: 0, down: 0, pending: 1 }), CHART_COLORS.pending)
  assert.equal(bucketColor({ up: 1, down: 1, pending: 0 }), CHART_COLORS.pending)
  assert.equal(bucketColor({ up: 1, down: 0, pending: 1 }), CHART_COLORS.pending)
  assert.equal(bucketColor({ up: 3, down: 0, pending: 0 }), CHART_COLORS.none)
})

test('aggregates: a column has the value 1 with Down or Pending, whatever the count', () => {
  const series = bucketDatasets([
    bucket(0, { up: 2, avgMs: 20, minMs: 10, maxMs: 30 }),
    bucket(MINUTE, { down: 7 }),
    bucket(2 * MINUTE, { pending: 3 }),
    bucket(3 * MINUTE, { up: 1, down: 5, avgMs: 15, minMs: 15, maxMs: 15 })
  ], '3h', 60)
  assert.deepEqual(ys(series.bars), [0, 1, 1, 1])
  assert.deepEqual(series.barColors, [CHART_COLORS.none, CHART_COLORS.down, CHART_COLORS.pending, CHART_COLORS.pending])
  assert.deepEqual(chartSummary(series), { linePoints: 2, downColumns: 1, pendingColumns: 2 })
})

test('aggregates: the lines only have values with Up and an average above 0', () => {
  const series = bucketDatasets([
    bucket(0, { up: 2, avgMs: 20, minMs: 10, maxMs: 30 }),
    bucket(MINUTE, { up: 3, avgMs: null }),
    bucket(2 * MINUTE, { down: 1 }),
    bucket(3 * MINUTE, { up: 1, avgMs: 0, minMs: 0, maxMs: 0 })
  ], '6h', 60)
  assert.deepEqual(ys(series.line), [20, null, null, null])
  assert.deepEqual(ys(series.min), [10, null, null, null])
  assert.deepEqual(ys(series.max), [30, null, null, null])
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 2 * MINUTE, BASE + 3 * MINUTE])
})

test('aggregates: empty aggregates are ignored', () => {
  const series = bucketDatasets([bucket(0, { up: 1, avgMs: 5, minMs: 5, maxMs: 5 }), bucket(MINUTE, {})], '3h', null)
  assert.equal(series.line.length, 1)
})

test('aggregates: windows of 4 up to 6h start from the newest aggregate and advance half of the window', () => {
  // 9 aggregates (more than twice the window), with the averages 1..9 from the oldest to the newest
  const buckets = Array.from({ length: 9 }, (_, i) => bucket(i * MINUTE, { up: 1, avgMs: i + 1, minMs: i + 1, maxMs: i + 1 }))
  const series = bucketDatasets(buckets, '3h', 60)
  // From the newest (indexes): [8,7,6,5], [6,5,4,3], [4,3,2,1] and the rest [2,1,0]
  assert.deepEqual(ys(series.line), [2, 3.5, 5.5, 7.5])
  assert.deepEqual(ys(series.min), [1, 2, 4, 6])
  assert.deepEqual(ys(series.max), [3, 5, 7, 9])
  // Middle of each window in the order of the walk (index 2 of 4, index 1 of 3)
  assert.deepEqual(xs(series.line), [BASE + MINUTE, BASE + 2 * MINUTE, BASE + 4 * MINUTE, BASE + 6 * MINUTE])
})

test('aggregates: windows of 12 in 24h and 1w', () => {
  for (const [period, step] of [['24h', MINUTE], ['1w', HOUR]]) {
    const buckets = Array.from({ length: 25 }, (_, i) => bucket(i * step, { up: 1, avgMs: 10, minMs: 10, maxMs: 10 }))
    const series = bucketDatasets(buckets, period, 60)
    // [24..13], [18..7], [12..1], [6..0]
    assert.equal(series.line.length, 4, period)
    assert.deepEqual(xs(series.line), [BASE + 3 * step, BASE + 6 * step, BASE + 12 * step, BASE + 18 * step], period)
  }
})

test('aggregates: no windows with twice the window or fewer aggregates', () => {
  const buckets = Array.from({ length: 8 }, (_, i) => bucket(i * MINUTE, { up: 1, avgMs: i + 1, minMs: i + 1, maxMs: i + 1 }))
  assert.deepEqual(ys(bucketDatasets(buckets, '6h', 60).line), [1, 2, 3, 4, 5, 6, 7, 8])
})

test('aggregates: only aggregates with Up are joined, the others close the window', () => {
  const buckets = Array.from({ length: 9 }, (_, i) => bucket(i * MINUTE, { up: 1, avgMs: 10, minMs: 10, maxMs: 10 }))
  buckets[6] = bucket(6 * MINUTE, { down: 2 })
  const series = bucketDatasets(buckets, '3h', 60)
  // From the newest (indexes): [8,7] closed by the Down at 6, the Down, [5,4,3,2], [3,2,1,0] and the rest [1,0]
  assert.deepEqual(xs(series.bars), [BASE, BASE + MINUTE, BASE + 3 * MINUTE, BASE + 6 * MINUTE, BASE + 7 * MINUTE])
  assert.deepEqual(series.barColors, [CHART_COLORS.none, CHART_COLORS.none, CHART_COLORS.none, CHART_COLORS.down, CHART_COLORS.none])
})

test('aggregates: joined windows add Down and Pending to the color', () => {
  const buckets = Array.from({ length: 9 }, (_, i) => bucket(i * MINUTE, { up: 1, avgMs: 10, minMs: 10, maxMs: 10 }))
  buckets[8] = bucket(8 * MINUTE, { up: 1, pending: 1, avgMs: 10, minMs: 10, maxMs: 10 })
  const series = bucketDatasets(buckets, '3h', 60)
  assert.equal(series.barColors[series.barColors.length - 1], CHART_COLORS.pending)
  assert.equal(series.bars[series.bars.length - 1].y, 1)
})

test('aggregates: in 24h, a gap longer than max(10 min, 10 intervals) gets the null points of Uptime Kuma', () => {
  const series = bucketDatasets([
    bucket(0, { up: 1, avgMs: 10, minMs: 10, maxMs: 10 }),
    bucket(30 * MINUTE, { up: 1, avgMs: 20, minMs: 20, maxMs: 20 })
  ], '24h', 60)
  // Walking from the newest: previous (30 min) - interval, then current (0) + 60 s
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 29 * MINUTE, BASE + 30 * MINUTE])
  assert.deepEqual(ys(series.line), [10, null, null, 20])
  assert.deepEqual(ys(series.min), [10, null, null, 20])
  assert.deepEqual(ys(series.bars), [0, null, null, 0])
})

test('aggregates: in 24h, the threshold uses 10 intervals when longer than 10 minutes', () => {
  const buckets = [bucket(0, { up: 1, avgMs: 10 }), bucket(30 * MINUTE, { up: 1, avgMs: 20 })]
  assert.equal(bucketDatasets(buckets, '24h', 300).line.length, 2)
  const series = bucketDatasets([bucket(0, { up: 1, avgMs: 10 }), bucket(51 * MINUTE, { up: 1, avgMs: 20 })], '24h', 300)
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 46 * MINUTE, BASE + 51 * MINUTE])
})

test('aggregates: in 1w, a gap longer than max(10 h, 10 intervals) gets the null points, with 60 s after the current', () => {
  const buckets = [bucket(0, { up: 1, avgMs: 10 }), bucket(9 * HOUR, { up: 1, avgMs: 20 })]
  assert.equal(bucketDatasets(buckets, '1w', 60).line.length, 2)
  const series = bucketDatasets([bucket(0, { up: 1, avgMs: 10 }), bucket(12 * HOUR, { up: 1, avgMs: 20 })], '1w', 60)
  assert.deepEqual(xs(series.line), [BASE, BASE + MINUTE, BASE + 12 * HOUR - MINUTE, BASE + 12 * HOUR])
  assert.deepEqual(ys(series.line), [10, null, null, 20])
})

test('aggregates: without an interval, gaps do not break the line', () => {
  const series = bucketDatasets([bucket(0, { up: 1, avgMs: 10 }), bucket(5 * HOUR, { up: 1, avgMs: 20 })], '24h', null)
  assert.deepEqual(ys(series.line), [10, 20])
})

test('aggregates: a gap closes the window before the null points', () => {
  const buckets = Array.from({ length: 9 }, (_, i) => bucket((i < 3 ? i : i + 60) * MINUTE, { up: 1, avgMs: 10, minMs: 10, maxMs: 10 }))
  const series = bucketDatasets(buckets, '3h', 60)
  const nullIndexes = series.line.map((point, index) => (point.y === null ? index : -1)).filter((index) => index >= 0)
  assert.equal(nullIndexes.length, 2)
  // The points before the nulls are older than the gap and the points after are newer
  const gapStart = BASE + 2 * MINUTE
  series.line.forEach((point, index) => {
    if (index < nullIndexes[0]) {
      assert.ok(point.x <= gapStart)
    } else if (index > nullIndexes[1]) {
      assert.ok(point.x > gapStart)
    }
  })
})

// Stored period

test('stored period: Recent without a stored value, and the stored value when valid', () => {
  const storage = new MemoryStorage()
  assert.equal(readStoredPeriod(storage), 'recent')
  storePeriod('6h', storage)
  assert.equal(storage.values[CHART_PERIOD_STORAGE_KEY], '6h')
  assert.equal(readStoredPeriod(storage), '6h')
})

test('stored period: an invalid value goes back to Recent', () => {
  assert.equal(readStoredPeriod(new MemoryStorage({ [CHART_PERIOD_STORAGE_KEY]: '30d' })), 'recent')
  const storage = new MemoryStorage()
  storePeriod('7d', storage)
  assert.equal(CHART_PERIOD_STORAGE_KEY in storage.values, false)
})

test('stored period: an unreadable storage goes back to Recent and writing does not throw', () => {
  const broken = {
    getItem() {
      throw new Error('blocked')
    },
    setItem() {
      throw new Error('blocked')
    }
  }
  assert.equal(readStoredPeriod(broken), 'recent')
  assert.doesNotThrow(() => storePeriod('1w', broken))
  assert.equal(readStoredPeriod(null), 'recent')
  // Without a browser (Node), the default storage is missing
  assert.equal(readStoredPeriod(), 'recent')
  assert.doesNotThrow(() => storePeriod('1w'))
})

const series = (items) => items.map((id) => id)

test('legend: Recent without columns has only the response time', () => {
  const items = chartLegend('recent', { linePoints: 4, downColumns: 0, pendingColumns: 0 })
  assert.deepEqual(series(items.map((item) => item.id)), ['response-time'])
  assert.equal(items[0].color, CHART_LEGEND_COLORS.line)
  assert.equal(items[0].dash, 'solid')
})

test('legend: Recent with Down and with Pending', () => {
  assert.deepEqual(chartLegend('recent', { downColumns: 2, pendingColumns: 0 }).map((item) => item.id), ['response-time', 'down'])
  assert.deepEqual(chartLegend('recent', { downColumns: 0, pendingColumns: 1 }).map((item) => item.id), ['response-time', 'pending'])
})

test('legend: the aggregates name the three lines, in the order they are drawn', () => {
  const items = chartLegend('24h', { linePoints: 10, downColumns: 0, pendingColumns: 0 })
  assert.deepEqual(items.map((item) => item.id), ['average', 'minimum', 'maximum'])
  assert.deepEqual(items.map((item) => item.dash), ['solid', 'dashed', 'dotted'])
  assert.deepEqual(items.map((item) => item.color), [CHART_LEGEND_COLORS.line, CHART_LEGEND_COLORS.minLine, CHART_LEGEND_COLORS.maxLine])
})

test('legend: the columns come after the lines, Down before Pending', () => {
  const items = chartLegend('6h', { downColumns: 1, pendingColumns: 3 })
  assert.deepEqual(items.map((item) => item.id), ['average', 'minimum', 'maximum', 'down', 'pending'])
  assert.equal(items[3].column, true)
  assert.equal(items[4].column, true)
})

test('legend: an aggregate period without a line keeps the three lines', () => {
  assert.deepEqual(chartLegend('1w', { linePoints: 0, downColumns: 1, pendingColumns: 0 }).map((item) => item.id), ['average', 'minimum', 'maximum', 'down'])
  assert.deepEqual(chartLegend('1w').map((item) => item.id), ['average', 'minimum', 'maximum'])
})
