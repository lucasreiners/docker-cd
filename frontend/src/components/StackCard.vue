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

      <!-- Containers List -->
      <div v-if="containers.length > 0 || containersLoading || containersError" class="containers-section">
        <!-- Loading State -->
        <div v-if="containersLoading" class="containers-loading">
          <n-text :depth="3" style="font-size: 12px">
            Loading containers...
          </n-text>
        </div>

        <!-- Error State -->
        <div v-else-if="containersError" class="containers-error">
          <n-text type="error" style="font-size: 12px">
            Failed to load containers
          </n-text>
        </div>

        <!-- Containers List -->
        <div v-else class="containers-list">
          <div
            v-for="container in sortedContainers"
            :key="container.id"
            class="container-item"
          >
            <!-- Container with port link -->
            <a
              v-if="container.hasExternalPorts"
              class="container-link"
              @click="handleContainerClick($event, container)"
              :title="`Open ${container.name} on port ${container.lowestExternalPort}`"
            >
              <span class="arrow">↗</span>
              <span class="underlined">{{ container.name }}</span>
            </a>

            <!-- Container without port -->
            <span v-else class="container-text">
              {{ container.name }}
            </span>
          </div>
        </div>
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
import { computed, onMounted, ref } from 'vue'
import type { ContainerInfo, StackRecord } from '../services/api'
import { fetchContainers } from '../services/api'
import { buildPortURL, getLowestExternalPort, parsePortString } from '../utils/portUtils'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  stack: StackRecord
}>()

// Container state
const containers = ref<ContainerInfo[]>([])
const containersLoading = ref(false)
const containersError = ref<string | null>(null)

// Fetch containers on mount
onMounted(async () => {
  try {
    containersLoading.value = true
    containersError.value = null
    containers.value = await fetchContainers(props.stack.path)
  } catch (error) {
    console.error(`Failed to fetch containers for ${props.stack.path}:`, error)
    containersError.value = error instanceof Error ? error.message : 'Failed to load containers'
  } finally {
    containersLoading.value = false
  }
})

// Sorted containers with port information
interface ContainerWithPorts extends ContainerInfo {
  parsedPorts: ReturnType<typeof parsePortString>
  lowestExternalPort: number | null
  hasExternalPorts: boolean
}

const sortedContainers = computed<ContainerWithPorts[]>(() => {
  return containers.value
    .map((c) => {
      const parsedPorts = parsePortString(c.ports || '')
      const lowestExternalPort = getLowestExternalPort(c.ports || '')
      const hasExternalPorts = lowestExternalPort !== null

      return {
        ...c,
        parsedPorts,
        lowestExternalPort,
        hasExternalPorts,
      }
    })
    .sort((a, b) => a.name.localeCompare(b.name))
})

// Handle container click to open port
function handleContainerClick(event: MouseEvent, container: ContainerWithPorts): void {
  if (!container.hasExternalPorts || container.lowestExternalPort === null) {
    return
  }

  event.stopPropagation() // Prevent card click from navigating
  const url = buildPortURL(container.lowestExternalPort)
  window.open(url, '_blank', 'noopener,noreferrer')
}

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
  display: flex;
  flex-direction: column;
  height: 100%;
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
  flex: 1;
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

.containers-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.containers-loading,
.containers-error {
  padding: 8px 0;
}

.containers-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.container-item {
  font-size: 12px;
  line-height: 1.6;
}

.container-link {
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: opacity 0.2s ease;
}

.container-link:hover {
  opacity: 0.7;
}

.container-link .arrow {
  font-size: 10px;
  opacity: 0.8;
}

.container-link .underlined {
  text-decoration: underline;
}

.container-text {
  color: var(--text-color-secondary);
}

.card-footer {
  padding-top: 8px;
  border-top: 1px solid var(--border-color);
  margin-top: 4px;
}
</style>
