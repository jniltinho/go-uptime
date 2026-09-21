// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { afterEach, beforeEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { dialogOpen, dialogPosition, isTopDialog, nextDialogId, openDialogs, pushDialog, removeDialog, topDialog, uniqueId } from './dialogStack.js'

// A fake document that records its listeners and the classes of the body
const createDocument = () => {
  const listeners = []
  const classes = new Set()
  return {
    listeners,
    classes,
    addEventListener: (type, listener, capture) => listeners.push({ type, listener, capture }),
    removeEventListener: (type, listener, capture) => {
      const index = listeners.findIndex((item) => item.type === type && item.listener === listener && item.capture === capture)
      if (index !== -1) {
        listeners.splice(index, 1)
      }
    },
    dispatch: (type, event) => listeners.filter((item) => item.type === type).forEach((item) => item.listener(event)),
    body: { classList: { add: (name) => classes.add(name), remove: (name) => classes.delete(name) } }
  }
}

let fakeDocument

beforeEach(() => {
  fakeDocument = createDocument()
  global.document = fakeDocument
})

afterEach(() => {
  for (const id of openDialogs()) {
    removeDialog(id)
  }
  delete global.document
})

test('ids are incremental', () => {
  const first = nextDialogId()
  assert.equal(nextDialogId(), first + 1)
  assert.equal(uniqueId('message'), 'message-1')
  assert.equal(uniqueId('message'), 'message-2')
  assert.equal(uniqueId('other'), 'other-1')
})

test('dialogs are stacked in the order they open and removed by id', () => {
  assert.equal(dialogOpen.value, false)
  pushDialog(1)
  pushDialog(2)
  assert.equal(dialogOpen.value, true)
  assert.deepEqual(openDialogs(), [1, 2])
  assert.equal(dialogPosition(1), 0)
  assert.equal(dialogPosition(2), 1)
  assert.equal(dialogPosition(3), -1)
  assert.equal(isTopDialog(2), true)
  assert.equal(isTopDialog(1), false)

  // The preview closes and the results open in the same tick
  pushDialog(3)
  assert.equal(removeDialog(1), false)
  assert.deepEqual(openDialogs(), [2, 3])
  assert.equal(removeDialog(3), true)
  assert.equal(isTopDialog(2), true)
  assert.equal(removeDialog(3), false)
  assert.equal(removeDialog(2), true)
  assert.equal(dialogOpen.value, false)
})

test('a single capturing listener and the body class only exist while the stack is not empty', () => {
  assert.equal(fakeDocument.listeners.length, 0)
  pushDialog(1)
  pushDialog(2)
  assert.deepEqual(fakeDocument.listeners.map((item) => [item.type, item.capture]), [['keydown', true], ['focusin', true]])
  assert.equal(fakeDocument.classes.has('overflow-hidden'), true)
  removeDialog(2)
  assert.equal(fakeDocument.listeners.length, 2)
  removeDialog(1)
  assert.equal(fakeDocument.listeners.length, 0)
  assert.equal(fakeDocument.classes.has('overflow-hidden'), false)
})

test('events go to the dialog on top only', () => {
  const received = []
  pushDialog(1, { onKeydown: () => received.push('keydown 1'), onFocusin: () => received.push('focusin 1') })
  pushDialog(2, { onKeydown: () => received.push('keydown 2'), onFocusin: () => received.push('focusin 2') })
  assert.equal(topDialog().onKeydown !== undefined, true)
  fakeDocument.dispatch('keydown', {})
  fakeDocument.dispatch('focusin', {})
  removeDialog(2)
  fakeDocument.dispatch('keydown', {})
  assert.deepEqual(received, ['keydown 2', 'focusin 2', 'keydown 1'])
  removeDialog(1)
  assert.equal(topDialog(), null)
})
