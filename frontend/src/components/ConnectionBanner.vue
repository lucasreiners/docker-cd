<template>
  <div
    class="connection"
    :class="{ stale: !store.isConnected || !work.restored }"
    role="status"
    data-testid="connection-banner"
  >
    <span class="connection-dot" aria-hidden="true" />
    <span v-if="store.isConnected && work.restored">Connected</span>
    <span v-else-if="store.isConnected">Restoring work state…</span>
    <span v-else-if="store.isReconnecting"
      >Reconnecting<span class="connection-detail">
        · Data may be stale</span
      ></span
    >
    <span v-else-if="hasConnectedOnce"
      >Disconnected<span class="connection-detail">
        · Data may be stale</span
      ></span
    >
    <span v-else
      >Connecting<span class="connection-detail">
        · Waiting for live data</span
      ></span
    >
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useStacksStore } from '../store/stacks'
import { useWorkStore } from '../store/work'

const store = useStacksStore()
const work = useWorkStore()
const hasConnectedOnce = ref(false)
watch(
  () => store.connectionState,
  (state) => {
    if (state === 'connected') hasConnectedOnce.value = true
  },
  { immediate: true },
)
</script>

<style scoped>
.connection {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--success-text);
  font-size: 12px;
}
.connection-dot {
  width: 6px;
  height: 6px;
  background: currentColor;
  border-radius: 50%;
  flex-shrink: 0;
}
.stale {
  color: var(--warning-text);
}
@media (max-width: 760px) {
  .connection-detail {
    display: none;
  }
}
</style>
