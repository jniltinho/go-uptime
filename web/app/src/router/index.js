// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import {createRouter, createWebHistory} from 'vue-router'
import Home from '@/views/Home'
import EndpointDetails from "@/views/EndpointDetails";
import SuiteDetails from '@/views/SuiteDetails';
import AdminEndpoints from '@/views/admin/AdminEndpoints';
import AdminEndpointForm from '@/views/admin/AdminEndpointForm';
import AdminStatusPages from '@/views/admin/AdminStatusPages';
import AdminStatusPageForm from '@/views/admin/AdminStatusPageForm';
import AdminPushKeys from '@/views/admin/AdminPushKeys';
import AdminBackup from '@/views/admin/AdminBackup';
import StatusPage from '@/views/public/StatusPage';
import StatusPageEndpoint from '@/views/public/StatusPageEndpoint';
import LoginPage from '@/views/LoginPage';

const routes = [
    {
        path: '/',
        name: 'Home',
        component: Home
    },
    // Login screen of security.basic (fork): no dashboard header, see App.vue
    {
        path: '/login',
        name: 'Login',
        component: LoginPage,
        meta: { login: true }
    },
    {
        path: '/endpoints/:key',
        name: 'EndpointDetails',
        component: EndpointDetails,
    },
    {
        path: '/suites/:key',
        name: 'SuiteDetails',
        component: SuiteDetails
    },
    // Administration of endpoints (fork): meta.admin gives a compact header, meta.adminList a layout that fills the window
    {
        path: '/admin',
        name: 'AdminEndpoints',
        component: AdminEndpoints,
        meta: { admin: true, adminList: true }
    },
    {
        path: '/admin/endpoints/new',
        name: 'AdminEndpointNew',
        component: AdminEndpointForm,
        meta: { admin: true }
    },
    {
        path: '/admin/endpoints/:endpointKey/edit',
        name: 'AdminEndpointEdit',
        component: AdminEndpointForm,
        props: true,
        meta: { admin: true }
    },
    // Administration of the status pages (fork)
    {
        path: '/admin/status-pages',
        name: 'AdminStatusPages',
        component: AdminStatusPages,
        meta: { admin: true, adminList: true }
    },
    {
        path: '/admin/status-pages/new',
        name: 'AdminStatusPageNew',
        component: AdminStatusPageForm,
        // Fork: the form fills the window and scrolls inside its columns, like the lists and the Backup tab
        meta: { admin: true, adminList: true }
    },
    {
        path: '/admin/status-pages/:slug/edit',
        name: 'AdminStatusPageEdit',
        component: AdminStatusPageForm,
        props: true,
        meta: { admin: true, adminList: true }
    },
    // Global push keys (fork)
    {
        path: '/admin/push-keys',
        name: 'AdminPushKeys',
        component: AdminPushKeys,
        meta: { admin: true, adminList: true }
    },
    // Backup and restore of the administration (fork): a form page, it does not fill the window like the lists
    {
        path: '/admin/backup',
        name: 'AdminBackup',
        component: AdminBackup,
        meta: { admin: true, adminList: true }
    },
    // Public status pages (fork): no login screen and no call to /api/v1/config, see App.vue
    {
        path: '/status/:slug([a-z0-9-]{1,64})',
        name: 'PublicStatusPage',
        component: StatusPage,
        meta: { public: true }
    },
    {
        path: '/status/:slug([a-z0-9-]{1,64})/endpoints/:key',
        name: 'PublicStatusPageEndpoint',
        component: StatusPageEndpoint,
        meta: { public: true }
    },
    {
        path: '/status/:pathMatch(.*)*',
        name: 'PublicStatusPageNotFound',
        component: StatusPage,
        meta: { public: true }
    }
];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL),
    routes
});

export default router;
