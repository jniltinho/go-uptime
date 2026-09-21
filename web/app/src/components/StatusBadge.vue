<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <Badge :variant="variant" class="flex items-center gap-1">
    <span :class="['w-2 h-2 rounded-full', dotClass]"></span>
    {{ label }}
  </Badge>
</template>

<script setup>
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'

const props = defineProps({
  status: {
    type: String,
    required: true,
    validator: (value) => ['healthy', 'unhealthy', 'degraded', 'pending', 'unknown'].includes(value)
  }
})

const variant = computed(() => {
  switch (props.status) {
    case 'healthy':
      return 'success'
    case 'unhealthy':
      return 'destructive'
    case 'degraded':
      return 'warning'
    case 'pending':
      return 'pending'
    default:
      return 'secondary'
  }
})

const label = computed(() => {
  switch (props.status) {
    case 'healthy':
      return 'Healthy'
    case 'unhealthy':
      return 'Unhealthy'
    case 'degraded':
      return 'Degraded'
    case 'pending':
      return 'Pending'
    default:
      return 'Unknown'
  }
})

const dotClass = computed(() => {
  switch (props.status) {
    case 'healthy':
      return 'bg-green-400'
    case 'unhealthy':
      return 'bg-red-400'
    case 'degraded':
      return 'bg-yellow-400'
    case 'pending':
      return 'bg-yellow-700'
    default:
      return 'bg-gray-400'
  }
})
</script>