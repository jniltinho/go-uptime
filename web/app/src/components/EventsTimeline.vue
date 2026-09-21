<template>
  <!-- Fork: events of an endpoint, collapsed by default like the Checks table, on the dashboard and on the public status pages -->
  <div>
    <button
      type="button"
      :aria-expanded="expanded"
      aria-controls="endpoint-events-list"
      data-testid="events-toggle"
      class="flex w-full items-center gap-2 border px-3 py-2 text-left text-sm font-medium text-foreground hover:bg-accent dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
      @click="toggle"
    >
      <ChevronDown v-if="expanded" class="h-4 w-4" />
      <ChevronRight v-else class="h-4 w-4" />
      Events
      <span class="font-normal text-muted-foreground dark:text-gray-400">({{ items.length }})</span>
      <span v-if="latest" class="ml-auto truncate text-xs font-normal text-muted-foreground dark:text-gray-400" data-testid="events-latest">Latest: {{ latest.text }} · {{ latest.timeAgo }}</span>
    </button>
    <ul v-if="expanded" id="endpoint-events-list" class="space-y-4 border border-t-0 px-4 py-4 dark:border-gray-700" data-testid="events-list">
      <li v-for="item in items" :key="item.key" class="flex items-start gap-4 border-b pb-4 last:border-0 last:pb-0 dark:border-gray-800">
        <div class="mt-1" aria-hidden="true">
          <ArrowUpCircle v-if="item.type === 'HEALTHY'" class="h-5 w-5 text-green-500" />
          <ArrowDownCircle v-else-if="item.type === 'UNHEALTHY'" class="h-5 w-5 text-red-500" />
          <PlayCircle v-else class="h-5 w-5 text-muted-foreground" />
        </div>
        <div class="flex-1">
          <p class="font-medium">{{ item.text }}</p>
          <p class="text-sm text-muted-foreground">{{ item.dateTime }} • {{ item.timeAgo }}</p>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ArrowDownCircle, ArrowUpCircle, ChevronDown, ChevronRight, PlayCircle } from 'lucide-vue-next'
import { readPreference, writePreference } from '@/utils/storage'

const props = defineProps({
  // Events from the most recent to the oldest: { key, type, text, dateTime, timeAgo }
  items: { type: Array, default: () => [] }
})

const PREFERENCE = 'show-events'

const readExpanded = () => {
  try {
    return readPreference(PREFERENCE) === 'true'
  } catch (e) {
    return false
  }
}

const expanded = ref(readExpanded())

const latest = computed(() => props.items[0] || null)

const toggle = () => {
  expanded.value = !expanded.value
  try {
    writePreference(PREFERENCE, expanded.value ? 'true' : 'false')
  } catch (e) {
    // The events keep working without the browser storage
  }
}
</script>
