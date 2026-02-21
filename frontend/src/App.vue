<template>
  <n-config-provider :theme="theme">
    <n-message-provider>
      <n-layout style="min-height: 100vh">
        <n-layout-header bordered style="padding: 12px 24px; display: flex; align-items: center; gap: 12px">
          <n-text strong style="font-size: 18px">Docker-CD</n-text>
          <div style="flex: 1" />
          <ConnectionBanner />
          <ThemeToggle />
        </n-layout-header>
        <n-layout-content style="padding: 24px">
          <router-view />
        </n-layout-content>
      </n-layout>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { darkTheme } from 'naive-ui'
import { computed, onMounted, onUnmounted } from 'vue'
import ConnectionBanner from './components/ConnectionBanner.vue'
import ThemeToggle from './components/ThemeToggle.vue'
import { useStacksStore } from './store/stacks'
import { useThemeStore } from './store/theme'

const store = useStacksStore()
const themeStore = useThemeStore()

const theme = computed(() => (themeStore.isDark ? darkTheme : null))

onMounted(async () => {
  await store.loadInitial()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>
