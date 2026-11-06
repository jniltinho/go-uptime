// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import router from './router'

// Fork: --view-h is the usable height of the window in CSS pixels after the zoom of large screens (style.css). A fixed
// element that fills the window measures it, which works whether or not the engine scales dvh with the zoom.
const measureViewHeight = () => {
  const probe = document.createElement('div')
  probe.style.cssText = 'position:fixed;inset:0;visibility:hidden;pointer-events:none'
  document.body.appendChild(probe)
  document.documentElement.style.setProperty('--view-h', `${probe.offsetHeight}px`)
  probe.remove()
}
measureViewHeight()
window.addEventListener('resize', measureViewHeight)

createApp(App).use(router).mount('#app')
