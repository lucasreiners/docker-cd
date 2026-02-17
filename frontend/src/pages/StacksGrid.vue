<template>
  <div>
    <!-- Refresh status -->
    <n-card
      v-if="store.refreshStatus"
      size="small"
      title="Git state sync"
      style="margin-bottom: 16px"
      :bordered="true"
    >
      <div class="refresh-bar">
        <div class="refresh-info">
          <div style="display: flex; align-items: center; gap: 8px">
            <StatusBadge :status="store.refreshStatus.refreshStatus" />
            <n-text :depth="2" style="font-size: 13px">
              {{ store.refreshStatus.ref }}
            </n-text>
            <n-text code style="font-size: 12px">
              {{ store.refreshStatus.revision?.substring(0, 8) }}
            </n-text>
          </div>
          <n-text
            v-if="store.refreshStatus.commitMessage"
            :depth="3"
            style="font-size: 12px; margin-top: 2px"
          >
            {{ truncate(store.refreshStatus.commitMessage, 120) }}
          </n-text>
          <n-text :depth="3" style="font-size: 11px; margin-top: 2px">
            {{ formatTime(store.refreshStatus.refreshedAt) }}
          </n-text>
        </div>
        <n-button
          size="small"
          :loading="refreshing"
          :disabled="store.refreshStatus.refreshStatus === 'refreshing'"
          @click="doRefresh"
        >
          Refresh
        </n-button>
      </div>
      <n-text
        v-if="refreshError"
        type="error"
        style="font-size: 12px; margin-top: 4px; display: block"
      >
        {{ refreshError }}
      </n-text>
    </n-card>

    <!-- Filter pills and search -->
    <div class="filter-bar" style="margin-bottom: 16px">
      <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
        <n-tag
          v-for="(count, status) in store.statusCounts"
          :key="status"
          size="small"
          round
          :type="statusTagType(status as string)"
          :bordered="filterStatus === status"
          style="cursor: pointer"
          @click="toggleFilter(status as string)"
        >
          {{ status }}: {{ count }}
        </n-tag>
      </div>
      <n-input
        v-model:value="searchQuery"
        placeholder="Search stacks..."
        clearable
        size="small"
        style="max-width: 280px"
        @update:value="store.setSearchQuery"
      />
    </div>

    <!-- Loading state -->
    <n-spin
      v-if="store.loading"
      size="large"
      style="display: flex; justify-content: center; margin-top: 48px"
    />

    <!-- Error state -->
    <n-alert
      v-else-if="store.error"
      type="error"
      :title="store.error"
      style="margin-top: 24px"
    />

    <!-- Empty state -->
    <n-empty
      v-else-if="store.filteredStacks.length === 0"
      description="No stacks found"
      style="margin-top: 48px"
    />

    <!-- Grid -->
    <div v-else class="stack-grid" data-testid="stacks-grid">
      <StackCard
        v-for="stack in store.filteredStacks"
        :key="stack.path"
        :stack="stack"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import StackCard from '../components/StackCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { triggerRefresh } from '../services/api'
import { useStacksStore } from '../store/stacks'

const store = useStacksStore()
const searchQuery = ref('')
const filterStatus = ref('')
const refreshing = ref(false)
const refreshError = ref<string | null>(null)

async function doRefresh() {
  refreshing.value = true
  refreshError.value = null
  try {
    await triggerRefresh()
  } catch (e) {
    refreshError.value = e instanceof Error ? e.message : 'Refresh failed'
  } finally {
    refreshing.value = false
  }
}

function truncate(s: string, max: number): string {
  return s.length > max ? `${s.substring(0, max)}...` : s
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

function statusTagType(status: string): 'success' | 'warning' | 'error' | 'info' | 'default' {
  switch (status) {
    case 'synced':
      return 'success'
    case 'syncing':
      return 'warning'
    case 'failed':
      return 'error'
    case 'missing':
      return 'info'
    case 'deleting':
      return 'warning'
    default:
      return 'default'
  }
}

function toggleFilter(status: string) {
  if (filterStatus.value === status) {
    filterStatus.value = ''
    store.setFilterStatus('')
  } else {
    filterStatus.value = status
    store.setFilterStatus(status)
  }
}
</script>

<style scoped>
.refresh-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.refresh-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
