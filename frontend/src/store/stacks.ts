// Pinia store for stack state management.
// Maintains an in-memory map of stacks, updated via SSE push events.
// Provides filtering and search computed properties.

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { RefreshSnapshot, StackRecord } from '../services/api'
import { fetchRefreshStatus, fetchStacks } from '../services/api'
import type { ConnectionState } from '../services/sse'
import { SSEClient } from '../services/sse'

export const useStacksStore = defineStore('stacks', () => {
  // State
  const stackMap = ref<Map<string, StackRecord>>(new Map())
  const refreshStatus = ref<RefreshSnapshot | null>(null)
  const connectionState = ref<ConnectionState>('disconnected')
  const filterStatus = ref<string>('')
  const searchQuery = ref<string>('')
  const loading = ref(false)
  const error = ref<string | null>(null)

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
