<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <AdminListLayout title="Notifications" description="Alerting providers that send the alerts of the endpoints: configured here or in the configuration file" active="notifications">
    <template #actions>
      <router-link to="/" class="inline-flex h-9 items-center border border-input bg-background px-3 text-sm font-medium hover:bg-accent dark:border-gray-700 dark:hover:bg-gray-800">Dashboard</router-link>
      <Button size="sm" class="h-9" :disabled="!canCreate" :title="canCreate ? undefined : storageMessage" data-testid="alert-provider-add" @click="pickerOpen = true">
        <Plus class="mr-1 h-4 w-4" aria-hidden="true" />Add notification
      </Button>
    </template>

    <template #notices>
      <div v-if="listing && !listing.storageSupported" role="status" data-testid="alert-providers-storage" class="mt-3 shrink-0 border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-900/30 dark:text-amber-200">
        {{ storageMessage }}
      </div>
      <div v-if="listing && listing.managedUnavailable" role="alert" data-testid="alert-providers-unavailable" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">
        The notifications configured through the web could not be loaded: only the ones from the configuration file are sending alerts until the next reload.
      </div>
      <div v-if="error" role="alert" data-testid="admin-error" class="mt-3 shrink-0 whitespace-pre-line border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">{{ error }}</div>
    </template>

    <div v-if="loading" class="py-12 flex justify-center"><Loading /></div>

    <div v-else-if="configured.length === 0" class="flex flex-col items-center gap-3 px-4 py-12 text-center" data-testid="alert-providers-empty">
      <BellOff class="h-8 w-8 text-muted-foreground dark:text-gray-500" aria-hidden="true" />
      <p class="text-sm font-medium text-foreground dark:text-gray-100">No notification is configured yet</p>
      <p class="max-w-md text-sm text-muted-foreground dark:text-gray-400">Add a provider such as Telegram, Slack, email or ntfy, then add an alert of that type to the endpoints that should notify.</p>
      <Button v-if="canCreate" size="sm" data-testid="alert-provider-add-empty" @click="pickerOpen = true">Add notification</Button>
    </div>

    <template v-else>
      <!-- Fork: fixed layout so that the width of the table never depends on the content, and cards below md -->
      <table class="hidden w-full table-fixed text-xs md:table" data-testid="alert-providers-table">
        <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
          <tr class="uppercase tracking-wide">
            <th class="px-2 py-1.5 font-medium">Provider</th>
            <th class="hidden w-28 px-2 py-1.5 font-medium lg:table-cell">Category</th>
            <th class="w-20 px-2 py-1.5 font-medium">Source</th>
            <th class="w-24 px-2 py-1.5 font-medium">State</th>
            <th class="w-24 px-2 py-1.5 font-medium">Used by</th>
            <th class="hidden w-[22%] px-2 py-1.5 font-medium lg:table-cell">Updated</th>
            <th class="w-24 px-2 py-1.5 text-right font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in configured" :key="item.type" class="border-t hover:bg-muted/40 dark:border-gray-700 dark:hover:bg-gray-800/50" :data-testid="`alert-provider-row-${item.type}`">
            <td class="px-2 py-1">
              <span class="block truncate text-sm font-medium text-foreground dark:text-gray-100" :title="item.name">{{ item.name }}</span>
            </td>
            <td class="hidden px-2 py-1 text-muted-foreground dark:text-gray-400 lg:table-cell"><span class="block truncate">{{ item.category }}</span></td>
            <td class="px-2 py-1"><span :class="['border px-1 text-[11px] leading-4', sourceClass(item)]">{{ item.source === 'web' ? 'Web' : 'YAML' }}</span></td>
            <td class="px-2 py-1">
              <span :class="['inline-flex items-center gap-1 whitespace-nowrap', stateClass(item)]" :title="stateTitle(item)" :aria-label="`${providerState(item).label}${stateTitle(item) ? ': ' + stateTitle(item) : ''}`">
                <component :is="stateIcon(item)" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />{{ providerState(item).label }}
              </span>
              <span v-if="item.shadowed" class="ml-1 border border-amber-300 px-1 text-[11px] leading-4 text-amber-800 dark:border-amber-700 dark:text-amber-200" title="A notification configured through the web exists for this type, but the configuration file takes precedence">+ Web shadowed</span>
            </td>
            <td class="px-2 py-1">
              <button type="button" class="whitespace-nowrap underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" :aria-label="`Endpoints using ${item.name}: ${usageCount(item)}`" :data-testid="`alert-provider-usage-${item.type}`" @click="usageItem = item">
                {{ usageCount(item) }} {{ usageCount(item) === 1 ? 'endpoint' : 'endpoints' }}
              </button>
            </td>
            <td class="hidden px-2 py-1 text-muted-foreground dark:text-gray-400 lg:table-cell">
              <span class="block truncate">
                <template v-if="item.updatedAt">{{ new Date(item.updatedAt).toLocaleString() }}<span v-if="item.updatedBy"> · {{ item.updatedBy }}</span></template>
                <template v-else>—</template>
              </span>
            </td>
            <td class="whitespace-nowrap px-2 py-0 text-right">
              <AdminActionButton :icon="Send" :label="`Send a test notification with ${item.name}, then its resolution`" :testid="`alert-provider-test-${item.type}`" :disabled="busy || item.state !== 'valid'" @click="sendTest(item)" />
              <AdminActionButton :icon="item.source === 'web' ? Pencil : Eye" :label="`${item.source === 'web' ? 'Edit' : 'View'} ${item.name}`" :testid="`alert-provider-edit-${item.type}`" @click="openItem(item)" />
              <AdminActionButton v-if="item.source === 'web' || item.shadowed" :icon="Trash2" :label="`Remove ${item.shadowed ? 'the shadowed web notification of ' : ''}${item.name}`" :testid="`alert-provider-remove-${item.type}`" :disabled="busy" destructive @click="askRemoval(item)" />
              <span v-else class="inline-block h-7 w-7 align-middle" aria-hidden="true"></span>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Fork: below md the table gives way to one card per provider, with every field and the same actions -->
      <div class="divide-y md:hidden dark:divide-gray-700">
        <div v-for="item in configured" :key="`card-${item.type}`" class="px-3 py-2.5" :data-testid="`alert-provider-card-${item.type}`">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="truncate font-medium text-foreground dark:text-gray-100">{{ item.name }}</p>
              <p class="truncate text-xs text-muted-foreground dark:text-gray-400">{{ item.category }} · {{ item.source === 'web' ? 'Web' : 'YAML' }}<template v-if="item.shadowed"> · web shadowed</template></p>
            </div>
            <span :class="['inline-flex shrink-0 items-center gap-1 text-xs', stateClass(item)]" :title="stateTitle(item)">
              <component :is="stateIcon(item)" class="h-3.5 w-3.5" aria-hidden="true" />{{ providerState(item).label }}
            </span>
          </div>
          <p v-if="item.error" class="mt-1 break-words text-xs text-red-700 dark:text-red-400">{{ item.error }}</p>
          <button type="button" class="mt-1 text-xs text-muted-foreground underline-offset-2 hover:underline dark:text-gray-400" @click="usageItem = item">
            Used by {{ usageCount(item) }} {{ usageCount(item) === 1 ? 'endpoint' : 'endpoints' }}
          </button>
          <div class="mt-1 flex gap-1">
            <AdminActionButton :icon="Send" :label="`Send a test notification with ${item.name}`" :testid="`alert-provider-test-${item.type}`" :disabled="busy || item.state !== 'valid'" :compact="false" @click="sendTest(item)" />
            <AdminActionButton :icon="item.source === 'web' ? Pencil : Eye" :label="`${item.source === 'web' ? 'Edit' : 'View'} ${item.name}`" :testid="`alert-provider-edit-${item.type}`" :compact="false" @click="openItem(item)" />
            <AdminActionButton v-if="item.source === 'web' || item.shadowed" :icon="Trash2" :label="`Remove ${item.name}`" :testid="`alert-provider-remove-${item.type}`" :disabled="busy" destructive :compact="false" @click="askRemoval(item)" />
          </div>
        </div>
      </div>
    </template>

    <template #footer>
      {{ configured.length }} configured<span v-if="yamlCount"> · {{ yamlCount }} from the configuration file, read-only</span> · {{ providers.length }} types available
    </template>

    <template #overlay>
      <!-- Picker of the type of a new notification, grouped by category -->
      <AdminDialog :open="pickerOpen" title="Add notification" description="Pick the service that will receive the alerts." size="lg" testid="alert-provider-picker" :initial-focus="() => searchInput" @close="pickerOpen = false">
        <label for="alert-provider-search" class="sr-only">Search providers</label>
        <input id="alert-provider-search" ref="searchInput" v-model="search" type="search" placeholder="Search, e.g. telegram" class="mb-3 h-9 w-full border border-input bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-gray-700 dark:text-gray-100" data-testid="alert-provider-search" />
        <p v-if="pickerGroups.length === 0" class="py-6 text-center text-sm text-muted-foreground dark:text-gray-400">No provider matches “{{ search }}”.</p>
        <div v-for="group in pickerGroups" :key="group.category" class="mb-3 last:mb-0">
          <h3 class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground dark:text-gray-400">{{ group.category }}</h3>
          <div class="grid gap-1 sm:grid-cols-2">
            <button
              v-for="type in group.types"
              :key="type.type"
              type="button"
              :disabled="isConfigured(type.type)"
              :data-testid="`alert-provider-pick-${type.type}`"
              class="flex items-center justify-between gap-2 border px-3 py-2 text-left text-sm hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-700 dark:hover:bg-gray-800"
              @click="pick(type)"
            >
              <span class="min-w-0">
                <span class="block truncate font-medium text-foreground dark:text-gray-100">{{ type.name }}</span>
                <span v-if="type.deprecated" class="text-xs text-amber-700 dark:text-amber-300">Deprecated</span>
                <span v-else-if="type.untested" class="text-xs text-muted-foreground dark:text-gray-400">Not validated by the community</span>
              </span>
              <span v-if="isConfigured(type.type)" class="shrink-0 text-xs text-muted-foreground dark:text-gray-400">Configured</span>
            </button>
          </div>
        </div>
      </AdminDialog>

      <!-- Endpoints with an alert of a type -->
      <AdminDialog :open="usageItem !== null" :title="usageItem ? `Endpoints using ${usageItem.name}` : ''" size="md" testid="alert-provider-usage" @close="usageItem = null">
        <template v-if="usageItem">
          <p v-if="usageCount(usageItem) === 0" class="text-sm text-muted-foreground dark:text-gray-400">No endpoint has an alert of this type yet. Add one in the Alerts section of an endpoint.</p>
          <EndpointKeyList :managed="usageItem.usage.managed" :file="usageItem.usage.file" />
        </template>
      </AdminDialog>

      <ConfirmDialog
        :open="pendingRemoval !== null"
        title="Remove notification"
        :message="removalMessage"
        confirm-label="Remove"
        @confirm="confirmRemoval"
        @cancel="pendingRemoval = null"
      />

      <!-- A provider used by managed endpoints cannot be removed: the dialog lists them -->
      <AdminDialog :open="blockedRemoval !== null" title="This notification is in use" size="md" testid="alert-provider-in-use" @close="blockedRemoval = null">
        <template v-if="blockedRemoval">
          <p class="mb-3 text-sm text-muted-foreground dark:text-gray-400">Remove the {{ blockedRemoval.item.name }} alerts of these endpoints first, then remove the notification.</p>
          <EndpointKeyList :managed="blockedRemoval.keys" :file="[]" />
        </template>
      </AdminDialog>
    </template>
  </AdminListLayout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { AlertTriangle, BellOff, CheckCircle2, Eye, Pencil, Plus, Send, Trash2, XCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import Loading from '@/components/Loading.vue'
import AdminActionButton from '@/components/admin/AdminActionButton.vue'
import AdminDialog from '@/components/admin/AdminDialog.vue'
import AdminListLayout from '@/components/admin/AdminListLayout.vue'
import ConfirmDialog from '@/components/admin/ConfirmDialog.vue'
import EndpointKeyList from '@/components/admin/EndpointKeyList.vue'
import { alertProvidersApi, describeAlertProviderError } from '@/utils/adminApi'
import { groupByCategory, providerState, usageCount } from '@/utils/alertProviderForm'
import { toast } from '@/utils/toast'

const router = useRouter()

const listing = ref(null)
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const pickerOpen = ref(false)
const search = ref('')
const searchInput = ref(null)
const usageItem = ref(null)
const pendingRemoval = ref(null)
const blockedRemoval = ref(null)

const storageMessage = 'The configured storage keeps no data (memory): notifications can only be configured in the configuration file.'

const providers = computed(() => (listing.value && listing.value.providers) || [])
// Providers in use first, then the shadowed web ones, each by name
const configured = computed(() => providers.value.filter((item) => item.source !== 'none').sort((a, b) => a.name.localeCompare(b.name)))
const yamlCount = computed(() => configured.value.filter((item) => item.source === 'yaml').length)
const canCreate = computed(() => Boolean(listing.value && listing.value.storageSupported))
const pickerGroups = computed(() => groupByCategory((listing.value && listing.value.types) || [], search.value))

const isConfigured = (type) => providers.value.some((item) => item.type === type && (item.source !== 'none' || item.shadowed))

const sourceClass = (item) => item.source === 'web'
  ? 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200'
  : 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300'

const stateClass = (item) => ({
  success: 'text-green-700 dark:text-green-400',
  error: 'text-red-700 dark:text-red-400',
  muted: 'text-muted-foreground dark:text-gray-400',
}[providerState(item).tone])

const stateIcon = (item) => ({ success: CheckCircle2, error: XCircle, muted: AlertTriangle }[providerState(item).tone])

const stateTitle = (item) => item.error || ''

const removalMessage = computed(() => {
  const item = pendingRemoval.value
  if (!item) {
    return ''
  }
  return item.shadowed
    ? `The web notification of ${item.name} is not in use: the configuration file takes precedence. It will be removed.`
    : `${item.name} will be removed. Alerts of this type stop being sent.`
})

const load = async () => {
  try {
    const { data } = await alertProvidersApi.list()
    listing.value = data
    error.value = ''
  } catch (e) {
    error.value = describeAlertProviderError(e)
  } finally {
    loading.value = false
  }
}

const pick = (type) => {
  pickerOpen.value = false
  search.value = ''
  router.push({ name: 'AdminAlertProviderNew', query: { type: type.type } })
}

const openItem = (item) => router.push({ name: 'AdminAlertProviderEdit', params: { providerType: item.type } })

const sendTest = async (item) => {
  busy.value = true
  try {
    // The quick test of the list closes what it opens
    const { data } = await alertProvidersApi.testConfigured(item.type, '', true)
    if (data.success) {
      toast.success(`Test notification sent with ${item.name}${data.resolved ? ' and resolved' : ''} in ${data.durationMs} ms.`)
    } else {
      toast.error(`${item.name} did not accept the test notification: ${data.error}`)
    }
  } catch (e) {
    toast.error(describeAlertProviderError(e))
  } finally {
    busy.value = false
  }
}

const askRemoval = (item) => {
  if (!item.shadowed && item.usage.managed.length > 0) {
    blockedRemoval.value = { item, keys: item.usage.managed }
    return
  }
  pendingRemoval.value = item
}

const confirmRemoval = async () => {
  const item = pendingRemoval.value
  pendingRemoval.value = null
  if (!item) {
    return
  }
  busy.value = true
  try {
    await alertProvidersApi.remove(item.type, item.version)
    toast.success(`${item.name} removed.`)
  } catch (e) {
    if (e.status === 409 && e.data && Array.isArray(e.data.endpoints)) {
      blockedRemoval.value = { item, keys: e.data.endpoints }
    } else {
      toast.error(describeAlertProviderError(e))
    }
  } finally {
    busy.value = false
    await load()
  }
}

onMounted(load)
</script>
