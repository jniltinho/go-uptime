// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { beforeEach, afterEach, mock, test } from 'node:test'
import assert from 'node:assert/strict'
import { REOPEN_DELAYS_MS, RESULT_DELAY_MS, reopenDelayMs, urlWithLastEventId, watchEndpointResults } from './liveUpdates.js'

const URL = '/api/v1/endpoints/jobs_backup/events'

class FakeEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances = []

  constructor(url) {
    this.url = url
    this.readyState = FakeEventSource.CONNECTING
    this.listeners = {}
    this.closed = false
    FakeEventSource.instances.push(this)
  }

  addEventListener(type, listener) {
    (this.listeners[type] ||= []).push(listener)
  }

  removeEventListener(type, listener) {
    this.listeners[type] = (this.listeners[type] || []).filter((l) => l !== listener)
  }

  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
  }

  emit(type, event = {}) {
    for (const listener of this.listeners[type] || []) {
      listener(event)
    }
  }

  open() {
    this.readyState = FakeEventSource.OPEN
    this.emit('open')
  }

  result(id) {
    this.emit('result', { lastEventId: String(id), data: '{}' })
  }

  fail(readyState) {
    this.readyState = readyState
    this.emit('error')
  }
}

class FakeDocument {
  constructor() {
    this.visibilityState = 'visible'
    this.listeners = new Set()
  }

  addEventListener(type, listener) {
    if (type === 'visibilitychange') this.listeners.add(listener)
  }

  removeEventListener(type, listener) {
    if (type === 'visibilitychange') this.listeners.delete(listener)
  }

  setVisibility(state) {
    this.visibilityState = state
    for (const listener of this.listeners) listener()
  }
}

const latest = () => FakeEventSource.instances[FakeEventSource.instances.length - 1]
const openSources = () => FakeEventSource.instances.filter((source) => !source.closed)

let doc
let calls
let watcher

const start = (options = {}) => {
  watcher = watchEndpointResults(URL, () => { calls++ }, { EventSource: FakeEventSource, document: doc, ...options })
  return watcher
}

beforeEach(() => {
  mock.timers.enable({ apis: ['setTimeout', 'Date'] })
  FakeEventSource.instances = []
  doc = new FakeDocument()
  calls = 0
  watcher = null
})

afterEach(() => {
  watcher?.stop()
  mock.timers.reset()
})

test('the waits before reopening grow up to 5 minutes', () => {
  assert.deepEqual(REOPEN_DELAYS_MS, [30000, 60000, 120000, 300000])
  assert.equal(reopenDelayMs(0), 30000)
  assert.equal(reopenDelayMs(1), 60000)
  assert.equal(reopenDelayMs(2), 120000)
  assert.equal(reopenDelayMs(3), 300000)
  assert.equal(reopenDelayMs(4), 300000)
  assert.equal(reopenDelayMs(100), 300000)
  assert.equal(reopenDelayMs(-1), 30000)
  assert.equal(reopenDelayMs(undefined), 30000)
})

test('the address carries the last event id', () => {
  assert.equal(urlWithLastEventId(URL, ''), URL)
  assert.equal(urlWithLastEventId(URL, null), URL)
  assert.equal(urlWithLastEventId(URL, undefined), URL)
  assert.equal(urlWithLastEventId(URL, '42'), `${URL}?lastEventId=42`)
  assert.equal(urlWithLastEventId(URL, 7), `${URL}?lastEventId=7`)
  assert.equal(urlWithLastEventId(`${URL}?a=1`, '42'), `${URL}?a=1&lastEventId=42`)
  assert.equal(urlWithLastEventId(`${URL}?`, '42'), `${URL}?lastEventId=42`)
  assert.equal(urlWithLastEventId(`${URL}#x`, '42'), `${URL}?lastEventId=42#x`)
  assert.equal(urlWithLastEventId(URL, 'a b'), `${URL}?lastEventId=a%20b`)
})

test('close notifications are merged into a single call', () => {
  start()
  assert.equal(openSources().length, 1)
  assert.equal(latest().url, URL)
  latest().open()
  latest().result(1)
  latest().result(2)
  mock.timers.tick(RESULT_DELAY_MS - 1)
  latest().result(3)
  assert.equal(calls, 0)
  mock.timers.tick(1)
  assert.equal(calls, 1)
  latest().result(4)
  mock.timers.tick(RESULT_DELAY_MS)
  assert.equal(calls, 2)
})

test('the native reconnection is left alone', () => {
  start()
  latest().open()
  latest().fail(FakeEventSource.CONNECTING)
  mock.timers.tick(10 * 60 * 1000)
  assert.equal(FakeEventSource.instances.length, 1)
  assert.equal(openSources().length, 1)
})

test('a channel closed by an error is reopened with backoff and the last event id', () => {
  start()
  latest().open()
  latest().result(5)
  mock.timers.tick(RESULT_DELAY_MS)
  latest().fail(FakeEventSource.CLOSED)
  assert.equal(openSources().length, 0)

  for (const delay of [30000, 60000, 120000, 300000, 300000]) {
    const count = FakeEventSource.instances.length
    mock.timers.tick(delay - 1)
    assert.equal(FakeEventSource.instances.length, count)
    mock.timers.tick(1)
    assert.equal(FakeEventSource.instances.length, count + 1)
    assert.equal(latest().url, `${URL}?lastEventId=5`)
    latest().fail(FakeEventSource.CLOSED)
  }

  // A successful connection starts the backoff over
  mock.timers.tick(300000)
  latest().open()
  latest().fail(FakeEventSource.CLOSED)
  const count = FakeEventSource.instances.length
  mock.timers.tick(30000)
  assert.equal(FakeEventSource.instances.length, count + 1)
})

test('a successful refresh reopens a closed channel before the backoff, at most once every 30 seconds', () => {
  const { notifyRefreshSucceeded } = start()
  // Nothing to do while the channel is open
  notifyRefreshSucceeded()
  assert.equal(FakeEventSource.instances.length, 1)

  // Refused right away: the refreshes of the next 30 seconds do not reopen it
  latest().fail(FakeEventSource.CLOSED)
  mock.timers.tick(10000)
  notifyRefreshSucceeded()
  assert.equal(FakeEventSource.instances.length, 1)
  mock.timers.tick(20000)
  assert.equal(FakeEventSource.instances.length, 2)

  // Refused again: the backoff is now 60 seconds, but a refresh 30 seconds later reopens it and cancels the wait
  latest().fail(FakeEventSource.CLOSED)
  assert.equal(openSources().length, 0)
  mock.timers.tick(30000)
  notifyRefreshSucceeded()
  assert.equal(FakeEventSource.instances.length, 3)
  assert.equal(openSources().length, 1)
  notifyRefreshSucceeded()
  assert.equal(FakeEventSource.instances.length, 3)
  mock.timers.tick(30000)
  assert.equal(FakeEventSource.instances.length, 3)

  // A channel that opens resets the backoff to 30 seconds
  latest().open()
  latest().fail(FakeEventSource.CLOSED)
  mock.timers.tick(29999)
  assert.equal(FakeEventSource.instances.length, 3)
  mock.timers.tick(1)
  assert.equal(FakeEventSource.instances.length, 4)
  assert.equal(openSources().length, 1)
})

test('the channel is closed while the tab is hidden', () => {
  start()
  latest().open()
  latest().result(9)
  doc.setVisibility('hidden')
  assert.equal(openSources().length, 0)
  mock.timers.tick(10 * 60 * 1000)
  assert.equal(calls, 0)

  doc.setVisibility('visible')
  assert.equal(openSources().length, 1)
  assert.equal(latest().url, `${URL}?lastEventId=9`)
  // The event sent right away by the server is merged with the refresh of the return
  latest().result(10)
  mock.timers.tick(RESULT_DELAY_MS)
  assert.equal(calls, 1)

  // A second visible notification does not open another channel
  doc.setVisibility('visible')
  assert.equal(openSources().length, 1)
})

test('without refreshOnVisible, the return only reopens the channel', () => {
  start({ refreshOnVisible: false })
  doc.setVisibility('hidden')
  doc.setVisibility('visible')
  mock.timers.tick(RESULT_DELAY_MS)
  assert.equal(calls, 0)
  assert.equal(openSources().length, 1)
})

test('a page opened in a hidden tab waits to be shown', () => {
  doc.visibilityState = 'hidden'
  start()
  assert.equal(FakeEventSource.instances.length, 0)
  doc.setVisibility('visible')
  assert.equal(openSources().length, 1)
})

test('the channel closed by an error while hidden is reopened when shown', () => {
  start()
  latest().fail(FakeEventSource.CLOSED)
  doc.setVisibility('hidden')
  mock.timers.tick(30000)
  assert.equal(FakeEventSource.instances.length, 1)
  watcher.notifyRefreshSucceeded()
  assert.equal(FakeEventSource.instances.length, 1)
  doc.setVisibility('visible')
  assert.equal(openSources().length, 1)
})

test('stop closes everything', () => {
  const { stop, notifyRefreshSucceeded } = start()
  latest().open()
  latest().result(1)
  stop()
  assert.equal(openSources().length, 0)
  assert.equal(doc.listeners.size, 0)
  mock.timers.tick(10 * 60 * 1000)
  assert.equal(calls, 0)
  notifyRefreshSucceeded()
  doc.setVisibility('visible')
  assert.equal(FakeEventSource.instances.length, 1)
})

test('without EventSource, nothing is opened', () => {
  const { stop, notifyRefreshSucceeded } = watchEndpointResults(URL, () => {}, { EventSource: null, document: doc })
  notifyRefreshSucceeded()
  stop()
  assert.equal(FakeEventSource.instances.length, 0)
})
