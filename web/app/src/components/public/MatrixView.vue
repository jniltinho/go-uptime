<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: the matrix view of a public status page: one row per group, one column per application -->
<template>
  <div class="mt-4" data-testid="status-view-matrix">
    <p
      v-if="model.rows.length === 0 || model.columns.length === 0"
      class="py-8 text-center text-sm text-muted-foreground"
      data-testid="status-view-empty"
    >
      No services match the filters.
    </p>
    <div v-else class="overflow-x-auto border bg-card dark:border-gray-800">
      <table class="w-full border-collapse text-sm">
        <caption class="sr-only">
          Status of each application by
          {{
            groupLabel.toLowerCase()
          }}
        </caption>
        <thead class="text-xs text-muted-foreground">
          <tr class="border-b dark:border-gray-800">
            <th scope="col" class="sticky left-0 z-10 bg-card px-4 py-2 text-left font-normal">{{ groupLabel }}</th>
            <th
              v-for="column in model.columns"
              :key="`col-${column}`"
              scope="col"
              class="min-w-[8.5rem] px-1.5 py-2 text-center font-medium text-foreground"
            >
              {{ column }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in model.rows"
            :key="`row-${row.name}`"
            class="border-t dark:border-gray-800"
            :data-testid="`status-matrix-row-${row.name || 'outros'}`"
          >
            <th scope="row" class="sticky left-0 z-10 bg-card px-4 py-2 text-left align-middle font-normal">
              <span class="block font-semibold">{{ row.name || 'Other services' }}</span>
              <span v-if="row.description || row.tags.length" class="block text-xs text-muted-foreground">{{
                [row.description, row.tags.join(' · ')].filter(Boolean).join(' · ')
              }}</span>
            </th>
            <td
              v-for="cell in row.cells"
              :key="`cell-${row.name}-${cell.application}`"
              class="px-1.5 py-1.5 align-middle"
              :data-testid="`status-matrix-cell-${row.name || 'outros'}-${cell.application}`"
            >
              <component
                :is="cell.endpoints.length === 1 ? RouterLink : 'div'"
                :to="cell.endpoints.length === 1 ? detailsRoute(cell.endpoints[0]) : undefined"
                :class="[
                  'flex min-h-[3.5rem] flex-col items-center justify-center border px-2 py-1.5 text-center',
                  cellClass(cell),
                  cell.endpoints.length === 1
                    ? 'hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:hover:brightness-125'
                    : '',
                ]"
                :data-status="cell.kind === 'cell' ? cell.status : cell.kind"
              >
                <template v-if="cell.kind === 'cell'">
                  <span class="flex items-center gap-1.5 font-semibold">
                    <span :class="['inline-block h-2 w-2 rounded-full', dotClass(cell.status)]" aria-hidden="true"></span
                    >{{ formatUptime(cell.uptime) }}
                  </span>
                  <span class="text-xs">{{ statusLabel(cell.status) }}</span>
                  <span v-if="cell.environments.length" class="text-[10px] font-medium tracking-wide text-muted-foreground">{{
                    cell.environments.join(' · ')
                  }}</span>
                </template>
                <template v-else>
                  <span class="text-muted-foreground" aria-hidden="true">—</span>
                  <span class="text-xs text-muted-foreground">{{ emptyLabel(cell.kind) }}</span>
                </template>
              </component>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="mt-3 text-xs text-muted-foreground">
      Each cell shows the status of the endpoints of the application in the filtered environments and the 30-day uptime of
      {{ primaryName ? primaryName : 'the endpoints' }}{{ primaryName ? ' (or of the first environment available)' : '' }}.
    </p>
    <ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground" aria-label="Legend">
      <li v-for="item in LEGEND" :key="item.label" class="flex items-center gap-1.5">
        <span :class="['inline-block h-2.5 w-2.5', item.class]" aria-hidden="true"></span>{{ item.label }}
      </li>
    </ul>
  </div>
</template>

<script setup>
import { RouterLink } from 'vue-router'
import { endpointKey, formatUptime, STATUS_LABELS } from '@/utils/statusPage'
import { CELL_NOT_IN_ENVIRONMENT, CELL_NOT_SHOWN } from '@/utils/statusPageViews'

const props = defineProps({
  model: { type: Object, required: true },
  slug: { type: String, required: true },
  groupLabel: { type: String, default: 'Group' },
  primaryName: { type: String, default: '' },
})

const LEGEND = [
  { label: 'Operational', class: 'bg-green-500' },
  { label: 'Partial outage', class: 'bg-amber-500' },
  { label: 'Major outage', class: 'bg-red-500' },
  { label: 'No data', class: 'bg-statusgray-400' },
]

const detailsRoute = (endpoint) => ({
  name: 'PublicStatusPageEndpoint',
  params: { slug: props.slug, key: endpointKey(endpoint.group, endpoint.name) },
})

const statusLabel = (status) => STATUS_LABELS[status]?.group || STATUS_LABELS.unknown.group

const emptyLabel = (kind) =>
  ({ [CELL_NOT_IN_ENVIRONMENT]: 'Not in this environment', [CELL_NOT_SHOWN]: 'Not shown' })[kind] || 'Not configured'

const dotClass = (status) => ({ operational: 'bg-green-500', degraded: 'bg-amber-500', down: 'bg-red-500' })[status] || 'bg-statusgray-400'

const cellClass = (cell) => {
  if (cell.kind !== 'cell') {
    return 'border-dashed border-gray-300 dark:border-gray-700'
  }
  return (
    {
      operational: 'border-green-300 bg-green-50 text-green-900 dark:border-green-800 dark:bg-green-900/30 dark:text-green-100',
      degraded: 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100',
      down: 'border-red-300 bg-red-50 text-red-900 dark:border-red-800 dark:bg-red-900/30 dark:text-red-100',
    }[cell.status] ||
    'border-statusgray-300 bg-statusgray-50 text-statusgray-800 dark:border-statusgray-700 dark:bg-statusgray-900 dark:text-statusgray-200'
  )
}
</script>
