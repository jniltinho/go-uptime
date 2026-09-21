<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <AdminListLayout title="Push keys" description="Global keys accept push for every endpoint that receives push, at /api/push/<key>/<endpoint-key>" active="push-keys">
    <template #actions>
      <router-link to="/" class="inline-flex h-9 items-center border border-input bg-background px-3 text-sm font-medium hover:bg-accent dark:border-gray-700 dark:hover:bg-gray-800">Dashboard</router-link>
    </template>

    <template #notices>
      <div v-if="listing && listing.managedUnavailable" role="alert" data-testid="push-keys-unavailable" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">
        The push keys created through the web could not be loaded: only the ones from the configuration file are accepted until the next reload.
      </div>
      <div v-if="notice" role="status" data-testid="admin-notice" class="mt-3 shrink-0 border border-green-300 bg-green-50 px-4 py-2 text-sm text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-200">{{ notice }}</div>
      <div v-if="error" role="alert" data-testid="admin-error" class="mt-3 shrink-0 border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">{{ error }}</div>

      <div v-if="created" role="status" data-testid="push-key-created" class="mt-3 shrink-0 border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100">
        <div class="flex items-start justify-between gap-4">
          <p class="font-medium">Copy the key {{ created.name }} now: it will not be shown again.</p>
          <Button variant="ghost" size="sm" class="-mt-1 shrink-0" data-testid="push-key-done" @click="created = null">Done</Button>
        </div>
        <div class="mt-2 flex gap-2">
          <input :value="created.token" readonly class="h-10 w-full min-w-0 border border-input bg-background px-3 font-mono text-sm text-foreground dark:border-gray-700 dark:text-gray-100" data-testid="push-key-token" @focus="$event.target.select()" />
          <Button variant="outline" class="w-24 shrink-0" data-testid="push-key-copy" @click="copyText(created.token)">{{ copied ? 'Copied' : 'Copy' }}</Button>
        </div>
        <p class="mt-2 break-all text-xs">Example: <span class="font-mono">{{ exampleUrl(created.token) }}</span></p>
      </div>
    </template>

    <template #toolbar>
      <form class="flex flex-wrap items-center gap-2" @submit.prevent="create">
        <label for="push-key-name" class="sr-only">Name of the new key</label>
        <Input id="push-key-name" v-model="name" placeholder="Name of the new key, e.g. akamai" maxlength="64" class="h-9 max-w-md dark:border-gray-700" data-testid="push-key-name" />
        <Button type="submit" size="sm" :disabled="busy || !name.trim()" data-testid="push-key-create">Create key</Button>
      </form>
    </template>

    <div v-if="loading" class="py-12 flex justify-center"><Loading /></div>
    <!-- Fork: fixed layout so that the width of the table never depends on the content, and cards below md -->
    <table v-else class="hidden w-full table-fixed text-xs md:table" data-testid="push-keys-table">
      <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
        <tr class="uppercase tracking-wide">
          <th class="px-2 py-1.5 font-medium">Name</th>
          <th class="w-28 px-2 py-1.5 font-medium">Key</th>
          <th class="hidden w-20 px-2 py-1.5 font-medium lg:table-cell">Source</th>
          <th class="w-[30%] px-2 py-1.5 font-medium">Created</th>
          <th class="w-16 px-2 py-1.5 text-right font-medium">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="keys.length === 0">
          <td colspan="5" class="px-2 py-8 text-center text-muted-foreground dark:text-gray-400">No push keys yet.</td>
        </tr>
        <tr v-for="key in keys" :key="`${key.origin}-${key.id || key.name}`" class="border-t hover:bg-muted/40 dark:border-gray-700 dark:hover:bg-gray-800/50" :data-testid="`push-key-row-${key.origin}-${key.name}`">
          <td class="px-2 py-1 font-medium text-foreground dark:text-gray-100"><span class="block truncate text-sm" :title="key.name">{{ key.name }}</span></td>
          <td class="px-2 py-1 font-mono text-[11px] text-muted-foreground dark:text-gray-400">{{ key.hint ? `…${key.hint}` : '—' }}</td>
          <td class="hidden px-2 py-1 lg:table-cell">
            <span :class="['border px-1 text-[11px] leading-4', key.origin === 'admin' ? 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200' : 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300']">{{ key.origin === 'admin' ? 'Web' : 'YAML' }}</span>
          </td>
          <td class="px-2 py-1 text-muted-foreground dark:text-gray-400">
            <span class="block truncate">
              <template v-if="key.createdAt">{{ new Date(key.createdAt).toLocaleString() }}<span v-if="key.createdBy"> · {{ key.createdBy }}</span></template>
              <template v-else>—</template>
            </span>
          </td>
          <td class="whitespace-nowrap px-2 py-0 text-right">
            <AdminActionButton v-if="key.origin === 'admin'" :icon="Trash2" :label="`Revoke ${key.name}`" :testid="`push-key-revoke-${key.name}`" :disabled="busy" destructive @click="pendingRevocation = key" />
            <!-- Keeps the height of the row equal to the ones that have the action -->
            <span v-else class="block h-7" aria-hidden="true"></span>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- Fork: below md the table gives way to one card per key, with every field and the same action -->
    <div v-if="!loading" class="divide-y md:hidden dark:divide-gray-700">
      <p v-if="keys.length === 0" class="px-3 py-8 text-center text-sm text-muted-foreground dark:text-gray-400">No push keys yet.</p>
      <div v-for="key in keys" :key="`card-${key.origin}-${key.id || key.name}`" class="px-3 py-2.5" :data-testid="`push-key-card-${key.origin}-${key.name}`">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="truncate font-medium text-foreground dark:text-gray-100" :title="key.name">{{ key.name }}</p>
            <p class="truncate font-mono text-xs text-muted-foreground dark:text-gray-400">{{ key.hint ? `…${key.hint}` : '—' }}</p>
          </div>
          <span class="shrink-0 text-xs text-muted-foreground dark:text-gray-400">{{ key.origin === 'admin' ? 'Web' : 'YAML' }}</span>
        </div>
        <p class="mt-1 truncate text-xs text-muted-foreground dark:text-gray-400">
          <template v-if="key.createdAt">{{ new Date(key.createdAt).toLocaleString() }}<span v-if="key.createdBy"> · {{ key.createdBy }}</span></template>
          <template v-else>—</template>
        </p>
        <div v-if="key.origin === 'admin'" class="mt-1">
          <AdminActionButton :icon="Trash2" :label="`Revoke ${key.name}`" :testid="`push-key-revoke-${key.name}`" :disabled="busy" destructive :compact="false" @click="pendingRevocation = key" />
        </div>
      </div>
    </div>

    <template #footer>
      {{ keys.length }} {{ keys.length === 1 ? 'key' : 'keys' }}<span v-if="configKeyCount"> · {{ configKeyCount }} from the configuration file, read-only</span>
    </template>

    <template #overlay>
      <ConfirmDialog
        :open="pendingRevocation !== null"
        title="Revoke push key"
        :message="revocationMessage"
        confirm-label="Revoke"
        @confirm="confirmRevocation"
        @cancel="pendingRevocation = null"
      />
    </template>
  </AdminListLayout>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import AdminActionButton from '@/components/admin/AdminActionButton.vue'
import { Input } from '@/components/ui/input'
import Loading from '@/components/Loading.vue'
import ConfirmDialog from '@/components/admin/ConfirmDialog.vue'
import AdminListLayout from '@/components/admin/AdminListLayout.vue'
import { describePushKeyError, pushKeysApi } from '@/utils/adminApi'

const listing = ref(null)
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const name = ref('')
const created = ref(null)
const pendingRevocation = ref(null)
const copied = ref(false)
let copiedTimer = null

const keys = computed(() => (listing.value && listing.value.keys) || [])
const configKeyCount = computed(() => keys.value.filter((key) => key.origin !== 'admin').length)

const revocationMessage = computed(() => (pendingRevocation.value ? `Push key ${pendingRevocation.value.name} will be revoked: pushes with it are rejected immediately.` : ''))

const exampleUrl = (token) => `${window.location.origin}/api/push/${encodeURIComponent(token)}/<endpoint-key>?status=up&msg=OK&ping=`

const load = async () => {
  try {
    const { data } = await pushKeysApi.list()
    listing.value = data
  } catch (e) {
    error.value = describePushKeyError(e)
  } finally {
    loading.value = false
  }
}

const create = async () => {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const { data } = await pushKeysApi.create(name.value.trim())
    created.value = data
    name.value = ''
  } catch (e) {
    error.value = describePushKeyError(e)
  } finally {
    busy.value = false
    await load()
  }
}

const copyText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copied.value = false }, 2000)
  } catch (e) {
    notice.value = 'The browser did not allow copying: select the key and copy it.'
  }
}

const confirmRevocation = async () => {
  const key = pendingRevocation.value
  pendingRevocation.value = null
  if (!key) {
    return
  }
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await pushKeysApi.remove(key.id)
    notice.value = `Push key ${key.name} revoked.`
  } catch (e) {
    error.value = describePushKeyError(e)
  } finally {
    busy.value = false
    await load()
  }
}

onMounted(load)
onUnmounted(() => clearTimeout(copiedTimer))
</script>
