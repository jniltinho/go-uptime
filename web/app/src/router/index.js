// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { createRouter, createWebHistory } from 'vue-router'
import Home from '@/pages/Home'
import EndpointDetails from '@/pages/EndpointDetails'
import SuiteDetails from '@/pages/SuiteDetails'
import AdminEndpoints from '@/pages/admin/AdminEndpoints'
import AdminEndpointForm from '@/pages/admin/AdminEndpointForm'
import AdminStatusPages from '@/pages/admin/AdminStatusPages'
import AdminStatusPageForm from '@/pages/admin/AdminStatusPageForm'
import AdminPushKeys from '@/pages/admin/AdminPushKeys'
import AdminBackup from '@/pages/admin/AdminBackup'
import AdminAlertProviders from '@/pages/admin/AdminAlertProviders'
import AdminAlertProviderForm from '@/pages/admin/AdminAlertProviderForm'
import StatusPage from '@/pages/public/StatusPage'
import StatusPageEndpoint from '@/pages/public/StatusPageEndpoint'
import LoginPage from '@/pages/LoginPage'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: Home,
  },
  // Login screen of security.basic (fork): no dashboard header, see App.vue
  {
    path: '/login',
    name: 'Login',
    component: LoginPage,
    meta: { login: true },
  },
  {
    path: '/endpoints/:key',
    name: 'EndpointDetails',
    component: EndpointDetails,
  },
  {
    path: '/suites/:key',
    name: 'SuiteDetails',
    component: SuiteDetails,
  },
  // Administration of endpoints (fork): meta.admin gives a compact header, meta.adminList a layout that fills the window
  {
    path: '/admin',
    name: 'AdminEndpoints',
    component: AdminEndpoints,
    meta: { admin: true, adminList: true },
  },
  {
    path: '/admin/endpoints/new',
    name: 'AdminEndpointNew',
    component: AdminEndpointForm,
    meta: { admin: true },
  },
  {
    path: '/admin/endpoints/:endpointKey/edit',
    name: 'AdminEndpointEdit',
    component: AdminEndpointForm,
    props: true,
    meta: { admin: true },
  },
  // Administration of the status pages (fork)
  {
    path: '/admin/status-pages',
    name: 'AdminStatusPages',
    component: AdminStatusPages,
    meta: { admin: true, adminList: true },
  },
  {
    path: '/admin/status-pages/new',
    name: 'AdminStatusPageNew',
    component: AdminStatusPageForm,
    // Fork: the form fills the window and scrolls inside its columns, like the lists and the Backup tab
    meta: { admin: true, adminList: true },
  },
  {
    path: '/admin/status-pages/:slug/edit',
    name: 'AdminStatusPageEdit',
    component: AdminStatusPageForm,
    props: true,
    meta: { admin: true, adminList: true },
  },
  // Global push keys (fork)
  {
    path: '/admin/push-keys',
    name: 'AdminPushKeys',
    component: AdminPushKeys,
    meta: { admin: true, adminList: true },
  },
  // Alerting providers managed through the web (fork): the list, and the form of a type, new or configured
  {
    path: '/admin/alert-providers',
    name: 'AdminAlertProviders',
    component: AdminAlertProviders,
    meta: { admin: true, adminList: true },
  },
  {
    path: '/admin/alert-providers/new',
    name: 'AdminAlertProviderNew',
    component: AdminAlertProviderForm,
    props: (route) => ({ providerType: String(route.query.type || ''), isNew: true }),
    meta: { admin: true },
  },
  {
    path: '/admin/alert-providers/:providerType/edit',
    name: 'AdminAlertProviderEdit',
    component: AdminAlertProviderForm,
    props: true,
    meta: { admin: true },
  },
  // Backup and restore of the administration (fork): a form page, it does not fill the window like the lists
  {
    path: '/admin/backup',
    name: 'AdminBackup',
    component: AdminBackup,
    meta: { admin: true, adminList: true },
  },
  // Public status pages (fork): no login screen and no call to /api/v1/config, see App.vue
  {
    path: '/status/:slug([a-z0-9-]{1,64})',
    name: 'PublicStatusPage',
    component: StatusPage,
    meta: { public: true },
  },
  {
    path: '/status/:slug([a-z0-9-]{1,64})/endpoints/:key',
    name: 'PublicStatusPageEndpoint',
    component: StatusPageEndpoint,
    meta: { public: true },
  },
  {
    path: '/status/:pathMatch(.*)*',
    name: 'PublicStatusPageNotFound',
    component: StatusPage,
    meta: { public: true },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

export default router
