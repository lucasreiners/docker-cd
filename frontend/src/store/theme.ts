import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { palettes } from '../styles/tokens'

export const useThemeStore = defineStore('theme', () => {
  const STORAGE_KEY = 'docker-cd-theme'
  const isDark = ref(true)

  // Initialize theme from localStorage or system preference
  const initializeTheme = () => {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored !== null) {
      // User has explicitly set a preference
      isDark.value = stored === 'true'
    } else {
      // Fall back to system preference
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      isDark.value = prefersDark
    }
    updateDocumentTheme()
  }

  // Update document theme attribute and color-scheme
  const updateDocumentTheme = () => {
    const theme = isDark.value ? 'dark' : 'light'
    document.documentElement.setAttribute('data-theme', theme)
    document.documentElement.style.colorScheme = theme
    for (const [key, value] of Object.entries(palettes[theme])) {
      document.documentElement.style.setProperty(`--${key}`, value)
    }
  }

  // Toggle between light and dark with smooth coordinated transition
  const toggleTheme = () => {
    isDark.value = !isDark.value
  }

  // Set theme explicitly
  const setTheme = (dark: boolean) => {
    isDark.value = dark
  }

  // Watch for theme changes and persist
  watch(isDark, (newValue) => {
    localStorage.setItem(STORAGE_KEY, String(newValue))
    updateDocumentTheme()
  })

  // Listen for system preference changes (only if user hasn't set preference)
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  mediaQuery.addEventListener('change', (e) => {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === null) {
      // Only update if user hasn't set explicit preference
      isDark.value = e.matches
    }
  })

  // Initialize on store creation
  initializeTheme()

  return {
    isDark,
    toggleTheme,
    setTheme,
  }
})
