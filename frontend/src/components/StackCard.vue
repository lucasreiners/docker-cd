<template>
  <n-card
    hoverable
    class="stack-card"
    :class="{ 'stack-card--failed': stack.status === 'failed' }"
    @click="$router.push({ name: 'stack-detail', params: { path: stack.path.split('/') } })"
    style="cursor: pointer"
  >
    <template #header>
      <div style="display: flex; align-items: center; justify-content: space-between">
        <n-text strong :depth="1" style="font-size: 14px; word-break: break-all">
          {{ stack.path }}
        </n-text>
        <div style="display: flex; align-items: center; gap: 8px; flex-shrink: 0">
          <n-tag
            v-if="stack.containersTotal != null && stack.containersTotal > 0"
            size="small"
            round
            :type="containerPillType"
            :bordered="false"
          >
            {{ stack.containersRunning }}/{{ stack.containersTotal }}
          </n-tag>
          <StatusBadge :status="stack.status" />
        </div>
      </div>
    </template>
    <n-space vertical :size="4">
      <n-text :depth="3" style="font-size: 12px">
        {{ stack.composeFile }}
      </n-text>
      <n-text v-if="stack.syncedRevision" :depth="3" style="font-size: 12px; font-family: monospace">
        {{ stack.syncedRevision.substring(0, 8) }}
      </n-text>
      <n-text v-if="stack.syncedCommitMessage" :depth="3" style="font-size: 12px">
        {{ truncate(stack.syncedCommitMessage, 60) }}
      </n-text>
      <n-text v-if="stack.lastSyncError" type="error" style="font-size: 12px">
        {{ truncate(stack.lastSyncError, 80) }}
      </n-text>
      <n-text v-if="stack.lastSyncAt" :depth="3" style="font-size: 11px">
        Last sync: {{ formatTime(stack.lastSyncAt) }}
      </n-text>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { StackRecord } from '../services/api'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  stack: StackRecord
}>()

const containerPillType = computed<'success' | 'warning' | 'error'>(() => {
  if (props.stack.containersRunning === props.stack.containersTotal) return 'success'
  if (props.stack.containersRunning === 0) return 'error'
  return 'warning'
})

function truncate(s: string, max: number): string {
  return s.length > max ? `${s.substring(0, max)}...` : s
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleString()
  } catch {
    return iso
  }
}
</script>

<style scoped>
.stack-card {
  transition: border-color 0.2s;
}
.stack-card--failed {
  border-left: 3px solid var(--n-color-error, #e88080);
}
</style>
