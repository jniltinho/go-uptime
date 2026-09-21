<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Layout of the lists of the administration (fork): on larger screens it fills the window and only the panel scrolls -->
  <div class="container mx-auto flex max-w-7xl flex-col px-4 py-4 md:min-h-0 md:flex-1" data-testid="admin-list-layout">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold tracking-tight text-foreground dark:text-gray-100">{{ title }}</h1>
        <p class="mt-0.5 truncate text-sm text-muted-foreground dark:text-gray-400" :title="description">{{ description }}</p>
      </div>
      <div class="flex shrink-0 flex-wrap items-center gap-2">
        <slot name="actions" />
      </div>
    </div>

    <AdminTabs :active="active" compact class="mt-3 shrink-0" />

    <!-- Notices bring their own top margin, so that hidden ones take no space -->
    <slot name="notices" />

    <div v-if="$slots.toolbar" class="mt-3 shrink-0">
      <slot name="toolbar" />
    </div>

    <!-- Without panel, the content fills the rest of the window and handles its own scroll (e.g. the Backup tab) -->
    <div v-if="!panel" class="mt-3 md:flex md:min-h-0 md:flex-1 md:flex-col" data-testid="admin-list-content">
      <slot />
    </div>
    <div v-else class="mt-3 flex flex-col border bg-card dark:border-gray-700 dark:bg-gray-900 md:min-h-0 md:flex-1" data-testid="admin-list-panel">
      <div class="overflow-x-auto md:min-h-0 md:flex-1 md:overflow-auto" data-testid="admin-list-scroll">
        <slot />
      </div>
      <div v-if="$slots.footer" class="shrink-0 border-t px-3 py-2 text-xs text-muted-foreground dark:border-gray-700 dark:text-gray-400" data-testid="admin-list-footer">
        <slot name="footer" />
      </div>
    </div>

    <slot name="overlay" />
  </div>
</template>

<script setup>
import AdminTabs from '@/components/admin/AdminTabs.vue'

defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  // Tab of AdminTabs: endpoints, status-pages, push-keys or backup
  active: { type: String, required: true },
  // Whether the content is inside the bordered panel with its own scroll, as in the lists
  panel: { type: Boolean, default: true }
})
</script>
