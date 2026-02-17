<template>
  <n-config-provider :theme="darkTheme">
    <n-message-provider>
      <n-layout style="min-height: 100vh">
        <n-layout-header bordered style="padding: 12px 24px; display: flex; align-items: center; gap: 12px">
          <n-text strong style="font-size: 18px">Docker-CD</n-text>
          <ConnectionBanner />
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
import { onMounted, onUnmounted } from 'vue'
import ConnectionBanner from './components/ConnectionBanner.vue'
import { useStacksStore } from './store/stacks'

const store = useStacksStore()

onMounted(async () => {
  await store.loadInitial()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>
