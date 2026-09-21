// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { detailsSummaryItems, formatCurrentResponseTime } from './detailsSummary.js'

// The percentages use the locale of the runtime, as in the browser
const percent = (value) => new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 2 }).format(value)

test('shows the numbers of an HTTP endpoint', () => {
  const items = detailsSummaryItems({
    currentResponseTime: 5,
    responseTime: { '24h': 12, '7d': 15, '30d': 20 },
    uptime: { '24h': 1, '7d': 0.995, '30d': 0.9925 },
    push: false,
  })
  assert.deepEqual(items.map((item) => item.label), ['Response (Current)', 'Avg. Response (24h)', 'Uptime (24h)', 'Uptime (7d)', 'Uptime (30d)'])
  assert.deepEqual(items.map((item) => item.value), ['5 ms', '12 ms', percent(1), percent(0.995), percent(0.9925)])
  assert.equal(percent(0.9925).replace(',', '.').replace(/\s/g, ''), '99.25%')
  assert.equal(percent(0.995).replace(',', '.').replace(/\s/g, ''), '99.5%')
  assert.equal(percent(1).replace(/\s/g, ''), '100%')
})

test('labels the push endpoints with ping', () => {
  const items = detailsSummaryItems({ push: true, currentResponseTime: 30, responseTime: { '24h': 100 }, uptime: { '24h': 0.75 } })
  assert.equal(items[0].label, 'Ping (Current)')
  assert.equal(items[1].label, 'Avg. Ping (24h)')
  assert.equal(items[1].value, '100 ms')
})

test('shows dashes without execution or without results', () => {
  const nulls = { '24h': null, '7d': null, '30d': null }
  assert.deepEqual(detailsSummaryItems({ currentResponseTime: null, uptime: nulls, responseTime: nulls }).map((item) => item.value), ['—', '—', '—', '—', '—'])
  assert.deepEqual(detailsSummaryItems({}).map((item) => item.value), ['—', '—', '—', '—', '—'])
  assert.deepEqual(detailsSummaryItems().map((item) => item.value), ['—', '—', '—', '—', '—'])
})

test('shows a dash for a current response time of zero, but keeps an average and an uptime of zero', () => {
  assert.equal(formatCurrentResponseTime(0), '—')
  assert.equal(formatCurrentResponseTime(undefined), '—')
  assert.equal(formatCurrentResponseTime(7), '7 ms')
  const items = detailsSummaryItems({ currentResponseTime: 0, responseTime: { '24h': 0 }, uptime: { '24h': 0 } })
  assert.equal(items[0].value, '—')
  assert.equal(items[1].value, '0 ms')
  assert.equal(items[2].value, percent(0))
})
