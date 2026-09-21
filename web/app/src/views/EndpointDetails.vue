<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="dashboard-container bg-background">
    <div class="container mx-auto px-4 py-4 max-w-7xl">
      <div class="mb-6">
        <Button variant="ghost" size="sm" class="-ml-2 mb-2" @click="goBack">
          <ArrowLeft class="h-4 w-4 mr-2" />
          Back to Dashboard
        </Button>
        
        <div v-if="endpointStatus && endpointStatus.name" class="space-y-6">
          <div class="flex items-start justify-between">
            <div>
              <h1 class="text-2xl font-semibold tracking-tight break-words" data-testid="endpoint-name">{{ endpointStatus.name }}</h1>
              <div class="flex items-center gap-3 text-sm text-muted-foreground mt-1">
                <span v-if="endpointStatus.group">Group: {{ endpointStatus.group }}</span>
                <span v-if="endpointStatus.group && hostname">•</span>
                <span v-if="hostname">{{ hostname }}</span>
              </div>
              <!-- Fork: expiration of the TLS certificate, discreet like the "Cert Exp." of the Uptime Kuma -->
              <p v-if="certificate" :class="['mt-1 text-xs', certificate.className]" data-testid="endpoint-certificate-expiration">
                {{ certificate.text }} · {{ certificate.date }}
              </p>
            </div>
            <StatusBadge :status="currentHealthStatus" />
          </div>

          <!-- Fork: same order as the monitor page of the Uptime Kuma: heartbeat bars, numbers, chart and table of checks -->
          <Card data-testid="recent-checks-card">
            <CardHeader>
              <div class="flex items-center justify-between">
                <CardTitle>Recent Checks</CardTitle>
                <div class="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="icon"
                    @click="toggleShowAverageResponseTime"
                    :title="showAverageResponseTime ? 'Show min-max response time' : 'Show average response time'"
                  >
                    <Activity v-if="showAverageResponseTime" class="h-5 w-5" />
                    <Timer v-else class="h-5 w-5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    @click="fetchData()"
                    title="Refresh data"
                    :disabled="isRefreshing"
                  >
                    <RefreshCw :class="['h-4 w-4', isRefreshing && 'animate-spin']" />
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <!-- Fork: the bars, the numbers and the chart always show the latest results (page 1), whatever the page of the table -->
              <EndpointCard
                v-if="currentStatus"
                compact
                :endpoint="currentStatus"
                :maxResults="resultPageSize"
                :showAverageResponseTime="showAverageResponseTime"
                @showTooltip="showTooltip"
                class="border-0 shadow-none bg-transparent p-0"
              />
            </CardContent>
          </Card>

          <!-- Fork: numbers of the endpoint, in the place of the cards and of the uptime badges of the original Gatus -->
          <DetailsSummary
            :push="(currentStatus && currentStatus.push) === true"
            :current-response-time="currentStatus ? currentStatus.currentResponseTime : null"
            :uptime="currentStatus ? currentStatus.uptime : null"
            :response-time="currentStatus ? currentStatus.responseTime : null"
          />

          <Card v-if="showResponseTimeChartAndBadges" data-testid="response-time-trend">
            <CardHeader>
              <div class="flex items-center justify-between">
                <CardTitle>Response Time Trend</CardTitle>
                <!-- Fork: periods of the chart of Uptime Kuma, remembered in the browser -->
                <select
                  v-model="selectedChartPeriod"
                  aria-label="Period of the response time chart"
                  class="text-sm bg-background border rounded-md px-3 py-1 focus:outline-none focus:ring-2 focus:ring-ring"
                  data-testid="response-time-chart-period"
                >
                  <option v-for="option in CHART_PERIOD_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option>
                </select>
              </div>
            </CardHeader>
            <CardContent>
              <!-- Fork: protected route of the chart of Uptime Kuma -->
              <ResponseTimeChart
                v-if="currentStatus && currentStatus.key"
                :chartUrl="`/api/v1/endpoints/${encodeURIComponent(currentStatus.key)}/response-time-chart`"
                :period="selectedChartPeriod"
                :refreshKey="latestResult ? latestResult.timestamp : null"
              />
            </CardContent>
          </Card>

          <Card data-testid="checks-table-card">
            <div class="p-6 space-y-4">
              <RecentChecksTable v-if="endpointStatus" :results="endpointStatus.results || []" />
              <div v-if="endpointStatus && endpointStatus.key" class="pt-4 border-t">
                <Pagination @page="changePage" :numberOfResultsPerPage="resultPageSize" :currentPageProp="currentPage" />
              </div>
            </div>
          </Card>

          <div v-if="showResponseTimeChartAndBadges" class="grid gap-4 md:grid-cols-2 lg:grid-cols-4" data-testid="details-badges">
            <Card v-for="period in ['30d', '7d', '24h', '1h']" :key="period">
              <CardHeader class="pb-2">
                <CardTitle class="text-sm font-medium text-muted-foreground text-center">
                  {{ period === '30d' ? 'Last 30 days' : period === '7d' ? 'Last 7 days' : period === '24h' ? 'Last 24 hours' : 'Last hour' }}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <img :src="generateResponseTimeBadgeImageURL(period)" :alt="`${period} response time`" class="mx-auto mt-2" />
              </CardContent>
            </Card>
          </div>

          <Card data-testid="details-health">
            <CardHeader>
              <CardTitle>Current Health</CardTitle>
            </CardHeader>
            <CardContent>
              <div class="text-center">
                <img :src="generateHealthBadgeImageURL()" alt="health badge" class="mx-auto" />
              </div>
            </CardContent>
          </Card>

          <!-- Fork: the events are collapsed by default, like the Checks table -->
          <Card v-if="events && events.length > 0" data-testid="endpoint-events">
            <CardContent class="pt-6">
              <EventsTimeline :items="eventItems" />
            </CardContent>
          </Card>
        </div>

        <div v-else class="flex items-center justify-center py-20">
          <Loading size="lg" />
        </div>
      </div>
    </div>

    <Settings @refreshData="fetchData()" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft, RefreshCw, Activity, Timer } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import StatusBadge from '@/components/StatusBadge.vue'
import EndpointCard from '@/components/EndpointCard.vue'
import Settings from '@/components/Settings.vue'
import Pagination from '@/components/Pagination.vue'
import Loading from '@/components/Loading.vue'
import ResponseTimeChart from '@/components/ResponseTimeChart.vue'
import RecentChecksTable from '@/components/RecentChecksTable.vue'
import EventsTimeline from '@/components/EventsTimeline.vue'
import DetailsSummary from '@/components/DetailsSummary.vue'
import { generatePrettyTimeAgo, generatePrettyTimeDifference } from '@/utils/time'
import { certificateClass, certificateDate, certificateOfResults, certificateText } from '@/utils/certificate'
import { PROTECTED_API_HEADERS, notifyUnauthorized } from '@/utils/auth'
import { watchEndpointResults } from '@/utils/liveUpdates'
import { CHART_PERIOD_OPTIONS, readStoredPeriod, storePeriod } from '@/utils/responseTimeChart'
import { readPreference, writePreference } from '@/utils/storage'

const router = useRouter()
const route = useRoute()
const emit = defineEmits(['showTooltip'])

const endpointStatus = ref(null) // For paginated historical data
const currentStatus = ref(null) // For current/latest status (always page 1)
const events = ref([])
// Fork: items of the collapsible events, see components/EventsTimeline.vue
const eventItems = computed(() => events.value.map((event) => ({ key: `${event.type}-${event.timestamp}`, type: event.type, text: event.fancyText, dateTime: prettifyTimestamp(event.timestamp), timeAgo: event.fancyTimeAgo })))
const currentPage = ref(1)
const resultPageSize = 50
const showResponseTimeChartAndBadges = ref(false)
const showAverageResponseTime = ref(readPreference('show-average-response-time') !== 'false')
// Fork: period of the response time chart, remembered in the browser
const selectedChartPeriod = ref(readStoredPeriod())
const isRefreshing = ref(false)

const latestResult = computed(() => {
  // Use currentStatus for the actual latest result
  if (!currentStatus.value || !currentStatus.value.results || currentStatus.value.results.length === 0) {
    return null
  }
  return currentStatus.value.results[currentStatus.value.results.length - 1]
})

const currentHealthStatus = computed(() => {
  if (!latestResult.value) return 'unknown'
  // Fork: a Pending result is not a success, but is shown in yellow
  if (latestResult.value.pending) return 'pending'
  return latestResult.value.success ? 'healthy' : 'unhealthy'
})

const hostname = computed(() => {
  return latestResult.value?.hostname || null
})

// Expiration of the TLS certificate, from the most recent result with a certificate of the first page (fork)
const certificate = computed(() => {
  const expiration = certificateOfResults(currentStatus.value && currentStatus.value.results)
  if (!expiration) {
    return null
  }
  return {
    text: certificateText(expiration.days),
    className: certificateClass(expiration.days),
    date: certificateDate(expiration.expiresAt)
  }
})

const toggleShowAverageResponseTime = () => {
  showAverageResponseTime.value = !showAverageResponseTime.value
  writePreference('show-average-response-time', showAverageResponseTime.value ? 'true' : 'false')
}

// describeEvents returns the events of the endpoint, from the newest to the oldest, with their texts
const describeEvents = (rawEvents) => {
  let processedEvents = []
  if (rawEvents && rawEvents.length > 0) {
    for (let i = rawEvents.length - 1; i >= 0; i--) {
      let event = rawEvents[i]
      if (i === rawEvents.length - 1) {
        if (event.type === 'UNHEALTHY') {
          event.fancyText = 'Endpoint is unhealthy'
        } else if (event.type === 'HEALTHY') {
          event.fancyText = 'Endpoint is healthy'
        } else if (event.type === 'START') {
          event.fancyText = 'Monitoring started'
        }
      } else {
        let nextEvent = rawEvents[i + 1]
        if (event.type === 'HEALTHY') {
          event.fancyText = 'Endpoint became healthy'
        } else if (event.type === 'UNHEALTHY') {
          if (nextEvent) {
            event.fancyText = 'Endpoint was unhealthy for ' + generatePrettyTimeDifference(nextEvent.timestamp, event.timestamp)
          } else {
            event.fancyText = 'Endpoint became unhealthy'
          }
        } else if (event.type === 'START') {
          event.fancyText = 'Monitoring started'
        }
      }
      event.fancyTimeAgo = generatePrettyTimeAgo(event.timestamp)
      processedEvents.push(event)
    }
  }
  return processedEvents
}

// Fork: generation of the requests of fetchData. A response is only applied when no newer request was applied before
// it, so that a response that arrives out of order (notification, periodic refresh or click) is discarded
let requestGeneration = 0
let appliedGeneration = 0
let refreshingGeneration = 0
// Fork: real-time channel of the endpoint, see utils/liveUpdates.js
let liveUpdates = null

const fetchStatuses = (key, page) => fetch(`/api/v1/endpoints/${key}/statuses?page=${page}&pageSize=${resultPageSize}`, {
  credentials: 'include',
  headers: PROTECTED_API_HEADERS
})

// fetchData fetches the page of results of the table and, on another page, also the first page, which feeds the bars,
// the numbers and the chart. With silent, used by the real-time notifications, the refresh button does not spin.
const fetchData = async ({ silent = false } = {}) => {
  const generation = ++requestGeneration
  const key = route.params.key
  const page = currentPage.value
  if (!silent) {
    refreshingGeneration = generation
    isRefreshing.value = true
  }
  try {
    const [pageResponse, firstPageResponse] = await Promise.all(page === 1 ? [fetchStatuses(key, page)] : [fetchStatuses(key, page), fetchStatuses(key, 1)])
    if (pageResponse.status === 401 || (firstPageResponse && firstPageResponse.status === 401)) {
      notifyUnauthorized()
      return
    }
    if (pageResponse.status !== 200) {
      console.error('[Details][fetchData] Error:', await pageResponse.text())
      return
    }
    const data = await pageResponse.json()
    let firstPageData = page === 1 ? data : null
    if (firstPageResponse) {
      if (firstPageResponse.status === 200) {
        firstPageData = await firstPageResponse.json()
      } else {
        console.error('[Details][fetchData] Error:', await firstPageResponse.text())
      }
    }
    liveUpdates?.notifyRefreshSucceeded()
    // Discards the response of an older request, of another endpoint or of another page of the table
    if (generation <= appliedGeneration || key !== route.params.key || page !== currentPage.value) {
      return
    }
    appliedGeneration = generation
    endpointStatus.value = data
    if (firstPageData) {
      currentStatus.value = firstPageData
      // The events do not depend on the page of the results
      events.value = describeEvents(firstPageData.events)
    }
    // Fork: the chart is shown as soon as there is a result, even when every duration is zero (pushes without ping),
    // so that the periods out of service are also shown
    if ((data.results && data.results.length > 0) || (firstPageData && firstPageData.results && firstPageData.results.length > 0)) {
      showResponseTimeChartAndBadges.value = true
    }
  } catch (error) {
    console.error('[Details][fetchData] Error:', error)
  } finally {
    if (generation === refreshingGeneration) {
      isRefreshing.value = false
    }
  }
}

// startLiveUpdates opens the real-time channel of the endpoint of the route, closing the previous one
const startLiveUpdates = () => {
  liveUpdates?.stop()
  liveUpdates = null
  const key = route.params.key
  if (!key) {
    return
  }
  liveUpdates = watchEndpointResults(`/api/v1/endpoints/${encodeURIComponent(key)}/events`, () => fetchData({ silent: true }))
}

const goBack = () => {
  router.push('/')
}

const changePage = (page) => {
  currentPage.value = page
  fetchData()
}

const showTooltip = (result, event, action = 'hover') => {
  emit('showTooltip', result, event, action)
}

const prettifyTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString()
}

const generateHealthBadgeImageURL = () => {
  return `/api/v1/endpoints/${endpointStatus.value.key}/health/badge.svg`
}

const generateResponseTimeBadgeImageURL = (duration) => {
  return `/api/v1/endpoints/${endpointStatus.value.key}/response-times/${duration}/badge.svg`
}

// Fork: the chosen period of the chart is remembered in the browser
watch(selectedChartPeriod, (period) => {
  storePeriod(period)
})

// Fork: another endpoint on the same screen starts from scratch, with its own real-time channel
watch(() => route.params.key, (key, previousKey) => {
  if (!key || key === previousKey || route.name !== 'EndpointDetails') {
    return
  }
  endpointStatus.value = null
  currentStatus.value = null
  events.value = []
  currentPage.value = 1
  showResponseTimeChartAndBadges.value = false
  startLiveUpdates()
  fetchData()
})

onMounted(() => {
  fetchData()
  startLiveUpdates()
})

onUnmounted(() => {
  liveUpdates?.stop()
  liveUpdates = null
})
</script>