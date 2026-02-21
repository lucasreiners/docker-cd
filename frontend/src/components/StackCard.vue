<template>
  <n-card
    hoverable
    class="stack-card"
    :class="{ 
      'stack-card--failed': stack.status === 'failed',
      'stack-card--synced': stack.status === 'synced',
      'stack-card--syncing': stack.status === 'syncing'
    }"
    @click="$router.push({ name: 'stack-detail', params: { path: stack.path.split('/') } })"
    :segmented="{ content: true }"
  >
    <template #header>
      <div class="card-header">
        <div class="header-left">
          <n-text strong style="font-size: 14px; word-break: break-all; line-height: 1.4">
            {{ stack.path }}
          </n-text>
        </div>
        <div class="header-badges">
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
    
    <div class="card-content">
      <!-- Git Information -->
      <div v-if="stack.syncedRevision || stack.syncedCommitMessage" class="info-section">
        <div class="git-info">
          <div v-if="stack.syncedRevision" class="git-hash">
            <n-text :depth="3" style="font-size: 11px; font-family: monospace">
              {{ stack.syncedRevision.substring(0, 8) }}
            </n-text>
          </div>
          <n-text v-if="stack.syncedCommitMessage" :depth="2" style="font-size: 13px; line-height: 1.5">
            {{ truncate(stack.syncedCommitMessage, 65) }}
          </n-text>
        </div>
      </div>

      <!-- Error State -->
      <div v-if="stack.lastSyncError" class="error-section">
        <n-text type="error" strong style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px">
          Error
        </n-text>
        <n-text type="error" style="font-size: 12px; line-height: 1.5">
          {{ truncate(stack.lastSyncError, 100) }}
        </n-text>
      </div>

      <!-- Footer -->
      <div class="card-footer">
        <n-text :depth="3" style="font-size: 11px">
          Last sync: {{ formatTime(stack.lastSyncAt) }}
        </n-text>
      </div>
    </div>
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

function formatTime(iso: string | undefined): string {
  if (!iso) return 'Never'

  try {
    const date = new Date(iso)
    return date.toLocaleString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}
</script>

<style scoped>
.stack-card {
  transition: border-color 0.2s ease;
  cursor: pointer;
  position: relative;
  overflow: hidden;
}

.stack-card--failed {
  border-left: 4px solid var(--error-text);
}

.stack-card--synced {
  border-left: 4px solid var(--success-text);
}

.stack-card--syncing {
  border-left: 4px solid var(--warning-text);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  min-height: 36px;
}

.header-left {
  flex: 1;
  min-width: 0;
}

.header-badges {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.card-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.info-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-label {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 2px;
}

.git-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.git-hash {
  display: inline-flex;
  opacity: 0.7;
}

.error-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  border-radius: 6px;
  background: var(--accent-bg);
  border-left: 3px solid var(--error-text);
}

.card-footer {
  padding-top: 8px;
  border-top: 1px solid var(--border-color);
  margin-top: 4px;
}
</style>
