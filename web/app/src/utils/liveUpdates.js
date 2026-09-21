// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Fork: real-time updates of the endpoint details pages. The server only notifies that the endpoint has a new result,
// through Server-Sent Events, and the page fetches the data again with its usual APIs.

// Notifications that arrive close together are merged into a single refresh
export const RESULT_DELAY_MS = 500

// Waits before opening again a channel closed by an error (502 of the proxy during a reload, 503, 429, 404 or 401).
// After the last one, the channel is opened again every 5 minutes.
export const REOPEN_DELAYS_MS = [30000, 60000, 120000, 300000]

// readyState of an EventSource closed for good, which the browser no longer reconnects
const CLOSED = 2

// reopenDelayMs returns the wait before the given reopening attempt (0 for the first one)
export function reopenDelayMs(attempt) {
  const index = Number.isInteger(attempt) && attempt > 0 ? attempt : 0
  return REOPEN_DELAYS_MS[Math.min(index, REOPEN_DELAYS_MS.length - 1)]
}

// urlWithLastEventId returns the address of the channel with the last event id received, because a new EventSource
// does not inherit it and cannot send the Last-Event-ID header by itself
export function urlWithLastEventId(url, lastEventId) {
  if (lastEventId === undefined || lastEventId === null || lastEventId === '') {
    return url
  }
  const hashIndex = url.indexOf('#')
  const base = hashIndex === -1 ? url : url.slice(0, hashIndex)
  const hash = hashIndex === -1 ? '' : url.slice(hashIndex)
  const separator = base.includes('?') ? (base.endsWith('?') || base.endsWith('&') ? '' : '&') : '?'
  return `${base}${separator}lastEventId=${encodeURIComponent(String(lastEventId))}${hash}`
}

// watchEndpointResults opens the events channel at url and calls onResult, at most once every RESULT_DELAY_MS, when
// the endpoint has a new result. The channel is closed while the tab is hidden and, when the tab is shown again, it is
// opened again and onResult is called once (unless refreshOnVisible is false, for pages that already refresh then).
//
// It returns stop, which closes everything, and notifyRefreshSucceeded, which the page calls after each successful
// refresh so that a channel closed by an error is opened again right away, if it was opened at least REOPEN_DELAYS_MS[0]
// ago.
export function watchEndpointResults(url, onResult, options = {}) {
  const EventSourceClass = 'EventSource' in options ? options.EventSource : (typeof window !== 'undefined' ? window.EventSource : undefined)
  const doc = 'document' in options ? options.document : (typeof document !== 'undefined' ? document : undefined)
  const refreshOnVisible = options.refreshOnVisible !== false

  let source = null
  let lastEventId = ''
  let closedByError = false
  let attempt = 0
  let resultTimer = null
  let reopenTimer = null
  let stopped = false
  // When the channel was last opened, so that the refreshes of a page do not reopen a refused channel more often than
  // the first wait (a dashboard refreshing every 10 seconds would otherwise retry a 429 every 10 seconds)
  let openedAt = -Infinity

  const noop = { stop() {}, notifyRefreshSucceeded() {} }
  if (typeof EventSourceClass !== 'function') {
    // Without EventSource, the page keeps only its periodic refresh
    return noop
  }

  const isHidden = () => Boolean(doc) && doc.visibilityState === 'hidden'

  const clearReopenTimer = () => {
    clearTimeout(reopenTimer)
    reopenTimer = null
  }

  const clearResultTimer = () => {
    clearTimeout(resultTimer)
    resultTimer = null
  }

  const scheduleResult = () => {
    if (resultTimer !== null) {
      return
    }
    resultTimer = setTimeout(() => {
      resultTimer = null
      if (!stopped) {
        onResult()
      }
    }, RESULT_DELAY_MS)
  }

  const rememberLastEventId = (event) => {
    if (event && typeof event.lastEventId === 'string' && event.lastEventId !== '') {
      lastEventId = event.lastEventId
    }
  }

  const closeSource = () => {
    if (source === null) {
      return
    }
    source.removeEventListener('open', handleOpen)
    source.removeEventListener('message', rememberLastEventId)
    source.removeEventListener('result', handleResult)
    source.removeEventListener('error', handleError)
    source.close()
    source = null
  }

  function handleOpen() {
    attempt = 0
    closedByError = false
  }

  function handleResult(event) {
    rememberLastEventId(event)
    scheduleResult()
  }

  function handleError() {
    const closedState = typeof EventSourceClass.CLOSED === 'number' ? EventSourceClass.CLOSED : CLOSED
    // While CONNECTING, the browser reconnects by itself (for example at the end of the 5 minutes of a connection)
    // with the Last-Event-ID header, so nothing is done here to not open a second connection
    if (source === null || source.readyState !== closedState) {
      return
    }
    closeSource()
    closedByError = true
    clearReopenTimer()
    reopenTimer = setTimeout(() => {
      reopenTimer = null
      open()
    }, reopenDelayMs(attempt))
    attempt++
  }

  function open() {
    if (stopped || source !== null || isHidden()) {
      return
    }
    openedAt = Date.now()
    source = new EventSourceClass(urlWithLastEventId(url, lastEventId))
    source.addEventListener('open', handleOpen)
    source.addEventListener('message', rememberLastEventId)
    source.addEventListener('result', handleResult)
    source.addEventListener('error', handleError)
  }

  const handleVisibilityChange = () => {
    if (stopped) {
      return
    }
    if (isHidden()) {
      clearReopenTimer()
      clearResultTimer()
      closeSource()
      return
    }
    if (source !== null) {
      return
    }
    clearReopenTimer()
    closedByError = false
    open()
    if (refreshOnVisible) {
      // Merged with the event that the server sends right away when results arrived while the tab was hidden
      scheduleResult()
    }
  }

  const stop = () => {
    stopped = true
    clearReopenTimer()
    clearResultTimer()
    closeSource()
    if (doc) {
      doc.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }

  const notifyRefreshSucceeded = () => {
    if (stopped || !closedByError || source !== null || isHidden() || Date.now() - openedAt < REOPEN_DELAYS_MS[0]) {
      return
    }
    clearReopenTimer()
    closedByError = false
    open()
  }

  if (doc) {
    doc.addEventListener('visibilitychange', handleVisibilityChange)
  }
  open()

  return { stop, notifyRefreshSucceeded }
}
