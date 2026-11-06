<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: compact history of the latest results of an endpoint, for the rows of the grouped view. It is decorative: the
     row carries the status and the uptimes in text, and the full history is on the details page. -->
<template>
  <div class="flex h-4 gap-px" aria-hidden="true">
    <span v-for="(result, index) in displayed" :key="index" :class="['flex-1', barClass(result)]" :title="result ? title(result) : undefined"></span>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { formatDateTime } from '@/utils/statusPage'

const props = defineProps({
  results: { type: Array, default: () => [] },
  bars: { type: Number, default: 30 }
})

const displayed = computed(() => {
  const results = (props.results || []).slice(-props.bars)
  return [...Array(props.bars - results.length).fill(null), ...results]
})

const barClass = (result) => {
  if (!result) return 'bg-gray-200 dark:bg-gray-800'
  if (result.success) return 'bg-green-500'
  if (result.pending) return 'bg-yellow-400'
  return 'bg-red-500'
}

const title = (result) => `${formatDateTime(result.timestamp)} · ${result.success ? 'Success' : (result.pending ? 'Pending' : 'Failure')} · ${result.durationMs} ms`
</script>
