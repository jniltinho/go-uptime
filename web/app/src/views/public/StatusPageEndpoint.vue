<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="container mx-auto px-4 py-8 max-w-5xl">
    <RouterLink
      v-if="validAddress"
      :to="{ name: 'PublicStatusPage', params: { slug } }"
      class="mb-4 inline-flex h-9 items-center gap-2 px-3 text-sm font-medium hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:hover:bg-gray-800"
      data-testid="status-endpoint-back"
    >
      <ArrowLeft class="h-4 w-4" aria-hidden="true" />
      Back to {{ details ? details.page.title : 'the status page' }}
    </RouterLink>

    <div v-if="state === 'loading'" class="py-16 flex justify-center"><Loading /></div>

    <section v-else-if="state === 'not-found'" class="py-16 text-center" data-testid="status-page-not-found">
      <h1 class="text-2xl font-bold tracking-tight">Page not found</h1>
      <p class="mt-2 text-muted-foreground">Check the address of the status page.</p>
    </section>

    <!-- Fork: the page asks for a login of its own, and the browser only asks for it again on a new navigation -->
    <section v-else-if="state === 'unauthorized'" class="py-16 text-center" data-testid="status-page-unauthorized">
      <h1 class="text-2xl font-bold tracking-tight">Login required</h1>
      <p class="mt-2 text-muted-foreground">This status page asks for a username and a password.</p>
      <button type="button" class="mt-4 border px-4 py-2 text-sm font-medium hover:bg-muted dark:border-gray-700" @click="reload">Reload the page</button>
    </section>

    <template v-else>
      <div
        v-if="errorMessage"
        role="alert"
        data-testid="status-page-error"
        :class="['mb-4 border px-4 py-3 text-sm', details ? 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100' : 'border-red-300 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200']"
      >
        {{ errorMessage }}
      </div>

      <div v-if="details" class="space-y-6" data-testid="status-endpoint-details">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            <!-- Fork: same header as the details page of the dashboard -->
            <h1 class="text-2xl font-semibold tracking-tight break-words" data-testid="status-endpoint-name">{{ details.name }}</h1>
            <div class="mt-1 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
              <span v-if="details.group">Group: {{ details.group }}</span>
              <span v-if="details.group">•</span>
              <span>Updated {{ relativeTimeLabel(details.updatedAt, now) }}</span>
            </div>
            <!-- Fork: expiration of the TLS certificate, when the page shows it, with the date like the dashboard -->
            <p v-if="certificateDays !== null" :class="['mt-1 text-xs', certificateClass(certificateDays)]" data-testid="status-endpoint-certificate">
              {{ certificateText(certificateDays) }}<template v-if="certificateExpiresAt"> · {{ certificateExpiresAt }}</template>
            </p>
          </div>
          <StatusBadge :status="healthStatus" />
        </div>

        <!-- Fork: same order as the monitor page of the Uptime Kuma and the endpoint details page of the dashboard -->
        <Card data-testid="status-endpoint-recent-checks">
          <CardHeader>
            <CardTitle>Recent Checks</CardTitle>
          </CardHeader>
          <CardContent>
            <ul>
              <EndpointRow :endpoint="details" :group="details.group" :bars="bars" :show-header="false" />
            </ul>
          </CardContent>
        </Card>

        <!-- Fork: same panel of numbers as the dashboard, with the uptimes and the averages of the payload -->
        <DetailsSummary
          :current-response-time="lastResult ? lastResult.durationMs : null"
          :uptime="details.uptime"
          :response-time="details.responseTime"
        />

        <Card v-if="hasResponseTimes" data-testid="status-endpoint-chart">
          <CardHeader>
            <div class="flex items-center justify-between gap-4">
              <CardTitle>Response Time Trend</CardTitle>
              <select
                v-model="chartPeriod"
                aria-label="Period of the response time chart"
                class="border border-input bg-background px-3 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-ring dark:border-gray-700"
                data-testid="status-endpoint-chart-duration"
              >
                <option v-for="option in CHART_PERIOD_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </div>
          </CardHeader>
          <CardContent>
            <ResponseTimeChart :key="key" :chart-url="chartUrl" :period="chartPeriod" :refresh-key="lastResult ? lastResult.timestamp : null" public-route />
          </CardContent>
        </Card>

        <Card v-if="results.length > 0" data-testid="status-endpoint-checks-table">
          <div class="p-6">
            <RecentChecksTable :results="results" :show-message="showMessages" sanitized />
          </div>
        </Card>

        <div v-if="hasResponseTimes" class="grid gap-4 md:grid-cols-2 lg:grid-cols-4" data-testid="details-badges">
          <Card v-for="period in BADGE_PERIODS" :key="period.value">
            <CardHeader class="pb-2">
              <CardTitle class="text-sm font-medium text-muted-foreground text-center">{{ period.label }}</CardTitle>
            </CardHeader>
            <CardContent>
              <img :src="badgeURL(`response-times/${period.value}/badge.svg`)" :alt="`Average response time over the ${period.label.toLowerCase()}`" class="mx-auto mt-2" />
            </CardContent>
          </Card>
        </div>

        <Card data-testid="details-health">
          <CardHeader>
            <CardTitle>Current Health</CardTitle>
          </CardHeader>
          <CardContent>
            <div class="text-center">
              <img :src="badgeURL('health/badge.svg')" alt="Current health" class="mx-auto" />
            </div>
          </CardContent>
        </Card>

        <!-- Fork: the events are collapsed by default, like the Checks table -->
        <Card v-if="events.length > 0" data-testid="status-endpoint-events">
          <CardContent class="pt-6">
            <EventsTimeline :items="eventItems" />
          </CardContent>
        </Card>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import Loading from '@/components/Loading.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import ResponseTimeChart from '@/components/ResponseTimeChart.vue'
import EndpointRow from '@/components/public/EndpointRow.vue'
import RecentChecksTable from '@/components/RecentChecksTable.vue'
import EventsTimeline from '@/components/EventsTimeline.vue'
import DetailsSummary from '@/components/DetailsSummary.vue'
import { describeEvents, formatDateTime, relativeTimeLabel, SLUG_PATTERN } from '@/utils/statusPage'
import { CHART_PERIOD_OPTIONS, readStoredPeriod, storePeriod } from '@/utils/responseTimeChart'
import { certificateClass, certificateDate, certificateText } from '@/utils/certificate'
import { watchEndpointResults } from '@/utils/liveUpdates'

const REFRESH_INTERVAL_MS = 60000
const CLOCK_INTERVAL_MS = 10000
const MAXIMUM_KEY_LENGTH = 400

// Periods of the badges, like the endpoint details page of the dashboard
const BADGE_PERIODS = [
  { value: '30d', label: 'Last 30 days' },
  { value: '7d', label: 'Last 7 days' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '1h', label: 'Last hour' }
]

const route = useRoute()

const details = ref(null)
// loading, ready (with or without details, see errorMessage) or not-found
const state = ref('loading')
const errorMessage = ref('')
const now = ref(Date.now())
// Period of the response time chart of Uptime Kuma, remembered in the browser
const chartPeriod = ref(readStoredPeriod())
const narrowScreen = window.matchMedia('(max-width: 639px)')
const bars = ref(narrowScreen.matches ? 25 : 50)

let refreshTimer = null
let clockTimer = null
let abortController = null
let requestGeneration = 0
// Fork: real-time channel of the endpoint, see utils/liveUpdates.js
let liveUpdates = null

const slug = computed(() => String(route.params.slug || ''))
const key = computed(() => String(route.params.key || ''))
const validAddress = computed(() => SLUG_PATTERN.test(slug.value) && key.value.length > 0 && key.value.length <= MAXIMUM_KEY_LENGTH)

const results = computed(() => (details.value && details.value.results) || [])
const lastResult = computed(() => (results.value.length > 0 ? results.value[results.value.length - 1] : null))
const events = computed(() => (details.value ? describeEvents(details.value.events || []) : []))
// Fork: items of the collapsible events, see components/EventsTimeline.vue
const eventItems = computed(() => events.value.map((event) => ({ key: `${event.type}-${event.timestamp}`, type: event.type, text: event.text, dateTime: formatDateTime(event.timestamp), timeAgo: event.timeAgo })))
// Days until the TLS certificate expires, only published when the page shows it (fork)
const certificateDays = computed(() => (details.value && Number.isInteger(details.value.certificateExpiresInDays) ? details.value.certificateExpiresInDays : null))
// Fork: the details page shows the date next to the days, like the dashboard; the rows of the lists show only the days
const certificateExpiresAt = computed(() => (details.value && details.value.certificateExpiresAt ? certificateDate(details.value.certificateExpiresAt) : ''))
// Like the dashboard, which shows the chart as soon as a result has a duration: results faster than 1 ms have a
// durationMs of 0 in the public payload, but still have points in the chart
const hasResponseTimes = computed(() => results.value.length > 0)

const healthStatus = computed(() => ({ up: 'healthy', pending: 'pending', down: 'unhealthy' }[details.value?.status] || 'unknown'))
// With show-messages, the table of checks has the same columns as the dashboard, even without any message (fork)
const showMessages = computed(() => details.value?.page?.showMessages === true)

// Public route of the response time chart
const chartUrl = computed(() => `/api/v1/status-pages/${encodeURIComponent(slug.value)}/endpoints/${encodeURIComponent(key.value)}/response-time-chart`)

// badgeURL returns the address of a badge of the endpoint under the page, which follows the login of the page (fork).
// The routes by key of the original Gatus, which the dashboard uses, stay public.
const badgeURL = (path) => `/api/v1/status-pages/${encodeURIComponent(slug.value)}/endpoints/${encodeURIComponent(key.value)}/${path}`

const stopRefreshing = () => {
  clearTimeout(refreshTimer)
  refreshTimer = null
}

const scheduleRefresh = (delayMs) => {
  stopRefreshing()
  if (document.visibilityState !== 'hidden') {
    refreshTimer = setTimeout(load, delayMs)
  }
}

const stopLiveUpdates = () => {
  liveUpdates?.stop()
  liveUpdates = null
}

// startLiveUpdates opens the real-time channel of the endpoint of the route, closing the previous one. Each notification
// refreshes the page silently, and the page already refreshes when the tab is shown again.
const startLiveUpdates = () => {
  stopLiveUpdates()
  if (!validAddress.value) {
    return
  }
  liveUpdates = watchEndpointResults(
    `/api/v1/status-pages/${encodeURIComponent(slug.value)}/endpoints/${encodeURIComponent(key.value)}/events`,
    () => load(),
    { refreshOnVisible: false }
  )
}

// Fork: a page with a login of its own answers 401 when the credential changes with the tab open
const showUnauthorized = () => {
  stopRefreshing()
  stopLiveUpdates()
  details.value = null
  errorMessage.value = ''
  state.value = 'unauthorized'
  document.title = 'Login required'
}

const reload = () => window.location.reload()

const showNotFound = () => {
  stopRefreshing()
  stopLiveUpdates()
  details.value = null
  errorMessage.value = ''
  state.value = 'not-found'
  document.title = 'Page not found'
}

const load = async () => {
  const generation = ++requestGeneration
  abortController?.abort()
  if (!validAddress.value) {
    showNotFound()
    return
  }
  abortController = new AbortController()
  let retryDelayMs = REFRESH_INTERVAL_MS
  try {
    // Fork: the credential of a page with a login goes with every request of the page
    const response = await fetch(`/api/v1/status-pages/${encodeURIComponent(slug.value)}/endpoints/${encodeURIComponent(key.value)}`, { credentials: 'same-origin', signal: abortController.signal })
    if (generation !== requestGeneration) {
      return
    }
    if (response.status === 404) {
      showNotFound()
      return
    }
    if (response.status === 401) {
      showUnauthorized()
      return
    }
    if (response.status === 429 || response.status === 503) {
      const retryAfterSeconds = Number(response.headers.get('Retry-After'))
      if (retryAfterSeconds > REFRESH_INTERVAL_MS / 1000) {
        retryDelayMs = retryAfterSeconds * 1000
      }
      errorMessage.value = response.status === 429
        ? 'Too many requests right now. This page will refresh automatically.'
        : 'This page is temporarily unavailable. It will refresh automatically.'
    } else if (!response.ok || !(response.headers.get('Content-Type') || '').includes('application/json')) {
      errorMessage.value = 'Could not load the details of the service. It will refresh automatically.'
    } else {
      const data = await response.json()
      if (generation !== requestGeneration) {
        return
      }
      details.value = data
      errorMessage.value = ''
      liveUpdates?.notifyRefreshSucceeded()
      document.title = `${details.value.name} · ${details.value.page.title}`
    }
  } catch (error) {
    if (error.name === 'AbortError' || generation !== requestGeneration) {
      return
    }
    errorMessage.value = 'Could not reach the server. This page will refresh automatically.'
  }
  now.value = Date.now()
  state.value = 'ready'
  scheduleRefresh(retryDelayMs)
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'hidden') {
    stopRefreshing()
  } else if (state.value !== 'not-found' && state.value !== 'unauthorized') {
    load()
  }
}

const handleScreenChange = (event) => {
  bars.value = event.matches ? 25 : 50
}

watch(chartPeriod, (period) => {
  storePeriod(period)
})

watch([slug, key], () => {
  details.value = null
  errorMessage.value = ''
  state.value = 'loading'
  startLiveUpdates()
  load()
})

onMounted(() => {
  startLiveUpdates()
  load()
  clockTimer = setInterval(() => { now.value = Date.now() }, CLOCK_INTERVAL_MS)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  narrowScreen.addEventListener('change', handleScreenChange)
})

onUnmounted(() => {
  stopRefreshing()
  stopLiveUpdates()
  clearInterval(clockTimer)
  abortController?.abort()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  narrowScreen.removeEventListener('change', handleScreenChange)
})
</script>
