<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="min-h-screen flex flex-col bg-background text-foreground" data-testid="public-layout">
    <header class="app-header border-b bg-card/50 dark:border-gray-800">
      <div class="container mx-auto px-4 py-3 max-w-5xl flex items-center justify-between gap-4">
        <component
          :is="link ? 'a' : 'div'"
          :href="link || undefined"
          :target="link ? '_blank' : undefined"
          :rel="link ? 'noopener' : undefined"
          class="flex items-center gap-3 min-w-0"
        >
          <img v-if="logo" :src="logo" alt="" class="w-10 h-10 object-contain flex-shrink-0" />
          <span class="text-lg font-semibold truncate">{{ header }}</span>
        </component>
        <ThemeSelector testid="public-theme-toggle" />
      </div>
    </header>
    <main class="flex-1">
      <slot />
    </main>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import ThemeSelector from '@/components/ThemeSelector.vue'

const templateValue = (value, placeholder) => (value && value !== placeholder ? value : '')

const logo = computed(() => templateValue(window.config?.logo, '{{ .UI.Logo }}'))
const header = computed(() => templateValue(window.config?.header, '{{ .UI.Header }}') || 'Status')
const link = computed(() => templateValue(window.config?.link, '{{ .UI.Link }}') || null)

</script>
