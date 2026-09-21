<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <li :class="featured ? 'border bg-card p-3 dark:border-gray-800' : 'py-2'" :data-testid="`status-endpoint-${endpoint.name}`">
    <div v-if="showHeader" class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex items-center gap-2 min-w-0">
        <span :class="['inline-block h-2.5 w-2.5 rounded-full flex-shrink-0', dotClass]" aria-hidden="true"></span>
        <component
          :is="slug ? RouterLink : 'span'"
          :to="slug ? detailsRoute : undefined"
          :class="['truncate', featured ? 'text-lg font-semibold' : 'font-medium', slug ? 'underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring' : '']"
          :title="endpoint.name"
          :data-testid="slug ? `status-endpoint-link-${endpoint.name}` : undefined"
        >{{ endpoint.name }}</component>
        <span :class="['text-xs', statusTextClass]" aria-hidden="true">{{ statusLabel }}</span>
        <span v-if="featured && group" class="truncate text-xs text-muted-foreground" :title="group">{{ group }}</span>
      </div>
      <!-- Fork: from sm on, the uptimes become fixed columns aligned with the ones of the group header -->
      <dl v-if="!featured" class="flex shrink-0 gap-4 text-xs text-muted-foreground sm:grid sm:grid-cols-3 sm:gap-0" aria-hidden="true">
        <div v-for="period in periods" :key="period.key" class="flex gap-1 sm:w-16 sm:justify-end">
          <dt class="sm:hidden">{{ period.label }}</dt>
          <dd class="font-medium text-foreground">{{ formatUptime(endpoint.uptime[period.key]) }}</dd>
        </div>
      </dl>
    </div>
    <!-- Fork: expiration of the TLS certificate, when the page shows it -->
    <p v-if="showHeader && certificateDays !== null" :class="['mt-0.5 text-xs', certificateClass(certificateDays)]" :data-testid="`status-endpoint-certificate-${endpoint.name}`">
      {{ certificateText(certificateDays) }}
    </p>
    <table v-if="featured" class="mt-2 w-full text-sm" data-testid="status-featured-stats">
      <thead>
        <tr class="text-xs text-muted-foreground">
          <th scope="col" class="py-0.5 pr-2 text-left font-normal"><span class="sr-only">Metric</span></th>
          <th v-for="period in periods" :key="`period-${period.key}`" scope="col" class="py-0.5 pl-2 text-right font-normal">{{ period.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <th scope="row" class="py-0.5 pr-2 text-left text-xs font-normal text-muted-foreground">Uptime</th>
          <td v-for="period in periods" :key="`uptime-${period.key}`" class="py-0.5 pl-2 text-right font-medium">{{ formatUptime(endpoint.uptime[period.key]) }}</td>
        </tr>
        <tr>
          <th scope="row" class="py-0.5 pr-2 text-left text-xs font-normal text-muted-foreground">Avg response</th>
          <td v-for="period in periods" :key="`response-time-${period.key}`" class="py-0.5 pl-2 text-right font-medium">{{ formatMilliseconds(responseTime[period.key]) }}</td>
        </tr>
      </tbody>
    </table>
    <div v-if="featured" class="mt-1 flex items-center justify-between gap-4 text-xs text-muted-foreground">
      <p>Last response <span class="font-medium text-foreground">{{ formatMilliseconds(lastResult ? lastResult.durationMs : null) }}</span></p>
      <RouterLink
        v-if="slug"
        :to="detailsRoute"
        class="font-medium text-foreground underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        :data-testid="`status-endpoint-details-${endpoint.name}`"
      >View details<span class="sr-only"> of {{ endpoint.name }}</span></RouterLink>
    </div>
    <p class="sr-only">{{ accessibleSummary }}</p>
    <div
      :class="['relative flex gap-px outline-none focus-visible:ring-2 focus-visible:ring-ring', showHeader && !featured ? 'mt-1.5' : 'mt-2']"
      role="group"
      tabindex="0"
      :aria-label="`Check history of ${endpoint.name}. Use the arrow keys to browse it.`"
      @keydown="handleKeydown"
      @mouseleave="hoveredIndex = null"
      @blur="selectedIndex = null"
    >
      <span
        v-for="(result, index) in displayedResults"
        :key="index"
        aria-hidden="true"
        :class="['h-5 flex-1', barClass(result, index)]"
        @mouseenter="result && (hoveredIndex = index)"
        @click="result && selectBar(index)"
      ></span>
      <!-- Fork: the detail floats over the bars instead of taking a line of its own. It is anchored by the edge that is
           closer to the active bar, with the max-width closing the other end, so that it can never leave the row -->
      <span
        v-if="activeResult"
        aria-hidden="true"
        :class="['pointer-events-none absolute z-10 w-max border bg-card px-2 py-1 text-xs text-muted-foreground shadow-sm dark:border-gray-700', tooltipAbove ? 'bottom-full mb-1' : 'top-full mt-1']"
        :style="tooltipStyle"
        data-testid="status-endpoint-detail"
      >
        {{ activeResultText }}
      </span>
    </div>
    <!-- Fork: on the details page the ends of the history are labelled, like the card of the dashboard -->
    <div v-if="!showHeader" class="mt-1 flex items-center justify-between text-xs text-muted-foreground">
      <span>{{ oldestResultTime }}</span>
      <span>{{ newestResultTime }}</span>
    </div>
    <!-- The live region stays on the page even without an active result: one that only appears with the text announces nothing -->
    <span class="sr-only" aria-live="polite" aria-atomic="true">{{ activeResultText }}</span>
  </li>
</template>

<script setup>
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { endpointKey, formatDateTime, formatMilliseconds, formatUptime, STATUS_LABELS } from '@/utils/statusPage'
import { generatePrettyTimeAgo } from '@/utils/time'
import { certificateClass, certificateText } from '@/utils/certificate'

const props = defineProps({
  endpoint: { type: Object, required: true },
  bars: { type: Number, default: 50 },
  // Featured endpoints are shown as a card, with more details
  featured: { type: Boolean, default: false },
  group: { type: String, default: '' },
  // Slug of the status page: when set, the name of the endpoint links to its details page
  slug: { type: String, default: '' },
  // The details page shows the bars without the name and the uptimes, which it already shows
  showHeader: { type: Boolean, default: true }
})

const periods = [
  { key: '24h', label: '24h' },
  { key: '7d', label: '7d' },
  { key: '30d', label: '30d' }
]

const hoveredIndex = ref(null)
const selectedIndex = ref(null)

const detailsRoute = computed(() => ({
  name: 'PublicStatusPageEndpoint',
  params: { slug: props.slug, key: endpointKey(props.group, props.endpoint.name) }
}))

const displayedResults = computed(() => {
  const results = (props.endpoint.results || []).slice(-props.bars)
  return [...Array(props.bars - results.length).fill(null), ...results]
})

const lastResult = computed(() => {
  const results = props.endpoint.results || []
  return results.length > 0 ? results[results.length - 1] : null
})

const firstResult = computed(() => {
  const results = props.endpoint.results || []
  return results.length > 0 ? results[0] : null
})

const oldestResultTime = computed(() => (firstResult.value ? generatePrettyTimeAgo(firstResult.value.timestamp) : ''))
const newestResultTime = computed(() => (lastResult.value ? generatePrettyTimeAgo(lastResult.value.timestamp) : ''))

const responseTime = computed(() => props.endpoint.responseTime || {})

// Days until the TLS certificate expires, only published when the page shows it (fork)
const certificateDays = computed(() => (Number.isInteger(props.endpoint.certificateExpiresInDays) ? props.endpoint.certificateExpiresInDays : null))

const activeIndex = computed(() => (hoveredIndex.value !== null ? hoveredIndex.value : selectedIndex.value))
const activeResult = computed(() => (activeIndex.value !== null ? displayedResults.value[activeIndex.value] : null))

const activeResultText = computed(() => {
  const result = activeResult.value
  if (!result) {
    return ''
  }
  const state = result.success ? 'Success' : (result.pending ? 'Pending' : 'Failure')
  return `${formatDateTime(result.timestamp)} · ${state} · ${result.durationMs} ms`
})

// In a row of a group the only thing above the bars is the space between rows; in the featured card and on the details
// page there is content there, so the tooltip goes below
const tooltipAbove = computed(() => props.showHeader && !props.featured)

// The tooltip is anchored by the edge closer to the active bar, and the max-width closes the other end: no measuring,
// and it can never leave the row
const tooltipStyle = computed(() => {
  const total = displayedResults.value.length
  if (activeIndex.value === null || total === 0) {
    return {}
  }
  const barWidth = 100 / total
  const start = activeIndex.value * barWidth
  if (start + barWidth / 2 <= 50) {
    return { left: `${start}%`, maxWidth: `calc(100% - ${start}%)` }
  }
  const end = 100 - (start + barWidth)
  return { right: `${end}%`, maxWidth: `calc(100% - ${end}%)` }
})

const statusLabel = computed(() => STATUS_LABELS[props.endpoint.status]?.endpoint || STATUS_LABELS.unknown.endpoint)

const dotClass = computed(() => ({ up: 'bg-green-500', pending: 'bg-yellow-400', down: 'bg-red-500' }[props.endpoint.status] || 'bg-statusgray-400'))

const statusTextClass = computed(() => ({
  up: 'text-green-700 dark:text-green-400',
  pending: 'text-yellow-700 dark:text-yellow-400',
  down: 'text-red-700 dark:text-red-400'
}[props.endpoint.status] || 'text-muted-foreground'))

const accessibleSummary = computed(() => {
  const results = props.endpoint.results || []
  const successes = results.filter((result) => result.success).length
  return `${props.endpoint.name}: ${statusLabel.value}, ${successes} of ${results.length} checks successful, 24-hour uptime ${formatUptime(props.endpoint.uptime['24h'])}, 24-hour average response time ${formatMilliseconds(responseTime.value['24h'])}`
})

const barClass = (result, index) => {
  if (!result) {
    return 'bg-gray-200 dark:bg-gray-800'
  }
  const active = activeIndex.value === index
  if (result.success) {
    return active ? 'bg-green-700' : 'bg-green-500'
  }
  // Fork: Pending results are not successful, but are shown in yellow
  if (result.pending) {
    return active ? 'bg-yellow-600' : 'bg-yellow-400'
  }
  return active ? 'bg-red-700' : 'bg-red-500'
}

const firstResultIndex = () => displayedResults.value.findIndex((result) => result !== null)

const selectBar = (index) => {
  selectedIndex.value = selectedIndex.value === index ? null : index
}

const handleKeydown = (event) => {
  const first = firstResultIndex()
  if (first < 0) {
    return
  }
  const last = displayedResults.value.length - 1
  const current = selectedIndex.value === null ? last + 1 : selectedIndex.value
  const moves = {
    ArrowLeft: Math.max(first, current - 1),
    ArrowRight: Math.min(last, current + 1),
    Home: first,
    End: last
  }
  if (event.key === 'Escape') {
    selectedIndex.value = null
    return
  }
  if (event.key in moves) {
    event.preventDefault()
    selectedIndex.value = moves[event.key]
  }
}
</script>
