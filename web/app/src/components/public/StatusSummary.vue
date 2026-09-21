<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div :class="['border px-4 py-3 flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between', bannerClass]" data-testid="status-summary">
    <p role="status" class="font-semibold flex items-center gap-2">
      <span :class="['inline-block h-3 w-3 rounded-full flex-shrink-0', dotClass]" aria-hidden="true"></span>
      {{ label }}
    </p>
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm opacity-80">
      <!-- Fork: how many endpoints are up and down, counted by the server so that a truncated page stays right -->
      <p v-if="counts" data-testid="status-summary-counts">
        <span v-for="(count, index) in counts" :key="count.label">{{ index > 0 ? ' · ' : '' }}{{ count.value }} {{ count.label }}</span>
      </p>
      <p data-testid="status-summary-updated">{{ updatedLabel }}</p>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { relativeTimeLabel, STATUS_LABELS } from '@/utils/statusPage'

const props = defineProps({
  status: { type: String, required: true },
  updatedAt: { type: String, required: true },
  now: { type: Number, required: true },
  // Fork: count of the endpoints of the page by status, from the payload
  summary: { type: Object, default: null }
})

// up and down are always shown, including "0 down"; pending and unknown only when there is any
const counts = computed(() => {
  if (!props.summary || typeof props.summary.total !== 'number') {
    return null
  }
  const counted = [
    { label: 'up', value: props.summary.up || 0 },
    { label: 'down', value: props.summary.down || 0 }
  ]
  if (props.summary.pending) {
    counted.push({ label: 'pending', value: props.summary.pending })
  }
  if (props.summary.unknown) {
    counted.push({ label: 'no data', value: props.summary.unknown })
  }
  return counted
})

const label = computed(() => STATUS_LABELS[props.status]?.page || STATUS_LABELS.unknown.page)

// The Tailwind version of the project has no 950 shade: dark backgrounds use the 900 shade with opacity
const bannerClass = computed(() => ({
  operational: 'border-green-300 bg-green-50 text-green-900 dark:border-green-800 dark:bg-green-900/30 dark:text-green-100',
  degraded: 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100',
  down: 'border-red-300 bg-red-50 text-red-900 dark:border-red-800 dark:bg-red-900/30 dark:text-red-100'
}[props.status] || 'border-statusgray-300 bg-statusgray-50 text-statusgray-800 dark:border-statusgray-700 dark:bg-statusgray-900 dark:text-statusgray-200'))

const dotClass = computed(() => ({
  operational: 'bg-green-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500'
}[props.status] || 'bg-statusgray-400'))

const updatedLabel = computed(() => `Updated ${relativeTimeLabel(props.updatedAt, props.now)}`)
</script>
