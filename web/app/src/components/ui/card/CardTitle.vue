<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <h3 :class="combineClasses('text-2xl font-semibold leading-none tracking-tight', $attrs.class ?? '')">
    <slot />
  </h3>
</template>

<script setup>
import { combineClasses } from '@/utils/misc'
</script>