<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <div :class="combineClasses('p-6 pt-0', $attrs.class ?? '')">
    <slot />
  </div>
</template>

<script setup>
import { combineClasses } from '@/utils/misc'
</script>