<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Backup and restore of the items managed through the web (fork): fills the window on larger screens, like the lists -->
  <AdminListLayout
    title="Backup"
    description="Endpoints, status pages and push keys created through the web. History and the configuration file are not included."
    active="backup"
    :panel="false"
    data-testid="admin-backup"
  >
    <template #actions>
      <router-link to="/" class="inline-flex h-9 items-center border border-input bg-background px-3 text-sm font-medium hover:bg-accent dark:border-gray-700 dark:hover:bg-gray-800">Dashboard</router-link>
    </template>

    <div class="grid gap-4 md:min-h-0 md:flex-1 md:grid-cols-2 md:grid-rows-1">
      <!-- Download -->
      <section class="flex min-h-0 flex-col border bg-card dark:border-gray-700 dark:bg-gray-900" data-testid="backup-section-download">
        <header class="shrink-0 border-b px-5 py-4 dark:border-gray-700">
          <h2 class="text-base font-semibold text-foreground dark:text-gray-100">Download backup</h2>
          <p class="mt-0.5 text-xs text-muted-foreground dark:text-gray-400">A JSON file with the complete definitions, that can be restored in another installation with any supported database.</p>
        </header>

        <div class="min-h-0 flex-1 space-y-4 overflow-auto overscroll-contain px-5 py-4">
          <div class="grid grid-cols-3 gap-2">
            <div v-for="counter in counters" :key="counter.id" class="border px-3 py-2 dark:border-gray-700">
              <span class="block truncate text-xs text-muted-foreground dark:text-gray-400" :title="counter.label">{{ counter.label }}</span>
              <span class="block text-lg font-semibold text-foreground dark:text-gray-100" :data-testid="`backup-count-${counter.id}`">{{ counter.value === null ? '—' : counter.value }}</span>
            </div>
          </div>

          <label :class="['flex items-start gap-3 border px-3 py-2.5 text-sm dark:border-gray-700', encrypt ? 'border-green-300 bg-green-50/60 dark:border-green-800 dark:bg-green-900/20' : '']">
            <input v-model="encrypt" type="checkbox" class="mt-0.5 h-4 w-4 accent-gray-900 dark:accent-gray-100" data-testid="backup-encrypt" />
            <span>
              <span class="block font-medium text-foreground dark:text-gray-200">Encrypt with a password</span>
              <span class="block text-xs text-muted-foreground dark:text-gray-400">Argon2id and AES-256-GCM. The password is not stored anywhere: without it the file cannot be restored.</span>
            </span>
          </label>

          <div v-if="encrypt" class="grid gap-3 sm:grid-cols-2">
            <label class="block">
              <span class="flex items-baseline justify-between text-sm font-medium text-foreground dark:text-gray-200">
                Password
                <span class="text-xs font-normal text-muted-foreground dark:text-gray-400" data-testid="backup-password-bytes">{{ downloadPasswordBytes }}/{{ MAX_PASSWORD_BYTES }} bytes</span>
              </span>
              <Input v-model="downloadPassword" type="password" autocomplete="new-password" class="mt-1.5 dark:border-gray-700" data-testid="backup-password" />
            </label>
            <label class="block">
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Confirm the password</span>
              <Input v-model="downloadPasswordConfirmation" type="password" autocomplete="new-password" class="mt-1.5 dark:border-gray-700" data-testid="backup-password-confirm" />
            </label>
            <p class="text-xs text-muted-foreground sm:col-span-2 dark:text-gray-400" data-testid="backup-password-hint">
              <span v-if="downloadPasswordError && (downloadPassword || downloadPasswordConfirmation)" class="text-red-700 dark:text-red-300">{{ downloadPasswordError }}</span>
              <span v-else>Between {{ MIN_PASSWORD_BYTES }} and {{ MAX_PASSWORD_BYTES }} bytes in UTF-8.</span>
            </p>
          </div>

          <div v-else role="status" class="border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100" data-testid="backup-plaintext-warning">
            Without a password, the file contains tokens, passwords, headers and webhooks in plain text. Keep it somewhere safe.
          </div>
        </div>

        <footer class="flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3 dark:border-gray-700">
          <Button :disabled="downloading || (encrypt && Boolean(downloadPasswordError))" data-testid="backup-download" @click="download">{{ downloading ? 'Downloading…' : 'Download' }}</Button>
        </footer>
      </section>

      <!-- Restore -->
      <section class="flex min-h-0 flex-col border bg-card dark:border-gray-700 dark:bg-gray-900" data-testid="backup-section-restore">
        <header class="shrink-0 border-b px-5 py-4 dark:border-gray-700">
          <h2 class="text-base font-semibold text-foreground dark:text-gray-100">Restore</h2>
          <p class="mt-0.5 text-xs text-muted-foreground dark:text-gray-400">Merges a backup into this installation: nothing is removed, and the preview shows exactly what will be applied.</p>
        </header>

        <div class="min-h-0 flex-1 space-y-4 overflow-auto overscroll-contain px-5 py-4">
          <div>
            <label for="restore-file" class="block text-sm font-medium text-foreground dark:text-gray-200">Backup file</label>
            <div class="mt-1.5 flex gap-2">
              <input
                id="restore-file"
                ref="fileInput"
                type="file"
                class="flex h-10 w-full min-w-0 border border-input bg-background px-3 py-2 text-sm text-foreground file:mr-3 file:border-0 file:bg-transparent file:text-sm file:font-medium dark:border-gray-700 dark:text-gray-100"
                data-testid="restore-file"
                @change="onFileChange"
              />
              <Button variant="outline" class="shrink-0" :disabled="!selectedFile || reading || previewing || restoring" data-testid="restore-reload" @click="readFile(selectedFile)">Read again</Button>
            </div>
            <p :class="['mt-1 text-xs', fileError && !reading ? 'text-red-700 dark:text-red-300' : 'text-muted-foreground dark:text-gray-400']" data-testid="restore-file-status">
              <template v-if="reading">Reading the file…</template>
              <!-- The refused file keeps explaining why Preview is disabled, announced once when it changes -->
              <span v-else-if="fileError" :key="fileErrorKey" role="alert">{{ fileError }}</span>
              <template v-else-if="fileFormat === 'plain'">Backup in plain text.</template>
              <template v-else-if="fileFormat === 'encrypted'">Encrypted backup: enter its password.</template>
              <template v-else>Any file up to 2.7 MiB; the format is detected from its content.</template>
            </p>
          </div>

          <label v-if="fileFormat === 'encrypted'" class="block">
            <span class="block text-sm font-medium text-foreground dark:text-gray-200">Password of the backup</span>
            <Input
              v-model="restorePassword"
              type="password"
              autocomplete="off"
              class="mt-1.5 dark:border-gray-700"
              :aria-invalid="wrongPassword ? 'true' : undefined"
              :aria-describedby="wrongPassword ? 'restore-password-error' : undefined"
              data-testid="restore-password"
            />
            <span v-if="wrongPassword" id="restore-password-error" class="mt-1 block text-xs text-red-700 dark:text-red-300" data-testid="restore-password-error">{{ WRONG_PASSWORD_MESSAGE }}</span>
            <span v-else-if="restorePassword && restorePasswordError" class="mt-1 block text-xs text-red-700 dark:text-red-300" data-testid="restore-password-hint">{{ restorePasswordError }}</span>
          </label>

          <label class="flex items-start gap-3 border px-3 py-2.5 text-sm dark:border-gray-700">
            <input v-model="overwrite" type="checkbox" class="mt-0.5 h-4 w-4 accent-gray-900 dark:accent-gray-100" data-testid="restore-overwrite" />
            <span>
              <span class="block font-medium text-foreground dark:text-gray-200">Overwrite existing items</span>
              <span class="block text-xs text-muted-foreground dark:text-gray-400">Items that already exist with a different definition are updated. Without it they are skipped.</span>
            </span>
          </label>
          <label class="flex items-start gap-3 border px-3 py-2.5 text-sm dark:border-gray-700">
            <input v-model="disableEndpoints" type="checkbox" class="mt-0.5 h-4 w-4 accent-gray-900 dark:accent-gray-100" data-testid="restore-disable-endpoints" />
            <span>
              <span class="block font-medium text-foreground dark:text-gray-200">Restore endpoints as disabled</span>
              <span class="block text-xs text-muted-foreground dark:text-gray-400">Created or updated endpoints are saved disabled: no checks and no alerts until they are enabled.</span>
            </span>
          </label>
        </div>

        <footer class="flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3 dark:border-gray-700">
          <Button ref="previewButton" :disabled="!canPreview" data-testid="restore-preview" @click="runPreview">{{ previewing ? 'Previewing…' : 'Preview' }}</Button>
        </footer>
      </section>
    </div>

    <template #overlay>
      <!-- Plan of the restore -->
      <AdminDialog
        :open="Boolean(plan)"
        title="Restore preview"
        :description="plan ? planSummaryText(plan) : ''"
        size="xl"
        :busy="restoring"
        :return-focus="previewElement"
        testid="restore-plan"
        @close="closePlan"
      >
        <div v-if="plan" class="space-y-3">
          <div v-if="notices.monitoringStarts || notices.withAlerts" role="status" class="border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100" data-testid="restore-notices">
            <p v-if="notices.monitoringStarts" data-testid="restore-notice-monitoring">
              {{ notices.monitoringStarts }} enabled {{ notices.monitoringStarts === 1 ? 'endpoint' : 'endpoints' }} will start monitoring right away, from the network of this installation.
            </p>
            <p v-if="notices.withAlerts" data-testid="restore-notice-alerts">
              {{ notices.withAlerts }} of them {{ notices.withAlerts === 1 ? 'has' : 'have' }} alerts that can be sent to the real providers.
            </p>
          </div>

          <div class="flex flex-wrap" role="group" aria-label="Filter by action" data-testid="restore-summary">
            <button
              v-for="option in filterOptions"
              :key="option.id"
              type="button"
              :aria-pressed="actionFilter === option.id"
              :class="[
                '-ml-px h-9 border px-3 text-xs font-medium first:ml-0 dark:border-gray-700',
                actionFilter === option.id ? 'relative z-10 border-gray-900 bg-gray-900 text-white dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900' : 'bg-background text-foreground hover:bg-accent dark:text-gray-200 dark:hover:bg-gray-800'
              ]"
              :data-testid="`restore-filter-${option.id}`"
              @click="actionFilter = option.id"
            >{{ option.label }} ({{ option.count }})</button>
          </div>

          <div class="border dark:border-gray-700">
            <table class="w-full text-sm" data-testid="restore-plan-table">
              <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
                <tr>
                  <th class="px-3 py-2 font-medium">Type</th>
                  <th class="px-3 py-2 font-medium">ID</th>
                  <th class="px-3 py-2 font-medium">Action</th>
                  <th class="px-3 py-2 font-medium">Reason and warnings</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredItems.length === 0">
                  <td colspan="4" class="px-3 py-6 text-center text-muted-foreground dark:text-gray-400">No items.</td>
                </tr>
                <tr
                  v-for="item in filteredItems"
                  :key="`${item.type}-${item.id}`"
                  class="border-t align-top dark:border-gray-700"
                  :data-testid="`restore-plan-row-${item.type}-${item.id}`"
                  :data-action="item.action"
                >
                  <td class="whitespace-nowrap px-3 py-1.5 text-muted-foreground dark:text-gray-400">{{ typeLabel(item.type) }}</td>
                  <td class="px-3 py-1.5 font-mono text-xs text-foreground break-all dark:text-gray-100">{{ item.id }}</td>
                  <td class="px-3 py-1.5"><span :class="['border px-1.5 py-0.5 text-xs', badgeClass(item.action)]">{{ item.action }}</span></td>
                  <td class="px-3 py-1.5 text-xs">
                    <span v-if="item.reason" class="text-foreground dark:text-gray-200">{{ item.reason }}</span>
                    <ul v-if="item.warnings && item.warnings.length" class="list-disc pl-4 text-amber-800 dark:text-amber-300">
                      <li v-for="(warning, index) in item.warnings" :key="index">{{ formatWarning(warning) }}</li>
                    </ul>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <template #footer>
          <Button variant="outline" :disabled="restoring" data-testid="restore-plan-close" @click="closePlan">Cancel</Button>
          <Button variant="destructive" :disabled="!canRestore" data-testid="restore-apply" @click="confirming = true">{{ restoring ? 'Restoring…' : 'Restore' }}</Button>
        </template>
      </AdminDialog>

      <!-- Results of the restore -->
      <AdminDialog
        :open="results !== null"
        title="Restore results"
        :description="resultsSummaryText"
        size="xl"
        :return-focus="previewElement"
        testid="restore-results"
        @close="results = null"
      >
        <div v-if="results" class="border dark:border-gray-700">
          <table class="w-full text-sm" data-testid="restore-results-table">
            <thead class="sticky top-0 z-10 bg-gray-50 text-left text-muted-foreground shadow-[0_1px_0_0_rgb(229,231,235)] dark:bg-gray-800 dark:text-gray-400 dark:shadow-[0_1px_0_0_rgb(55,65,81)]">
              <tr>
                <th class="px-3 py-2 font-medium">Type</th>
                <th class="px-3 py-2 font-medium">ID</th>
                <th class="px-3 py-2 font-medium">Result</th>
                <th class="px-3 py-2 font-medium">Message and warnings</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="results.length === 0">
                <td colspan="4" class="px-3 py-6 text-center text-muted-foreground dark:text-gray-400">No items.</td>
              </tr>
              <tr
                v-for="item in results"
                :key="`${item.type}-${item.id}`"
                class="border-t align-top dark:border-gray-700"
                :data-testid="`restore-result-row-${item.type}-${item.id}`"
                :data-result="item.result"
              >
                <td class="whitespace-nowrap px-3 py-1.5 text-muted-foreground dark:text-gray-400">{{ typeLabel(item.type) }}</td>
                <td class="px-3 py-1.5 font-mono text-xs text-foreground break-all dark:text-gray-100">{{ item.id }}</td>
                <td class="px-3 py-1.5"><span :class="['border px-1.5 py-0.5 text-xs', badgeClass(item.result)]">{{ item.result }}</span></td>
                <td class="px-3 py-1.5 text-xs">
                  <span v-if="item.message" class="text-foreground dark:text-gray-200">{{ item.message }}</span>
                  <ul v-if="item.warnings && item.warnings.length" class="list-disc pl-4 text-amber-800 dark:text-amber-300">
                    <li v-for="(warning, index) in item.warnings" :key="index">{{ formatWarning(warning) }}</li>
                  </ul>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <template #footer>
          <Button data-testid="restore-results-close" @click="results = null">Close</Button>
        </template>
      </AdminDialog>

      <ConfirmDialog
        :open="confirming"
        title="Restore backup"
        :message="confirmationMessage"
        confirm-label="Restore"
        @confirm="applyRestore"
        @cancel="confirming = false"
      />
    </template>
  </AdminListLayout>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import AdminDialog from '@/components/admin/AdminDialog.vue'
import AdminListLayout from '@/components/admin/AdminListLayout.vue'
import ConfirmDialog from '@/components/admin/ConfirmDialog.vue'
import { adminApi, backupApi, pushKeysApi, statusPagesApi } from '@/utils/adminApi'
import {
  MAX_PASSWORD_BYTES,
  MIN_PASSWORD_BYTES,
  buildRestoreBody,
  describeBackupError,
  detectBackupFormat,
  filterPlanItems,
  formatWarning,
  isRestoreFileTooLarge,
  passwordByteLength,
  planCounts,
  planSummaryText,
  restoreConfirmationMessage,
  resultCounts,
  validatePassword
} from '@/utils/adminBackup'
import { toast } from '@/utils/toast'

// Errors with an instruction (conflict, size, limits and rate limit) stay until they are dismissed
const PERSISTENT_ERROR_STATUSES = [409, 413, 422, 429]

const errorToastOptions = (error, title) => ({
  title,
  duration: error && PERSISTENT_ERROR_STATUSES.includes(error.status) ? 0 : undefined
})

// Answer of the server to a wrong password (or a changed encrypted file)
const WRONG_PASSWORD_MESSAGE = 'Invalid password or corrupted file.'

// Quantities of the items managed through the web
const counts = ref({ endpoints: null, statusPages: null, pushKeys: null })

const counters = computed(() => [
  { id: 'endpoints', label: 'Managed endpoints', value: counts.value.endpoints },
  { id: 'status-pages', label: 'Managed status pages', value: counts.value.statusPages },
  { id: 'push-keys', label: 'Web push keys', value: counts.value.pushKeys }
])

const loadCounts = async () => {
  const [endpoints, statusPages, pushKeys] = await Promise.allSettled([adminApi.list(), statusPagesApi.list(), pushKeysApi.list()])
  const failures = []
  const value = (outcome, pick) => {
    if (outcome.status === 'fulfilled') {
      return pick(outcome.value.data)
    }
    failures.push(outcome.reason)
    return null
  }
  counts.value = {
    endpoints: value(endpoints, (data) => (Array.isArray(data) ? data : []).filter((item) => item.source === 'admin').length),
    statusPages: value(statusPages, (data) => ((data && data.statusPages) || []).filter((item) => item.origin === 'admin').length),
    pushKeys: value(pushKeys, (data) => ((data && data.keys) || []).filter((item) => item.origin === 'admin').length)
  }
  if (failures.length) {
    toast.error(`The quantities could not be loaded: ${describeBackupError(failures[0])}`)
  }
}

// Download
const encrypt = ref(false)
const downloadPassword = ref('')
const downloadPasswordConfirmation = ref('')
const downloading = ref(false)

const downloadPasswordBytes = computed(() => passwordByteLength(downloadPassword.value))
const downloadPasswordError = computed(() => validatePassword(downloadPassword.value, downloadPasswordConfirmation.value))

const saveBlob = (blob, filename) => {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const download = async () => {
  if (encrypt.value && downloadPasswordError.value) {
    toast.error(downloadPasswordError.value)
    return
  }
  downloading.value = true
  try {
    const { blob, filename } = await backupApi.download(encrypt.value ? downloadPassword.value : '')
    saveBlob(blob, filename)
    toast.success(`Backup downloaded as ${filename}.`)
  } catch (e) {
    toast.error(describeBackupError(e), errorToastOptions(e, 'The backup could not be downloaded'))
  } finally {
    downloading.value = false
  }
}

// Restore
const fileInput = ref(null)
const selectedFile = ref(null)
const parsedFile = ref(null)
const fileFormat = ref('')
const reading = ref(false)
const restorePassword = ref('')
const overwrite = ref(false)
const disableEndpoints = ref(false)
const previewing = ref(false)
const restoring = ref(false)
const confirming = ref(false)
const plan = ref(null)
const results = ref(null)
const actionFilter = ref('all')
const previewButton = ref(null)
// Why the selected file was refused, kept next to the file field while it is selected
const fileError = ref('')
const fileErrorKey = ref(0)
// The last preview answered that the password is wrong: the field stays invalid until it is edited
const wrongPassword = ref(false)

const previewElement = () => (previewButton.value && previewButton.value.$el) || null
// Each change of the file, of the password or of the options starts a new generation: answers of older ones are ignored
let generation = 0

const invalidate = () => {
  generation++
  plan.value = null
  results.value = null
  confirming.value = false
  actionFilter.value = 'all'
}

watch([restorePassword, overwrite, disableEndpoints], () => {
  invalidate()
})

watch(restorePassword, () => {
  wrongPassword.value = false
})

const refuseFile = (message) => {
  fileError.value = message
  fileErrorKey.value++
  toast.error(message)
}

// Closing the preview discards it: a restore always needs a new preview
const closePlan = () => {
  if (!restoring.value) {
    invalidate()
  }
}

const readFile = async (file) => {
  invalidate()
  parsedFile.value = null
  fileFormat.value = ''
  fileError.value = ''
  wrongPassword.value = false
  if (!file) {
    return
  }
  if (isRestoreFileTooLarge(file.size)) {
    refuseFile('The file is too large: a backup has at most 2 MiB, about 2.7 MiB when encrypted.')
    return
  }
  const current = generation
  reading.value = true
  try {
    const text = await file.text()
    if (current !== generation) {
      return
    }
    let parsed
    try {
      parsed = JSON.parse(text)
    } catch (e) {
      refuseFile('The file is not a backup: it is not valid JSON.')
      return
    }
    const format = detectBackupFormat(parsed)
    if (format === 'unknown') {
      refuseFile('The file is not a backup: unknown format.')
      return
    }
    parsedFile.value = parsed
    fileFormat.value = format
  } catch (e) {
    if (current === generation) {
      refuseFile('The file could not be read.')
    }
  } finally {
    if (current === generation) {
      reading.value = false
    }
  }
}

const onFileChange = (event) => {
  const file = (event.target.files && event.target.files[0]) || null
  selectedFile.value = file
  reading.value = false
  readFile(file)
}

const restorePasswordError = computed(() => (fileFormat.value === 'encrypted' ? validatePassword(restorePassword.value) : ''))

const canPreview = computed(() => Boolean(parsedFile.value) && !reading.value && !previewing.value && !restoring.value && !restorePasswordError.value)
const canRestore = computed(() => Boolean(plan.value && plan.value.fingerprint) && !reading.value && !previewing.value && !restoring.value)

const restoreBody = (fingerprint) => buildRestoreBody({
  file: parsedFile.value,
  format: fileFormat.value,
  password: restorePassword.value,
  overwrite: overwrite.value,
  disableEndpoints: disableEndpoints.value,
  fingerprint
})

const runPreview = async () => {
  if (!canPreview.value) {
    return
  }
  invalidate()
  const current = generation
  previewing.value = true
  try {
    const { data } = await backupApi.preview(restoreBody())
    if (current === generation) {
      plan.value = data
    }
  } catch (e) {
    if (current === generation) {
      const message = describeBackupError(e)
      if (e && e.status === 400 && message === WRONG_PASSWORD_MESSAGE && fileFormat.value === 'encrypted') {
        wrongPassword.value = true
      }
      toast.error(message, errorToastOptions(e, 'The preview failed'))
    }
  } finally {
    previewing.value = false
  }
}

const applyRestore = async () => {
  confirming.value = false
  if (!canRestore.value) {
    return
  }
  restoring.value = true
  try {
    const { data } = await backupApi.restore(restoreBody(plan.value.fingerprint))
    // The plan was applied, even if the options changed meanwhile: a new restore needs a new preview
    invalidate()
    results.value = (data && data.results) || []
    const summary = resultCounts(results.value)
    const message = `${summary.created} created · ${summary.updated} updated · ${summary.unchanged} unchanged · ${summary.skipped} skipped · ${summary.failed} failed.`
    if (summary.failed > 0) {
      toast.warning(message, { title: 'Restore finished with failures' })
    } else {
      toast.success(message, { title: 'Restore finished' })
    }
    loadCounts()
  } catch (e) {
    toast.error(describeBackupError(e), errorToastOptions(e, 'The restore failed'))
    if (e && e.status === 409) {
      invalidate()
    }
  } finally {
    restoring.value = false
  }
}

const notices = computed(() => (plan.value && plan.value.notices) || { monitoringStarts: 0, withAlerts: 0 })

const filterOptions = computed(() => {
  const counted = planCounts(plan.value)
  const items = (plan.value && plan.value.items) || []
  return [
    { id: 'all', label: 'All', count: items.length },
    { id: 'create', label: 'Create', count: counted.create },
    { id: 'update', label: 'Update', count: counted.update },
    { id: 'unchanged', label: 'Unchanged', count: counted.unchanged },
    { id: 'skip', label: 'Skip', count: counted.skip }
  ]
})

const filteredItems = computed(() => filterPlanItems(plan.value && plan.value.items, actionFilter.value))

const confirmationMessage = computed(() => `${restoreConfirmationMessage(plan.value)}\nNothing is removed, and items that are skipped in the preview are not changed.`)

const resultsSummaryText = computed(() => {
  const summary = resultCounts(results.value)
  return `${summary.created} created · ${summary.updated} updated · ${summary.unchanged} unchanged · ${summary.skipped} skipped · ${summary.failed} failed`
})

const TYPE_LABELS = { pushKey: 'Push key', endpoint: 'Endpoint', statusPage: 'Status page' }
const typeLabel = (type) => TYPE_LABELS[type] || type

const BADGES = {
  green: 'border-green-300 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-200',
  blue: 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200',
  gray: 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300',
  amber: 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100',
  red: 'border-red-300 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200'
}
const BADGE_COLORS = { create: 'green', created: 'green', update: 'blue', updated: 'blue', unchanged: 'gray', skip: 'amber', skipped: 'amber', failed: 'red' }
const badgeClass = (value) => BADGES[BADGE_COLORS[value] || 'gray']

onMounted(loadCounts)
</script>
