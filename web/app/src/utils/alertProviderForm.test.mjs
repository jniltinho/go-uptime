// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { INHERIT, MASKED, NO, YES, documentFromForm, formFromDocument, groupByCategory, mapToText, missingRequired, providerState, textToMap, usageCount } from './alertProviderForm.js'

const telegram = {
  type: 'telegram',
  name: 'Telegram',
  category: 'Chat',
  fields: [
    { key: 'token', kind: 'string', secret: true, required: true },
    { key: 'id', kind: 'string', secret: false, required: true },
    { key: 'topic-id', kind: 'string', secret: false },
  ],
}

test('a definition goes to the form and back without losing the keys the form does not show', () => {
  const original = {
    token: MASKED,
    id: '42',
    client: { timeout: '5s' },
    overrides: [{ group: 'core', id: '7' }],
    'default-alert': { 'failure-threshold': 5, 'send-on-resolved': true, 'provider-override': { id: '9' } },
  }
  const form = formFromDocument(telegram, original)
  assert.equal(form.values.token, MASKED)
  assert.equal(form.defaultAlert.failureThreshold, '5')
  assert.equal(form.defaultAlert.sendOnResolved, YES)
  assert.deepEqual(documentFromForm(telegram, form, original), original)
})

test('empty fields remove their keys, and the type key is never sent', () => {
  const form = formFromDocument(telegram, { token: 't', id: '1', 'topic-id': '3' })
  form.values['topic-id'] = ''
  form.defaultAlert.sendOnResolved = NO
  assert.deepEqual(documentFromForm(telegram, form, { type: 'telegram', token: 't', id: '1', 'topic-id': '3' }), {
    token: 't',
    id: '1',
    'default-alert': { 'send-on-resolved': false },
  })
})

test('send-on-resolved inherited removes the key, and an empty default alert disappears', () => {
  const form = formFromDocument(telegram, { 'default-alert': { 'send-on-resolved': true } })
  form.defaultAlert.sendOnResolved = INHERIT
  assert.equal('default-alert' in documentFromForm(telegram, form, {}), false)
})

test('lists, maps, integers and booleans', () => {
  const info = {
    fields: [
      { key: 'to', kind: 'list' },
      { key: 'headers', kind: 'map' },
      { key: 'priority', kind: 'integer' },
      { key: 'disable-cache', kind: 'boolean' },
      { key: 'insecure', kind: 'boolean' },
    ],
  }
  const form = formFromDocument(info, { to: ['a@example.com', 'b@example.com'], headers: { Authorization: MASKED }, 'disable-cache': true })
  assert.equal(form.values.to, 'a@example.com, b@example.com')
  assert.equal(form.values.headers, `Authorization: ${MASKED}`)
  form.values.priority = '4'
  form.values['disable-cache'] = false
  assert.deepEqual(documentFromForm(info, form, { 'disable-cache': true }), {
    to: ['a@example.com', 'b@example.com'],
    headers: { Authorization: MASKED },
    priority: 4,
    'disable-cache': false,
  })
  assert.deepEqual(textToMap(mapToText({ a: '1', 'X-Key': 'b: c' })), { a: '1', 'X-Key': 'b: c' })
})

test('required fields, with a stored secret counting as filled', () => {
  const form = formFromDocument(telegram, { token: MASKED })
  assert.deepEqual(missingRequired(telegram, form), ['id'])
})

test('state, usage and categories of the list', () => {
  assert.equal(providerState({ source: 'none' }).label, 'Not configured')
  assert.equal(providerState({ source: 'web', state: 'invalid' }).tone, 'error')
  assert.equal(usageCount({ usage: { managed: ['a'], file: ['b', 'c'] } }), 3)
  const groups = groupByCategory([telegram, { type: 'ntfy', name: 'Ntfy', category: 'Push' }, { type: 'slack', name: 'Slack', category: 'Chat' }], 'sl')
  assert.deepEqual(groups.map((group) => [group.category, group.types.map((type) => type.type)]), [['Chat', ['slack']]])
})

test('every category of the providers is known to the picker', () => {
  const docs = JSON.parse(readFileSync(new URL('../../../../internal/alertprovider/docs.json', import.meta.url)))
  const groups = groupByCategory(Object.entries(docs).map(([type, doc]) => ({ type, name: doc.name, category: doc.category })))
  assert.equal(groups.reduce((total, group) => total + group.types.length, 0), Object.keys(docs).length)
})
