// Pinia store for stack state management.
// Maintains an in-memory map of stacks, updated via SSE push events.
// Provides filtering and search computed properties.

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { RefreshSnapshot, StackRecord } from '../services/api'
import { fetchRefreshStatus, fetchStacks } from '../services/api'
import type { ConnectionState } from '../services/sse'
import { SSEClient } from '../services/sse'

export interface ImagePullStatus {
  image: string
  status: string
  progress: string
  done: boolean
  layers: LayerPullStatus[]
}

export interface LayerPullStatus {
  id: string
  status: string
  progress: string
  bytesCurrent: number
  bytesTotal: number
  done: boolean
}

export interface PullProgressState {
  stack: string
  images: ImagePullStatus[]
  current: number
  total: number
}

export const useStacksStore = defineStore('stacks', () => {
  // State
  const stackMap = ref<Map<string, StackRecord>>(new Map())
  const refreshStatus = ref<RefreshSnapshot | null>(null)
  const connectionState = ref<ConnectionState>('disconnected')
  const filterStatus = ref<string>('')
  const searchQuery = ref<string>('')
  const loading = ref(false)
  const error = ref<string | null>(null)
  // biome-ignore lint/suspicious/noExplicitAny: Progress type varies by event
  const updateProgress = ref<any>(null)
  const isUpdating = ref(false)
  const pullProgress = ref<PullProgressState | null>(null)

  let sseClient: SSEClient | null = null
  let progressTimer: ReturnType<typeof setTimeout> | null = null
  let stackVersion = 0
  let refreshVersion = 0

  // Getters
  const stacks = computed<StackRecord[]>(() => {
    return Array.from(stackMap.value.values())
  })

  const filteredStacks = computed<StackRecord[]>(() => {
    let result = stacks.value

    if (filterStatus.value) {
      result = result.filter((s) => s.status === filterStatus.value)
    }

    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase()
      result = result.filter((s) => s.path.toLowerCase().includes(q))
    }

    return result.sort((a, b) => a.path.localeCompare(b.path))
  })

  const statusCounts = computed(() => {
    const counts: Record<string, number> = {
      synced: 0,
      syncing: 0,
      failed: 0,
      missing: 0,
      deleting: 0,
    }
    for (const s of stacks.value) {
      counts[s.status] = (counts[s.status] ?? 0) + 1
    }
    return counts
  })

  const isConnected = computed(() => connectionState.value === 'connected')
  const isReconnecting = computed(() => connectionState.value === 'reconnecting')

  // Actions
  async function loadInitial() {
    const initialStackVersion = stackVersion
    const initialRefreshVersion = refreshVersion
    loading.value = true
    error.value = null
    try {
      const [stackList, refresh] = await Promise.all([fetchStacks(), fetchRefreshStatus()])
      if (stackVersion === initialStackVersion) {
        stackMap.value = new Map(stackList.map((s) => [s.path, s]))
      }
      if (refreshVersion === initialRefreshVersion) refreshStatus.value = refresh
    } catch (e) {
      if (stackVersion === initialStackVersion) {
        error.value = e instanceof Error ? e.message : 'Failed to load stacks'
      }
    } finally {
      loading.value = false
    }
  }

  function connectSSE() {
    if (sseClient) {
      sseClient.close()
    }

    sseClient = new SSEClient({
      onSnapshot(records) {
        stackVersion++
        loading.value = false
        error.value = null
        stackMap.value = new Map(records.map((s) => [s.path, s]))
        if (pullProgress.value && !records.some((s) => s.path === pullProgress.value?.stack)) {
          pullProgress.value = null
        }
      },
      onUpsert(record) {
        stackVersion++
        const next = new Map(stackMap.value)
        next.set(record.path, record)
        stackMap.value = next
      },
      onDelete(path) {
        stackVersion++
        const next = new Map(stackMap.value)
        next.delete(path)
        stackMap.value = next
      },
      onRefreshStatus(snapshot) {
        refreshVersion++
        refreshStatus.value = snapshot as RefreshSnapshot
      },
      onUpdateSnapshot(progress) {
        if (progressTimer) clearTimeout(progressTimer)
        updateProgress.value = null
        pullProgress.value = null
        isUpdating.value = false
        for (const item of progress) {
          const p = item as { type: string; source?: string }
          const terminal =
            ['completed', 'cancelled'].includes(p.type) ||
            (p.source === 'reconcile' && ['stack_success', 'stack_error'].includes(p.type))
          if (!terminal) handleProgress(p)
        }
      },
      onUpdateProgress: handleProgress,
      onConnectionChange(state) {
        connectionState.value = state
      },
    })

    sseClient.connect()
  }

  // biome-ignore lint/suspicious/noExplicitAny: Progress type varies by event
  function handleProgress(progress: any) {
    if (progressTimer) clearTimeout(progressTimer)
    if (progress.type !== 'image_pull_progress') updateProgress.value = progress
    const terminal = progress.type === 'completed' || progress.type === 'cancelled'
    if (progress.source !== 'reconcile' && progress.cycle_id) {
      isUpdating.value = !terminal
    }
    if (progress.type === 'started') {
      isUpdating.value = true
      pullProgress.value = null
    } else if (progress.type === 'stack_sync_started' || progress.type === 'stack_deploying') {
      pullProgress.value = null
    } else if (progress.type === 'image_pull_progress') {
      const isDone =
        progress.done === true || (!progress.layerId && progress.status === 'Pull complete')

      if (!pullProgress.value || pullProgress.value.stack !== progress.stack) {
        pullProgress.value = {
          stack: progress.stack,
          images: [],
          current: progress.current,
          total: progress.total,
        }
      }

      let existing = pullProgress.value.images.find((i) => i.image === progress.image)
      if (!existing) {
        existing = {
          image: progress.image,
          status: 'Pulling',
          progress: '',
          done: false,
          layers: [],
        }
        pullProgress.value.images.push(existing)
        existing = pullProgress.value.images[pullProgress.value.images.length - 1]
      }
      if (progress.layerId) {
        const layer: LayerPullStatus = {
          id: progress.layerId,
          status: progress.status,
          progress: progress.progress ?? '',
          bytesCurrent: progress.bytesCurrent ?? 0,
          bytesTotal: progress.bytesTotal ?? 0,
          done: progress.status === 'Pull complete' || progress.status === 'Already exists',
        }
        const index = existing.layers.findIndex((l) => l.id === layer.id)
        if (index < 0) existing.layers.push(layer)
        else existing.layers[index] = layer
      } else {
        existing.status = progress.status
        existing.progress = progress.progress ?? ''
        existing.done = isDone
      }
      pullProgress.value.current = progress.current
      pullProgress.value.total = progress.total
    } else if (progress.type === 'stack_success' || progress.type === 'stack_error' || terminal) {
      if (terminal || pullProgress.value?.stack === progress.stack) pullProgress.value = null
      if (terminal) {
        isUpdating.value = false
      }
      if (terminal || (progress.source === 'reconcile' && !isUpdating.value)) {
        // Clear progress after a short delay to show completion message
        progressTimer = setTimeout(() => {
          updateProgress.value = null
        }, 3000)
      }
    }
  }

  function disconnectSSE() {
    if (progressTimer) clearTimeout(progressTimer)
    sseClient?.close()
    sseClient = null
  }

  function setFilterStatus(status: string) {
    filterStatus.value = status
  }

  function setSearchQuery(query: string) {
    searchQuery.value = query
  }

  function getStack(path: string): StackRecord | undefined {
    return stackMap.value.get(path)
  }

  return {
    // State
    stackMap,
    refreshStatus,
    connectionState,
    filterStatus,
    searchQuery,
    loading,
    error,
    updateProgress,
    isUpdating,
    pullProgress,
    // Getters
    stacks,
    filteredStacks,
    statusCounts,
    isConnected,
    isReconnecting,
    // Actions
    loadInitial,
    connectSSE,
    disconnectSSE,
    setFilterStatus,
    setSearchQuery,
    getStack,
  }
})
