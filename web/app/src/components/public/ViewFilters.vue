<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: filters of the grouped and matrix views: environment and application chips, and only the groups with problems -->
<template>
  <div class="mt-4 space-y-2 text-sm" data-testid="status-view-filters">
    <div v-if="environments.length" class="flex flex-wrap items-center gap-1.5" role="group" aria-label="Environment">
      <span class="w-24 shrink-0 text-xs text-muted-foreground">Environment</span>
      <button
        v-for="option in environmentOptions"
        :key="`env-${option.id}`"
        type="button"
        :aria-pressed="modelValue.environment === option.id"
        :class="chipClass(modelValue.environment === option.id)"
        :data-testid="`status-filter-environment-${option.id || 'all'}`"
        @click="update({ environment: option.id })"
      >
        {{ option.label }}
      </button>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      <div v-if="applications.length > 1" class="flex flex-1 flex-wrap items-center gap-1.5" role="group" aria-label="Application">
        <span class="w-24 shrink-0 text-xs text-muted-foreground">Application</span>
        <button
          v-for="option in applicationOptions"
          :key="`app-${option.id}`"
          type="button"
          :aria-pressed="modelValue.application === option.id"
          :class="chipClass(modelValue.application === option.id)"
          :data-testid="`status-filter-application-${option.id || 'all'}`"
          @click="update({ application: option.id })"
        >
          {{ option.label }}
        </button>
      </div>
      <label class="ml-auto flex shrink-0 cursor-pointer items-center gap-2 text-xs text-fg-muted">
        <input
          type="checkbox"
          class="h-4 w-4 accent-check"
          :checked="modelValue.onlyProblems"
          data-testid="status-filter-only-problems"
          @change="update({ onlyProblems: $event.target.checked })"
        />
        Only {{ groupLabel.toLowerCase() }}s with problems
      </label>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  modelValue: { type: Object, required: true },
  environments: { type: Array, default: () => [] },
  applications: { type: Array, default: () => [] },
  groupLabel: { type: String, default: 'Group' },
})

const emit = defineEmits(['update:modelValue'])

const environmentOptions = computed(() => [
  { id: '', label: 'All' },
  ...props.environments.map((environment) => ({ id: environment.id, label: environment.name })),
])
const applicationOptions = computed(() => [
  { id: '', label: 'All' },
  ...props.applications.map((application) => ({ id: application, label: application })),
])

const update = (change) => emit('update:modelValue', { ...props.modelValue, ...change })

const chipClass = (active) => [
  'border px-3 py-0.5 text-xs font-medium focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-focus',
  active
    ? 'border-sky-300 bg-sky-100 text-sky-900 dark:border-sky-700 dark:bg-sky-900/40 dark:text-sky-100'
    : 'border-field text-fg hover:bg-muted dark:hover:bg-gray-800',
]
</script>
