<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: action of the administration lists as an icon. A link stays a link (Open), and a disabled action keeps its
       tooltip, which the disabled:pointer-events-none of the Button would swallow -->
  <component
    :is="href ? 'a' : 'button'"
    :type="href ? undefined : 'button'"
    :href="href || undefined"
    :target="href ? '_blank' : undefined"
    :rel="href ? 'noopener' : undefined"
    :aria-label="label"
    :title="label"
    :aria-disabled="disabled ? 'true' : undefined"
    :tabindex="disabled ? -1 : undefined"
    :data-testid="testid"
    :class="[
      'inline-flex shrink-0 items-center justify-center border border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
      compact ? 'h-7 w-7' : 'h-9 w-9',
      disabled ? 'cursor-not-allowed opacity-50' : 'hover:bg-accent dark:hover:bg-gray-800',
      destructive ? 'text-red-600 dark:text-red-400' : 'text-foreground dark:text-gray-200'
    ]"
    @click="onClick"
  >
    <component :is="icon" :class="compact ? 'h-3.5 w-3.5' : 'h-4 w-4'" aria-hidden="true" />
  </component>
</template>

<script setup>
const props = defineProps({
  // Icon component of lucide-vue-next
  icon: { type: [Object, Function], required: true },
  // Accessible name and tooltip: the action and the name of the item, e.g. "Disable api-gateway"
  label: { type: String, required: true },
  testid: { type: String, required: true },
  disabled: { type: Boolean, default: false },
  destructive: { type: Boolean, default: false },
  // Renders an anchor that opens in another tab, for actions that lead to another page
  href: { type: String, default: '' },
  // 28 px in the tables, 36 px in the cards of the touch screens
  compact: { type: Boolean, default: true }
})

const emit = defineEmits(['click'])

const onClick = (event) => {
  if (props.disabled) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  emit('click', event)
}
</script>
