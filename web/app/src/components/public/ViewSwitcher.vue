<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: segmented switcher of the views of a public status page, shown when the page enables two or more -->
<template>
  <div class="inline-flex shrink-0 border border-input bg-card p-0.5 dark:border-gray-700" role="group" aria-label="View" data-testid="status-view-switcher">
    <button
      v-for="view in views"
      :key="view"
      type="button"
      :aria-pressed="view === modelValue"
      :class="[
        'px-3 py-1 text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-blue-600 dark:focus-visible:outline-blue-400',
        view === modelValue ? 'bg-foreground text-background dark:bg-gray-100 dark:text-gray-900' : 'text-muted-foreground hover:text-foreground dark:text-gray-400 dark:hover:text-gray-100'
      ]"
      :data-testid="`status-view-switch-${view}`"
      @click="$emit('update:modelValue', view)"
    >{{ viewLabel(view, groupLabel) }}</button>
  </div>
</template>

<script setup>
import { viewLabel } from '@/utils/statusPageViews'

defineProps({
  views: { type: Array, required: true },
  modelValue: { type: String, required: true },
  groupLabel: { type: String, default: 'Group' }
})

defineEmits(['update:modelValue'])
</script>
