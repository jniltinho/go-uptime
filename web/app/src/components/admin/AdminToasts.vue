<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: toast messages of the administration, see utils/toast.js. Centered, at the top on larger screens and below
       the header of the app on phones, above the dialogs, and only the toasts take clicks -->
  <div class="pointer-events-none fixed left-1/2 top-14 z-70 w-[calc(100%-2rem)] -translate-x-1/2 sm:w-md md:top-3" data-testid="toasts">
    <!-- Live regions, always present before the messages: the visual stack below has no role -->
    <div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="toast-status-region">
      <span v-if="announcements.status.text" :key="announcements.status.id">{{ announcements.status.text }}</span>
    </div>
    <div class="sr-only" role="alert" aria-live="assertive" aria-atomic="true" data-testid="toast-alert-region">
      <span v-if="announcements.alert.text" :key="announcements.alert.id">{{ announcements.alert.text }}</span>
    </div>

    <div
      ref="list"
      class="flex flex-col gap-2"
      @mouseover="hovered = true"
      @mouseout="onMouseout"
      @focusin="focused = true"
      @focusout="onFocusout"
    >
      <div
        v-for="item in toasts"
        :key="item.id"
        :class="[
          'pointer-events-auto flex w-full items-start gap-3 border border-l-4 bg-surface px-4 py-3 text-sm text-card-foreground shadow-lg',
          STYLES[item.type],
        ]"
        data-testid="toast"
        :data-type="item.type"
      >
        <component :is="ICONS[item.type]" class="mt-0.5 h-4 w-4 shrink-0" :class="ICON_COLORS[item.type]" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p v-if="item.title" class="font-medium text-fg-strong" data-testid="toast-title">{{ item.title }}</p>
          <p class="wrap-break-word text-fg" data-testid="toast-message">{{ item.message }}</p>
        </div>
        <button
          type="button"
          class="-mr-1 shrink-0 p-0.5 text-fg-muted hover:text-fg-strong"
          aria-label="Dismiss"
          data-testid="toast-dismiss"
          @click="dismissToast(item.id)"
        >
          <X class="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from 'lucide-vue-next'
import { dialogOpen } from '@/utils/dialogStack'
import { announcements, dismissToast, pauseToasts, resumeToasts, toasts } from '@/utils/toast'

const ICONS = { success: CheckCircle2, info: Info, warning: AlertTriangle, error: AlertCircle }

const STYLES = {
  success: 'border-line border-l-success',
  info: 'border-line border-l-info',
  warning: 'border-line border-l-warning',
  error: 'border-line border-l-danger',
}

const ICON_COLORS = {
  success: 'text-success',
  info: 'text-info',
  warning: 'text-warning',
  error: 'text-danger',
}

const list = ref(null)
// The pointer is over a toast, or the focus is inside one (WCAG 2.2.1)
const hovered = ref(false)
const focused = ref(false)

const updateHover = () => {
  hovered.value = Boolean(list.value && list.value.querySelector('[data-testid="toast"]:hover'))
}

const onMouseout = (event) => {
  if (
    !event.relatedTarget ||
    !list.value ||
    !list.value.contains(event.relatedTarget) ||
    !event.relatedTarget.closest('[data-testid="toast"]')
  ) {
    hovered.value = false
  }
}

const onFocusout = (event) => {
  focused.value = Boolean(list.value && event.relatedTarget && list.value.contains(event.relatedTarget))
}

// A dismissed toast leaves the list without mouseout or focusout events
watch(
  () => toasts.length,
  async () => {
    await nextTick()
    updateHover()
    focused.value = Boolean(list.value && list.value.contains(document.activeElement))
  },
)

// The timers are paused while the pointer or the focus is on the toasts, and while a dialog is open: the dismiss
// buttons are outside of the focus trap of the dialog
const shouldPause = computed(() => hovered.value || focused.value || dialogOpen.value)

watch(
  shouldPause,
  (pause) => {
    if (pause) {
      pauseToasts()
    } else {
      resumeToasts()
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  resumeToasts()
})
</script>
