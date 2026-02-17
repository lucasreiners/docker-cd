<template>
  <div>
    <n-page-header @back="$router.push('/')" style="margin-bottom: 16px">
      <template #title>
        <n-text strong>{{ stackPath }}</n-text>
      </template>
      <template #extra>
        <n-space :size="8" align="center" :wrap="false">
          <n-tag
            v-if="stack && stack.containersTotal > 0"
            size="small"
            round
            :type="containerPillType"
            :bordered="false"
          >
            {{ stack.containersRunning }}/{{ stack.containersTotal }}
          </n-tag>
          <StatusBadge v-if="stack" :status="stack.status" />
        </n-space>
      </template>
    </n-page-header>

    <n-alert v-if="!stack" type="warning" title="Stack not found">
      The stack "{{ stackPath }}" was not found in the current state.
    </n-alert>

    <template v-else>
      <n-card style="margin-bottom: 16px">
        <n-descriptions :column="1" label-placement="left" bordered>
          <n-descriptions-item label="Path">{{ stack.path }}</n-descriptions-item>
          <n-descriptions-item label="Compose File">{{ stack.composeFile }}</n-descriptions-item>
          <n-descriptions-item label="Compose Hash">
            <n-text code>{{ stack.composeHash }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item label="Status">
            <StatusBadge :status="stack.status" />
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedRevision" label="Synced Revision">
            <n-text code>{{ stack.syncedRevision }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedCommitMessage" label="Commit Message">
            {{ stack.syncedCommitMessage }}
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedComposeHash" label="Synced Compose Hash">
            <n-text code>{{ stack.syncedComposeHash }}</n-text>
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.syncedAt" label="Synced At">
            {{ formatTime(stack.syncedAt) }}
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.lastSyncAt" label="Last Sync At">
            {{ formatTime(stack.lastSyncAt) }}
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.lastSyncStatus" label="Last Sync Status">
            <StatusBadge :status="stack.lastSyncStatus" />
          </n-descriptions-item>
          <n-descriptions-item v-if="stack.lastSyncError" label="Last Sync Error">
            <n-text type="error">{{ stack.lastSyncError }}</n-text>
          </n-descriptions-item>
        </n-descriptions>
      </n-card>

      <!-- Containers section -->
      <n-card title="Containers" style="margin-bottom: 16px">
        <n-spin v-if="containersLoading" size="small" />
        <n-text v-else-if="containersError" type="error" style="font-size: 13px">
          {{ containersError }}
        </n-text>
        <n-empty v-else-if="containers.length === 0" description="No containers found" />
        <div v-else class="container-list">
          <div
            v-for="c in containers"
            :key="c.id"
            class="container-row"
          >
            <div class="container-service">
              <n-tag
                size="small"
                round
                :type="containerStateType(c.state)"
                :bordered="false"
                style="min-width: 70px; text-align: center"
              >
                {{ c.state }}
              </n-tag>
              <n-text strong style="font-size: 13px">{{ c.service }}</n-text>
            </div>
            <div class="container-details">
              <n-text :depth="3" style="font-size: 12px">{{ c.image }}</n-text>
              <n-tag
                v-if="c.health && c.health !== 'none'"
                size="tiny"
                round
                :type="healthType(c.health)"
                :bordered="false"
              >
                {{ c.health }}
              </n-tag>
              <n-text v-if="c.ports" :depth="3" style="font-size: 11px; font-family: monospace">
                {{ c.ports }}
              </n-text>
            </div>
            <n-text :depth="3" style="font-size: 11px; font-family: monospace">
              {{ c.id }}
            </n-text>
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
    return new Date(iso).toLocaleString()
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
  gap: 12px;
}

.container-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 12px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.container-service {
  display: flex;
  align-items: center;
  gap: 8px;
}

.container-details {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-left: 78px;
}
</style>
