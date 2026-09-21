// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { afterEach, beforeEach, mock, test } from 'node:test'
import assert from 'node:assert/strict'
import {
  MAXIMUM_TOASTS,
  TOAST_DURATIONS,
  announcements,
  clearToasts,
  dismissToast,
  pauseToasts,
  pendingToastTimers,
  resumeToasts,
  showToast,
  toast,
  toasts,
  toastsPaused
} from './toast.js'

beforeEach(() => {
  mock.timers.enable({ apis: ['setTimeout', 'Date'] })
})

afterEach(() => {
  resumeToasts()
  clearToasts()
  mock.timers.reset()
})

const messages = () => toasts.map((item) => item.message)

test('the durations are 5 s for success and info, 8 s for warnings and 10 s for errors', () => {
  assert.deepEqual({ ...TOAST_DURATIONS }, { success: 5000, info: 5000, warning: 8000, error: 10000 })
  const id = toast.success('Success')
  assert.deepEqual({ ...toasts[0] }, { id, type: 'success', title: '', message: 'Success' })
  toast.info('Info')
  toast.warning('Warning')
  toast.error('Error')
  mock.timers.tick(4999)
  assert.deepEqual(messages(), ['Success', 'Info', 'Warning', 'Error'])
  mock.timers.tick(1)
  assert.deepEqual(messages(), ['Warning', 'Error'])
  mock.timers.tick(2999)
  assert.deepEqual(messages(), ['Warning', 'Error'])
  mock.timers.tick(1)
  assert.deepEqual(messages(), ['Error'])
  mock.timers.tick(1999)
  assert.deepEqual(messages(), ['Error'])
  mock.timers.tick(1)
  assert.deepEqual(messages(), [])
  assert.equal(pendingToastTimers(), 0)
})

test('an unknown type is info, with the optional title', () => {
  showToast('other', 'Something', { title: 'Title' })
  assert.equal(toasts[0].type, 'info')
  assert.equal(toasts[0].title, 'Title')
})

test('a duration of 0 keeps the toast until it is dismissed', () => {
  const id = toast.error('Sticky', { duration: 0 })
  assert.equal(pendingToastTimers(), 0)
  mock.timers.tick(60000)
  assert.deepEqual(messages(), ['Sticky'])
  dismissToast(id)
  assert.deepEqual(messages(), [])
})

test('the oldest toasts are removed above the maximum, with their timers', () => {
  for (let i = 0; i < MAXIMUM_TOASTS + 2; i++) {
    toast.info(`Message ${i}`)
  }
  assert.equal(toasts.length, MAXIMUM_TOASTS)
  assert.equal(toasts[0].message, 'Message 2')
  assert.equal(pendingToastTimers(), MAXIMUM_TOASTS)
})

test('dismissing a toast with a pending timer removes the timer', () => {
  const first = toast.success('First')
  toast.success('Second')
  dismissToast(first)
  assert.deepEqual(messages(), ['Second'])
  assert.equal(pendingToastTimers(), 1)
  mock.timers.tick(TOAST_DURATIONS.success)
  assert.deepEqual(messages(), [])
  // Dismissing an unknown or already dismissed toast does nothing
  dismissToast(first)
})

test('pausing freezes the remaining time, and resuming counts it again', () => {
  toast.success('Success')
  mock.timers.tick(3000)
  pauseToasts()
  assert.equal(toastsPaused(), true)
  mock.timers.tick(60000)
  assert.deepEqual(messages(), ['Success'])
  // A toast shown while paused waits too
  toast.error('Error')
  mock.timers.tick(60000)
  assert.equal(toasts.length, 2)
  resumeToasts()
  assert.equal(toastsPaused(), false)
  mock.timers.tick(1999)
  assert.deepEqual(messages(), ['Success', 'Error'])
  mock.timers.tick(1)
  assert.deepEqual(messages(), ['Error'])
  mock.timers.tick(TOAST_DURATIONS.error - 2000)
  assert.deepEqual(messages(), [])
})

test('clearToasts removes every toast, the timers and the announcements', () => {
  toast.success('Success')
  toast.error('Error', { title: 'Failed' })
  assert.equal(announcements.status.text, 'Success')
  assert.equal(announcements.alert.text, 'Failed: Error')
  clearToasts()
  assert.deepEqual(messages(), [])
  assert.equal(pendingToastTimers(), 0)
  assert.equal(announcements.status.text, '')
  assert.equal(announcements.alert.text, '')
  mock.timers.tick(TOAST_DURATIONS.error)
})

test('errors are announced by the alert region and the others by the status region', () => {
  const first = toast.warning('Warning')
  toast.error('Error')
  const last = toast.info('Info')
  assert.deepEqual({ ...announcements.status }, { id: last, text: 'Info' })
  assert.notEqual(announcements.status.id, first)
  assert.equal(announcements.alert.text, 'Error')
})
