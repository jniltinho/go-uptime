// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Unit tests of the views of the public status pages (fork): npm run test:unit
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  aggregateStatus, groupStatuses, CELL_NOT_CONFIGURED, CELL_NOT_IN_ENVIRONMENT, CELL_NOT_SHOWN, collectGroups, counters, filterOptions,
  groupedModel, matrixModel, normalizeViews, resolveView, sanitizeFilters, viewLabel
} from './statusPageViews.js'

const endpoint = (name, status, application, environment, uptime30 = 1) => ({
  name, status, application, ...(environment ? { environment } : {}), uptime: { '24h': uptime30, '7d': uptime30, '30d': uptime30 }, results: []
})

const payload = {
  truncated: false,
  views: {
    enabled: ['list', 'grouped', 'matrix'],
    default: 'grouped',
    groupLabel: 'Client',
    environments: [{ id: 'prod', name: 'Production', short: 'PROD' }, { id: 'stg', name: 'Staging', short: 'STG' }, { id: 'qa', name: 'qa', short: 'QA' }],
    applications: ['Billing', 'API']
  },
  featured: [{ ...endpoint('billing-qa', 'down', 'Billing', 'qa', 0.9), group: 'Client A' }],
  groups: [
    {
      name: 'Client A', description: 'Acme customer', tags: ['BR'],
      endpoints: [endpoint('billing', 'up', 'Billing', 'prod', 0.995), endpoint('api', 'up', 'API', 'prod'), endpoint('api-stg', 'pending', 'API', 'stg')]
    },
    { name: 'Client B', endpoints: [endpoint('Portal', 'down', 'Portal', undefined, null), endpoint('billing', 'up', 'Billing', 'prod')] }
  ]
}

test('views default to the list', () => {
  assert.deepEqual(normalizeViews(undefined).enabled, ['list'])
  assert.equal(resolveView(undefined, 'matrix'), 'list')
  assert.equal(resolveView(payload.views, 'matrix'), 'matrix')
  assert.equal(resolveView({ enabled: ['list'] }, 'matrix'), 'list')
  assert.equal(viewLabel('grouped', 'Client'), 'By client')
})

test('aggregateStatus follows the rule of the groups', () => {
  assert.equal(aggregateStatus(['up', 'up']), 'operational')
  assert.equal(aggregateStatus(['down', 'down', 'unknown']), 'down')
  assert.equal(aggregateStatus(['up', 'down']), 'degraded')
  assert.equal(aggregateStatus(['up', 'pending']), 'degraded')
  assert.equal(aggregateStatus(['unknown']), 'unknown')
  assert.equal(aggregateStatus([]), 'unknown')
})

test('featured endpoints go back to their group', () => {
  const groups = collectGroups(payload)
  assert.deepEqual(groups[0].endpoints.map((e) => e.name), ['api', 'api-stg', 'billing', 'billing-qa'])
  const onlyFeatured = collectGroups({ groups: [], featured: [{ ...endpoint('x', 'up', 'X'), group: 'Solo' }] })
  assert.equal(onlyFeatured[0].name, 'Solo')
})

test('counters cover the whole page', () => {
  const c = counters(payload)
  assert.equal(c.groups, 2)
  assert.equal(c.monitors, 6)
  assert.equal(c.primaryDown, 0)
  assert.equal(c.otherDown, 2)
  assert.equal(c.affectedGroups, 2)
  assert.equal(c.pending, 1)
})

test('grouped model splits by environment and filters', () => {
  const model = groupedModel(payload)
  const a = model[0]
  assert.equal(a.status, 'degraded')
  assert.deepEqual(a.environments, ['PROD', 'STG', 'QA'])
  assert.deepEqual(a.sections.map((s) => s.id), ['prod', 'stg', 'qa'])
  assert.deepEqual(a.sections[0].endpoints.map((e) => e.name), ['billing', 'api'])
  assert.deepEqual(a.applications.map((x) => x.name), ['Billing', 'API'])
  const b = model[1]
  assert.deepEqual(b.sections.map((s) => s.id), ['prod', ''])
  const stg = groupedModel(payload, { environment: 'stg' })
  assert.deepEqual(stg.map((g) => g.name), ['Client A'])
  assert.equal(groupedModel(payload, { application: 'Nothing' }).length, 0)
  const problems = groupedModel(payload, { environment: 'prod', onlyProblems: true })
  assert.equal(problems.length, 0)
})

test('matrix cells', () => {
  const { columns, rows } = matrixModel(payload)
  assert.deepEqual(columns, ['Billing', 'API', 'Portal'])
  const [billing, api, portal] = rows[0].cells
  assert.equal(billing.status, 'degraded')
  assert.equal(billing.uptime, 0.995)
  assert.deepEqual(billing.environments, ['PROD', 'QA'])
  assert.equal(api.status, 'degraded')
  assert.equal(portal.kind, CELL_NOT_CONFIGURED)
  const bPortal = rows[1].cells[2]
  assert.equal(bPortal.status, 'down')
  assert.equal(bPortal.uptime, null)
  const qa = matrixModel(payload, { environment: 'qa' })
  assert.equal(qa.rows[0].cells[1].kind, CELL_NOT_IN_ENVIRONMENT)
  assert.equal(qa.rows[0].cells[0].uptime, 0.9)
  const truncated = matrixModel({ ...payload, truncated: true })
  assert.equal(truncated.rows[0].cells[2].kind, CELL_NOT_SHOWN)
  assert.deepEqual(matrixModel(payload, { application: 'API' }).columns, ['API'])
})

test('uptime falls back to the first environment with data', () => {
  const p = { views: payload.views, groups: [{ name: 'G', endpoints: [endpoint('a', 'up', 'A', 'prod', null), endpoint('b', 'up', 'A', 'stg', 0.97), endpoint('c', 'up', 'A', 'qa', 0.5)] }] }
  assert.equal(matrixModel(p).rows[0].cells[0].uptime, 0.97)
})

test('filters that disappeared fall back to all', () => {
  assert.deepEqual(filterOptions(payload).environments.map((e) => e.id), ['prod', 'stg', 'qa'])
  assert.deepEqual(sanitizeFilters(payload, { environment: 'uat', application: 'API', onlyProblems: true }), { environment: '', application: 'API', onlyProblems: true })
  assert.deepEqual(sanitizeFilters(payload, { environment: 'qa', application: 'Gone' }), { environment: 'qa', application: '', onlyProblems: false })
})

test('only problems honours the application filter in the matrix', () => {
  // Client A has a problem in Billing (qa) and API (stg pending); Client B only in Portal
  assert.deepEqual(matrixModel(payload, { application: 'Billing', onlyProblems: true }).rows.map((r) => r.name), ['Client A'])
  assert.deepEqual(matrixModel(payload, { application: 'Integrations', onlyProblems: true }).rows.length, 0)
})

test('featured-only groups keep their details and their status', () => {
  const p = { views: { groups: [{ name: 'Solo', description: 'Only featured', tags: ['X'] }] }, groups: [{ name: 'G', status: 'operational', endpoints: [endpoint('a', 'up', 'A')] }], featured: [{ ...endpoint('f', 'down', 'F'), group: 'Solo' }, { ...endpoint('g', 'down', 'F'), group: 'G' }] }
  const solo = collectGroups(p).find((g) => g.name === 'Solo')
  assert.equal(solo.description, 'Only featured')
  assert.deepEqual(solo.tags, ['X'])
  const statuses = groupStatuses(p)
  assert.equal(statuses.get('Solo'), 'down')
  assert.equal(statuses.get('G'), 'degraded')
})
