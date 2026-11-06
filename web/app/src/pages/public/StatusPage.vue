<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div :class="['mx-auto w-full px-4 py-5 sm:px-12', activeView === VIEW_LIST ? 'max-w-6xl 2xl:max-w-[1400px]' : 'max-w-[1760px]']">
    <div v-if="state === 'loading'" class="py-16 flex justify-center"><Loading /></div>

    <section v-else-if="state === 'not-found'" class="py-16 text-center" data-testid="status-page-not-found">
      <h1 class="text-2xl font-bold tracking-tight">Page not found</h1>
      <p class="mt-2 text-muted-foreground">Check the address of the status page.</p>
    </section>

    <!-- Fork: the page asks for a login of its own, and the browser only asks for it again on a new navigation -->
    <section v-else-if="state === 'unauthorized'" class="py-16 text-center" data-testid="status-page-unauthorized">
      <h1 class="text-2xl font-bold tracking-tight">Login required</h1>
      <p class="mt-2 text-muted-foreground">This status page asks for a username and a password.</p>
      <button type="button" class="mt-4 border px-4 py-2 text-sm font-medium hover:bg-muted border-line" @click="reload">
        Reload the page
      </button>
    </section>

    <template v-else>
      <div
        v-if="errorMessage"
        role="alert"
        data-testid="status-page-error"
        :class="[
          'mb-4 border px-4 py-3 text-sm',
          page ? 'border-warning bg-warning-soft text-warning-strong' : 'border-danger bg-danger-soft text-danger-strong',
        ]"
      >
        {{ errorMessage }}
      </div>

      <template v-if="page">
        <header class="mb-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold tracking-tight sm:text-2xl" data-testid="status-page-title">{{ page.title }}</h1>
            <p
              v-if="page.description"
              class="mt-0.5 text-sm text-muted-foreground whitespace-pre-line"
              data-testid="status-page-description"
            >
              {{ page.description }}
            </p>
          </div>
          <!-- Fork: the views of the page, when it enables more than one -->
          <ViewSwitcher
            v-if="pageViews.enabled.length > 1"
            :views="pageViews.enabled"
            :model-value="activeView"
            :group-label="pageViews.groupLabel"
            @update:model-value="selectView"
          />
        </header>

        <StatusSummary :status="page.status" :updated-at="page.updatedAt" :now="now" :summary="page.summary" />

        <!-- Fork: the grouped and matrix views, computed in the browser from the payload (utils/statusPageViews.js) -->
        <template v-if="activeView !== VIEW_LIST">
          <p v-if="page.truncated" class="mt-2 text-sm text-muted-foreground" data-testid="status-page-truncated">
            Showing the first {{ page.summary?.total ?? 0 }} services.
          </p>
          <ViewCounters class="mt-4" :counts="viewCounters" :group-label="pageViews.groupLabel" :primary-name="primaryEnvironmentName" />
          <ViewFilters
            v-model="filters"
            :environments="viewFilterOptions.environments"
            :applications="viewFilterOptions.applications"
            :group-label="pageViews.groupLabel"
          />
          <GroupedView
            v-if="activeView === VIEW_GROUPED"
            :groups="groupedGroups"
            :collapsed="groupedCollapsed"
            :slug="slug"
            :has-environments="pageViews.environments.length > 0"
            @toggle="toggleGroupByName"
          />
          <MatrixView v-else :model="matrix" :slug="slug" :group-label="pageViews.groupLabel" :primary-name="primaryEnvironmentName" />
        </template>

        <template v-else>
          <!-- The number is the one of the payload: the limit is status-pages.maximum-endpoints-per-page, not a constant -->
          <p v-if="page.truncated" class="mt-2 text-sm text-muted-foreground" data-testid="status-page-truncated">
            Showing the first {{ page.summary?.total ?? 0 }} services.
          </p>
          <p v-if="page.groups.length === 0 && featuredEndpoints.length === 0" class="mt-6 text-center text-muted-foreground">
            No services on this page.
          </p>

          <section v-if="featuredEndpoints.length" class="mt-6" aria-labelledby="status-featured-title" data-testid="status-featured">
            <h2 id="status-featured-title" class="mb-2 text-lg font-semibold">Featured</h2>
            <ul :class="['grid gap-3', featuredEndpoints.length > 1 ? 'md:grid-cols-2' : '']">
              <EndpointRow
                v-for="endpoint in featuredEndpoints"
                :key="`${endpoint.group}-${endpoint.name}`"
                :endpoint="endpoint"
                :group="endpoint.group"
                :bars="bars"
                :slug="slug"
                featured
              />
            </ul>
          </section>

          <section
            v-for="(group, groupIndex) in page.groups"
            :key="`group:${group.name || ''}`"
            class="mt-6"
            :aria-labelledby="`status-group-${groupIndex}`"
            :data-testid="`status-group-${group.name || 'outros'}`"
          >
            <div class="flex items-baseline justify-between gap-4 border-b pb-1.5 dark:border-gray-800">
              <!-- The header is a real button, so that it works with a keyboard and a screen reader: its text carries the
                 name, the status and the counts of the group, which is what is announced instead of the rows of a
                 collapsed group -->
              <h2 :id="`status-group-${groupIndex}`" class="min-w-0 flex-1">
                <button
                  type="button"
                  class="flex w-full min-w-0 items-baseline gap-3 text-left focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
                  :aria-expanded="!collapsedGroups.has(group.name || '')"
                  :aria-controls="`status-group-panel-${groupIndex}`"
                  :data-testid="`status-group-toggle-${group.name || 'outros'}`"
                  @click="toggleGroup(group)"
                >
                  <component
                    :is="collapsedGroups.has(group.name || '') ? ChevronRight : ChevronDown"
                    class="h-4 w-4 shrink-0 self-center text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span class="truncate text-lg font-semibold">{{ group.name || 'Other services' }}</span>
                  <span :class="['shrink-0 text-sm font-normal', groupStatusClass(group.status)]">{{
                    groupStatusLabel(group.status)
                  }}</span>
                  <span
                    class="shrink-0 text-xs font-normal text-muted-foreground"
                    :data-testid="`status-group-counts-${group.name || 'outros'}`"
                    >{{ groupCounts(group.summary) }}</span
                  >
                </button>
              </h2>
              <!-- Fork: the labels of the periods appear once per group, aligned with the columns of each row -->
              <dl
                v-if="!collapsedGroups.has(group.name || '')"
                class="hidden shrink-0 grid-cols-3 text-xs text-muted-foreground sm:grid"
                aria-hidden="true"
              >
                <dt v-for="period in UPTIME_PERIODS" :key="period" class="w-16 text-right">{{ period }}</dt>
              </dl>
            </div>
            <!-- The panel always exists, so that aria-controls points to something; the rows of a collapsed group do not -->
            <div :id="`status-group-panel-${groupIndex}`">
              <ul v-if="!collapsedGroups.has(group.name || '')" class="divide-y dark:divide-gray-800">
                <EndpointRow
                  v-for="endpoint in group.endpoints"
                  :key="endpoint.name"
                  :endpoint="endpoint"
                  :group="group.name"
                  :bars="bars"
                  :slug="slug"
                />
              </ul>
            </div>
          </section>
        </template>
      </template>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Loading from '@/components/Loading.vue'
import StatusSummary from '@/components/public/StatusSummary.vue'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import EndpointRow from '@/components/public/EndpointRow.vue'
import { SLUG_PATTERN, STATUS_LABELS } from '@/utils/statusPage'
import { COLLAPSED, EXPANDED, groupCounts, isCollapsed, rememberedChoices, writeChoice } from '@/utils/statusPageGroups'
import ViewSwitcher from '@/components/public/ViewSwitcher.vue'
import ViewCounters from '@/components/public/ViewCounters.vue'
import ViewFilters from '@/components/public/ViewFilters.vue'
import GroupedView from '@/components/public/GroupedView.vue'
import MatrixView from '@/components/public/MatrixView.vue'
import {
  ALL,
  VIEW_GROUPED,
  VIEW_LIST,
  collectGroups,
  counters,
  filterOptions,
  groupedModel,
  groupStatuses,
  matrixModel,
  normalizeViews,
  resolveView,
  sanitizeFilters,
} from '@/utils/statusPageViews'

const UPTIME_PERIODS = ['24h', '7d', '30d']

const REFRESH_INTERVAL_MS = 60000
const CLOCK_INTERVAL_MS = 10000

const route = useRoute()
const router = useRouter()

const page = ref(null)
// loading, ready (with or without page, see errorMessage) or not-found
const state = ref('loading')
const errorMessage = ref('')
const now = ref(Date.now())
const narrowScreen = window.matchMedia('(max-width: 639px)')
const bars = ref(narrowScreen.matches ? 25 : 50)

let refreshTimer = null
let clockTimer = null
let abortController = null
let requestGeneration = 0

// Collapsed groups, see utils/statusPageGroups.js. The maps are keyed by the raw name of the group, an empty string for
// the one without name. visitChoices lives until the page is closed, with or without storage; rememberedChoices comes
// from another visit and is resolved before each payload is shown; incidentCollapsed holds the groups that are not
// operational and that the visitor collapsed, which only lasts until the next payload.
const visitChoices = ref(new Map())
const rememberedGroupChoices = ref(new Map())
const incidentCollapsed = ref(new Set())
let groupStorageKeys = null

const collapsedGroups = computed(() => {
  const collapsed = new Set()
  for (const group of page.value?.groups || []) {
    const name = group.name || ''
    if (
      isCollapsed({
        status: group.status,
        visitChoice: visitChoices.value.get(name),
        rememberedChoice: rememberedGroupChoices.value.get(name),
        pageDefault: page.value.groupsCollapsed === true,
        incidentCollapsed: incidentCollapsed.value.has(name),
      })
    ) {
      collapsed.add(name)
    }
  }
  return collapsed
})

const toggleGroup = (group) => {
  const name = group.name || ''
  if (group.status !== 'operational') {
    // Allowed, but it does not stick: the next payload expands the group again, and nothing is remembered
    const next = new Set(incidentCollapsed.value)
    if (!next.delete(name)) {
      next.add(name)
    }
    incidentCollapsed.value = next
    return
  }
  const choice = collapsedGroups.value.has(name) ? EXPANDED : COLLAPSED
  visitChoices.value = new Map(visitChoices.value).set(name, choice)
  const key = groupStorageKeys?.get(name)
  if (key) {
    writeChoice(key, choice)
  }
}

// Fork: views of the page. The view comes from ?view= when the page enables it, else the default of the page; the
// filters live in memory and fall back to "All" when a refresh removes their value.
const pageViews = computed(() => normalizeViews(page.value?.views))
const activeView = computed(() => resolveView(page.value?.views, typeof route.query.view === 'string' ? route.query.view : ''))
const filters = ref({ environment: ALL, application: ALL, onlyProblems: false })
const viewCounters = computed(() => counters(page.value))
const viewFilterOptions = computed(() => filterOptions(page.value))
const primaryEnvironmentName = computed(() => pageViews.value.environments[0]?.name || '')
const groupedGroups = computed(() => (activeView.value === VIEW_GROUPED ? groupedModel(page.value, filters.value) : []))
const matrix = computed(() =>
  activeView.value !== VIEW_LIST && activeView.value !== VIEW_GROUPED ? matrixModel(page.value, filters.value) : { columns: [], rows: [] },
)

const selectView = (view) => {
  const query = { ...route.query }
  if (view === pageViews.value.default) {
    delete query.view
  } else {
    query.view = view
  }
  router.replace({ query })
}

// The grouped view follows the collapsing rules of the list, with the same choices, but over the status of the group
// with its featured endpoints: a group with a featured endpoint down, or reached only through featured endpoints, is
// expanded during an incident like any other
const viewGroupStatuses = computed(() => groupStatuses(page.value))
const groupedCollapsed = computed(() => {
  const collapsed = new Set()
  for (const [name, status] of viewGroupStatuses.value) {
    if (
      isCollapsed({
        status,
        visitChoice: visitChoices.value.get(name),
        rememberedChoice: rememberedGroupChoices.value.get(name),
        pageDefault: page.value?.groupsCollapsed === true,
        incidentCollapsed: incidentCollapsed.value.has(name),
      })
    ) {
      collapsed.add(name)
    }
  }
  return collapsed
})

const toggleGroupByName = (name) => {
  if (viewGroupStatuses.value.get(name) !== 'operational') {
    const next = new Set(incidentCollapsed.value)
    if (!next.delete(name)) {
      next.add(name)
    }
    incidentCollapsed.value = next
    return
  }
  const choice = groupedCollapsed.value.has(name) ? EXPANDED : COLLAPSED
  visitChoices.value = new Map(visitChoices.value).set(name, choice)
  const key = groupStorageKeys?.get(name)
  if (key) {
    writeChoice(key, choice)
  }
}

const slug = computed(() => (route.name === 'PublicStatusPage' ? String(route.params.slug || '') : ''))

const groupStatusLabel = (status) => STATUS_LABELS[status]?.group || STATUS_LABELS.unknown.group

const groupStatusClass = (status) =>
  ({
    operational: 'text-success',
    degraded: 'text-amber-700 dark:text-amber-400',
    down: 'text-red-700 dark:text-red-400',
  })[status] || 'text-muted-foreground'

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

// Fork: a page with a login of its own answers 401 when the credential changes with the tab open: the refresh cycle
// stops, because repeating it would not open the dialog of the browser again
const showUnauthorized = () => {
  stopRefreshing()
  page.value = null
  errorMessage.value = ''
  state.value = 'unauthorized'
  document.title = 'Login required'
}

const reload = () => window.location.reload()

const showNotFound = () => {
  stopRefreshing()
  page.value = null
  errorMessage.value = ''
  state.value = 'not-found'
  document.title = 'Page not found'
}

const load = async () => {
  const currentSlug = slug.value
  const generation = ++requestGeneration
  abortController?.abort()
  if (!SLUG_PATTERN.test(currentSlug)) {
    showNotFound()
    return
  }
  abortController = new AbortController()
  let retryDelayMs = REFRESH_INTERVAL_MS
  try {
    // Fork: the credential of a page with a login goes with every request of the page
    const response = await fetch(`/api/v1/status-pages/${encodeURIComponent(currentSlug)}`, {
      credentials: 'same-origin',
      signal: abortController.signal,
    })
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
      errorMessage.value =
        response.status === 429
          ? 'Too many requests right now. This page will refresh automatically.'
          : 'This status page is temporarily unavailable. It will refresh automatically.'
    } else if (!response.ok || !(response.headers.get('Content-Type') || '').includes('application/json')) {
      errorMessage.value = 'Could not load the status page. It will refresh automatically.'
    } else {
      // The keys and the remembered choices are resolved before the payload is shown, so that no group appears in one
      // state and changes right after. Deriving the keys is asynchronous: after each await, a request that is not the
      // current one anymore (another slug, another fetch of the same slug that answered 401 or 404, or the page closed)
      // publishes nothing.
      const payload = await response.json()
      if (generation !== requestGeneration) {
        return
      }
      const { hashes, remembered } = await rememberedChoices(currentSlug, collectGroups(payload))
      if (generation !== requestGeneration) {
        return
      }
      groupStorageKeys = hashes
      rememberedGroupChoices.value = remembered
      incidentCollapsed.value = new Set()
      filters.value = sanitizeFilters(payload, filters.value)
      page.value = payload
      errorMessage.value = ''
      document.title = page.value.title
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

const featuredEndpoints = computed(() => (page.value && page.value.featured) || [])

watch(slug, () => {
  page.value = null
  visitChoices.value = new Map()
  rememberedGroupChoices.value = new Map()
  incidentCollapsed.value = new Set()
  groupStorageKeys = null
  filters.value = { environment: ALL, application: ALL, onlyProblems: false }
  errorMessage.value = ''
  state.value = 'loading'
  load()
})

onMounted(() => {
  load()
  clockTimer = setInterval(() => {
    now.value = Date.now()
  }, CLOCK_INTERVAL_MS)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  narrowScreen.addEventListener('change', handleScreenChange)
})

onUnmounted(() => {
  // Invalidates a request that is still deriving the keys of the groups
  requestGeneration++
  stopRefreshing()
  clearInterval(clockTimer)
  abortController?.abort()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  narrowScreen.removeEventListener('change', handleScreenChange)
})
</script>
