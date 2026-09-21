<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: chart in the format of the monitor page of Uptime Kuma, see utils/responseTimeChart.js -->
  <div class="w-full">
    <!-- The height of the Kuma belongs to this element, which also carries the attributes read by the tests: the
         legend is a sibling of it, never a child -->
    <div
      class="relative w-full"
      :style="{ height: `${height}px` }"
      data-testid="response-time-chart"
      :data-period="loadedPeriod"
      :data-loading="loading ? 'true' : 'false'"
      :data-line-points="summary.linePoints"
      :data-down-columns="summary.downColumns"
      :data-pending-columns="summary.pendingColumns"
    >
      <div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-background/50">
        <Loading />
      </div>
      <div v-else-if="error" class="absolute inset-0 flex items-center justify-center text-muted-foreground">
        {{ error }}
      </div>
      <Line v-else-if="series" :data="chartData" :options="chartOptions" />
    </div>
    <!-- Fork: the Kuma has no legend, but three lines of almost the same green need a name and a stroke of their own -->
    <ul
      v-if="legend.length"
      role="list"
      aria-label="Chart series"
      class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground"
      data-testid="response-time-chart-legend"
    >
      <li v-for="item in legend" :key="item.id" class="flex items-center gap-1.5" :data-series="item.id">
        <span
          v-if="item.column"
          aria-hidden="true"
          class="inline-block h-2.5 w-2.5 border border-muted-foreground"
          :style="{ backgroundColor: item.color }"
        ></span>
        <span
          v-else
          aria-hidden="true"
          class="inline-block w-3.5"
          :style="{ borderTopWidth: '2px', borderTopStyle: item.dash, borderTopColor: item.color }"
        ></span>
        {{ item.label }}
      </li>
    </ul>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, BarController, BarElement, Filler, LinearScale, LineController, LineElement, PointElement, TimeScale, Tooltip } from 'chart.js'
import 'chartjs-adapter-date-fns'
import { PROTECTED_API_HEADERS, notifyUnauthorized } from '@/utils/auth'
import { CHART_COLORS, CHART_LINE_DASHES, RECENT_PERIOD, bucketDatasets, chartLegend, chartSummary, isChartPeriod, recentDatasets } from '@/utils/responseTimeChart'
import Loading from './Loading.vue'

ChartJS.register(LineController, BarController, LineElement, BarElement, PointElement, LinearScale, TimeScale, Tooltip, Filler)

// Fork: the chart draws its labels on a canvas with the default stack of Chart.js, which would be the only block of the
// interface outside of the font of the theme
if (typeof document !== 'undefined') {
  const bodyFont = getComputedStyle(document.body).fontFamily
  if (bodyFont) {
    ChartJS.defaults.font.family = bodyFont
  }
}

const props = defineProps({
  // Base of the route of the chart, protected or public, without the query
  chartUrl: {
    type: String,
    required: true
  },
  period: {
    type: String,
    default: RECENT_PERIOD,
    validator: (value) => isChartPeriod(value)
  },
  // Changes with the data of the page (the timestamp of the latest result), so that the chart is fetched again
  refreshKey: {
    type: [String, Number],
    default: null
  },
  // Public route of a status page: fetched without credentials
  publicRoute: {
    type: Boolean,
    default: false
  }
})

// The aggregates are not fetched again more often than this
const AGGREGATES_REFRESH_INTERVAL_MS = 60000

const loading = ref(true)
const error = ref(null)
const payload = ref(null)
// Fork: the colours of the chart come from CSS variables of the theme (--chart-*, in index.css), read again whenever
// the class of <html> changes. A boolean "is it dark" cannot tell the light theme from the bio one.
const themeClass = ref(document.documentElement.className)
const CHART_COLOR_FALLBACKS = { '--chart-grid': 'rgba(0,0,0,0.1)', '--chart-text': 'rgba(12,12,18,1.0)', '--chart-tick': '#6b7280', '--chart-tooltip': 'rgba(212,232,222,1.0)' }
const chartColors = computed(() => {
  // The dependency on themeClass makes this recompute when the theme changes
  const style = themeClass.value !== undefined ? getComputedStyle(document.documentElement) : null
  const read = (name) => (style && style.getPropertyValue(name).trim()) || CHART_COLOR_FALLBACKS[name]
  return { grid: read('--chart-grid'), text: read('--chart-text'), tick: read('--chart-tick'), tooltip: read('--chart-tooltip') }
})
const windowWidth = ref(window.innerWidth)

// Height of Uptime Kuma, by the width of the window instead of the width of the screen
const height = computed(() => {
  if (windowWidth.value < 576) {
    return 275
  }
  if (windowWidth.value < 768) {
    return 320
  }
  if (windowWidth.value < 992) {
    return 300
  }
  return 250
})

const loadedPeriod = computed(() => (payload.value ? payload.value.period : ''))

const series = computed(() => {
  const data = payload.value
  if (!data) {
    return null
  }
  if (data.period === RECENT_PERIOD) {
    return recentDatasets(data.results, data.intervalSeconds)
  }
  return bucketDatasets(data.buckets, data.period, data.intervalSeconds)
})

const summary = computed(() => chartSummary(series.value))

// The legend is hidden while the chart is loading or in error, so that it never describes what is not drawn
const legend = computed(() => {
  if (loading.value || error.value || !payload.value || !series.value) {
    return []
  }
  return chartLegend(payload.value.period, summary.value)
})

const barDataset = (data) => ({
  type: 'bar',
  data: data.bars,
  borderColor: CHART_COLORS.none,
  backgroundColor: data.barColors,
  yAxisID: 'y1',
  barThickness: 'flex',
  barPercentage: 1,
  categoryPercentage: 1,
  inflateAmount: 0.05,
  label: 'status'
})

const lineDataset = (data, label, borderColor, backgroundColor, borderDash = []) => ({
  data,
  fill: 'origin',
  tension: 0.2,
  borderColor,
  backgroundColor,
  borderDash,
  yAxisID: 'y',
  label
})

const chartData = computed(() => {
  const data = series.value
  if (!data) {
    return { datasets: [] }
  }
  if (payload.value.period === RECENT_PERIOD) {
    return {
      datasets: [
        lineDataset(data.line, 'ping', CHART_COLORS.line, CHART_COLORS.recentFill),
        barDataset(data)
      ]
    }
  }
  return {
    datasets: [
      lineDataset(data.line, 'avg-ping', CHART_COLORS.line, CHART_COLORS.bucketFill, CHART_LINE_DASHES.average),
      lineDataset(data.min, 'min-ping', CHART_COLORS.minLine, CHART_COLORS.bucketFill, CHART_LINE_DASHES.minimum),
      lineDataset(data.max, 'max-ping', CHART_COLORS.maxLine, CHART_COLORS.bucketFill, CHART_LINE_DASHES.maximum),
      barDataset(data)
    ]
  }
})

const numberFormat = new Intl.NumberFormat()

const chartOptions = computed(() => {
  const gridColor = chartColors.value.grid
  const textColor = chartColors.value.text
  const tickColor = chartColors.value.tick
  return {
    responsive: true,
    maintainAspectRatio: false,
    layout: {
      padding: {
        left: 10,
        right: 30,
        top: 30,
        bottom: 10
      }
    },
    elements: {
      point: {
        // Points hidden unless hovered
        radius: 0,
        hitRadius: 100
      }
    },
    scales: {
      x: {
        type: 'time',
        time: {
          minUnit: 'minute',
          round: 'second',
          tooltipFormat: 'yyyy-MM-dd HH:mm:ss',
          displayFormats: {
            minute: 'HH:mm',
            hour: 'MM-dd HH:mm'
          }
        },
        ticks: {
          sampleSize: 3,
          maxRotation: 0,
          autoSkipPadding: 30,
          padding: 3,
          color: tickColor
        },
        grid: {
          color: gridColor,
          offset: false
        }
      },
      y: {
        title: {
          display: true,
          text: 'Resp. Time (ms)',
          color: tickColor
        },
        offset: false,
        ticks: {
          color: tickColor
        },
        grid: {
          color: gridColor
        }
      },
      y1: {
        display: false,
        position: 'right',
        grid: {
          drawOnChartArea: false
        },
        min: 0,
        max: 1,
        offset: false
      }
    },
    bounds: 'ticks',
    plugins: {
      tooltip: {
        mode: 'nearest',
        intersect: false,
        padding: 10,
        backgroundColor: chartColors.value.tooltip,
        bodyColor: textColor,
        titleColor: textColor,
        // Only the line of the response time, not the columns nor the minimum and maximum
        filter: (tooltipItem) => tooltipItem.datasetIndex === 0,
        callbacks: {
          label: (context) => ` ${numberFormat.format(context.parsed.y)} ms`
        }
      },
      legend: {
        display: false
      }
    }
  }
})

// Generation of the requests, so that a response of an older request does not replace a newer one
let requestGeneration = 0
let fetching = false
// A refresh asked during a fetch, done when it ends
let pendingRefresh = false
// When the chart was last fetched, and the fetch of the aggregates scheduled for the end of the interval
let lastFetchAt = 0
let refreshTimer = null

const cancelScheduledRefresh = () => {
  clearTimeout(refreshTimer)
  refreshTimer = null
}

// fetchChart fetches the data of the chart. With silent, used by the refreshes, the spinner is not shown and the chart
// is kept as it is while fetching, and also when the fetch fails.
const fetchChart = async ({ silent = false } = {}) => {
  const generation = ++requestGeneration
  cancelScheduledRefresh()
  fetching = true
  pendingRefresh = false
  lastFetchAt = Date.now()
  if (!silent) {
    loading.value = true
    error.value = null
  }
  // Fork: same-origin on the public routes, so that the credential of a page with a login goes with the request
  const options = props.publicRoute ? { credentials: 'same-origin' } : { credentials: 'include', headers: PROTECTED_API_HEADERS }
  try {
    const response = await fetch(`${props.chartUrl}?period=${encodeURIComponent(props.period)}`, options)
    if (generation !== requestGeneration) {
      return
    }
    if (response.status === 401) {
      if (!props.publicRoute) {
        notifyUnauthorized()
      }
      // Fork: on a page with a login of its own, the credential has to be given again on a new navigation
      error.value = props.publicRoute ? 'Reload the page to sign in again' : 'Failed to load chart data'
      return
    }
    if (response.status !== 200) {
      if (!silent) {
        error.value = 'Failed to load chart data'
      }
      console.error('[ResponseTimeChart] Error:', await response.text())
      return
    }
    const data = await response.json()
    if (generation !== requestGeneration) {
      return
    }
    payload.value = data
    error.value = null
  } catch (err) {
    if (generation !== requestGeneration) {
      return
    }
    if (!silent) {
      error.value = 'Failed to load chart data'
    }
    console.error('[ResponseTimeChart] Error:', err)
  } finally {
    if (generation === requestGeneration) {
      fetching = false
      loading.value = false
      if (pendingRefresh) {
        pendingRefresh = false
        refresh()
      }
    }
  }
}

// refresh fetches the chart again without the spinner: right away in Recent, and at most once every
// AGGREGATES_REFRESH_INTERVAL_MS in the other periods, with the fetch scheduled for the end of the interval
const refresh = () => {
  if (fetching) {
    pendingRefresh = true
    return
  }
  if (props.period === RECENT_PERIOD) {
    fetchChart({ silent: true })
    return
  }
  if (refreshTimer !== null) {
    return
  }
  const waitMs = lastFetchAt + AGGREGATES_REFRESH_INTERVAL_MS - Date.now()
  if (waitMs <= 0) {
    fetchChart({ silent: true })
    return
  }
  refreshTimer = setTimeout(() => {
    refreshTimer = null
    refresh()
  }, waitMs)
}

// Another period or endpoint: the chart is cleared and the spinner is shown
watch(() => [props.chartUrl, props.period], ([url, period], [previousUrl, previousPeriod]) => {
  if (url === previousUrl && period === previousPeriod) {
    return
  }
  payload.value = null
  fetchChart()
})

watch(() => props.refreshKey, (value, previous) => {
  if (value === previous) {
    return
  }
  refresh()
})

const onResize = () => {
  windowWidth.value = window.innerWidth
}

let themeObserver = null

onMounted(() => {
  fetchChart()
  window.addEventListener('resize', onResize)
  themeObserver = new MutationObserver(() => {
    themeClass.value = document.documentElement.className
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onUnmounted(() => {
  requestGeneration++
  pendingRefresh = false
  cancelScheduledRefresh()
  window.removeEventListener('resize', onResize)
  themeObserver?.disconnect()
})
</script>
