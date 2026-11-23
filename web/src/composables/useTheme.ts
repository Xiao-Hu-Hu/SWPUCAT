import { readonly, ref } from 'vue'

type Theme = 'light' | 'dark'
const storageKey = 'swpucat-theme'
const theme = ref<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light')
const system = window.matchMedia('(prefers-color-scheme: dark)')
let preference: Theme | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let initialized = false

function parse(value: string | null): Theme | null {
  return value === 'light' || value === 'dark' ? value : null
}

function apply(value: Theme, animate = true) {
  const root = document.documentElement
  if (theme.value !== value && animate && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    clearTimeout(timer)
    root.classList.add('theme-changing')
    timer = setTimeout(() => root.classList.remove('theme-changing'), 600)
  }
  root.dataset.theme = value
  root.classList.toggle('dark', value === 'dark')
  root.style.colorScheme = value
  theme.value = value
}

// One application-wide listener, independent of page/component lifetimes.
export function initializeTheme() {
  if (initialized) return
  initialized = true
  try { preference = parse(localStorage.getItem(storageKey)) } catch { /* Use system preference. */ }
  apply(preference ?? (system.matches ? 'dark' : 'light'), false)
  system.addEventListener('change', event => {
    if (!preference) apply(event.matches ? 'dark' : 'light')
  })
  window.addEventListener('storage', event => {
    if (event.key !== storageKey && event.key !== null) return
    preference = parse(event.newValue)
    apply(preference ?? (system.matches ? 'dark' : 'light'))
  })
}

export function useTheme() {
  return {
    theme: readonly(theme),
    toggleTheme() {
      preference = theme.value === 'dark' ? 'light' : 'dark'
      apply(preference)
      try { localStorage.setItem(storageKey, preference) } catch { /* Keep the current session usable. */ }
    }
  }
}
