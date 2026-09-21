<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: dialog of the administration, teleported to the body in the order it opens, see utils/dialogStack.js -->
  <Teleport v-if="open" to="body">
    <div class="fixed inset-0 flex items-center justify-center bg-black/50 p-4 dark:bg-black/70" :style="{ zIndex: 50 + Math.max(position, 0) }">
      <div
        ref="panel"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        :aria-describedby="describedbyId"
        tabindex="-1"
        :data-testid="testid"
        :class="['flex max-h-[calc(100dvh-2rem)] w-full flex-col border bg-card text-card-foreground shadow-lg outline-none dark:border-gray-700 dark:bg-gray-900', SIZES[size] || SIZES.lg]"
      >
        <div class="flex shrink-0 items-start justify-between gap-4 border-b px-6 py-4 dark:border-gray-700">
          <div class="min-w-0">
            <h2 :id="titleId" class="text-lg font-semibold text-foreground dark:text-gray-100">{{ title }}</h2>
            <p v-if="description" :id="descriptionId" class="mt-0.5 text-sm text-muted-foreground dark:text-gray-400">{{ description }}</p>
          </div>
          <button
            type="button"
            class="-mr-2 shrink-0 p-1 text-muted-foreground hover:text-foreground disabled:opacity-50 dark:text-gray-400 dark:hover:text-gray-100"
            aria-label="Close"
            :disabled="busy"
            :data-testid="`${testid}-x`"
            @click="requestClose"
          >
            <X class="h-5 w-5" aria-hidden="true" />
          </button>
        </div>
        <div class="min-h-0 flex-1 overflow-auto overscroll-contain px-6 py-4">
          <slot />
        </div>
        <div v-if="$slots.footer" class="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-6 py-4 dark:border-gray-700">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import { dialogPosition, isTopDialog, nextDialogId, pushDialog, removeDialog, topDialog } from '@/utils/dialogStack'

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, required: true },
  description: { type: String, default: '' },
  // md, lg or xl
  size: { type: String, default: 'lg' },
  // While busy, Escape and the close button do nothing
  busy: { type: Boolean, default: false },
  testid: { type: String, default: 'admin-dialog' },
  // Returns the element focused when the dialog opens (the panel otherwise)
  initialFocus: { type: Function, default: null },
  // Returns the element focused when the last dialog closes (the element focused before opening, or the h1 of the page,
  // otherwise)
  returnFocus: { type: Function, default: null },
  // Id of an element of the body that describes the dialog, used instead of the description
  describedby: { type: String, default: '' }
})

const emit = defineEmits(['close'])

const SIZES = { md: 'max-w-md', lg: 'max-w-3xl', xl: 'max-w-5xl' }

const FOCUSABLE = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'iframe',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])'
].join(',')

const id = nextDialogId()
const titleId = `admin-dialog-${id}-title`
const descriptionId = `admin-dialog-${id}-description`
const describedbyId = computed(() => props.describedby || (props.description ? descriptionId : undefined))
const position = computed(() => dialogPosition(id))

const panel = ref(null)
let previouslyFocused = null

const isVisible = (element) => element.getClientRects().length > 0

// An element that can take the focus back: in the document, visible and enabled
const isUsable = (element) => Boolean(element && element.isConnected && element !== document.body && !element.disabled && isVisible(element))

const resolve = (getter) => {
  if (!getter) {
    return null
  }
  const value = getter()
  // A ref to a component gives its root element
  return value && value.$el ? value.$el : value
}

const focusables = () => (panel.value ? Array.from(panel.value.querySelectorAll(FOCUSABLE)).filter(isVisible) : [])

const focusPanel = () => {
  if (panel.value) {
    panel.value.focus()
  }
}

const focusInitial = () => {
  if (props.busy) {
    focusPanel()
    return
  }
  const target = resolve(props.initialFocus)
  if (isUsable(target) && panel.value && panel.value.contains(target)) {
    target.focus()
  } else {
    focusPanel()
  }
}

const restoreFocus = () => {
  const candidates = [resolve(props.returnFocus), previouslyFocused]
  const target = candidates.find(isUsable)
  previouslyFocused = null
  if (target) {
    target.focus()
    return
  }
  // The trigger may be gone (e.g. the button of a removed item): the h1 of the page takes the focus
  const heading = document.querySelector('main h1') || document.querySelector('h1')
  if (heading) {
    if (!heading.hasAttribute('tabindex')) {
      heading.setAttribute('tabindex', '-1')
    }
    heading.focus()
  }
}

const requestClose = () => {
  if (!props.busy) {
    emit('close')
  }
}

const onKeydown = (event) => {
  if (event.key === 'Escape' || event.key === 'Esc') {
    event.preventDefault()
    event.stopPropagation()
    requestClose()
    return
  }
  if (event.key !== 'Tab' || !panel.value) {
    return
  }
  const elements = focusables()
  if (elements.length === 0) {
    event.preventDefault()
    focusPanel()
    return
  }
  const first = elements[0]
  const last = elements[elements.length - 1]
  const active = document.activeElement
  const inside = panel.value.contains(active) && active !== panel.value
  if (event.shiftKey && (!inside || active === first)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && (!inside || active === last)) {
    event.preventDefault()
    first.focus()
  }
}

const onFocusin = (event) => {
  if (panel.value && !panel.value.contains(event.target)) {
    focusPanel()
  }
}

const controller = { onKeydown, onFocusin, focusInitial }

let active = false

const activate = async () => {
  if (active) {
    return
  }
  active = true
  const focused = document.activeElement
  previouslyFocused = focused && focused !== document.body ? focused : null
  pushDialog(id, controller)
  await nextTick()
  if (active && isTopDialog(id)) {
    focusInitial()
  }
}

const deactivate = async ({ moveFocus = true } = {}) => {
  if (!active) {
    return
  }
  active = false
  const wasTop = removeDialog(id)
  if (!moveFocus) {
    return
  }
  // After the DOM update: the closed dialog is gone and the page reflects the new state (e.g. enabled buttons)
  await nextTick()
  const top = topDialog()
  if (!top) {
    restoreFocus()
  } else if (wasTop && top.focusInitial) {
    // The focused button was unmounted and focusin does not fire when the focus falls on the body
    top.focusInitial()
  }
}

watch(() => props.open, (open) => {
  if (open) {
    activate()
  } else {
    deactivate()
  }
}, { immediate: true })

// E.g. the Back button of the browser with the dialog open
onBeforeUnmount(() => {
  deactivate({ moveFocus: false })
})
</script>
