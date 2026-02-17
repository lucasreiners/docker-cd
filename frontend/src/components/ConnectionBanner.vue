<template>
  <n-alert
    v-if="store.isReconnecting"
    type="warning"
    :bordered="false"
    style="padding: 4px 12px"
    data-testid="connection-banner"
  >
    Reconnecting to server...
  </n-alert>
  <n-alert
    v-else-if="store.connectionState === 'disconnected' && hasConnectedOnce"
    type="error"
    :bordered="false"
    style="padding: 4px 12px"
    data-testid="connection-banner"
  >
    Disconnected from server. Data may be stale.
  </n-alert>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useStacksStore } from '../store/stacks'

const store = useStacksStore()
const hasConnectedOnce = ref(false)

watch(
  () => store.connectionState,
  (state) => {
    if (state === 'connected') {
      hasConnectedOnce.value = true
    }
  },
)
</script>
