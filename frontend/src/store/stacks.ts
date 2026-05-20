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
    loading.value = true
    error.value = null
    try {
      const [stackList, refresh] = await Promise.all([fetchStacks(), fetchRefreshStatus()])
      stackMap.value = new Map(stackList.map((s) => [s.path, s]))
      refreshStatus.value = refresh
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load stacks'
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
        stackMap.value = new Map(records.map((s) => [s.path, s]))
      },
      onUpsert(record) {
        const next = new Map(stackMap.value)
        next.set(record.path, record)
        stackMap.value = next
      },
      onDelete(path) {
        const next = new Map(stackMap.value)
        next.delete(path)
        stackMap.value = next
      },
      onRefreshStatus(snapshot) {
        refreshStatus.value = snapshot as RefreshSnapshot
      },
      // biome-ignore lint/suspicious/noExplicitAny: Progress type varies by event
      onUpdateProgress(progress: any) {
        updateProgress.value = progress
        if (progress.type === 'started') {
          isUpdating.value = true
        } else if (progress.type === 'image_pull_progress') {
          const isDone =
            progress.status === 'Pull complete' ||
            progress.status === 'Already exists' ||
            progress.status === 'Download complete'

          if (!pullProgress.value || pullProgress.value.stack !== progress.stack) {
            pullProgress.value = {
              stack: progress.stack,
              images: [],
              current: progress.current,
              total: progress.total,
            }
          }

          const existing = pullProgress.value.images.find((i) => i.image === progress.image)
          if (existing) {
            existing.status = progress.status
            existing.progress = progress.progress ?? ''
            existing.done = isDone
          } else {
            pullProgress.value.images.push({
              image: progress.image,
              status: progress.status,
              progress: progress.progress ?? '',
              done: isDone,
            })
          }
          pullProgress.value.current = progress.current
          pullProgress.value.total = progress.total
        } else if (
          progress.type === 'stack_success' ||
          progress.type === 'stack_error' ||
          progress.type === 'completed'
        ) {
          pullProgress.value = null
          if (progress.type === 'completed') {
            isUpdating.value = false
            // Clear progress after a short delay to show completion message
            setTimeout(() => {
              updateProgress.value = null
            }, 3000)
          }
        }
      },
      onConnectionChange(state) {
        connectionState.value = state
      },
    })

    sseClient.connect()
  }

  function disconnectSSE() {
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
