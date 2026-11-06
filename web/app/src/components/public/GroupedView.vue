<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<!-- Fork: the grouped view of a public status page: one card per group, its endpoints split by environment -->
<template>
  <div class="mt-4 space-y-3" data-testid="status-view-grouped">
    <p v-if="groups.length === 0" class="py-8 text-center text-sm text-muted-foreground" data-testid="status-view-empty">
      No services match the filters.
    </p>
    <section
      v-for="(group, groupIndex) in groups"
      :key="`grouped:${group.name}`"
      class="border bg-card dark:border-gray-800"
      :aria-labelledby="`grouped-group-${groupIndex}`"
      :data-testid="`status-grouped-${group.name || 'outros'}`"
    >
      <h2 :id="`grouped-group-${groupIndex}`">
        <button
          type="button"
          class="flex w-full flex-wrap items-center gap-x-3 gap-y-1.5 px-4 py-2.5 text-left focus-visible:outline-solid focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-blue-600 dark:focus-visible:outline-blue-400"
          :aria-expanded="!collapsed.has(group.name)"
          :aria-controls="`grouped-panel-${groupIndex}`"
          :data-testid="`status-grouped-toggle-${group.name || 'outros'}`"
          @click="$emit('toggle', group.name)"
        >
          <component
            :is="collapsed.has(group.name) ? ChevronRight : ChevronDown"
            class="h-4 w-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span class="min-w-0">
            <span class="block truncate font-semibold">{{ group.name || 'Other services' }}</span>
            <span v-if="group.description" class="block truncate text-xs font-normal text-muted-foreground">{{ group.description }}</span>
          </span>
          <span class="ml-auto flex flex-wrap items-center justify-end gap-1.5 text-xs font-normal">
            <span
              v-for="tag in group.tags"
              :key="`tag-${tag}`"
              class="border px-1.5 py-px text-[11px] text-muted-foreground dark:border-gray-700"
              >{{ tag }}</span
            >
            <span
              v-for="short in group.environments"
              :key="`env-${short}`"
              class="border border-sky-300 px-1.5 py-px text-[11px] font-medium text-sky-800 dark:border-sky-800 dark:text-sky-300"
              >{{ short }}</span
            >
            <span
              v-for="application in group.applications"
              :key="`app-${application.name}`"
              :class="[
                'hidden items-center gap-1 border px-1.5 py-px text-[11px] md:inline-flex dark:border-gray-700',
                application.status === 'operational' ? '' : chipStatusClass(application.status),
              ]"
            >
              <span :class="['inline-block h-1.5 w-1.5 rounded-full', dotClass(application.status)]" aria-hidden="true"></span
              >{{ application.name }}
            </span>
            <span :class="['border px-2 py-px text-[11px] font-medium', statusBadgeClass(group.status)]">{{
              groupStatusLabel(group.status)
            }}</span>
            <span class="text-[11px] text-muted-foreground">{{ countsText(group) }}</span>
          </span>
        </button>
      </h2>
      <div :id="`grouped-panel-${groupIndex}`">
        <table v-if="!collapsed.has(group.name)" class="w-full border-t text-sm dark:border-gray-800">
          <thead class="text-xs text-muted-foreground">
            <tr class="border-b dark:border-gray-800">
              <th scope="col" class="w-32 px-4 py-1.5 text-left font-normal">Application</th>
              <th scope="col" class="px-2 py-1.5 text-left font-normal">Endpoint</th>
              <th scope="col" class="hidden w-[34%] px-2 py-1.5 text-left font-normal lg:table-cell">Latest checks</th>
              <th v-for="period in PERIODS" :key="period" scope="col" class="w-16 px-2 py-1.5 text-right font-normal last:pr-4">
                {{ period }}
              </th>
            </tr>
          </thead>
          <tbody v-for="section in group.sections" :key="`section-${section.id}`">
            <tr v-if="hasEnvironments" class="bg-muted/40 dark:bg-gray-800/40">
              <th colspan="7" scope="colgroup" class="px-4 py-1 text-left text-xs font-medium">
                <span
                  v-if="section.environment"
                  class="mr-1.5 border border-sky-300 px-1.5 py-px text-[11px] text-sky-800 dark:border-sky-800 dark:text-sky-300"
                  >{{ section.environment.short }}</span
                >
                <span>{{ section.environment ? section.environment.name : 'No environment' }}</span>
                <span class="ml-2 font-normal text-muted-foreground">{{ sectionCounts(section) }}</span>
              </th>
            </tr>
            <tr
              v-for="endpoint in section.endpoints"
              :key="`${endpoint.group}-${endpoint.name}`"
              class="border-t dark:border-gray-800"
              :data-testid="`status-grouped-endpoint-${endpoint.name}`"
            >
              <td class="px-4 py-1.5 align-middle">
                <span class="flex items-center gap-2 font-medium">
                  <span
                    :class="['inline-block h-2 w-2 shrink-0 rounded-full', endpointDotClass(endpoint.status)]"
                    aria-hidden="true"
                  ></span>
                  <span class="truncate">{{ endpoint.application || endpoint.name }}</span>
                </span>
              </td>
              <td class="max-w-0 px-2 py-1.5 align-middle">
                <RouterLink
                  :to="detailsRoute(endpoint)"
                  class="block truncate font-mono text-xs underline-offset-4 hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
                  :title="endpoint.name"
                  >{{ endpoint.name }}</RouterLink
                >
                <span class="block truncate text-[11px] text-muted-foreground">
                  <span :class="endpointTextClass(endpoint.status)">{{ endpointStatusLabel(endpoint.status) }}</span
                  ><template v-if="certificateDays(endpoint) !== null">
                    ·
                    <span :class="certificateClass(certificateDays(endpoint))">{{
                      certificateText(certificateDays(endpoint))
                    }}</span></template
                  >
                </span>
              </td>
              <td class="hidden px-2 py-1.5 align-middle lg:table-cell"><ResultBars :results="endpoint.results" :bars="30" /></td>
              <td v-for="period in PERIODS" :key="period" class="px-2 py-1.5 text-right align-middle text-xs font-medium last:pr-4">
                {{ formatUptime(endpoint.uptime[period]) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup>
import { RouterLink } from 'vue-router'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import ResultBars from '@/components/public/ResultBars.vue'
import { endpointKey, formatUptime, STATUS_LABELS } from '@/utils/statusPage'
import { certificateClass, certificateText } from '@/utils/certificate'

const PERIODS = ['24h', '7d', '30d']

const props = defineProps({
  groups: { type: Array, required: true },
  collapsed: { type: Set, required: true },
  slug: { type: String, required: true },
  hasEnvironments: { type: Boolean, default: false },
})

defineEmits(['toggle'])

const detailsRoute = (endpoint) => ({
  name: 'PublicStatusPageEndpoint',
  params: { slug: props.slug, key: endpointKey(endpoint.group, endpoint.name) },
})

const certificateDays = (endpoint) => (Number.isInteger(endpoint.certificateExpiresInDays) ? endpoint.certificateExpiresInDays : null)

const groupStatusLabel = (status) => STATUS_LABELS[status]?.group || STATUS_LABELS.unknown.group
const endpointStatusLabel = (status) => STATUS_LABELS[status]?.endpoint || STATUS_LABELS.unknown.endpoint

const countsText = (group) => [`${group.up} up`, `${group.down} down`, ...(group.pending ? [`${group.pending} pending`] : [])].join(' · ')
const sectionCounts = (section) =>
  [`${section.up} up`, `${section.down} down`, ...(section.pending ? [`${section.pending} pending`] : [])].join(' · ')

const dotClass = (status) => ({ operational: 'bg-green-500', degraded: 'bg-amber-500', down: 'bg-red-500' })[status] || 'bg-statusgray-400'
const endpointDotClass = (status) => ({ up: 'bg-green-500', pending: 'bg-yellow-400', down: 'bg-red-500' })[status] || 'bg-statusgray-400'
const endpointTextClass = (status) =>
  ({ up: 'text-green-700 dark:text-green-400', pending: 'text-yellow-700 dark:text-yellow-400', down: 'text-red-700 dark:text-red-400' })[
    status
  ] || ''
const chipStatusClass = (status) =>
  ({ degraded: 'border-amber-400 dark:border-amber-700', down: 'border-red-400 dark:border-red-800' })[status] || ''
const statusBadgeClass = (status) =>
  ({
    operational: 'border-green-300 text-green-700 dark:border-green-800 dark:text-green-400',
    degraded: 'border-amber-300 text-amber-700 dark:border-amber-700 dark:text-amber-400',
    down: 'border-red-300 text-red-700 dark:border-red-800 dark:text-red-400',
  })[status] || 'border-statusgray-300 text-statusgray-600 dark:border-statusgray-700 dark:text-statusgray-400'
</script>
