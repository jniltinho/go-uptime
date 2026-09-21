<template>
  <!-- Fork: with three themes a button that toggles does not say where it goes. This is a button that shows the theme in
       use and opens a menu with one option per theme, used by the settings of the dashboard, the header of the public
       pages and the login screen. -->
  <div ref="root" class="relative inline-block text-left">
    <button
      ref="button"
      type="button"
      :class="buttonClass"
      aria-haspopup="menu"
      :aria-expanded="open"
      :aria-controls="menuId"
      :aria-label="`Theme: ${THEMES[theme].label}. Change theme`"
      :data-testid="testid"
      @click="toggle"
      @keydown.down.prevent="openAt(0)"
      @keydown.up.prevent="openAt(themeNames.length - 1)"
    >
      <component :is="ICONS[theme]" :class="compact ? 'h-3.5 w-3.5' : 'h-4 w-4'" aria-hidden="true" />
      <span v-if="!compact" class="hidden sm:inline">{{ THEMES[theme].label }}</span>
    </button>
    <ul
      v-show="open"
      :id="menuId"
      role="menu"
      aria-label="Theme"
      :class="['absolute z-50 min-w-[9rem] border border-input bg-popover py-1 text-sm text-popover-foreground shadow-md dark:border-gray-700', placementClass]"
      :data-testid="`${testid}-menu`"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
      @keydown.home.prevent="focusItem(0)"
      @keydown.end.prevent="focusItem(themeNames.length - 1)"
      @keydown.esc.prevent="close(true)"
      @keydown.tab="close(false)"
    >
      <li v-for="(name, index) in themeNames" :key="name" role="none">
        <button
          :ref="(element) => { items[index] = element }"
          type="button"
          role="menuitemradio"
          :aria-checked="name === theme"
          tabindex="-1"
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent focus:bg-accent focus:outline-none"
          :data-testid="`${testid}-option-${name}`"
          @click="choose(name)"
        >
          <component :is="ICONS[name]" class="h-4 w-4 shrink-0" aria-hidden="true" />
          <span class="flex-1">{{ THEMES[name].label }}</span>
          <Check v-if="name === theme" class="h-4 w-4 shrink-0" aria-hidden="true" />
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { Check, Droplets, Moon, Sun } from 'lucide-vue-next'
import { THEMES, currentTheme, setTheme } from '@/utils/theme'

const props = defineProps({
  // Base of the data-testid of the button, of the menu (-menu) and of the options (-option-<theme>)
  testid: { type: String, default: 'theme-selector' },
  // compact shows only the icon, for the settings bar of the dashboard
  compact: { type: Boolean, default: false },
  // Where the menu opens: below the button aligned to its right edge, or above it aligned to its left edge
  placement: { type: String, default: 'bottom-end' }
})

const emit = defineEmits(['change'])

const ICONS = { light: Sun, dark: Moon, bio: Droplets }
const themeNames = Object.keys(THEMES)
const menuId = `${props.testid}-menu`

const theme = ref(currentTheme())
const open = ref(false)
const root = ref(null)
const button = ref(null)
const items = ref([])

const buttonClass = computed(() => (props.compact
  ? 'p-1.5 rounded-none hover:bg-accent transition-colors'
  : 'inline-flex h-9 items-center gap-2 border border-input bg-background px-3 text-sm hover:bg-accent motion-safe:transition-colors dark:border-gray-700 dark:hover:bg-gray-800'))
const placementClass = computed(() => (props.placement === 'top-start' ? 'bottom-full left-0 mb-2' : 'right-0 top-full mt-1'))

const focusItem = (index) => {
  items.value[(index + themeNames.length) % themeNames.length]?.focus()
}
const openAt = async (index) => {
  open.value = true
  await nextTick()
  focusItem(index)
}
const toggle = () => (open.value ? close(false) : openAt(themeNames.indexOf(theme.value)))
const move = (step) => {
  const index = items.value.findIndex((element) => element === document.activeElement)
  focusItem((index === -1 ? 0 : index) + step)
}
const close = (returnFocus) => {
  open.value = false
  if (returnFocus) {
    button.value?.focus()
  }
}
const choose = (name) => {
  theme.value = setTheme(name)
  emit('change', theme.value)
  close(true)
}

const handleOutsideClick = (event) => {
  if (open.value && root.value && !root.value.contains(event.target)) {
    close(false)
  }
}
// Another selector on the page, or the settings of the dashboard, changed the theme
const handleThemeChange = (event) => {
  theme.value = event.detail?.theme || currentTheme()
}

onMounted(() => {
  document.addEventListener('click', handleOutsideClick)
  window.addEventListener('go-uptime:theme', handleThemeChange)
})
onUnmounted(() => {
  document.removeEventListener('click', handleOutsideClick)
  window.removeEventListener('go-uptime:theme', handleThemeChange)
})
</script>
