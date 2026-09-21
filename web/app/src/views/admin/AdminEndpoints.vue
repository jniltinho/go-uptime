<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <AdminListLayout title="Endpoint administration" description="Endpoints managed through the web and endpoints from the configuration file" active="endpoints">
    <template #actions>
      <router-link to="/" class="inline-flex h-9 items-center border border-input bg-background px-3 text-sm font-medium hover:bg-accent dark:border-gray-700 dark:hover:bg-gray-800">Dashboard</router-link>
      <Button size="sm" data-testid="admin-new-endpoint" @click="router.push({ name: 'AdminEndpointNew' })">New endpoint</Button>
    </template>

    <template #notices>
      <div v-if="notice" role="status" data-testid="admin-notice" class="mt-3 shrink-0 border border-green-300 bg-green-50 px-4 py-2 text-sm text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-200">{{ notice }}</div>
      <div v-if="error" role="alert" data-testid="admin-error" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">{{ error }}</div>
    </template>

    <template #toolbar>
      <Input v-model="search" placeholder="Search by name, group or URL" class="h-9 max-w-md dark:border-gray-700" data-testid="admin-search" />
    </template>

    <div v-if="loading" class="py-12 flex justify-center"><Loading /></div>
    <!-- Fork: fixed layout so that the width of the table never depends on the content, and cards below md -->
    <table v-else class="hidden w-full table-fixed text-xs md:table" data-testid="admin-table">
      <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
        <tr class="uppercase tracking-wide">
          <th class="w-[26%] px-2 py-1.5 font-medium">Name</th>
          <th class="w-[12%] px-2 py-1.5 font-medium">Group</th>
          <th class="w-28 px-2 py-1.5 font-medium">Type</th>
          <th class="px-2 py-1.5 font-medium">URL</th>
          <th class="hidden w-24 px-2 py-1.5 text-right font-medium lg:table-cell">Interval</th>
          <th class="w-28 px-2 py-1.5 font-medium">Status</th>
          <th class="hidden w-20 px-2 py-1.5 font-medium lg:table-cell">Source</th>
          <th class="w-28 px-2 py-1.5 text-right font-medium">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="filteredItems.length === 0">
          <td colspan="8" class="px-2 py-8 text-center text-muted-foreground dark:text-gray-400">No endpoints found.</td>
        </tr>
        <tr v-for="item in filteredItems" :key="item.key" class="border-t hover:bg-muted/40 dark:border-gray-700 dark:hover:bg-gray-800/50" :data-testid="`admin-row-${item.key}`">
          <td class="px-2 py-1 font-medium text-foreground dark:text-gray-100">
            <span class="flex min-w-0 items-center gap-1">
              <span class="min-w-0 truncate text-sm" :title="item.name">{{ item.name }}</span>
              <!-- The warning is an icon so that it neither adds width nor pushes the row to a second line -->
              <AlertTriangle
                v-if="item.conflict || item.error"
                :class="['h-3.5 w-3.5 shrink-0', item.conflict ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400']"
                :aria-label="item.conflict ? `Conflicts with the configuration file: ${item.conflictOrigin}` : `Invalid definition: ${item.error}`"
                :title="item.conflict ? `Conflicts with YAML: ${item.conflictOrigin}` : `Invalid: ${item.error}`"
                :data-testid="`admin-warning-${item.key}`"
              />
            </span>
          </td>
          <td class="px-2 py-1 text-muted-foreground dark:text-gray-400"><span class="block truncate" :title="item.group">{{ item.group }}</span></td>
          <td class="whitespace-nowrap px-2 py-1 uppercase text-muted-foreground dark:text-gray-400">
            {{ item.type }}
            <span v-if="item.acceptsPush && item.type !== 'PUSH'" title="Also receives push" class="ml-1 whitespace-nowrap border border-violet-300 bg-violet-50 px-1 text-[11px] normal-case leading-4 text-violet-800 dark:border-violet-800 dark:bg-violet-900/30 dark:text-violet-200" :data-testid="`admin-accepts-push-${item.key}`">push</span>
          </td>
          <td class="px-2 py-1 font-mono text-[11px]" :title="item.url"><span class="block truncate">{{ item.url || (item.type === 'PUSH' ? '—' : '') }}</span></td>
          <td class="hidden px-2 py-1 text-right lg:table-cell">{{ item.interval }}</td>
          <td class="px-2 py-1">
            <span :class="item.enabled ? 'text-green-700 dark:text-green-400' : 'text-muted-foreground dark:text-gray-500'">{{ item.enabled ? 'Enabled' : 'Disabled' }}</span>
          </td>
          <td class="hidden px-2 py-1 lg:table-cell">
            <span :class="['border px-1 text-[11px] leading-4', item.source === 'admin' ? 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200' : 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300']">{{ item.source === 'admin' ? 'Web' : 'YAML' }}</span>
          </td>
          <td class="whitespace-nowrap px-2 py-0 text-right">
            <AdminActionButton :icon="item.source === 'admin' ? Pencil : Eye" :label="`${item.source === 'admin' ? 'Edit' : 'View'} ${item.name}`" :testid="`admin-open-${item.key}`" @click="open(item)" />
            <template v-if="item.source === 'admin'">
              <AdminActionButton
                :icon="item.enabled ? CirclePause : CirclePlay"
                :label="`${item.enabled ? 'Disable' : 'Enable'} ${item.name}${item.conflict ? ' (in conflict with the configuration file)' : ''}`"
                :testid="`admin-toggle-${item.key}`"
                :disabled="busyKey === item.key || item.conflict"
                @click="toggle(item)"
              />
              <AdminActionButton :icon="Trash2" :label="`Remove ${item.name}`" :testid="`admin-remove-${item.key}`" :disabled="busyKey === item.key" destructive @click="pendingRemoval = item" />
            </template>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- Fork: below md the table gives way to one card per endpoint, with every field and the same actions -->
    <div v-if="!loading" class="divide-y md:hidden dark:divide-gray-700">
      <p v-if="filteredItems.length === 0" class="px-3 py-8 text-center text-sm text-muted-foreground dark:text-gray-400">No endpoints found.</p>
      <div v-for="item in filteredItems" :key="`card-${item.key}`" class="px-3 py-2.5" :data-testid="`admin-card-${item.key}`">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="flex min-w-0 items-center gap-1 font-medium text-foreground dark:text-gray-100">
              <span class="min-w-0 truncate" :title="item.name">{{ item.name }}</span>
              <AlertTriangle
                v-if="item.conflict || item.error"
                :class="['h-3.5 w-3.5 shrink-0', item.conflict ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400']"
                :aria-label="item.conflict ? `Conflicts with the configuration file: ${item.conflictOrigin}` : `Invalid definition: ${item.error}`"
                :title="item.conflict ? `Conflicts with YAML: ${item.conflictOrigin}` : `Invalid: ${item.error}`"
              />
            </p>
            <p class="truncate font-mono text-xs text-muted-foreground dark:text-gray-400" :title="item.url">{{ item.url || (item.type === 'PUSH' ? '—' : '') }}</p>
          </div>
          <span :class="['shrink-0 text-xs', item.enabled ? 'text-green-700 dark:text-green-400' : 'text-muted-foreground dark:text-gray-500']">{{ item.enabled ? 'Enabled' : 'Disabled' }}</span>
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground dark:text-gray-400">
          <span v-if="item.group" class="truncate">{{ item.group }}</span>
          <span class="uppercase">{{ item.type }}</span>
          <span v-if="item.acceptsPush && item.type !== 'PUSH'">push</span>
          <span>{{ item.interval }}</span>
          <span>{{ item.source === 'admin' ? 'Web' : 'YAML' }}</span>
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-1">
          <AdminActionButton :icon="item.source === 'admin' ? Pencil : Eye" :label="`${item.source === 'admin' ? 'Edit' : 'View'} ${item.name}`" :testid="`admin-open-${item.key}`" :compact="false" @click="open(item)" />
          <template v-if="item.source === 'admin'">
            <AdminActionButton
              :icon="item.enabled ? CirclePause : CirclePlay"
              :label="`${item.enabled ? 'Disable' : 'Enable'} ${item.name}`"
              :testid="`admin-toggle-${item.key}`"
              :disabled="busyKey === item.key || item.conflict"
              :compact="false"
              @click="toggle(item)"
            />
            <AdminActionButton :icon="Trash2" :label="`Remove ${item.name}`" :testid="`admin-remove-${item.key}`" :disabled="busyKey === item.key" destructive :compact="false" @click="pendingRemoval = item" />
          </template>
        </div>
      </div>
    </div>

    <template #footer>
      <span data-testid="admin-count">{{ search.trim() ? `${filteredItems.length} of ${items.length}` : items.length }} {{ items.length === 1 ? 'endpoint' : 'endpoints' }}</span><span v-if="webCount"> · {{ webCount }} managed through the web</span><span v-if="configCount"> · {{ configCount }} from the configuration file</span>
    </template>

    <template #overlay>
      <ConfirmDialog
        :open="pendingRemoval !== null"
        title="Remove endpoint"
        :message="removalMessage"
        confirm-label="Remove"
        @confirm="confirmRemoval"
        @cancel="pendingRemoval = null"
      />
    </template>
  </AdminListLayout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { AlertTriangle, CirclePause, CirclePlay, Eye, Pencil, Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import AdminActionButton from '@/components/admin/AdminActionButton.vue'
import { Input } from '@/components/ui/input'
import Loading from '@/components/Loading.vue'
import ConfirmDialog from '@/components/admin/ConfirmDialog.vue'
import AdminListLayout from '@/components/admin/AdminListLayout.vue'
import { adminApi, describeAdminError } from '@/utils/adminApi'

const router = useRouter()

const items = ref([])
const loading = ref(true)
const error = ref('')
const notice = ref('')
const search = ref('')
const busyKey = ref('')
const pendingRemoval = ref(null)

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) {
    return items.value
  }
  return items.value.filter((item) =>
    [item.name, item.group, item.url].some((value) => (value || '').toLowerCase().includes(query))
  )
})

const webCount = computed(() => items.value.filter((item) => item.source === 'admin').length)
const configCount = computed(() => items.value.length - webCount.value)

const removalMessage = computed(() => {
  if (!pendingRemoval.value) {
    return ''
  }
  return `Endpoint ${pendingRemoval.value.key} and all of its history will be deleted.\nTriggered alerts will not be resolved with the alerting providers.`
})

const load = async () => {
  error.value = ''
  try {
    const { data } = await adminApi.list()
    items.value = data || []
  } catch (e) {
    error.value = describeAdminError(e)
  } finally {
    loading.value = false
  }
}

const open = (item) => {
  router.push({ name: 'AdminEndpointEdit', params: { endpointKey: item.key } })
}

const toggle = async (item) => {
  busyKey.value = item.key
  notice.value = ''
  error.value = ''
  try {
    await adminApi.setEnabled(item.key, !item.enabled, item.version)
    notice.value = `Endpoint ${item.key} ${item.enabled ? 'disabled' : 'enabled'}.`
  } catch (e) {
    error.value = describeAdminError(e)
  } finally {
    busyKey.value = ''
    await load()
  }
}

const confirmRemoval = async () => {
  const item = pendingRemoval.value
  pendingRemoval.value = null
  if (!item) {
    return
  }
  busyKey.value = item.key
  notice.value = ''
  error.value = ''
  try {
    const { data } = await adminApi.remove(item.key, item.version)
    const triggeredAlerts = (data && data.triggeredAlerts) || 0
    notice.value = triggeredAlerts > 0
      ? `Endpoint ${item.key} removed. ${triggeredAlerts} triggered alert(s) were not resolved with the alerting providers.`
      : `Endpoint ${item.key} removed.`
  } catch (e) {
    error.value = describeAdminError(e)
  } finally {
    busyKey.value = ''
    await load()
  }
}

onMounted(load)
</script>
