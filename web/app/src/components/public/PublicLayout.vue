<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div class="screen-scale min-h-screen flex flex-col bg-background text-foreground" data-testid="public-layout">
    <header class="app-header border-b bg-card/50 dark:border-gray-800" data-testid="public-header">
      <!-- Fork: a slim bar, with only the logo, the header and the theme -->
      <div class="mx-auto flex h-10 w-full max-w-[1760px] items-center justify-between gap-4 px-4 sm:px-12">
        <component
          :is="link ? 'a' : 'div'"
          :href="link || undefined"
          :target="link ? '_blank' : undefined"
          :rel="link ? 'noopener' : undefined"
          class="flex min-w-0 items-center gap-2"
        >
          <img v-if="logo" :src="logo" alt="" class="h-5 w-5 flex-shrink-0 object-contain" />
          <span class="truncate text-sm font-semibold tracking-tight">{{ header }}</span>
        </component>
        <ThemeSelector testid="public-theme-toggle" compact />
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
