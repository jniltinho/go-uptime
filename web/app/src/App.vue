<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div id="global" class="bg-background text-foreground">
    <!-- Waiting for the router: the layout depends on the route -->
    <div v-if="!routerReady" class="flex items-center justify-center min-h-screen">
      <Loading size="lg" />
    </div>

    <!-- Public status pages (fork): no dashboard header, no login screen and no call to /api/v1/config -->
    <PublicLayout v-else-if="isPublic">
      <router-view />
    </PublicLayout>

    <!-- Loading State, and redirection from or to the login screen of security.basic (fork) -->
    <div v-else-if="!retrievedConfig || pendingLoginRedirect" class="flex items-center justify-center min-h-screen">
      <Loading size="lg" />
    </div>

    <!-- Login screen of security.basic (fork): no dashboard header -->
    <router-view v-else-if="isLogin" :reload-config="reloadConfigAfterLogin" />

    <!-- Main App Container (fork: compact header, same height on the dashboard and on the administration) -->
    <!-- Fork: the lists of the administration fill the window on larger screens and scroll inside their tables -->
    <div v-else-if="!config || !config.oidc || config.authenticated" :class="['relative', isAdminList && 'md:flex md:h-screen md:flex-col md:overflow-hidden']">
      <!-- Header -->
      <header :class="['app-header border-b bg-card/50 backdrop-blur supports-[backdrop-filter]:bg-card/60', isAdminList && 'md:shrink-0']">
        <div class="container mx-auto px-4 py-2 max-w-7xl">
          <div class="flex items-center justify-between">
            <!-- Logo and Title -->
            <div class="flex items-center gap-4">
              <component
                :is="link ? 'a' : 'div'"
                :href="link"
                target="_blank"
                :class="['flex items-center gap-3', link && 'hover:opacity-80 transition-opacity']"
              >
                <div v-if="logo" class="flex items-center justify-center w-8 h-8">
                  <img
                    :src="logo"
                    :alt="header"
                    class="w-full h-full object-contain"
                  />
                </div>
                <div>
                  <h1 class="text-lg font-bold tracking-tight">{{ header }}</h1>
                </div>
              </component>
            </div>

            <!-- Right Side Actions -->
            <div class="flex items-center gap-2">
              <!-- Administration of endpoints (fork) -->
              <router-link
                v-if="showAdminLink"
                to="/admin"
                class="px-3 py-2 text-sm font-medium hover:bg-accent hover:text-accent-foreground dark:hover:bg-gray-800 transition-colors"
                data-testid="admin-link"
              >
                Admin
              </router-link>
              <!-- Logout of the login screen of security.basic (fork) -->
              <button
                v-if="showLogout"
                type="button"
                class="inline-flex items-center gap-2 px-3 py-2 text-sm font-medium hover:bg-accent hover:text-accent-foreground dark:hover:bg-gray-800 transition-colors disabled:opacity-50"
                :disabled="loggingOut"
                data-testid="logout-button"
                @click="logout"
              >
                <LogOut class="h-4 w-4" aria-hidden="true" />
                Logout
              </button>
              <!-- Navigation Links (Desktop) -->
              <nav v-if="buttons && buttons.length" class="hidden md:flex items-center gap-1">
                <a
                  v-for="button in buttons"
                  :key="button.name"
                  :href="button.link"
                  target="_blank"
                  class="px-3 py-2 text-sm font-medium rounded-md hover:bg-accent hover:text-accent-foreground transition-colors"
                >
                  {{ button.name }}
                </a>
              </nav>

              <!-- Mobile Menu Button -->
              <Button
                v-if="buttons && buttons.length"
                variant="ghost"
                size="icon"
                class="md:hidden"
                @click="mobileMenuOpen = !mobileMenuOpen"
              >
                <Menu v-if="!mobileMenuOpen" class="h-5 w-5" />
                <X v-else class="h-5 w-5" />
              </Button>
            </div>
          </div>

          <!-- Mobile Navigation -->
          <nav
            v-if="buttons && buttons.length && mobileMenuOpen"
            class="md:hidden mt-4 pt-4 border-t space-y-1"
          >
            <a
              v-for="button in buttons"
              :key="button.name"
              :href="button.link"
              target="_blank"
              class="block px-3 py-2 text-sm font-medium rounded-md hover:bg-accent hover:text-accent-foreground transition-colors"
              @click="mobileMenuOpen = false"
            >
              {{ button.name }}
            </a>
          </nav>
        </div>
      </header>

      <!-- Main Content -->
      <main :class="['relative', isAdminList && 'md:flex md:min-h-0 md:flex-1 md:flex-col']">
        <router-view @showTooltip="showTooltip" :announcements="announcements" />
      </main>

    </div>

    <!-- OIDC Login Screen -->
    <div v-else id="login-container" class="flex items-center justify-center min-h-screen p-4">
      <Card class="w-full max-w-md">
        <CardHeader class="text-center">
          <img v-if="logo" :src="logo" alt="" class="w-20 h-20 object-contain mx-auto mb-4" />
          <CardTitle class="text-3xl">{{ header }}</CardTitle>
          <p class="text-muted-foreground mt-2">{{ loginSubtitle }}</p>
        </CardHeader>
        <CardContent>
          <div v-if="route && route.query.error" class="mb-6">
            <div class="p-3 rounded-md bg-destructive/10 border border-destructive/20">
              <p class="text-sm text-destructive text-center">
                <span v-if="route.query.error === 'access_denied'">
                  You do not have access to this status page
                </span>
                <span v-else>{{ route.query.error }}</span>
              </p>
            </div>
          </div>

          <a
            :href="`/oidc/login`"
            class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90 h-11 px-8 w-full"
            @click="isOidcLoading = true"
          >
            <Loading v-if="isOidcLoading" size="xs" />
            <template v-else>
              <LogIn class="mr-2 h-4 w-4" />
              Login with OIDC
            </template>
          </a>
        </CardContent>
      </Card>
    </div>

    <!-- Tooltip -->
    <Tooltip v-if="routerReady && !isPublic && !isLogin" :result="tooltip.result" :event="tooltip.event" :isPersistent="tooltipIsPersistent" />

    <!-- Fork: toast messages, only with the authenticated app visible (never on public pages or login screens) -->
    <AdminToasts v-if="showsToasts" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu, X, LogIn, LogOut } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import Tooltip from './components/Tooltip.vue'
import AdminToasts from './components/admin/AdminToasts.vue'
import Loading from './components/Loading.vue'
import PublicLayout from './components/public/PublicLayout.vue'
import { PROTECTED_API_HEADERS, UNAUTHORIZED_EVENT } from '@/utils/auth'
import { clearToasts } from '@/utils/toast'
import { safeRedirect } from '@/utils/redirect'

const route = useRoute()
const router = useRouter()

// The layout depends on the route, so nothing is shown before the router resolves the first one
const routerReady = ref(false)
const isPublic = computed(() => route.meta.public === true)
// Login screen of security.basic (fork)
const isLogin = computed(() => route.meta.login === true)
// Fork: the header is compact on every page (dashboard and administration), and the lists of the administration fill
// the window
const isAdminList = computed(() => route.meta.adminList === true)
let configLoadingStarted = false

// State
const retrievedConfig = ref(false)
const config = ref({ oidc: false, authenticated: true })
const announcements = ref([])
const tooltip = ref({})
const mobileMenuOpen = ref(false)
const isOidcLoading = ref(false)
const tooltipIsPersistent = ref(false)
const loggingOut = ref(false)
let configInterval = null
// After a logout, the login screen opens without redirect back to the current page
let loggedOut = false

// Computed properties
const logo = computed(() => {
  return window.config && window.config.logo && window.config.logo !== '{{ .UI.Logo }}' ? window.config.logo : ""
})

const header = computed(() => {
  return window.config && window.config.header && window.config.header !== '{{ .UI.Header }}' ? window.config.header : "Status"
})

const link = computed(() => {
  return window.config && window.config.link && window.config.link !== '{{ .UI.Link }}' ? window.config.link : null
})

const buttons = computed(() => {
  return window.config && window.config.buttons ? window.config.buttons : []
})

const showAdminLink = computed(() => {
  return Boolean(config.value && config.value.admin && config.value.admin.enabled && config.value.admin.authorized)
})

const usesBasicLogin = computed(() => Boolean(config.value && config.value.login === 'basic'))

const showLogout = computed(() => usesBasicLogin.value && config.value.authenticated === true)

// With the login screen of security.basic, the protected screens wait for the redirection to /login without
// authentication, and /login waits for the redirection back once authenticated or without security.basic
const pendingLoginRedirect = computed(() => {
  if (!usesBasicLogin.value) {
    return isLogin.value
  }
  return isLogin.value ? config.value.authenticated === true : config.value.authenticated !== true
})

// Fork: the toasts only exist with the main app container visible, see the v-else-if chain of the template
const showsToasts = computed(() => routerReady.value && !isPublic.value && retrievedConfig.value && !pendingLoginRedirect.value
  && !isLogin.value && (!config.value || !config.value.oidc || config.value.authenticated))

const loginSubtitle = computed(() => {
  return window.config && window.config.loginSubtitle && window.config.loginSubtitle !== '{{ .UI.LoginSubtitle }}' ? window.config.loginSubtitle : "System Monitoring Dashboard"
})

// Methods
const fetchConfig = async () => {
  try {
    const response = await fetch(`/api/v1/config`, { credentials: 'include' })
    if (response.status === 200) {
      const data = await response.json()
      config.value = data
      announcements.value = data.announcements || []
    }
    retrievedConfig.value = true
  } catch (error) {
    console.error('Failed to fetch config:', error)
    retrievedConfig.value = true
  }
}

// Called by the login screen after a successful login: the session cookie is HttpOnly, so only the configuration tells
// whether the session is used
const reloadConfigAfterLogin = async () => {
  loggedOut = false
  await fetchConfig()
  return config.value.authenticated === true
}

const logout = async () => {
  if (loggingOut.value) {
    return
  }
  loggingOut.value = true
  try {
    await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include', headers: PROTECTED_API_HEADERS })
  } catch (error) {
    console.error('[App][logout] Failed to log out:', error)
  }
  loggedOut = true
  await fetchConfig()
  loggingOut.value = false
}

// A 401 of the protected API: the session expired or was closed elsewhere
const handleUnauthorized = () => {
  if (usesBasicLogin.value && !isPublic.value && !isLogin.value) {
    fetchConfig()
  }
}

const showTooltip = (result, event, action = 'hover') => {
  if (action === 'click') {
    if (!result) {
      // Deselecting
      tooltip.value = {}
      tooltipIsPersistent.value = false
    } else {
      // Selecting new data point
      tooltip.value = { result, event }
      tooltipIsPersistent.value = true
    }
  } else if (action === 'hover') {
    // Only update tooltip on hover if not in persistent mode
    if (!tooltipIsPersistent.value) {
      tooltip.value = { result, event }
    }
  }
}

const handleDocumentClick = (event) => {
  // Close persistent tooltip when clicking outside
  if (tooltipIsPersistent.value) {
    const tooltipElement = document.getElementById('tooltip')
    // Check if click is on a data point bar or inside tooltip
    const clickedDataPoint = event.target.closest('.flex-1.h-6, .flex-1.h-8')

    if (tooltipElement && !tooltipElement.contains(event.target) && !clickedDataPoint) {
      tooltip.value = {}
      tooltipIsPersistent.value = false
      // Emit event to clear selections in child components
      window.dispatchEvent(new CustomEvent('clear-data-point-selection'))
    }
  }
}

// The config (and the OIDC login screen) is only needed outside of the public status pages: it is fetched when the first
// non-public route is shown, and only then refreshed every 10 minutes for announcements
watch([routerReady, isPublic], ([ready, isPublicRoute]) => {
  if (!ready || isPublicRoute || configLoadingStarted) {
    return
  }
  configLoadingStarted = true
  fetchConfig()
  configInterval = setInterval(fetchConfig, 600000)
})

// Login screen of security.basic (fork): without authentication, the protected screens go to /login with the current
// path as redirect, and /login goes back to the validated redirect once authenticated
watch([routerReady, retrievedConfig, config, () => route.fullPath], () => {
  if (!routerReady.value || !retrievedConfig.value || isPublic.value || !pendingLoginRedirect.value) {
    return
  }
  if (isLogin.value) {
    router.replace(usesBasicLogin.value ? safeRedirect(route.query.redirect) : '/')
    return
  }
  const query = loggedOut ? {} : { redirect: route.fullPath }
  loggedOut = false
  router.replace({ path: '/login', query })
})

// Fork: the messages of a screen are discarded when another screen opens
const removeAfterEach = router.afterEach((to, from, failure) => {
  if (!failure && to.path !== from.path) {
    clearToasts()
  }
})

onMounted(() => {
  router.isReady().then(() => {
    routerReady.value = true
  })
  // Add click listener for closing persistent tooltips
  document.addEventListener('click', handleDocumentClick)
  window.addEventListener(UNAUTHORIZED_EVENT, handleUnauthorized)
})

// Clean up interval on unmount
onUnmounted(() => {
  removeAfterEach()
  if (configInterval) {
    clearInterval(configInterval)
    configInterval = null
  }
  // Remove click listener
  document.removeEventListener('click', handleDocumentClick)
  window.removeEventListener(UNAUTHORIZED_EVENT, handleUnauthorized)
})
</script>
