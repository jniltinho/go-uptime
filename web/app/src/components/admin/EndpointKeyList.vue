<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: keys of endpoints, the managed ones with a link to their edit screen and the ones of the configuration file
       read-only -->
  <div class="space-y-3 text-sm" data-testid="endpoint-key-list">
    <div v-if="managed.length">
      <h3 class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground dark:text-gray-400">Managed through the web</h3>
      <ul class="space-y-0.5">
        <li v-for="key in managed" :key="`managed-${key}`">
          <router-link :to="{ name: 'AdminEndpointEdit', params: { endpointKey: key } }" target="_blank" class="font-mono text-xs text-foreground underline underline-offset-2 dark:text-gray-100">{{ key }}</router-link>
        </li>
      </ul>
    </div>
    <div v-if="file.length">
      <h3 class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground dark:text-gray-400">Configuration file</h3>
      <ul class="space-y-0.5">
        <li v-for="key in file" :key="`file-${key}`" class="font-mono text-xs text-muted-foreground dark:text-gray-400">{{ key }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup>
defineProps({
  managed: { type: Array, default: () => [] },
  file: { type: Array, default: () => [] }
})
</script>
