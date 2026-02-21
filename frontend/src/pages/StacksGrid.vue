<template>
  <div>
    <!-- Summary Stats -->
    <div v-if="!store.loading && !store.error" class="summary-stats">
      <div class="stat-card">
        <n-text :depth="2" class="stat-label">Total Stacks</n-text>
        <n-text strong class="stat-value">{{ store.stacks.length }}</n-text>
      </div>
      <div class="stat-card stat-success">
        <n-text :depth="2" class="stat-label">Synced</n-text>
        <n-text strong class="stat-value">{{ store.statusCounts.synced || 0 }}</n-text>
      </div>
      <div class="stat-card stat-warning">
        <n-text :depth="2" class="stat-label">Syncing</n-text>
        <n-text strong class="stat-value">{{ store.statusCounts.syncing || 0 }}</n-text>
      </div>
      <div class="stat-card stat-error">
        <n-text :depth="2" class="stat-label">Failed</n-text>
        <n-text strong class="stat-value">{{ store.statusCounts.failed || 0 }}</n-text>
      </div>
    </div>

    <!-- Refresh status -->
    <n-card
      v-if="store.refreshStatus"
      size="small"
      title="Git State Sync"
      style="margin-bottom: 16px"
      :bordered="true"
      :segmented="{ content: true }"
    >
      <div class="refresh-bar">
        <div class="refresh-info">
          <div class="refresh-header">
            <StatusBadge :status="store.refreshStatus.refreshStatus" />
            <n-text strong style="font-size: 13px">
              {{ store.refreshStatus.ref }}
            </n-text>
            <n-text code style="font-size: 12px">
              {{ store.refreshStatus.revision?.substring(0, 8) }}
            </n-text>
          </div>
          <n-text
            v-if="store.refreshStatus.commitMessage"
            :depth="2"
            style="font-size: 12px; margin-top: 6px"
          >
            {{ truncate(store.refreshStatus.commitMessage, 120) }}
          </n-text>
          <n-text :depth="3" style="font-size: 11px; margin-top: 4px">
            Last refreshed: {{ formatTime(store.refreshStatus.refreshedAt) }}
          </n-text>
        </div>
        <n-button
          size="small"
          :loading="refreshing"
          :disabled="store.refreshStatus.refreshStatus === 'refreshing'"
          @click="doRefresh"
          secondary
        >
          Refresh Now
        </n-button>
      </div>
      <n-text
        v-if="refreshError"
        type="error"
        style="font-size: 12px; margin-top: 12px; display: block"
      >
        {{ refreshError }}
      </n-text>
    </n-card>

    <!-- Filter pills and search -->
    <div class="filter-section">
      <div class="filter-bar">
        <div class="filter-tags">
          <n-tag
            v-for="(count, status) in store.statusCounts"
            :key="status"
            size="small"
            round
            :type="statusTagType(status as string)"
            :bordered="filterStatus !== status"
            :class="{ 'filter-active': filterStatus === status }"
            style="cursor: pointer"
            @click="toggleFilter(status as string)"
          >
            {{ status }}: {{ count }}
          </n-tag>
          <n-button
            v-if="filterStatus || searchQuery"
            text
            size="small"
            @click="clearFilters"
            style="font-size: 12px"
          >
            Clear filters
          </n-button>
        </div>
        <n-input
          v-model:value="searchQuery"
          placeholder="Search stacks..."
          clearable
          size="small"
          style="max-width: 300px"
          @update:value="store.setSearchQuery"
        >
          <template #prefix>
            <n-icon>
              <svg viewBox="0 0 24 24" width="1em" height="1em" fill="currentColor">
                <path d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z" />
              </svg>
            </n-icon>
          </template>
        </n-input>
      </div>
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
      style="margin-top: 48px"
    >
      <template #description>
        <n-text v-if="filterStatus || searchQuery">
          No stacks match your filters
        </n-text>
        <n-text v-else>
          No stacks found. Stacks will appear here when they are synced from the repository.
        </n-text>
      </template>
      <template #extra>
        <n-button v-if="filterStatus || searchQuery" size="small" @click="clearFilters">
          Clear filters
        </n-button>
      </template>
    </n-empty>

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

function clearFilters() {
  filterStatus.value = ''
  searchQuery.value = ''
  store.setFilterStatus('')
  store.setSearchQuery('')
}
</script>

<style scoped>
.summary-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}

.stat-card {
  padding: 16px;
  border-radius: 8px;
  background: var(--bg-secondary);
  border: 2px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.stat-success {
  border-color: var(--success-text);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--accent-bg) 100%);
}

.stat-warning {
  border-color: var(--warning-text);
}

.stat-error {
  border-color: var(--error-text);
}

.stat-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 28px;
  line-height: 1;
}

.filter-section {
  margin-bottom: 24px;
}

.refresh-bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.refresh-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.refresh-header {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.filter-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  flex: 1;
}

.filter-active {
  box-shadow: 0 0 0 2px var(--border-hover);
  transform: scale(1.05);
}

/* Responsive adjustments */
@media (max-width: 768px) {
  .summary-stats {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }
  
  .filter-bar input {
    max-width: 100% !important;
  }
  
  .refresh-bar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
