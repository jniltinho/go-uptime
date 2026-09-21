<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <Card data-testid="details-summary">
    <dl class="grid grid-cols-2 gap-x-4 gap-y-5 p-6 md:grid-cols-5">
      <div v-for="(item, index) in items" :key="item.label" :data-testid="`details-summary-${index}`" class="min-w-0">
        <dt class="text-sm font-medium text-muted-foreground dark:text-gray-400">{{ item.label }}</dt>
        <dd class="mt-1 text-2xl font-bold text-foreground dark:text-gray-100">{{ item.value }}</dd>
      </div>
    </dl>
  </Card>
</template>

<script setup>
// Panel with the numbers of the endpoint details pages, of the dashboard and of the public status pages (fork), in the
// place of the cards of the original Gatus and of the uptime badges
import { computed } from 'vue'
import { Card } from '@/components/ui/card'
import { detailsSummaryItems } from '@/utils/detailsSummary'

const props = defineProps({
  // Duration of the last result, in milliseconds
  currentResponseTime: { type: Number, default: null },
  // Ratios between 0 and 1 and averages in milliseconds, by period (24h, 7d and 30d), null without execution
  uptime: { type: Object, default: null },
  responseTime: { type: Object, default: null },
  // Push endpoints are labeled "Ping"
  push: { type: Boolean, default: false }
})

const items = computed(() => detailsSummaryItems(props))
</script>
