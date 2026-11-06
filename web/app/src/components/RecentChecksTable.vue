<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div>
    <button
      type="button"
      :aria-expanded="expanded"
      data-testid="recent-checks-toggle"
      class="flex w-full items-center gap-2 border px-3 py-2 text-left text-sm font-medium text-fg hover:bg-hover border-line"
      @click="toggle"
    >
      <ChevronDown v-if="expanded" class="h-4 w-4" />
      <ChevronRight v-else class="h-4 w-4" />
      Checks table
      <span class="font-normal text-fg-muted">({{ rows.length }})</span>
      <span class="ml-auto text-xs font-normal text-fg-muted">{{
        showMessage ? 'Status, date and time, message and origin' : 'Status, date and time and response time'
      }}</span>
    </button>
    <div v-if="expanded" class="overflow-x-auto border border-t-0 border-line" data-testid="recent-checks-table">
      <table class="w-full text-sm">
        <thead class="bg-surface-2 text-left text-fg-muted">
          <tr>
            <th class="px-3 py-2 font-medium">Status</th>
            <th class="px-3 py-2 font-medium">Date and time</th>
            <template v-if="showMessage">
              <th class="px-3 py-2 font-medium">Message</th>
              <th class="px-3 py-2 font-medium">Origin</th>
            </template>
            <th v-else class="px-3 py-2 font-medium">Response time</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="rows.length === 0">
            <td :colspan="showMessage ? 4 : 3" class="px-3 py-6 text-center text-fg-muted">No checks yet.</td>
          </tr>
          <tr
            v-for="(row, index) in rows"
            :key="`${row.timestamp}-${index}`"
            class="border-t border-line"
            :data-testid="`recent-check-${index}`"
          >
            <td class="px-3 py-2">
              <span :class="['border px-1.5 py-0.5 text-xs font-medium', STATUS_CLASSES[row.status]]">{{ STATUS_TEXTS[row.status] }}</span>
            </td>
            <td class="px-3 py-2 whitespace-nowrap text-fg-muted">{{ row.dateTime }}</td>
            <template v-if="showMessage">
              <td class="px-3 py-2 wrap-break-word text-fg" data-testid="recent-check-message">
                {{ row.message }}
              </td>
              <td class="px-3 py-2 whitespace-nowrap">
                <span :class="['text-xs', row.push ? 'text-highlight' : 'text-fg-muted']">{{ row.push ? 'Push' : 'Check' }}</span>
              </td>
            </template>
            <td v-else class="px-3 py-2 whitespace-nowrap text-fg-muted">{{ row.responseTime }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
// Table of the results of the current page, from the most recent to the oldest (fork). The columns follow the table of
// the Uptime Kuma (Status, DateTime, Message), with the origin at the end. On the public status pages without
// show-messages (showMessage false), the results have no message, so the table shows the response time instead. With
// sanitized, the table only shows the message and the origin of the payload, never built from the errors or the HTTP
// status, so that nothing else is published by mistake. It starts collapsed, and the choice of the viewer is
// remembered in the browser.
import { computed, ref } from 'vue'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import { readPreference, writePreference } from '@/utils/storage'

const props = defineProps({
  results: { type: Array, default: () => [] },
  showMessage: { type: Boolean, default: true },
  sanitized: { type: Boolean, default: false },
})

// Pending results are not successful, but are shown in yellow
const STATUS_TEXTS = { up: 'Up', pending: 'Pending', down: 'Down' }
const STATUS_CLASSES = {
  up: 'border-success bg-success-soft text-success-strong',
  pending: 'border-pending bg-pending-soft text-pending-strong',
  down: 'border-danger bg-danger-soft text-danger-strong',
}

const PREFERENCE = 'show-recent-checks-table'

const readExpanded = () => {
  try {
    return readPreference(PREFERENCE) === 'true'
  } catch (e) {
    return false
  }
}

const expanded = ref(readExpanded())

const toggle = () => {
  expanded.value = !expanded.value
  try {
    writePreference(PREFERENCE, expanded.value ? 'true' : 'false')
  } catch (e) {
    // The table keeps working without the browser storage
  }
}

// The message of a push; otherwise the errors; otherwise the HTTP status of the check. Sanitized: only the message
const messageOf = (result) => {
  if (props.sanitized) {
    return typeof result.message === 'string' ? result.message : ''
  }
  if (result.message) {
    return result.message
  }
  if (result.errors && result.errors.length) {
    return result.errors.join('; ')
  }
  return result.status ? `HTTP ${result.status}` : ''
}

// Public results have durationMs; the results of the dashboard have duration in nanoseconds
const responseTimeOf = (result) => {
  const milliseconds = result.durationMs !== undefined ? result.durationMs : Math.trunc((result.duration || 0) / 1000000)
  return milliseconds > 0 ? `${milliseconds}ms` : '—'
}

const rows = computed(() =>
  [...props.results].reverse().map((result) => ({
    timestamp: result.timestamp,
    status: result.success ? 'up' : result.pending ? 'pending' : 'down',
    dateTime: new Date(result.timestamp).toLocaleString(),
    push: result.origin === 'push',
    message: props.showMessage ? messageOf(result) : '',
    responseTime: responseTimeOf(result),
  })),
)
</script>
