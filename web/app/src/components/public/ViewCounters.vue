<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: counters and incident banner of the grouped and matrix views, over the whole published page -->
<template>
  <div data-testid="status-view-counters">
    <dl class="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <div
        v-for="card in cards"
        :key="card.key"
        class="border bg-card px-3 py-2 border-line-soft"
        :data-testid="`status-counter-${card.key}`"
      >
        <dt class="text-xs text-muted-foreground">{{ card.label }}</dt>
        <dd :class="['text-xl font-semibold leading-tight', card.class]">{{ card.value }}</dd>
      </div>
    </dl>
    <div
      v-if="incidents > 0 || counts.pending > 0"
      :class="[
        'mt-3 flex flex-col gap-1 border px-3 py-2 text-sm sm:flex-row sm:items-center sm:justify-between',
        counts.primaryDown > 0
          ? 'border-danger bg-danger-soft text-red-900 dark:text-red-100'
          : 'border-warning bg-warning-soft text-warning-strong',
      ]"
      role="status"
      data-testid="status-view-banner"
    >
      <p class="flex items-center gap-2 font-medium">
        <span
          :class="['inline-block h-2.5 w-2.5 shrink-0 rounded-full', counts.primaryDown > 0 ? 'bg-red-500' : 'bg-amber-500']"
          aria-hidden="true"
        ></span>
        {{ bannerText }}
      </p>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  counts: { type: Object, required: true },
  groupLabel: { type: String, default: 'Group' },
  primaryName: { type: String, default: '' },
})

const plural = (count, noun) => `${count} ${noun.toLowerCase()}${count === 1 ? '' : 's'}`

const incidents = computed(() => props.counts.primaryDown + props.counts.otherDown)

const cards = computed(() => {
  const list = [
    { key: 'groups', label: `${props.groupLabel}s`, value: props.counts.groups, class: '' },
    { key: 'monitors', label: 'Monitors', value: props.counts.monitors, class: '' },
  ]
  const red = (value) => (value > 0 ? 'text-danger' : '')
  const amber = (value) => (value > 0 ? 'text-amber-600 dark:text-amber-400' : '')
  if (props.counts.hasEnvironments) {
    list.push({
      key: 'primary',
      label: `Incidents in ${props.primaryName}`,
      value: props.counts.primaryDown,
      class: red(props.counts.primaryDown),
    })
    list.push({
      key: 'other',
      label: `Incidents outside ${props.primaryName}`,
      value: props.counts.otherDown,
      class: amber(props.counts.otherDown),
    })
  } else {
    list.push({ key: 'incidents', label: 'Incidents', value: incidents.value, class: red(incidents.value) })
    list.push({
      key: 'affected',
      label: `Affected ${props.groupLabel.toLowerCase()}s`,
      value: props.counts.affectedGroups,
      class: amber(props.counts.affectedGroups),
    })
  }
  return list
})

const bannerText = computed(() => {
  const parts = []
  if (props.counts.hasEnvironments) {
    if (props.counts.primaryDown > 0)
      parts.push(`${props.counts.primaryDown} incident${props.counts.primaryDown === 1 ? '' : 's'} in ${props.primaryName}`)
    if (props.counts.otherDown > 0) parts.push(`${props.counts.otherDown} outside ${props.primaryName}`)
  } else if (incidents.value > 0) {
    parts.push(`${incidents.value} incident${incidents.value === 1 ? '' : 's'}`)
  }
  if (props.counts.pending > 0) parts.push(`${props.counts.pending} pending`)
  parts.push(`${plural(props.counts.affectedGroups, props.groupLabel)} affected`)
  return parts.join(' · ')
})
</script>
