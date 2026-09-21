<template>
  <div class="dashboard-container bg-background">
    <div class="container mx-auto px-4 py-4 max-w-7xl">
      <div class="mb-4">
        <!-- Fork: compact heading with a summary of the endpoints, like the administration -->
        <div class="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold tracking-tight">{{ dashboardHeading }}</h1>
            <p class="mt-0.5 truncate text-sm text-muted-foreground">{{ dashboardSubheading }}</p>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <div v-if="!loading && endpointStatuses.length" class="flex flex-wrap items-center gap-2 text-xs" data-testid="dashboard-summary">
              <span class="inline-flex items-center gap-1.5 border px-2 py-1 text-muted-foreground dark:border-gray-700" data-testid="dashboard-summary-up">
                <span class="h-2 w-2 rounded-full bg-green-500" aria-hidden="true"></span>{{ endpointSummary.up }} up
              </span>
              <span v-if="endpointSummary.down" class="inline-flex items-center gap-1.5 border border-red-300 bg-red-50 px-2 py-1 text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200" data-testid="dashboard-summary-down">
                <span class="h-2 w-2 rounded-full bg-red-500" aria-hidden="true"></span>{{ endpointSummary.down }} down
              </span>
              <span v-if="endpointSummary.pending" class="inline-flex items-center gap-1.5 border border-yellow-300 bg-yellow-50 px-2 py-1 text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-200" data-testid="dashboard-summary-pending">
                <span class="h-2 w-2 rounded-full bg-yellow-400" aria-hidden="true"></span>{{ endpointSummary.pending }} pending
              </span>
              <span v-if="endpointSummary.unknown" class="inline-flex items-center gap-1.5 border px-2 py-1 text-muted-foreground dark:border-gray-700" data-testid="dashboard-summary-unknown">
                <span class="h-2 w-2 rounded-full bg-statusgray-400" aria-hidden="true"></span>{{ endpointSummary.unknown }} no data
              </span>
            </div>
            <Button
              variant="ghost"
              size="icon"
              @click="toggleShowAverageResponseTime"
              :title="showAverageResponseTime ? 'Show min-max response time' : 'Show average response time'"
            >
              <Activity v-if="showAverageResponseTime" class="h-4 w-4" />
              <Timer v-else class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" @click="refreshData" title="Refresh data">
              <RefreshCw class="h-4 w-4" />
            </Button>
          </div>
        </div>
        <!-- Announcement Banner (Active Announcements) -->
        <AnnouncementBanner :announcements="activeAnnouncements" />
        <!-- Search bar -->
        <SearchBar
          @search="handleSearch"
          @update:showOnlyFailing="showOnlyFailing = $event"
          @update:showRecentFailures="showRecentFailures = $event"
          @update:groupByGroup="groupByGroup = $event"
          @update:sortBy="sortBy = $event"
          @initializeCollapsedGroups="initializeCollapsedGroups"
        />
      </div>

      <div v-if="loading" class="flex items-center justify-center py-20">
        <Loading size="lg" />
      </div>

      <div v-else-if="filteredEndpoints.length === 0 && filteredSuites.length === 0" class="text-center py-20">
        <AlertCircle class="h-12 w-12 text-muted-foreground mx-auto mb-4" />
        <h3 class="text-lg font-semibold mb-2">No endpoints or suites found</h3>
        <p class="text-muted-foreground">
          {{ searchQuery || showOnlyFailing || showRecentFailures 
            ? 'Try adjusting your filters' 
            : 'No endpoints or suites are configured' }}
        </p>
      </div>

      <div v-else>
        <!-- Grouped view -->
        <div v-if="groupByGroup" class="space-y-6">
          <div v-for="(items, group) in combinedGroups" :key="group" class="endpoint-group border rounded-lg overflow-hidden">
            <!-- Group Header -->
            <div 
              @click="toggleGroupCollapse(group)"
              class="endpoint-group-header flex items-center justify-between px-4 py-3 bg-card border-b cursor-pointer hover:bg-accent/50 transition-colors"
            >
              <div class="flex items-center gap-3">
                <ChevronDown v-if="uncollapsedGroups.has(group)" class="h-5 w-5 text-muted-foreground" />
                <ChevronUp v-else class="h-5 w-5 text-muted-foreground" />
                <h2 class="text-base font-semibold text-foreground">{{ group }}</h2>
              </div>
              <div class="flex items-center gap-2">
                <span v-if="calculateUnhealthyCount(items.endpoints) + calculateFailingSuitesCount(items.suites) > 0" 
                      class="bg-red-600 text-white px-2 py-0.5 rounded-none text-xs font-medium">
                  {{ calculateUnhealthyCount(items.endpoints) + calculateFailingSuitesCount(items.suites) }}
                </span>
                <CheckCircle v-else class="h-5 w-5 text-green-600" />
              </div>
            </div>
            
            <!-- Group Content -->
            <div v-if="uncollapsedGroups.has(group)" class="endpoint-group-content p-4">
              <!-- Suites Section -->
              <div v-if="items.suites.length > 0" class="mb-4">
                <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">Suites</h3>
                <div class="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                  <SuiteCard
                    v-for="suite in items.suites"
                    :key="suite.key"
                    :suite="suite"
                    :maxResults="resultPageSize"
                    @showTooltip="showTooltip"
                  />
                </div>
              </div>
              
              <!-- Endpoints Section -->
              <div v-if="items.endpoints.length > 0">
                <h3 v-if="items.suites.length > 0" class="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">Endpoints</h3>
                <div class="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                  <EndpointCard
                    v-for="endpoint in items.endpoints"
                    :key="endpoint.key"
                    :endpoint="endpoint"
                    :maxResults="resultPageSize"
                    :showAverageResponseTime="showAverageResponseTime"
                    @showTooltip="showTooltip"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <!-- Regular view -->
        <div v-else>
          <!-- Suites Section -->
          <div v-if="filteredSuites.length > 0" class="mb-6">
            <h2 class="text-lg font-semibold text-foreground mb-3">Suites</h2>
            <div class="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
              <SuiteCard
                v-for="suite in paginatedSuites"
                :key="suite.key"
                :suite="suite"
                :maxResults="resultPageSize"
                @showTooltip="showTooltip"
              />
            </div>
          </div>
          
          <!-- Endpoints Section -->
          <div v-if="filteredEndpoints.length > 0">
            <h2 v-if="filteredSuites.length > 0" class="text-lg font-semibold text-foreground mb-3">Endpoints</h2>
            <div class="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
              <EndpointCard
                v-for="endpoint in paginatedEndpoints"
                :key="endpoint.key"
                :endpoint="endpoint"
                :maxResults="resultPageSize"
                :showAverageResponseTime="showAverageResponseTime"
                @showTooltip="showTooltip"
              />
            </div>
          </div>
        </div>

        <div v-if="!groupByGroup && totalPages > 1" class="mt-8 flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="icon"
            :disabled="currentPage === 1"
            @click="goToPage(currentPage - 1)"
          >
            <ChevronLeft class="h-4 w-4" />
          </Button>
          
          <div class="flex gap-1">
            <Button
              v-for="page in visiblePages"
              :key="page"
              :variant="page === currentPage ? 'default' : 'outline'"
              size="sm"
              @click="goToPage(page)"
            >
              {{ page }}
            </Button>
          </div>

          <Button
            variant="outline"
            size="icon"
            :disabled="currentPage === totalPages"
            @click="goToPage(currentPage + 1)"
          >
            <ChevronRight class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <!-- Past Announcements Section -->
      <div v-if="archivedAnnouncements.length > 0" class="mt-12 pb-8">
        <PastAnnouncements :announcements="archivedAnnouncements" />
      </div>
    </div>

    <Settings @refreshData="fetchData" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Activity, Timer, RefreshCw, AlertCircle, ChevronLeft, ChevronRight, ChevronDown, ChevronUp, CheckCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import EndpointCard from '@/components/EndpointCard.vue'
import SuiteCard from '@/components/SuiteCard.vue'
import SearchBar from '@/components/SearchBar.vue'
import Settings from '@/components/Settings.vue'
import Loading from '@/components/Loading.vue'
import AnnouncementBanner from '@/components/AnnouncementBanner.vue'
import PastAnnouncements from '@/components/PastAnnouncements.vue'
import { PROTECTED_API_HEADERS, notifyUnauthorized } from '@/utils/auth'
import { readPreference, removePreference, writePreference } from '@/utils/storage'

const props = defineProps({
  announcements: {
    type: Array,
    default: () => []
  }
})

// Computed properties for active and archived announcements
const activeAnnouncements = computed(() => {
  return props.announcements ? props.announcements.filter(a => !a.archived) : []
})

const archivedAnnouncements = computed(() => {
  return props.announcements ? props.announcements.filter(a => a.archived) : []
})

const emit = defineEmits(['showTooltip'])

const endpointStatuses = ref([])
const suiteStatuses = ref([])
const loading = ref(false)
const currentPage = ref(1)
const itemsPerPage = 96
const searchQuery = ref('')
const showOnlyFailing = ref(false)
const showRecentFailures = ref(false)
const showAverageResponseTime = ref(readPreference('show-average-response-time') !== 'false')
const groupByGroup = ref(false)
const sortBy = ref(readPreference('sort-by') || 'name')
const uncollapsedGroups = ref(new Set())
const resultPageSize = 50

// Endpoints up, down, pending and without results, for the summary next to the heading (fork). A Pending endpoint only
// counts as pending; the failure counters of the groups and the filters still count it as a failure
const endpointSummary = computed(() => {
  const summary = { up: 0, down: 0, pending: 0, unknown: 0 }
  for (const endpoint of endpointStatuses.value) {
    const results = endpoint.results || []
    if (results.length === 0) {
      summary.unknown++
    } else if (results[results.length - 1].success) {
      summary.up++
    } else if (results[results.length - 1].pending) {
      summary.pending++
    } else {
      summary.down++
    }
  }
  return summary
})

const filteredEndpoints = computed(() => {
  let filtered = [...endpointStatuses.value]
  
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    filtered = filtered.filter(endpoint => 
      endpoint.name.toLowerCase().includes(query) ||
      (endpoint.group && endpoint.group.toLowerCase().includes(query))
    )
  }
  
  if (showOnlyFailing.value) {
    filtered = filtered.filter(endpoint => {
      if (!endpoint.results || endpoint.results.length === 0) return false
      const latestResult = endpoint.results[endpoint.results.length - 1]
      return !latestResult.success
    })
  }
  
  if (showRecentFailures.value) {
    filtered = filtered.filter(endpoint => {
      if (!endpoint.results || endpoint.results.length === 0) return false
      return endpoint.results.some(result => !result.success)
    })
  }
  
  // Sort by health if selected
  if (sortBy.value === 'health') {
    filtered.sort((a, b) => {
      const aHealthy = a.results && a.results.length > 0 && a.results[a.results.length - 1].success
      const bHealthy = b.results && b.results.length > 0 && b.results[b.results.length - 1].success
      
      // Unhealthy first
      if (!aHealthy && bHealthy) return -1
      if (aHealthy && !bHealthy) return 1
      
      // Then sort by name
      return a.name.localeCompare(b.name)
    })
  }
  
  return filtered
})

const filteredSuites = computed(() => {
  let filtered = [...(suiteStatuses.value || [])]
  
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    filtered = filtered.filter(suite => 
      suite.name.toLowerCase().includes(query) ||
      (suite.group && suite.group.toLowerCase().includes(query))
    )
  }
  
  if (showOnlyFailing.value) {
    filtered = filtered.filter(suite => {
      if (!suite.results || suite.results.length === 0) return false
      return !suite.results[suite.results.length - 1].success
    })
  }
  
  if (showRecentFailures.value) {
    filtered = filtered.filter(suite => {
      if (!suite.results || suite.results.length === 0) return false
      return suite.results.some(result => !result.success)
    })
  }
  
  // Sort by health if selected
  if (sortBy.value === 'health') {
    filtered.sort((a, b) => {
      const aHealthy = a.results && a.results.length > 0 && a.results[a.results.length - 1].success
      const bHealthy = b.results && b.results.length > 0 && b.results[b.results.length - 1].success
      
      // Unhealthy first
      if (!aHealthy && bHealthy) return -1
      if (aHealthy && !bHealthy) return 1
      
      // Then sort by name
      return a.name.localeCompare(b.name)
    })
  }
  
  return filtered
})

const totalPages = computed(() => {
  return Math.ceil((filteredEndpoints.value.length + filteredSuites.value.length) / itemsPerPage)
})

const groupedEndpoints = computed(() => {
  if (!groupByGroup.value) {
    return null
  }
  
  const grouped = {}
  filteredEndpoints.value.forEach(endpoint => {
    const group = endpoint.group || 'No Group'
    if (!grouped[group]) {
      grouped[group] = []
    }
    grouped[group].push(endpoint)
  })
  
  // Sort groups alphabetically, with 'No Group' at the end
  const sortedGroups = Object.keys(grouped).sort((a, b) => {
    if (a === 'No Group') return 1
    if (b === 'No Group') return -1
    return a.localeCompare(b)
  })
  
  const result = {}
  sortedGroups.forEach(group => {
    result[group] = grouped[group]
  })
  
  return result
})

const combinedGroups = computed(() => {
  if (!groupByGroup.value) {
    return null
  }
  
  const combined = {}
  
  // Add endpoints
  filteredEndpoints.value.forEach(endpoint => {
    const group = endpoint.group || 'No Group'
    if (!combined[group]) {
      combined[group] = { endpoints: [], suites: [] }
    }
    combined[group].endpoints.push(endpoint)
  })
  
  // Add suites
  filteredSuites.value.forEach(suite => {
    const group = suite.group || 'No Group'
    if (!combined[group]) {
      combined[group] = { endpoints: [], suites: [] }
    }
    combined[group].suites.push(suite)
  })
  
  // Sort groups alphabetically, with 'No Group' at the end
  const sortedGroups = Object.keys(combined).sort((a, b) => {
    if (a === 'No Group') return 1
    if (b === 'No Group') return -1
    return a.localeCompare(b)
  })
  
  const result = {}
  sortedGroups.forEach(group => {
    result[group] = combined[group]
  })
  
  return result
})

const paginatedEndpoints = computed(() => {
  if (groupByGroup.value) {
    // When grouping, we don't paginate
    return groupedEndpoints.value
  }
  
  const start = (currentPage.value - 1) * itemsPerPage
  const end = start + itemsPerPage
  return filteredEndpoints.value.slice(start, end)
})

const paginatedSuites = computed(() => {
  if (groupByGroup.value) {
    // When grouping, we don't paginate
    return filteredSuites.value
  }
  
  const start = (currentPage.value - 1) * itemsPerPage
  const end = start + itemsPerPage
  return filteredSuites.value.slice(start, end)
})

const visiblePages = computed(() => {
  const pages = []
  const maxVisible = 5
  let start = Math.max(1, currentPage.value - Math.floor(maxVisible / 2))
  let end = Math.min(totalPages.value, start + maxVisible - 1)
  
  if (end - start < maxVisible - 1) {
    start = Math.max(1, end - maxVisible + 1)
  }
  
  for (let i = start; i <= end; i++) {
    pages.push(i)
  }
  
  return pages
})

const fetchData = async () => {
  // Don't show loading state on refresh to prevent UI flicker
  const isInitialLoad = endpointStatuses.value.length === 0 && suiteStatuses.value.length === 0
  if (isInitialLoad) {
    loading.value = true
  }
  try {
    // Fetch endpoints
    const endpointResponse = await fetch(`/api/v1/endpoints/statuses?page=1&pageSize=${resultPageSize}`, {
      credentials: 'include',
      headers: PROTECTED_API_HEADERS
    })
    if (endpointResponse.status === 401) {
      notifyUnauthorized()
      return
    }
    if (endpointResponse.status === 200) {
      const data = await endpointResponse.json()
      endpointStatuses.value = data
    } else {
      console.error('[Home][fetchData] Error fetching endpoints:', await endpointResponse.text())
    }
    
    // Fetch suites
    const suiteResponse = await fetch(`/api/v1/suites/statuses?page=1&pageSize=${resultPageSize}`, {
      credentials: 'include',
      headers: PROTECTED_API_HEADERS
    })
    if (suiteResponse.status === 401) {
      notifyUnauthorized()
      return
    }
    if (suiteResponse.status === 200) {
      const suiteData = await suiteResponse.json()
      suiteStatuses.value = suiteData || []
    } else {
      console.error('[Home][fetchData] Error fetching suites:', await suiteResponse.text())
      // Ensure suiteStatuses stays as empty array instead of becoming null/undefined
      if (!suiteStatuses.value) {
        suiteStatuses.value = []
      }
    }
  } catch (error) {
    console.error('[Home][fetchData] Error:', error)
  } finally {
    if (isInitialLoad) {
      loading.value = false
    }
  }
}

const refreshData = () => {
  endpointStatuses.value = [];
  suiteStatuses.value = [];
  fetchData()
}

const handleSearch = (query) => {
  searchQuery.value = query
  currentPage.value = 1
}

const goToPage = (page) => {
  currentPage.value = page
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

const toggleShowAverageResponseTime = () => {
  showAverageResponseTime.value = !showAverageResponseTime.value
  writePreference('show-average-response-time', showAverageResponseTime.value ? 'true' : 'false')
}

const showTooltip = (result, event, action = 'hover') => {
  emit('showTooltip', result, event, action)
}

const calculateUnhealthyCount = (endpoints) => {
  return endpoints.filter(endpoint => {
    if (!endpoint.results || endpoint.results.length === 0) return false
    const latestResult = endpoint.results[endpoint.results.length - 1]
    return !latestResult.success
  }).length
}

const calculateFailingSuitesCount = (suites) => {
  return suites.filter(suite => {
    if (!suite.results || suite.results.length === 0) return false
    return !suite.results[suite.results.length - 1].success
  }).length
}

const toggleGroupCollapse = (groupName) => {
  if (uncollapsedGroups.value.has(groupName)) {
    uncollapsedGroups.value.delete(groupName)
  } else {
    uncollapsedGroups.value.add(groupName)
  }
  // Save to localStorage
  const uncollapsed = Array.from(uncollapsedGroups.value)
  writePreference('uncollapsed-groups', JSON.stringify(uncollapsed))
  removePreference('collapsed-groups') // A key that older versions wrote: removed under both prefixes
}

const initializeCollapsedGroups = () => {
  // Get saved uncollapsed groups from localStorage
  try {
    const saved = readPreference('uncollapsed-groups')
    if (saved) {
      uncollapsedGroups.value = new Set(JSON.parse(saved))
    }
    // If no saved state, uncollapsedGroups stays empty (all collapsed by default)
  } catch (e) {
    console.warn('Failed to parse saved uncollapsed groups:', e)
    removePreference('uncollapsed-groups')
    // On error, uncollapsedGroups stays empty (all collapsed by default)
  }
}

const dashboardHeading = computed(() => {
  return window.config && window.config.dashboardHeading && window.config.dashboardHeading !== '{{ .UI.DashboardHeading }}' ? window.config.dashboardHeading : "Health Dashboard"
})

const dashboardSubheading = computed(() => {
  return window.config && window.config.dashboardSubheading && window.config.dashboardSubheading !== '{{ .UI.DashboardSubheading }}' ? window.config.dashboardSubheading : "Monitor the health of your endpoints in real-time"
})

onMounted(() => {
  fetchData()
})
</script>