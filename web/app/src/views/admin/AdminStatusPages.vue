<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <AdminListLayout title="Status pages" description="Pages at /status/&lt;slug&gt;, open without login unless the page asks for one" active="status-pages">
    <template #actions>
      <router-link to="/" class="inline-flex h-9 items-center border border-input bg-background px-3 text-sm font-medium hover:bg-accent dark:border-gray-700 dark:hover:bg-gray-800">Dashboard</router-link>
      <Button size="sm" data-testid="admin-new-status-page" @click="router.push({ name: 'AdminStatusPageNew' })">New status page</Button>
    </template>

    <template #notices>
      <div v-if="listing && !listing.publicationEnabled" role="status" data-testid="status-pages-disabled" class="mt-3 shrink-0 border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100">
        Status pages are disabled in the configuration file (<code>status-pages.enabled: false</code>): no page is published, but you can still edit and preview them.
      </div>
      <div v-if="listing && listing.managedUnavailable" role="alert" data-testid="status-pages-unavailable" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">
        The status pages managed through the web could not be loaded: only the ones from the configuration file are published until the next reload.
      </div>
      <div v-if="listing && listing.sharedRateLimitWarning" role="status" data-testid="status-pages-shared-limit" class="mt-3 shrink-0 border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100">
        Visits come from <code>{{ listing.sharedRateLimitWarning }}</code> with X-Forwarded-For, but this IP is not in <code>status-pages.trusted-proxies</code>: all visitors share the same rate limit.
      </div>
      <div v-if="notice" role="status" data-testid="admin-notice" class="mt-3 shrink-0 border border-green-300 bg-green-50 px-4 py-2 text-sm text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-200">{{ notice }}</div>
      <div v-if="error" role="alert" data-testid="admin-error" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">{{ error }}</div>
      <div v-if="copyFallbackUrl" class="mt-3 shrink-0 border bg-card px-4 py-2 text-sm dark:border-gray-700 dark:bg-gray-900">
        <label class="block text-foreground dark:text-gray-200">Copy the address of the page:
          <input ref="copyInput" :value="copyFallbackUrl" readonly class="mt-1 h-9 w-full border border-input bg-background px-2 font-mono text-sm dark:border-gray-700" data-testid="status-page-copy-fallback" @focus="$event.target.select()" />
        </label>
      </div>
    </template>

    <div v-if="loading" class="py-12 flex justify-center"><Loading /></div>
    <!-- Fork: fixed layout so that the width of the table never depends on the content, and cards below md -->
    <table v-else class="hidden w-full table-fixed text-xs md:table" data-testid="status-pages-table">
      <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
        <tr class="uppercase tracking-wide">
          <th class="w-[20%] px-2 py-1.5 font-medium">Slug</th>
          <th class="px-2 py-1.5 font-medium">Title</th>
          <th class="hidden w-20 px-2 py-1.5 font-medium lg:table-cell">Source</th>
          <th class="w-32 px-2 py-1.5 font-medium">Status</th>
          <th class="hidden w-24 px-2 py-1.5 text-right font-medium lg:table-cell">Endpoints</th>
          <th class="w-44 px-2 py-1.5 text-right font-medium">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="items.length === 0">
          <td colspan="6" class="px-2 py-8 text-center text-muted-foreground dark:text-gray-400">No status pages yet.</td>
        </tr>
        <tr v-for="item in items" :key="`${item.origin}-${item.slug}`" class="border-t hover:bg-muted/40 dark:border-gray-700 dark:hover:bg-gray-800/50" :data-testid="`status-page-row-${item.origin}-${item.slug}`">
          <td class="px-2 py-1 font-mono text-[11px] text-foreground dark:text-gray-100">
            <span class="flex items-center gap-1">
              <!-- Fork: the page has a login of its own -->
              <Lock v-if="item.requiresLogin" class="h-3 w-3 shrink-0 text-amber-600 dark:text-amber-400" aria-label="Requires login" :data-testid="`status-page-requires-login-${item.slug}`" />
              <span class="truncate" :title="item.slug">{{ item.slug }}</span>
            </span>
          </td>
          <td class="px-2 py-1 font-medium text-foreground dark:text-gray-100"><span class="block truncate text-sm" :title="item.title || ''">{{ item.title || '—' }}</span></td>
          <td class="hidden px-2 py-1 lg:table-cell">
            <span :class="['border px-1 text-[11px] leading-4', item.origin === 'admin' ? 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200' : 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300']">{{ item.origin === 'admin' ? 'Web' : 'YAML' }}</span>
          </td>
          <td class="whitespace-nowrap px-2 py-1">
            <span :class="stateClass(item)" :title="item.conflictOrigin || item.error || ''">{{ stateLabel(item) }}</span>
          </td>
          <td class="hidden px-2 py-1 text-right lg:table-cell">
            <!-- The page selects more endpoints than status-pages.maximum-endpoints-per-page shows -->
            <span v-if="item.truncated" class="text-amber-700 dark:text-amber-400" :title="TRUNCATED_TITLE" :data-testid="`status-page-truncated-${item.slug}`">{{ item.endpoints }}+<span class="sr-only"> ({{ TRUNCATED_TITLE }})</span></span>
            <template v-else>{{ item.endpoints }}</template>
          </td>
          <td class="whitespace-nowrap px-2 py-0 text-right">
          <AdminActionButton :icon="ExternalLink" :label="`Open ${item.slug} in a new tab`" :testid="`status-page-open-${item.slug}`" :href="item.path" />
            <AdminActionButton :icon="Link2" :label="`Copy the link of ${item.slug}`" :testid="`status-page-copy-${item.slug}`" @click="copyLink(item)" />
            <AdminActionButton :icon="item.origin === 'admin' ? Pencil : Eye" :label="`${item.origin === 'admin' ? 'Edit' : 'View'} ${item.slug}`" :testid="`status-page-edit-${item.slug}`" @click="edit(item)" />
            <template v-if="item.origin === 'admin'">
              <AdminActionButton
                :icon="item.enabled ? CirclePause : CirclePlay"
                :label="`${item.enabled ? 'Disable' : 'Enable'} ${item.slug}`"
                :testid="`status-page-toggle-${item.slug}`"
                :disabled="busySlug === item.slug || item.conflict || Boolean(item.error)"
                COMPACT
                @click="toggle(item)"
              />
              <AdminActionButton :icon="Trash2" :label="`Remove ${item.slug}`" :testid="`status-page-remove-${item.slug}`" :disabled="busySlug === item.slug" destructive @click="pendingRemoval = item" />
            </template>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- Fork: below md the table gives way to one card per status page, with every field and the same actions -->
    <div v-if="!loading" class="divide-y md:hidden dark:divide-gray-700">
      <p v-if="items.length === 0" class="px-3 py-8 text-center text-sm text-muted-foreground dark:text-gray-400">No status pages yet.</p>
      <div v-for="item in items" :key="`card-${item.origin}-${item.slug}`" class="px-3 py-2.5" :data-testid="`status-page-card-${item.origin}-${item.slug}`">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="truncate font-medium text-foreground dark:text-gray-100" :title="item.title || ''">{{ item.title || '—' }}</p>
            <p class="truncate font-mono text-xs text-muted-foreground dark:text-gray-400" :title="item.slug">{{ item.slug }}</p>
          </div>
          <span :class="['shrink-0 text-xs', stateClass(item)]" :title="item.conflictOrigin || item.error || ''">{{ stateLabel(item) }}</span>
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground dark:text-gray-400">
          <span>{{ item.origin === 'admin' ? 'Web' : 'YAML' }}</span>
          <span :class="item.truncated ? 'text-amber-700 dark:text-amber-400' : ''" :title="item.truncated ? TRUNCATED_TITLE : ''">{{ item.endpoints }}{{ item.truncated ? '+' : '' }} {{ item.endpoints === 1 ? 'endpoint' : 'endpoints' }}<span v-if="item.truncated" class="sr-only"> ({{ TRUNCATED_TITLE }})</span></span>
          <!-- Fork: the page has a login of its own -->
          <span v-if="item.requiresLogin" class="flex items-center gap-1 text-amber-600 dark:text-amber-400" :data-testid="`status-page-requires-login-${item.slug}`">
            <Lock class="h-3 w-3 shrink-0" aria-hidden="true" />Requires login
          </span>
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-1">
          <AdminActionButton :icon="ExternalLink" :label="`Open ${item.slug} in a new tab`" :testid="`status-page-open-${item.slug}`" :href="item.path" :compact="false" />
          <AdminActionButton :icon="Link2" :label="`Copy the link of ${item.slug}`" :testid="`status-page-copy-${item.slug}`" :compact="false" @click="copyLink(item)" />
          <AdminActionButton :icon="item.origin === 'admin' ? Pencil : Eye" :label="`${item.origin === 'admin' ? 'Edit' : 'View'} ${item.slug}`" :testid="`status-page-edit-${item.slug}`" :compact="false" @click="edit(item)" />
          <template v-if="item.origin === 'admin'">
            <AdminActionButton
              :icon="item.enabled ? CirclePause : CirclePlay"
              :label="`${item.enabled ? 'Disable' : 'Enable'} ${item.slug}`"
              :testid="`status-page-toggle-${item.slug}`"
              :disabled="busySlug === item.slug || item.conflict || Boolean(item.error)"
              :compact="false"
              @click="toggle(item)"
            />
            <AdminActionButton :icon="Trash2" :label="`Remove ${item.slug}`" :testid="`status-page-remove-${item.slug}`" :disabled="busySlug === item.slug" destructive :compact="false" @click="pendingRemoval = item" />
          </template>
        </div>
      </div>
    </div>

    <template #footer>
      {{ items.length }} {{ items.length === 1 ? 'status page' : 'status pages' }}<span v-if="publishedCount"> · {{ publishedCount }} published</span><span v-if="requiresLoginCount"> · {{ requiresLoginCount }} with login</span>
    </template>

    <template #overlay>
      <ConfirmDialog
        :open="pendingRemoval !== null"
        title="Remove status page"
        :message="removalMessage"
        confirm-label="Remove"
        @confirm="confirmRemoval"
        @cancel="pendingRemoval = null"
      />
    </template>
  </AdminListLayout>
</template>

<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CirclePause, CirclePlay, ExternalLink, Eye, Link2, Lock, Pencil, Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import AdminActionButton from '@/components/admin/AdminActionButton.vue'
import Loading from '@/components/Loading.vue'
import ConfirmDialog from '@/components/admin/ConfirmDialog.vue'
import AdminListLayout from '@/components/admin/AdminListLayout.vue'
import { describeStatusPageError, statusPagesApi } from '@/utils/adminApi'

const router = useRouter()

const TRUNCATED_TITLE = 'The page selects more endpoints than status-pages.maximum-endpoints-per-page: only the first ones are shown'

const listing = ref(null)
const loading = ref(true)
const error = ref('')
const notice = ref('')
const busySlug = ref('')
const pendingRemoval = ref(null)
const copyFallbackUrl = ref('')
const copyInput = ref(null)

const items = computed(() => (listing.value && listing.value.statusPages) || [])
const publishedCount = computed(() => items.value.filter((item) => item.published).length)
// Fork: pages with a login of their own
const requiresLoginCount = computed(() => items.value.filter((item) => item.requiresLogin).length)

const removalMessage = computed(() => (pendingRemoval.value ? `Status page ${pendingRemoval.value.slug} will be removed and ${pendingRemoval.value.path} will stop responding.` : ''))

const stateLabel = (item) => {
  if (item.conflict) {
    return 'Conflicts with YAML'
  }
  if (item.error) {
    return 'Invalid'
  }
  if (!item.enabled) {
    return 'Disabled'
  }
  return item.published ? 'Published' : 'Not published'
}

const stateClass = (item) => {
  if (item.conflict) {
    return 'text-amber-700 dark:text-amber-400'
  }
  if (item.error) {
    return 'text-red-700 dark:text-red-400'
  }
  return item.published ? 'text-green-700 dark:text-green-400' : 'text-muted-foreground dark:text-gray-500'
}

const load = async () => {
  error.value = ''
  try {
    const { data } = await statusPagesApi.list()
    listing.value = data
  } catch (e) {
    error.value = describeStatusPageError(e)
  } finally {
    loading.value = false
  }
}

const edit = (item) => {
  router.push({ name: 'AdminStatusPageEdit', params: { slug: item.slug } })
}

const copyLink = async (item) => {
  const url = `${window.location.origin}${item.path}`
  notice.value = ''
  copyFallbackUrl.value = ''
  try {
    if (!navigator.clipboard) {
      throw new Error('clipboard unavailable')
    }
    await navigator.clipboard.writeText(url)
    notice.value = `Link copied: ${url}`
  } catch (e) {
    copyFallbackUrl.value = url
    await nextTick()
    if (copyInput.value) {
      copyInput.value.focus()
      copyInput.value.select()
    }
  }
}

const toggle = async (item) => {
  busySlug.value = item.slug
  notice.value = ''
  error.value = ''
  try {
    await statusPagesApi.setEnabled(item.slug, !item.enabled, item.version)
    notice.value = `Status page ${item.slug} ${item.enabled ? 'disabled' : 'enabled'}.`
  } catch (e) {
    error.value = describeStatusPageError(e)
  } finally {
    busySlug.value = ''
    await load()
  }
}

const confirmRemoval = async () => {
  const item = pendingRemoval.value
  pendingRemoval.value = null
  if (!item) {
    return
  }
  busySlug.value = item.slug
  notice.value = ''
  error.value = ''
  try {
    await statusPagesApi.remove(item.slug, item.version)
    notice.value = `Status page ${item.slug} removed.`
  } catch (e) {
    error.value = describeStatusPageError(e)
  } finally {
    busySlug.value = ''
    await load()
  }
}

onMounted(load)
</script>
