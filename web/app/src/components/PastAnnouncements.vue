<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div v-if="announcements && announcements.length" class="past-announcements">
    <h2 class="text-2xl font-semibold text-foreground mb-6">Past Announcements</h2>

    <div class="space-y-8">
      <div v-for="(group, date) in displayedAnnouncements" :key="date">
        <!-- Date Header -->
        <div class="mb-3">
          <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            {{ formatDate(date) }}
          </h3>
        </div>

        <!-- Announcements for this date or empty state -->
        <div v-if="group.length > 0" class="space-y-3">
          <div
            v-for="(announcement, index) in group"
            :key="`${date}-${index}-${announcement.timestamp}`"
            :class="[
              'border-l-4 p-4 transition-all duration-200',
              getTypeClasses(announcement.type).background,
              getTypeClasses(announcement.type).borderColor,
            ]"
          >
            <div class="flex items-start gap-3">
              <component
                :is="getTypeIcon(announcement.type)"
                :class="['w-5 h-5 shrink-0 mt-0.5', getTypeClasses(announcement.type).iconColor]"
              />
              <time
                :class="['text-sm font-mono whitespace-nowrap shrink-0 mt-0.5', getTypeClasses(announcement.type).text]"
                :title="formatFullTimestamp(announcement.timestamp)"
              >
                {{ formatTime(announcement.timestamp) }}
              </time>
              <div class="flex-1 min-w-0">
                <p class="text-sm leading-relaxed text-fg-strong" v-html="formatAnnouncementMessage(announcement.message)"></p>
              </div>
            </div>
          </div>
        </div>

        <!-- Empty state for dates without announcements -->
        <div v-else class="py-2">
          <p class="text-sm italic text-muted-foreground/60">No incidents reported on this day</p>
        </div>
      </div>

      <!-- View Older Announcements Link -->
      <div v-if="hasOlderAnnouncements && !showAllAnnouncements">
        <button
          @click="showAllAnnouncements = true"
          class="inline-flex items-center gap-2 text-sm font-medium text-info hover:text-info transition-colors duration-200 cursor-pointer group"
        >
          <ChevronDown class="w-4 h-4 group-hover:translate-y-0.5 transition-transform duration-200" />
          <span class="group-hover:underline">View older announcements</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { XCircle, AlertTriangle, Info, CheckCircle, Circle, ChevronDown } from 'lucide-vue-next'
import { formatAnnouncementMessage } from '@/utils/markdown'

// Props
const props = defineProps({
  announcements: {
    type: Array,
    default: () => [],
  },
})

// State
const showAllAnnouncements = ref(false)

// Type configurations (consistent with AnnouncementBanner)
const typeConfigs = {
  outage: {
    icon: XCircle,
    background: 'bg-danger-soft',
    borderColor: 'border-danger-strong',
    iconColor: 'text-danger',
    text: 'text-danger',
  },
  warning: {
    icon: AlertTriangle,
    background: 'bg-pending-soft',
    borderColor: 'border-pending-strong',
    iconColor: 'text-pending',
    text: 'text-pending',
  },
  information: {
    icon: Info,
    background: 'bg-info-soft',
    borderColor: 'border-info-strong',
    iconColor: 'text-info',
    text: 'text-info',
  },
  operational: {
    icon: CheckCircle,
    background: 'bg-success-soft',
    borderColor: 'border-success-strong',
    iconColor: 'text-success',
    text: 'text-success',
  },
  none: {
    icon: Circle,
    background: 'bg-surface-2',
    borderColor: 'border-line-strong',
    iconColor: 'text-fg-muted',
    text: 'text-fg-body',
  },
}

// Helper to normalize date to start of day
const normalizeDate = (date) => {
  const normalized = new Date(date)
  normalized.setHours(0, 0, 0, 0)
  return normalized
}

// Computed properties
const displayedAnnouncements = computed(() => {
  if (!props.announcements?.length) return {}

  // Group announcements by date and find oldest
  const grouped = {}
  let oldest = new Date()

  props.announcements.forEach((announcement) => {
    const date = new Date(announcement.timestamp)
    const key = date.toDateString()
    grouped[key] = grouped[key] || []
    grouped[key].push(announcement)
    if (date < oldest) oldest = date
  })

  // Calculate date range
  const today = normalizeDate(new Date())
  const endDate = showAllAnnouncements.value ? normalizeDate(oldest) : new Date(today.getTime() - 14 * 24 * 60 * 60 * 1000)

  // Build result: today (if has announcements) + yesterday backwards
  const result = {}
  const todayKey = today.toDateString()
  if (grouped[todayKey]) result[todayKey] = grouped[todayKey]

  for (let date = new Date(today.getTime() - 24 * 60 * 60 * 1000); date >= endDate; date.setDate(date.getDate() - 1)) {
    result[date.toDateString()] = grouped[date.toDateString()] || []
  }

  return result
})

// Check if there are announcements older than 14 days
const hasOlderAnnouncements = computed(() => {
  if (!props.announcements?.length) return false
  const fourteenDaysAgo = new Date(normalizeDate(new Date()).getTime() - 14 * 24 * 60 * 60 * 1000)
  return props.announcements.some((a) => new Date(a.timestamp) < fourteenDaysAgo)
})

// Helper functions
const getTypeIcon = (type) => {
  return typeConfigs[type]?.icon || Circle
}

const getTypeClasses = (type) => {
  return typeConfigs[type] || typeConfigs.none
}

const formatDate = (dateString) => {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

const formatTime = (timestamp) => {
  return new Date(timestamp).toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const formatFullTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  })
}
</script>
