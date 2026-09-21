<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div id="settings" class="fixed bottom-4 left-4 z-50">
    <div class="flex items-center gap-1 bg-background/95 backdrop-blur-sm border rounded-none shadow-md p-1">
      <!-- Refresh Rate -->
      <button 
        @click="showRefreshMenu = !showRefreshMenu"
        :aria-label="`Refresh interval: ${formatRefreshInterval(refreshIntervalValue)}`"
        :aria-expanded="showRefreshMenu"
        class="flex items-center gap-1.5 px-3 py-1.5 rounded-none hover:bg-accent transition-colors relative"
      >
        <RefreshCw class="w-3.5 h-3.5 text-muted-foreground" />
        <span class="text-xs font-medium">{{ formatRefreshInterval(refreshIntervalValue) }}</span>
        
        <!-- Refresh Rate Dropdown -->
        <div 
          v-if="showRefreshMenu"
          @click.stop
          class="absolute bottom-full left-0 mb-2 bg-popover border rounded-lg shadow-lg overflow-hidden"
        >
          <button
            v-for="interval in REFRESH_INTERVALS"
            :key="interval.value"
            @click="selectRefreshInterval(interval.value)"
            :class="[
              'block w-full px-4 py-2 text-xs text-left hover:bg-accent transition-colors',
              refreshIntervalValue === interval.value && 'bg-accent'
            ]"
          >
            {{ interval.label }}
          </button>
        </div>
      </button>

      <!-- Divider -->
      <div class="h-5 w-px bg-border/50" />

      <!-- Theme selector (fork): three themes, see ThemeSelector.vue -->
      <ThemeSelector testid="settings-theme-toggle" compact placement="top-start" />
    </div>
  </div>
</template>


<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
// Fork: the theme rule (cookie, otherwise the default theme of the server) lives in utils/theme.js
import ThemeSelector from '@/components/ThemeSelector.vue'
import { applyTheme as applyDocumentTheme, currentTheme } from '@/utils/theme'
import { readPreference, writePreference } from '@/utils/storage'

const emit = defineEmits(['refreshData'])

// Constants
const REFRESH_INTERVALS = [
  { value: '10', label: '10s' },
  { value: '30', label: '30s' },
  { value: '60', label: '1m' },
  { value: '120', label: '2m' },
  { value: '300', label: '5m' },
  { value: '600', label: '10m' }
]
const DEFAULT_REFRESH_INTERVAL = '300'
const STORAGE_KEYS = {
  REFRESH_INTERVAL: 'refresh-interval'
}

// Helper functions
function getStoredRefreshInterval() {
  const stored = readPreference(STORAGE_KEYS.REFRESH_INTERVAL)
  const parsedValue = stored && parseInt(stored)
  const isValid = parsedValue && parsedValue >= 10 && REFRESH_INTERVALS.some(i => i.value === stored)
  return isValid ? stored : DEFAULT_REFRESH_INTERVAL
}

// State
const refreshIntervalValue = ref(getStoredRefreshInterval())
const showRefreshMenu = ref(false)
let refreshIntervalHandler = null

// Methods
const formatRefreshInterval = (value) => {
  const interval = REFRESH_INTERVALS.find(i => i.value === value)
  return interval ? interval.label : `${value}s`
}

const setRefreshInterval = (seconds) => {
  writePreference(STORAGE_KEYS.REFRESH_INTERVAL, seconds)
  if (refreshIntervalHandler) {
    clearInterval(refreshIntervalHandler)
  }
  refreshIntervalHandler = setInterval(() => {
    refreshData()
  }, seconds * 1000)
}

const refreshData = () => {
  emit('refreshData')
}

const selectRefreshInterval = (value) => {
  refreshIntervalValue.value = value
  showRefreshMenu.value = false
  refreshData()
  setRefreshInterval(value)
}

// Close menu when clicking outside
const handleClickOutside = (event) => {
  const settings = document.getElementById('settings')
  if (settings && !settings.contains(event.target)) {
    showRefreshMenu.value = false
  }
}

const applyTheme = () => {
  applyDocumentTheme(currentTheme())
}

// Lifecycle
onMounted(() => {
  setRefreshInterval(refreshIntervalValue.value)
  applyTheme()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  if (refreshIntervalHandler) {
    clearInterval(refreshIntervalHandler)
  }
  document.removeEventListener('click', handleClickOutside)
})
</script>


<style scoped>
/* Animations for smooth transitions */
@keyframes slideIn {
  from {
    transform: translateX(-20px);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

#settings {
  animation: slideIn 0.3s ease-out;
}

#settings > div {
  transition: all 0.2s ease;
}

#settings > div:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1);
}
</style>
