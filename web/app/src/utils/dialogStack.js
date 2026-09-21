// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Fork: stack of the open dialogs of the administration (components/admin/AdminDialog.vue).
//
// - Dialogs are pushed when they open and removed by id when they close (never popped: a dialog can close and another
//   open in the same tick).
// - While the stack is not empty, a single capturing listener on the document sends Escape, Tab and focusin to the
//   dialog on top, and the body gets overflow-hidden. Both are removed when the stack is empty again.
import { computed, reactive } from 'vue'

const BODY_CLASS = 'overflow-hidden'

// Ids of the open dialogs, in the order they were opened (the last one is on top)
const stack = reactive([])
// Controllers of the open dialogs by id: { onKeydown(event), onFocusin(event) }
const controllers = new Map()

let nextId = 1
const ids = new Map()

// Whether at least one dialog is open, used by the toasts to pause their timers
export const dialogOpen = computed(() => stack.length > 0)

// nextDialogId returns an incremental id for a dialog
export function nextDialogId() {
  return nextId++
}

// uniqueId returns an incremental id for an element, e.g. uniqueId('confirm-dialog-message') -> confirm-dialog-message-1
export function uniqueId(prefix) {
  const value = (ids.get(prefix) || 0) + 1
  ids.set(prefix, value)
  return `${prefix}-${value}`
}

const currentDocument = () => (typeof document === 'undefined' ? null : document)

const topController = () => (stack.length ? controllers.get(stack[stack.length - 1]) : null)

const onKeydown = (event) => {
  const controller = topController()
  if (controller && controller.onKeydown) {
    controller.onKeydown(event)
  }
}

const onFocusin = (event) => {
  const controller = topController()
  if (controller && controller.onFocusin) {
    controller.onFocusin(event)
  }
}

let listening = null

const startListening = () => {
  const doc = currentDocument()
  if (!doc || listening) {
    return
  }
  doc.addEventListener('keydown', onKeydown, true)
  doc.addEventListener('focusin', onFocusin, true)
  if (doc.body && doc.body.classList) {
    doc.body.classList.add(BODY_CLASS)
  }
  listening = doc
}

const stopListening = () => {
  if (!listening) {
    return
  }
  listening.removeEventListener('keydown', onKeydown, true)
  listening.removeEventListener('focusin', onFocusin, true)
  if (listening.body && listening.body.classList) {
    listening.body.classList.remove(BODY_CLASS)
  }
  listening = null
}

// pushDialog puts an opened dialog on top of the stack
export function pushDialog(id, controller = {}) {
  controllers.set(id, controller)
  if (!stack.includes(id)) {
    stack.push(id)
  }
  startListening()
}

// removeDialog removes a closed dialog, wherever it is in the stack, and returns whether it was on top
export function removeDialog(id) {
  const index = stack.indexOf(id)
  controllers.delete(id)
  if (index === -1) {
    return false
  }
  const wasTop = index === stack.length - 1
  stack.splice(index, 1)
  if (stack.length === 0) {
    stopListening()
  }
  return wasTop
}

// dialogPosition returns the position of the dialog in the stack (0 for the first one), or -1 when it is not open
export function dialogPosition(id) {
  return stack.indexOf(id)
}

export function isTopDialog(id) {
  return stack.length > 0 && stack[stack.length - 1] === id
}

// topDialog returns the controller of the dialog on top, or null
export function topDialog() {
  return topController() || null
}

// openDialogs returns a copy of the ids in the stack (for tests)
export function openDialogs() {
  return [...stack]
}
