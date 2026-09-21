// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Fork: toast messages of the administration, shown by components/admin/AdminToasts.vue.
//
// - Types success, info, warning and error (an unknown type is info).
// - A toast is dismissed after 5 s (success and info), 8 s (warning) or 10 s (error), and duration 0 keeps it until it
//   is dismissed.
// - At most MAXIMUM_TOASTS at the same time: the oldest ones are removed with their timers.
// - pauseToasts freezes the remaining time of every toast, and resumeToasts starts counting again from it.
import { reactive } from 'vue'

// How long a toast stays, by type, in milliseconds
export const TOAST_DURATIONS = Object.freeze({ success: 5000, info: 5000, warning: 8000, error: 10000 })

export const MAXIMUM_TOASTS = 4

// Visible toasts, in chronological order
export const toasts = reactive([])

// Text of the last error (alert) and of the last other message (status), for the live regions. The id changes with
// every message, so that the same text is announced again
export const announcements = reactive({
  status: { id: 0, text: '' },
  alert: { id: 0, text: '' }
})

let nextId = 1
let paused = false
// Timers by id: { handle, remaining, startedAt }
const timers = new Map()

const startTimer = (id, timer) => {
  timer.startedAt = Date.now()
  timer.handle = setTimeout(() => dismissToast(id), timer.remaining)
}

const stopTimer = (timer) => {
  if (timer.handle !== null) {
    clearTimeout(timer.handle)
    timer.remaining = Math.max(0, timer.remaining - (Date.now() - timer.startedAt))
    timer.handle = null
  }
}

// dismissToast removes the toast with the given id and its timer
export function dismissToast(id) {
  const index = toasts.findIndex((item) => item.id === id)
  if (index !== -1) {
    toasts.splice(index, 1)
  }
  const timer = timers.get(id)
  if (timer) {
    if (timer.handle !== null) {
      clearTimeout(timer.handle)
    }
    timers.delete(id)
  }
}

// clearToasts removes every toast, their timers and the text of the live regions
export function clearToasts() {
  for (const timer of timers.values()) {
    if (timer.handle !== null) {
      clearTimeout(timer.handle)
    }
  }
  timers.clear()
  toasts.splice(0, toasts.length)
  announcements.status = { id: 0, text: '' }
  announcements.alert = { id: 0, text: '' }
}

// pauseToasts freezes the remaining time of every toast
export function pauseToasts() {
  if (paused) {
    return
  }
  paused = true
  for (const timer of timers.values()) {
    stopTimer(timer)
  }
}

// resumeToasts counts the remaining time of every toast again
export function resumeToasts() {
  if (!paused) {
    return
  }
  paused = false
  for (const [id, timer] of timers) {
    startTimer(id, timer)
  }
}

export const toastsPaused = () => paused

// showToast shows a message with an optional title and duration in milliseconds, and returns its id
export function showToast(type, message, options = {}) {
  const toastType = Object.prototype.hasOwnProperty.call(TOAST_DURATIONS, type) ? type : 'info'
  const id = nextId++
  const title = options.title ? String(options.title) : ''
  const text = String(message || '')
  toasts.push({ id, type: toastType, title, message: text })
  while (toasts.length > MAXIMUM_TOASTS) {
    dismissToast(toasts[0].id)
  }
  const announcement = { id, text: title ? `${title}: ${text}` : text }
  if (toastType === 'error') {
    announcements.alert = announcement
  } else {
    announcements.status = announcement
  }
  const duration = options.duration === undefined || options.duration === null ? TOAST_DURATIONS[toastType] : Number(options.duration)
  if (duration > 0) {
    const timer = { handle: null, remaining: duration, startedAt: 0 }
    timers.set(id, timer)
    if (!paused) {
      startTimer(id, timer)
    }
  }
  return id
}

export const toast = {
  success: (message, options) => showToast('success', message, options),
  info: (message, options) => showToast('info', message, options),
  warning: (message, options) => showToast('warning', message, options),
  error: (message, options) => showToast('error', message, options)
}

// pendingToastTimers returns how many toasts have a timer (for tests)
export const pendingToastTimers = () => timers.size
