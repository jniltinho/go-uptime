<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="container mx-auto max-w-4xl px-4 py-8">
    <div class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div class="min-w-0">
        <h1 class="text-3xl font-bold tracking-tight text-foreground dark:text-gray-100" data-testid="alert-provider-title">{{ title }}</h1>
        <p v-if="info" class="mt-1 text-sm text-muted-foreground dark:text-gray-400">
          {{ info.category }} · <span class="font-mono">{{ info.type }}</span><span v-if="version"> · version {{ version }}</span>
          · <a :href="documentationUrl" target="_blank" rel="noopener" class="underline underline-offset-2">Documentation</a>
        </p>
      </div>
      <Button variant="outline" data-testid="admin-back" @click="goBack">Back</Button>
    </div>

    <div v-if="loading" class="flex justify-center py-12"><Loading /></div>
    <template v-else-if="info">
      <div v-if="error" role="alert" data-testid="admin-error" class="mb-4 border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">
        <p class="whitespace-pre-line">{{ error }}</p>
        <Button v-if="versionConflict" variant="outline" size="sm" class="mt-2" data-testid="admin-reload" @click="reloadCurrentVersion">Load the current version</Button>
      </div>
      <div v-if="readOnly" class="mb-4 border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-900/30 dark:text-amber-200" data-testid="alert-provider-readonly">
        This notification is configured in the configuration file and can only be viewed. Secrets are masked.
        <template v-if="shadowedDefinition"> A notification configured through the web also exists for this type, but is not used while the configuration file has one.</template>
      </div>
      <div v-if="info.deprecated" class="mb-4 border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-900/30 dark:text-amber-200">
        {{ info.name }} is deprecated: prefer Microsoft Teams Workflows.
      </div>
      <div v-else-if="info.untested" class="mb-4 border border-gray-300 bg-gray-50 px-4 py-3 text-sm text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300">
        This provider has not been validated against the real service yet: send a test before relying on it.
      </div>
      <div v-if="itemError && !readOnly" role="alert" class="mb-4 border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200">
        The stored definition is invalid and sends nothing: {{ itemError }}
      </div>

      <div class="mb-4 flex border-b dark:border-gray-700" role="tablist">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          type="button"
          role="tab"
          :aria-selected="mode === tab.value"
          :data-testid="`admin-mode-${tab.value}`"
          :class="['-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors', mode === tab.value ? 'border-primary text-foreground dark:border-gray-100 dark:text-gray-100' : 'border-transparent text-muted-foreground hover:text-foreground dark:text-gray-400 dark:hover:text-gray-100']"
          @click="switchMode(tab.value)"
        >{{ tab.label }}</button>
      </div>

      <div v-if="mode === 'form'" class="space-y-4">
        <!-- Connection: the fields of the type -->
        <section class="border bg-card p-5 dark:border-gray-700 dark:bg-gray-900">
          <header class="mb-3">
            <h2 class="text-base font-semibold text-foreground dark:text-gray-100">Connection</h2>
            <p class="mt-0.5 text-xs text-muted-foreground dark:text-gray-400">Where {{ info.name }} sends the alerts. Fields marked with * are required.</p>
          </header>
          <p v-if="info.fields.length === 0" class="text-sm text-muted-foreground dark:text-gray-400">This provider has no field of its own: configure it in YAML mode.</p>
          <div class="grid gap-4 sm:grid-cols-2">
            <div v-for="field in info.fields" :key="field.key" :class="field.kind === 'map' || field.key === 'body' ? 'sm:col-span-2' : ''">
              <label :for="`field-${field.key}`" class="block text-sm font-medium text-foreground dark:text-gray-200">
                {{ labelOf(field.key) }}<span v-if="field.required" class="text-red-600 dark:text-red-400" aria-hidden="true"> *</span>
                <span v-if="field.secret" class="ml-1 text-xs font-normal text-muted-foreground dark:text-gray-400">secret</span>
              </label>

              <label v-if="field.kind === 'boolean'" class="mt-1.5 flex items-center gap-2 text-sm text-foreground dark:text-gray-200">
                <input :id="`field-${field.key}`" v-model="form.values[field.key]" type="checkbox" :disabled="readOnly" class="h-4 w-4 accent-gray-900 dark:accent-gray-100" :data-testid="`alert-provider-field-${field.key}`" />
                Enabled
              </label>

              <!-- A stored secret is never shown: it is kept unless replaced -->
              <div v-else-if="field.secret && form.values[field.key] === MASKED" class="mt-1.5 flex items-center gap-2">
                <span class="flex h-10 min-w-0 flex-1 items-center border border-dashed border-input px-3 text-sm text-muted-foreground dark:border-gray-700 dark:text-gray-400" :data-testid="`alert-provider-configured-${field.key}`">
                  <Lock class="mr-2 h-3.5 w-3.5 shrink-0" aria-hidden="true" />Configured — leave unchanged
                </span>
                <Button v-if="!readOnly" variant="outline" size="sm" :data-testid="`alert-provider-replace-${field.key}`" @click="replaceSecret(field.key)">Replace</Button>
              </div>

              <textarea
                v-else-if="field.kind === 'map' || field.key === 'body'"
                :id="`field-${field.key}`"
                v-model="form.values[field.key]"
                :readonly="readOnly"
                rows="4"
                spellcheck="false"
                :placeholder="field.kind === 'map' ? 'Name: value, one per line' : field.default"
                :data-testid="`alert-provider-field-${field.key}`"
                class="mt-1.5 w-full border border-input bg-background p-2 font-mono text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-gray-700 dark:text-gray-100"
              ></textarea>

              <div v-else class="relative mt-1.5">
                <Input
                  :id="`field-${field.key}`"
                  v-model="form.values[field.key]"
                  :type="inputType(field)"
                  :readonly="readOnly"
                  :placeholder="placeholderOf(field)"
                  :autocomplete="field.secret ? 'new-password' : 'off'"
                  :aria-describedby="field.help ? `help-${field.key}` : undefined"
                  :aria-invalid="invalidFields.includes(field.key) ? 'true' : undefined"
                  :class="['dark:border-gray-700', field.secret ? 'pr-10' : '', invalidFields.includes(field.key) ? 'border-red-500 dark:border-red-500' : '']"
                  :data-testid="`alert-provider-field-${field.key}`"
                />
                <button
                  v-if="field.secret && !readOnly"
                  type="button"
                  class="absolute inset-y-0 right-0 flex w-10 items-center justify-center text-muted-foreground hover:text-foreground dark:text-gray-400"
                  :aria-label="revealed[field.key] ? `Hide ${labelOf(field.key)}` : `Show ${labelOf(field.key)}`"
                  :aria-pressed="revealed[field.key] ? 'true' : 'false'"
                  @click="revealed[field.key] = !revealed[field.key]"
                >
                  <component :is="revealed[field.key] ? EyeOff : Eye" class="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <p v-if="field.help" :id="`help-${field.key}`" class="mt-1 text-xs text-muted-foreground dark:text-gray-400">{{ field.help }}</p>
            </div>
          </div>
        </section>

        <!-- Default alert: what the alerts of this type inherit -->
        <section class="border bg-card p-5 dark:border-gray-700 dark:bg-gray-900">
          <header class="mb-3">
            <h2 class="text-base font-semibold text-foreground dark:text-gray-100">Default alert</h2>
            <p class="mt-0.5 text-xs text-muted-foreground dark:text-gray-400">What every {{ info.name }} alert of an endpoint inherits when it does not set its own value.</p>
          </header>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="block">
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Failure threshold</span>
              <Input v-model="form.defaultAlert.failureThreshold" type="number" min="1" placeholder="3" :readonly="readOnly" class="mt-1.5 dark:border-gray-700" data-testid="alert-provider-failure-threshold" />
              <span class="mt-1 block text-xs text-muted-foreground dark:text-gray-400">Failures in a row that trigger the alert.</span>
            </label>
            <label class="block">
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Success threshold</span>
              <Input v-model="form.defaultAlert.successThreshold" type="number" min="1" placeholder="2" :readonly="readOnly" class="mt-1.5 dark:border-gray-700" data-testid="alert-provider-success-threshold" />
              <span class="mt-1 block text-xs text-muted-foreground dark:text-gray-400">Successes in a row that resolve it.</span>
            </label>
            <div>
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Send on resolved</span>
              <Select v-model="form.defaultAlert.sendOnResolved" :options="sendOnResolvedOptions" :disabled="readOnly" class="mt-1.5" data-testid="alert-provider-send-on-resolved" />
            </div>
            <label class="block">
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Minimum reminder interval</span>
              <Input v-model="form.defaultAlert.minimumReminderInterval" placeholder="e.g. 30m; empty sends no reminder" :readonly="readOnly" class="mt-1.5 dark:border-gray-700" />
              <span class="mt-1 block text-xs text-muted-foreground dark:text-gray-400">Repeats a triggered alert at most this often, at least 5m.</span>
            </label>
            <label class="block sm:col-span-2">
              <span class="block text-sm font-medium text-foreground dark:text-gray-200">Description</span>
              <Input v-model="form.defaultAlert.description" :readonly="readOnly" class="mt-1.5 dark:border-gray-700" />
            </label>
          </div>
        </section>

        <section class="border bg-card px-5 py-3 text-xs text-muted-foreground dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400">
          <template v-if="overridesCount">{{ overridesCount }} group {{ overridesCount === 1 ? 'override' : 'overrides' }} ({{ overrideGroups.join(', ') }}). </template>
          Group overrides<template v-if="hasClientSection">, the client options</template> and the provider-override of the default alert are edited in YAML mode, and kept as they are by this form.
        </section>
      </div>

      <div v-else class="border bg-card p-4 dark:border-gray-700 dark:bg-gray-900">
        <textarea
          v-model="yamlText"
          :readonly="readOnly"
          spellcheck="false"
          aria-label="Definition in YAML"
          data-testid="alert-provider-yaml"
          class="min-h-[22rem] w-full border border-input bg-background p-3 font-mono text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-gray-700 dark:text-gray-100"
        ></textarea>
        <p class="mt-2 text-xs text-muted-foreground dark:text-gray-400">The keys of the {{ info.type }} block of the alerting section, without "type". Environment variables such as ${TOKEN} are not expanded.</p>
      </div>

      <div v-if="shadowedDefinition" class="mt-4 border bg-card p-4 dark:border-gray-700 dark:bg-gray-900">
        <h2 class="mb-2 text-sm font-semibold text-foreground dark:text-gray-100">Shadowed web notification</h2>
        <pre class="overflow-x-auto bg-muted/40 p-3 font-mono text-xs text-foreground dark:bg-gray-800 dark:text-gray-200">{{ shadowedDefinition }}</pre>
      </div>

      <!-- Save is the primary action; validating and testing never store anything -->
      <div class="mt-6 flex flex-wrap items-center gap-2 border-t pt-4 dark:border-gray-700">
        <template v-if="!readOnly">
          <Button variant="outline" :disabled="busy" data-testid="admin-validate" @click="validate">Validate</Button>
        </template>
        <Button variant="secondary" :disabled="busy || (readOnly && itemState !== 'valid')" data-testid="alert-provider-send-test" @click="sendTest">
          <Send class="mr-1 h-4 w-4" aria-hidden="true" />{{ testing ? 'Sending…' : 'Send test' }}
        </Button>
        <label class="flex items-center gap-2 text-sm text-muted-foreground dark:text-gray-400" title="Sends the resolution right after the notification, so that an incident opened by the test (PagerDuty, Opsgenie, issues…) is closed">
          <input v-model="testResolve" type="checkbox" class="h-4 w-4 accent-gray-900 dark:accent-gray-100" data-testid="alert-provider-test-resolve" />
          Trigger and resolve
        </label>
        <label v-if="overrideGroups.length" class="flex items-center gap-2 text-sm text-muted-foreground dark:text-gray-400">
          as group
          <select v-model="testGroup" class="h-9 border border-input bg-background px-2 text-sm text-foreground dark:border-gray-700 dark:text-gray-100" data-testid="alert-provider-test-group">
            <option value="">(default)</option>
            <option v-for="group in overrideGroups" :key="group" :value="group">{{ group }}</option>
          </select>
        </label>
        <Button v-if="!readOnly" :disabled="busy" class="sm:ml-auto" data-testid="admin-save" @click="save">{{ isNew ? 'Create notification' : 'Save' }}</Button>
      </div>

      <div v-if="testResult" role="status" data-testid="alert-provider-test-result" :class="['mt-4 border px-4 py-3 text-sm', testResult.success ? 'border-green-300 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-200' : 'border-red-300 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200']">
        <p class="font-medium">
          {{ testResult.success ? 'The test notification was accepted' : 'The test notification failed' }}<template v-if="testResult.resolved">, and so was its resolution</template> · {{ testResult.durationMs }} ms
        </p>
        <p v-if="testResult.error" class="mt-1 whitespace-pre-line break-words font-mono text-xs">{{ testResult.error }}</p>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Eye, EyeOff, Lock, Send } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import Loading from '@/components/Loading.vue'
import { alertProvidersApi, describeAlertProviderError, jsonPayload, yamlPayload } from '@/utils/adminApi'
import { INHERIT, MASKED, NO, YES, documentFromForm, formFromDocument, missingRequired } from '@/utils/alertProviderForm'
import { toast } from '@/utils/toast'
import { toYaml } from '@/utils/yaml'

const props = defineProps({
  providerType: { type: String, default: '' },
  isNew: { type: Boolean, default: false },
})

const router = useRouter()

const tabs = [
  { label: 'Form', value: 'form' },
  { label: 'YAML', value: 'yaml' },
]
const sendOnResolvedOptions = [
  { label: 'Default (no)', value: INHERIT },
  { label: 'Yes', value: YES },
  { label: 'No', value: NO },
]
// Words written in capitals in the labels
const ACRONYMS = { url: 'URL', id: 'ID', api: 'API', sid: 'SID', hec: 'HEC', ttl: 'TTL' }

const loading = ref(true)
const busy = ref(false)
const testing = ref(false)
const error = ref('')
const versionConflict = ref(false)
const info = ref(null)
const version = ref(0)
const source = ref('none')
const itemState = ref('')
const itemError = ref('')
const shadowedDefinition = ref('')
const mode = ref('form')
const yamlText = ref('')
const testGroup = ref('')
// Sends the resolution after the notification, so that a test leaves no incident open
const testResolve = ref(true)
const testResult = ref(null)
const invalidFields = ref([])
const revealed = reactive({})
// The definition the form starts from: what the form does not show is kept from it
const baseDocument = ref({})
const form = reactive({ values: {}, defaultAlert: {} })

const readOnly = computed(() => source.value === 'yaml')
const title = computed(() => {
  if (!info.value) {
    return 'Notification'
  }
  if (props.isNew) {
    return `New ${info.value.name} notification`
  }
  return readOnly.value ? info.value.name : `Edit ${info.value.name}`
})
const documentationUrl = computed(() => `https://github.com/TwiN/gatus#configuring-${info.value.type.replace(/-/g, '')}-alerts`)
const currentDocument = () => documentFromForm(info.value, form, baseDocument.value)
const overrideGroups = computed(() => (Array.isArray(baseDocument.value.overrides) ? baseDocument.value.overrides.map((override) => override && override.group).filter(Boolean) : []))
const overridesCount = computed(() => overrideGroups.value.length)
const hasClientSection = computed(() => (info.value ? ['custom', 'email', 'gitea', 'googlechat', 'mattermost', 'sendgrid', 'teams', 'telegram'].includes(info.value.type) : false))

const labelOf = (key) => key.split('-').map((word, index) => ACRONYMS[word] || (index === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word)).join(' ')

const inputType = (field) => {
  if (field.secret && !revealed[field.key]) {
    return 'password'
  }
  return field.kind === 'integer' ? 'number' : 'text'
}

const placeholderOf = (field) => {
  if (field.default) {
    return field.default
  }
  if (field.kind === 'list') {
    return 'Comma-separated values'
  }
  if (field.kind === 'duration') {
    return 'e.g. 30s'
  }
  return ''
}

const applyDocument = (document) => {
  baseDocument.value = document || {}
  const values = formFromDocument(info.value, baseDocument.value)
  form.values = values.values
  form.defaultAlert = values.defaultAlert
}

const clearMessages = () => {
  error.value = ''
  versionConflict.value = false
  invalidFields.value = []
}

const payload = () => {
  if (mode.value === 'yaml') {
    return yamlPayload(yamlText.value)
  }
  return jsonPayload({ type: info.value.type, ...currentDocument() })
}

// The type being edited, whose stored secrets the masks stand for
const editedType = () => (props.isNew ? '' : info.value.type)

const load = async () => {
  loading.value = true
  clearMessages()
  try {
    const { data } = await alertProvidersApi.get(props.providerType)
    if (props.isNew && data.source !== 'none') {
      // The type is already configured: edit it instead of creating a second one
      router.replace({ name: 'AdminAlertProviderEdit', params: { providerType: data.type } })
      return
    }
    info.value = data.info
    version.value = props.isNew ? 0 : data.version || 0
    source.value = props.isNew ? 'none' : data.source
    itemState.value = data.state || ''
    itemError.value = data.error || ''
    shadowedDefinition.value = data.managedDefinition ? data.managedDefinition.yaml : ''
    applyDocument(data.definition ? data.definition.json : {})
    yamlText.value = data.definition && data.definition.yaml ? data.definition.yaml : ''
    if (props.isNew && info.value.fields.length === 0) {
      mode.value = 'yaml'
    }
  } catch (e) {
    error.value = describeAlertProviderError(e)
  } finally {
    loading.value = false
  }
}

const switchMode = async (target) => {
  if (target === mode.value) {
    return
  }
  clearMessages()
  if (target === 'yaml') {
    const document = currentDocument()
    yamlText.value = Object.keys(document).length ? `${toYaml(document)}\n` : ''
    mode.value = 'yaml'
    return
  }
  if (!yamlText.value.trim()) {
    applyDocument({})
    mode.value = 'form'
    return
  }
  busy.value = true
  try {
    const { data } = await alertProvidersApi.parse(yamlPayload(yamlText.value))
    const document = data.json || {}
    delete document.type
    applyDocument(document)
    mode.value = 'form'
  } catch (e) {
    error.value = describeAlertProviderError(e)
  } finally {
    busy.value = false
  }
}

const replaceSecret = (key) => {
  form.values[key] = ''
  revealed[key] = false
}

// Marks the fields named by a validation error, and keeps the message at the top otherwise
const showError = (e) => {
  error.value = describeAlertProviderError(e)
  versionConflict.value = e && e.status === 412
  const message = (e && e.message) || ''
  invalidFields.value = ((info.value && info.value.fields) || []).map((field) => field.key).filter((key) => message.includes(key))
}

const validate = async () => {
  clearMessages()
  busy.value = true
  try {
    await alertProvidersApi.validate(payload(), editedType())
    toast.success('The definition is valid.')
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
  }
}

const sendTest = async () => {
  clearMessages()
  busy.value = true
  testing.value = true
  testResult.value = null
  try {
    const { data } = readOnly.value
      ? await alertProvidersApi.testConfigured(info.value.type, testGroup.value, testResolve.value)
      : await alertProvidersApi.test(payload(), editedType(), testGroup.value, testResolve.value)
    testResult.value = data
    if (data.success) {
      toast.success(`Test notification sent with ${info.value.name}${data.resolved ? ', and resolved' : ''}.`)
    } else {
      toast.error(`${info.value.name} did not accept the test notification.`)
    }
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
    testing.value = false
  }
}

const save = async () => {
  clearMessages()
  if (mode.value === 'form') {
    const missing = missingRequired(info.value, form)
    if (missing.length) {
      invalidFields.value = missing
      error.value = `Fill in the required ${missing.length === 1 ? 'field' : 'fields'}: ${missing.map(labelOf).join(', ')}.`
      return
    }
  }
  busy.value = true
  try {
    const { data } = props.isNew
      ? await alertProvidersApi.create(payload())
      : await alertProvidersApi.update(info.value.type, payload(), version.value)
    const restarted = (data.restarted || []).length
    toast.success(`${info.value.name} saved.${restarted ? ` ${restarted} ${restarted === 1 ? 'endpoint was' : 'endpoints were'} restarted to use it.` : ''}`)
    for (const warning of data.warnings || []) {
      toast.warning(warning)
    }
    if (props.isNew) {
      router.replace({ name: 'AdminAlertProviderEdit', params: { providerType: data.type } })
      return
    }
    version.value = data.version
    applyDocument(data.definition ? data.definition.json : {})
    yamlText.value = data.definition ? data.definition.yaml : ''
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
  }
}

// Keeps the draft: only the version and the stored values come from the server, and the form is filled with them
const reloadCurrentVersion = async () => {
  await load()
  toast.info('Current version loaded.')
}

const goBack = () => router.push({ name: 'AdminAlertProviders' })

watch(() => [props.providerType, props.isNew], load)
onMounted(load)
</script>
