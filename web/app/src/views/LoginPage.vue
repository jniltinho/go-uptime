<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="relative min-h-screen bg-background px-4 text-foreground" data-testid="login-page">
    <div class="absolute right-4 top-4">
      <ThemeSelector testid="login-theme-toggle" />
    </div>

    <!-- The card starts at 10% of the height of the window -->
    <div class="mx-auto w-full max-w-sm pb-8 pt-[10vh]">
      <form class="border bg-card p-6 shadow-sm dark:border-gray-700" data-testid="login-card" @submit.prevent="submit">
        <div class="mb-6 flex flex-col items-center gap-3 text-center">
          <img v-if="logo" :src="logo" alt="" class="h-12 w-12 object-contain" />
          <div>
            <h1 class="text-xl font-semibold tracking-tight" data-testid="login-title">{{ header }}</h1>
            <p class="mt-1 text-sm text-muted-foreground">Sign in to continue</p>
          </div>
        </div>

        <div
          v-if="error"
          role="alert"
          class="mb-4 border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-900/30 dark:text-red-300"
          data-testid="login-error"
        >
          {{ error }}
        </div>

        <div class="space-y-4">
          <div class="space-y-1.5">
            <label for="login-username" class="text-sm font-medium">Username</label>
            <input
              id="login-username"
              ref="usernameInput"
              v-model="username"
              type="text"
              name="username"
              autocomplete="username"
              autocapitalize="none"
              spellcheck="false"
              required
              class="h-10 w-full border border-input bg-background px-3 text-sm text-foreground dark:border-gray-700 dark:text-gray-100"
              data-testid="login-username"
            />
          </div>
          <div class="space-y-1.5">
            <label for="login-password" class="text-sm font-medium">Password</label>
            <input
              id="login-password"
              v-model="password"
              type="password"
              name="password"
              autocomplete="current-password"
              required
              class="h-10 w-full border border-input bg-background px-3 text-sm text-foreground dark:border-gray-700 dark:text-gray-100"
              data-testid="login-password"
            />
          </div>
          <Button type="submit" class="h-10 w-full" :disabled="submitting" data-testid="login-submit">
            <Loading v-if="submitting" size="xs" />
            <template v-else>Sign in</template>
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { Button } from '@/components/ui/button'
import Loading from '@/components/Loading.vue'
import { PROTECTED_API_HEADERS } from '@/utils/auth'
import ThemeSelector from '@/components/ThemeSelector.vue'

const props = defineProps({
  // Reloads /api/v1/config in App.vue and returns whether the request is now authenticated
  reloadConfig: {
    type: Function,
    required: true,
  },
})

const templateValue = (value, placeholder) => (value && value !== placeholder ? value : '')

const logo = templateValue(window.config?.logo, '{{ .UI.Logo }}')
const header = templateValue(window.config?.header, '{{ .UI.Header }}') || 'Status'

const username = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)
const usernameInput = ref(null)
const submit = async () => {
  if (submitting.value) {
    return
  }
  error.value = ''
  submitting.value = true
  try {
    const response = await fetch('/api/v1/auth/login', {
      method: 'POST',
      credentials: 'include',
      headers: { ...PROTECTED_API_HEADERS, 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username.value, password: password.value }),
    })
    if (response.status === 204) {
      password.value = ''
      // The session cookie is HttpOnly: only the configuration tells whether the session is used. App.vue leaves the
      // login screen once it is.
      if (!(await props.reloadConfig())) {
        error.value = 'The session could not be started. Check that cookies are allowed for this site.'
      }
      return
    }
    if (response.status === 401) {
      error.value = 'Invalid username or password'
    } else if (response.status === 429) {
      error.value = 'Too many failed attempts. Try again in a minute.'
    } else {
      error.value = 'Unable to sign in. Try again later.'
    }
  } catch (e) {
    error.value = 'Unable to sign in. Try again later.'
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  usernameInput.value?.focus()
})
</script>
