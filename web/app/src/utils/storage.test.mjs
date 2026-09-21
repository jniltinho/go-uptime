import assert from 'node:assert/strict'
import test from 'node:test'
import { legacyPreferenceKey, preferenceKey, readPreference, removePreference, writePreference } from './storage.js'

const memoryStorage = (initial = {}) => {
  const items = new Map(Object.entries(initial))
  return {
    items,
    getItem: (key) => (items.has(key) ? items.get(key) : null),
    setItem: (key, value) => { items.set(key, String(value)) },
    removeItem: (key) => { items.delete(key) }
  }
}

test('the keys have the prefix of the project, and the legacy ones the prefix of Gatus', () => {
  assert.equal(preferenceKey('sort-by'), 'go-uptime:sort-by')
  assert.equal(legacyPreferenceKey('sort-by'), 'gatus:sort-by')
})

test('a preference stored by v6 is migrated the first time it is read', () => {
  const storage = memoryStorage({ 'gatus:sort-by': 'health' })
  assert.equal(readPreference('sort-by', storage), 'health')
  assert.equal(storage.items.get('go-uptime:sort-by'), 'health')
  assert.equal(storage.items.has('gatus:sort-by'), false)
  assert.equal(readPreference('sort-by', storage), 'health')
})

test('the current key wins over the legacy one, which is left alone until the next write', () => {
  const storage = memoryStorage({ 'go-uptime:sort-by': 'name', 'gatus:sort-by': 'health' })
  assert.equal(readPreference('sort-by', storage), 'name')
  assert.equal(writePreference('sort-by', 'group', storage), true)
  assert.equal(storage.items.get('go-uptime:sort-by'), 'group')
  assert.equal(storage.items.has('gatus:sort-by'), false)
})

test('an empty value under the current key is a value, not a missing preference', () => {
  const storage = memoryStorage({ 'go-uptime:filter-by': '', 'gatus:filter-by': 'failing' })
  assert.equal(readPreference('filter-by', storage), '')
})

test('a preference that was never stored reads as null', () => {
  assert.equal(readPreference('sort-by', memoryStorage()), null)
})

test('removing a preference removes both keys', () => {
  const storage = memoryStorage({ 'go-uptime:collapsed-groups': '[]', 'gatus:collapsed-groups': '[]' })
  removePreference('collapsed-groups', storage)
  assert.equal(storage.items.size, 0)
})

test('a storage that is missing or throws never breaks the interface', () => {
  const throwing = {
    getItem: () => { throw new Error('blocked') },
    setItem: () => { throw new Error('full') },
    removeItem: () => { throw new Error('blocked') }
  }
  assert.equal(readPreference('sort-by', null), null)
  assert.equal(writePreference('sort-by', 'name', null), false)
  assert.equal(readPreference('sort-by', throwing), null)
  assert.equal(writePreference('sort-by', 'name', throwing), false)
  assert.doesNotThrow(() => removePreference('sort-by', throwing))
  assert.doesNotThrow(() => removePreference('sort-by', null))
})

test('a legacy value is still returned when it cannot be migrated', () => {
  const readOnly = {
    getItem: (key) => (key === 'gatus:sort-by' ? 'health' : null),
    setItem: () => { throw new Error('full') },
    removeItem: () => {}
  }
  assert.equal(readPreference('sort-by', readOnly), 'health')
})

test('the write succeeds even when the legacy key cannot be removed', () => {
  const items = new Map()
  const storage = {
    getItem: (key) => (items.has(key) ? items.get(key) : null),
    setItem: (key, value) => { items.set(key, value) },
    removeItem: () => { throw new Error('blocked') }
  }
  assert.equal(writePreference('sort-by', 'name', storage), true)
  assert.equal(items.get('go-uptime:sort-by'), 'name')
})
