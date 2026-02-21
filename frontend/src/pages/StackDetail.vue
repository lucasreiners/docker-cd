<template>
  <div>
    <n-page-header @back="$router.push('/')" style="margin-bottom: 24px">
      <template #title>
        <n-text strong style="font-size: 20px">{{ stackPath }}</n-text>
      </template>
      <template #extra>
        <n-space :size="12" align="center" :wrap="false">
          <n-tag
            v-if="stack && stack.containersTotal != null && stack.containersTotal > 0"
            size="medium"
            round
            :type="containerPillType"
            :bordered="false"
          >
            {{ stack.containersRunning }}/{{ stack.containersTotal }} containers
          </n-tag>
          <StatusBadge v-if="stack" :status="stack.status" />
        </n-space>
      </template>
    </n-page-header>

    <n-alert v-if="!stack" type="warning" title="Stack not found">
      The stack "{{ stackPath }}" was not found in the current state.
    </n-alert>

    <template v-else>
      <!-- Overview Section -->
      <n-card title="Overview" :segmented="{ content: true }" style="margin-bottom: 16px">
        <n-descriptions :column="2" label-placement="left" :label-style="{ fontWeight: '600' }">
          <n-descriptions-item label="Path">
            <n-text style="font-family: monospace">{{ stack.path }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item label="Compose File">
            <n-text code>{{ stack.composeFile }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item label="Status">
            <StatusBadge :status="stack.status" />
          </n-descriptions-item>
          <n-descriptions-item label="Compose Hash">
            <n-text code style="font-size: 12px">{{ stack.composeHash }}</n-text>
          </n-descriptions-item>
        </n-descriptions>
      </n-card>

      <!-- Git Information Section -->
      <n-card 
        v-if="stack.syncedRevision || stack.syncedCommitMessage"
        title="Git Information" 
        :segmented="{ content: true }" 
        style="margin-bottom: 16px"
      >
        <n-descriptions :column="1" label-placement="left" :label-style="{ fontWeight: '600' }">
          <n-descriptions-item v-if="stack.syncedRevision" label="Revision">
            <n-text code>{{ stack.syncedRevision }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedCommitMessage" label="Commit Message">
            <n-text>{{ stack.syncedCommitMessage }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedComposeHash" label="Synced Compose Hash">
            <n-text code style="font-size: 12px">{{ stack.syncedComposeHash }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedAt" label="Synced At">
            <n-text>{{ formatTime(stack.syncedAt) }}</n-text>
          </n-descriptions-item>
        </n-descriptions>
      </n-card>

      <!-- Sync Status Section -->
      <n-card 
        v-if="stack.lastSyncAt || stack.lastSyncStatus || stack.lastSyncError"
        title="Last Sync Status" 
        :segmented="{ content: true }" 
        style="margin-bottom: 16px"
      >
        <n-descriptions :column="2" label-placement="left" :label-style="{ fontWeight: '600' }">
          <n-descriptions-item v-if="stack.lastSyncAt" label="Last Sync">
            <n-text>{{ formatTime(stack.lastSyncAt) }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.lastSyncStatus" label="Result">
            <StatusBadge :status="stack.lastSyncStatus" />
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.lastSyncError" label="Error" :span="2">
            <n-text type="error">{{ stack.lastSyncError }}</n-text>
          </n-descriptions-item>
        </n-descriptions>
      </n-card>

      <!-- Containers Section -->
      <n-card title="Containers" :segmented="{ content: true }">
        <n-spin v-if="containersLoading" size="small" />
        <n-text v-else-if="containersError" type="error" style="font-size: 13px">
          {{ containersError }}
        </n-text>
        <n-empty v-else-if="containers.length === 0" description="No containers found" />
        <div v-else class="container-list">
          <div
            v-for="c in containers"
            :key="c.id"
            class="container-card"
          >
            <div class="container-header">
              <div class="container-title">
                <n-tag
                  size="small"
                  round
                  :type="containerStateType(c.state)"
                  :bordered="false"
                  style="min-width: 70px; text-align: center"
                >
                  {{ c.state }}
                </n-tag>
                <n-text strong style="font-size: 14px">{{ c.service }}</n-text>
                <n-tag
                  v-if="c.health && c.health !== 'none'"
                  size="small"
                  round
                  :type="healthType(c.health)"
                  :bordered="false"
                >
                  {{ c.health }}
                </n-tag>
              </div>
            </div>
            <div class="container-info">
              <div class="info-row">
                <n-text :depth="2" style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px">Image</n-text>
                <n-text style="font-size: 12px; font-family: monospace">{{ c.image }}</n-text>
              </div>
              <div v-if="c.ports" class="info-row">
                <n-text :depth="2" style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px">Ports</n-text>
                <n-text style="font-size: 12px; font-family: monospace">{{ c.ports }}</n-text>
              </div>
              <div class="info-row">
                <n-text :depth="2" style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px">Container ID</n-text>
                <n-text :depth="3" style="font-size: 11px; font-family: monospace">{{ c.id }}</n-text>
              </div>
            </div>
          </div>
        </div>
      </n-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import StatusBadge from '../components/StatusBadge.vue'
import type { ContainerInfo } from '../services/api'
import { fetchContainers } from '../services/api'
import { useStacksStore } from '../store/stacks'

const route = useRoute()
const store = useStacksStore()

const containers = ref<ContainerInfo[]>([])
const containersLoading = ref(false)
const containersError = ref<string | null>(null)

const stackPath = computed(() => {
  const p = route.params.path
  return Array.isArray(p) ? p.join('/') : p
})

const stack = computed(() => store.getStack(stackPath.value))

const containerPillType = computed<'success' | 'warning' | 'error'>(() => {
  if (!stack.value) return 'error'
  if (stack.value.containersRunning === stack.value.containersTotal) return 'success'
  if (stack.value.containersRunning === 0) return 'error'
  return 'warning'
})

async function loadContainers() {
  containersLoading.value = true
  containersError.value = null
  try {
    containers.value = await fetchContainers(stackPath.value)
  } catch (e) {
    containersError.value = e instanceof Error ? e.message : 'Failed to load containers'
  } finally {
    containersLoading.value = false
  }
}

function containerStateType(state: string): 'success' | 'warning' | 'error' | 'info' | 'default' {
  switch (state) {
    case 'running':
      return 'success'
    case 'restarting':
      return 'warning'
    case 'paused':
      return 'info'
    case 'exited':
    case 'dead':
      return 'error'
    default:
      return 'default'
  }
}

function healthType(health: string): 'success' | 'warning' | 'error' | 'default' {
  switch (health) {
    case 'healthy':
      return 'success'
    case 'starting':
      return 'warning'
    case 'unhealthy':
      return 'error'
    default:
      return 'default'
  }
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

onMounted(() => {
  loadContainers()
})

// Reload containers when stack path changes
watch(stackPath, () => {
  loadContainers()
})
</script>

<style scoped>
.container-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.container-card {
  border-radius: 8px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  transition: all 0.2s ease;
}

.container-card:hover {
  border-color: var(--border-hover);
  background: var(--card-hover-bg);
}

.container-header {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-secondary);
  border-radius: 8px 8px 0 0;
}

.container-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.container-info {
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.info-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-row:not(:last-child) {
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border-color);
}

/* Responsive layout for larger screens */
@media (min-width: 768px) {
  .info-row {
    flex-direction: row;
    justify-content: space-between;
    align-items: center;
  }
}
</style>
