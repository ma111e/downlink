// sources page bundle: the topbar theme picker. The blocking pre-paint theme
// IIFE stays inline in the template (it must run before first paint and carries
// server-rendered theme data).
import '../css/sources.css'

const THEME_KEY = 'downlink.theme'
const sel = document.getElementById('theme') as HTMLSelectElement | null
if (sel) {
  const root = document.documentElement
  const current = localStorage.getItem(THEME_KEY) || root.dataset.theme || 'dark'
  root.dataset.theme = current
  sel.value = current
  sel.addEventListener('change', () => {
    root.dataset.theme = sel.value
    localStorage.setItem(THEME_KEY, sel.value)
  })
}
