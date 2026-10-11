<template>
  <button
    class="icon-button"
    :aria-label="theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'"
    :title="theme.isDark ? 'Light theme' : 'Dark theme'"
    @click="theme.toggleTheme()"
  >
    <Sun v-if="theme.isDark" :size="17" /><Moon v-else :size="17" />
  </button>
</template>
<script setup lang="ts">
import { Moon, Sun } from 'lucide-vue-next'
import { useThemeStore } from '../store/theme'

const theme = useThemeStore()
</script>
