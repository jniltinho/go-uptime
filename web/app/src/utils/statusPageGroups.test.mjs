// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  COLLAPSED,
  EXPANDED,
  MAXIMUM_CHOICES,
  STORAGE_KEY,
  groupCounts,
  groupHashes,
  isCollapsed,
  readChoices,
  rememberedChoices,
  writeChoice
} from './statusPageGroups.js'

const memoryStorage = (initial = {}) => {
  const items = new Map(Object.entries(initial))
  return {
    getItem: (key) => (items.has(key) ? items.get(key) : null),
    setItem: (key, value) => { items.set(key, String(value)) },
    removeItem: (key) => { items.delete(key) },
    raw: () => items.get(STORAGE_KEY)
  }
}

test('a group that is not operational is expanded whatever the visitor and the page say', () => {
  for (const status of ['degraded', 'down', 'unknown']) {
    assert.equal(isCollapsed({ status, visitChoice: COLLAPSED, rememberedChoice: COLLAPSED, pageDefault: true }), false, status)
  }
})

test('a group collapsed during an incident stays collapsed only while the component says so', () => {
  assert.equal(isCollapsed({ status: 'degraded', pageDefault: false, incidentCollapsed: true }), true)
  // the next payload clears incidentCollapsed, and the group is expanded again
  assert.equal(isCollapsed({ status: 'degraded', pageDefault: false, incidentCollapsed: false }), false)
})

test('an operational group follows the visit, then the remembered choice, then the page', () => {
  assert.equal(isCollapsed({ status: 'operational', pageDefault: false }), false)
  assert.equal(isCollapsed({ status: 'operational', pageDefault: true }), true)
  assert.equal(isCollapsed({ status: 'operational', rememberedChoice: EXPANDED, pageDefault: true }), false)
  assert.equal(isCollapsed({ status: 'operational', rememberedChoice: COLLAPSED, pageDefault: false }), true)
  assert.equal(isCollapsed({ status: 'operational', visitChoice: EXPANDED, rememberedChoice: COLLAPSED, pageDefault: true }), false)
})

test('the choice survives an incident: forced open while degraded, collapsed again when operational', () => {
  const choice = { visitChoice: COLLAPSED, pageDefault: false }
  assert.equal(isCollapsed({ ...choice, status: 'operational' }), true)
  assert.equal(isCollapsed({ ...choice, status: 'degraded' }), false)
  assert.equal(isCollapsed({ ...choice, status: 'operational' }), true)
})

test('the counts of a group leave out the statuses that have none', () => {
  assert.equal(groupCounts({ total: 3, up: 3, down: 0, pending: 0, unknown: 0 }), '3 up')
  assert.equal(groupCounts({ total: 5, up: 2, down: 1, pending: 1, unknown: 1 }), '2 up · 1 down · 1 pending · 1 no data')
  assert.equal(groupCounts({ total: 2, up: 0, down: 0, pending: 0, unknown: 2 }), '2 no data')
  assert.equal(groupCounts(undefined), '')
})

test('the key of a group is a hash of the slug and of its raw name, and stores no name', async () => {
  const hashes = await groupHashes('services', ['sites', '', 'Other services', '__without-group__'])
  const values = [...hashes.values()]
  assert.equal(new Set(values).size, 4, 'the group without name, "Other services" and "__without-group__" must not collide')
  for (const value of values) {
    assert.match(value, /^[0-9a-f]{64}$/)
  }
  const other = await groupHashes('internal', ['sites'])
  assert.notEqual(other.get('sites'), hashes.get('sites'), 'the same group of another page has another key')
  const storage = memoryStorage()
  writeChoice(hashes.get('sites'), COLLAPSED, storage)
  assert.ok(!storage.raw().includes('sites') && !storage.raw().includes('services'), storage.raw())
})

test('without crypto.subtle the keys are not derived and nothing is remembered', async () => {
  assert.equal(await groupHashes('services', ['sites'], null), null)
  assert.equal(await groupHashes('services', ['sites'], { digest: () => { throw new Error('insecure context') } }), null)
  const { hashes, remembered } = await rememberedChoices('services', [{ name: 'sites' }], { subtle: null, storage: memoryStorage() })
  assert.equal(hashes, null)
  assert.equal(remembered.size, 0)
})

test('a remembered choice comes back for the right group of the right page', async () => {
  const storage = memoryStorage()
  const hashes = await groupHashes('services', ['sites', 'apis'])
  assert.equal(writeChoice(hashes.get('sites'), COLLAPSED, storage), true)
  const services = await rememberedChoices('services', [{ name: 'sites' }, { name: 'apis' }, { name: '' }], { storage })
  assert.deepEqual([...services.remembered], [['sites', COLLAPSED]])
  const internal = await rememberedChoices('internal', [{ name: 'sites' }], { storage })
  assert.equal(internal.remembered.size, 0)
})

test('invalid stored data is ignored', () => {
  const key = 'a'.repeat(64)
  for (const raw of ['not json', '[]', '"text"', '42', 'null', JSON.stringify({ sites: COLLAPSED }), JSON.stringify({ [key]: 'x' }), JSON.stringify({ __proto__: COLLAPSED })]) {
    assert.equal(readChoices(memoryStorage({ [STORAGE_KEY]: raw })).size, 0, raw)
  }
  const mixed = JSON.stringify({ [key]: EXPANDED, sites: COLLAPSED, ['b'.repeat(64)]: 'zzz' })
  assert.deepEqual([...readChoices(memoryStorage({ [STORAGE_KEY]: mixed }))], [[key, EXPANDED]])
})

test('a storage that throws or is missing does not break anything', () => {
  const broken = { getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } }
  assert.equal(readChoices(broken).size, 0)
  assert.equal(writeChoice('a'.repeat(64), COLLAPSED, broken), false)
  assert.equal(readChoices(null).size, 0)
  assert.equal(writeChoice('a'.repeat(64), COLLAPSED, null), false)
  assert.equal(writeChoice('sites', COLLAPSED, memoryStorage()), false, 'a key that is not a hash is never stored')
})

test('above the maximum the oldest choices are discarded first, and touching a choice makes it the newest', () => {
  const storage = memoryStorage()
  const keyOf = (index) => index.toString(16).padStart(64, '0')
  for (let index = 0; index < MAXIMUM_CHOICES; index++) {
    writeChoice(keyOf(index), COLLAPSED, storage)
  }
  writeChoice(keyOf(0), EXPANDED, storage) // touched: now the newest
  writeChoice(keyOf(MAXIMUM_CHOICES), COLLAPSED, storage) // the 501st
  const choices = readChoices(storage)
  assert.equal(choices.size, MAXIMUM_CHOICES)
  assert.equal(choices.has(keyOf(1)), false, 'the oldest untouched choice is the one discarded')
  assert.equal(choices.get(keyOf(0)), EXPANDED)
  assert.equal(choices.has(keyOf(MAXIMUM_CHOICES)), true)
})
