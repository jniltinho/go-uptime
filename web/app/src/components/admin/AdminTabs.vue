<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <nav :class="['flex border-b dark:border-gray-700', compact ? '' : 'mb-6']" aria-label="Administration sections" data-testid="admin-tabs">
    <router-link
      v-for="tab in tabs"
      :key="tab.id"
      :to="{ name: tab.route }"
      :aria-current="active === tab.id ? 'page' : undefined"
      :data-testid="`admin-tab-${tab.id}`"
      :class="[
        '-mb-px border-b-2 px-4 text-sm font-medium',
        compact ? 'py-1.5' : 'py-2',
        active === tab.id
          ? 'border-foreground text-foreground dark:border-gray-100 dark:text-gray-100'
          : 'border-transparent text-muted-foreground hover:text-foreground dark:text-gray-400 dark:hover:text-gray-100'
      ]"
    >
      {{ tab.label }}
    </router-link>
  </nav>
</template>

<script setup>
defineProps({
  // endpoints, status-pages, push-keys or backup (fork)
  active: { type: String, required: true },
  // Without the bottom margin, for the layout of the lists of the administration
  compact: { type: Boolean, default: false }
})

const tabs = [
  { id: 'endpoints', label: 'Endpoints', route: 'AdminEndpoints' },
  { id: 'status-pages', label: 'Status pages', route: 'AdminStatusPages' },
  { id: 'push-keys', label: 'Push keys', route: 'AdminPushKeys' },
  // Fork: backup and restore of the items managed through the web
  { id: 'backup', label: 'Backup', route: 'AdminBackup' }
]
</script>
