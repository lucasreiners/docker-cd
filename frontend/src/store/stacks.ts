// Pinia store for stack state management.
// Maintains an in-memory map of stacks, updated via SSE push events.
// Provides filtering and search computed properties.

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Operation, RefreshSnapshot, StackRecord } from '../services/api'
import { fetchRefreshStatus, fetchStacks } from '../services/api'
import type { UpdateProgress } from '../services/progress'
import type { ConnectionState } from '../services/sse'
import { SSEClient } from '../services/sse'
import { useConsoleStore } from './console'
import { useWorkStore } from './work'

export const useStacksStore = defineStore('stacks', () => {
  // State
  const stackMap = ref<Map<string, StackRecord>>(new Map())
  const refreshStatus = ref<RefreshSnapshot | null>(null)
  const connectionState = ref<ConnectionState>('disconnected')
  const filterStatus = ref<string>('')
  const searchQuery = ref<string>('')
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isUpdating = ref(false)

  let sseClient: SSEClient | null = null
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

  function connectSSE(onOperation?: (operation: Operation) => void, onConnected?: () => void) {
    const work = useWorkStore()
    const consoleStore = useConsoleStore()
    work.connectionChanged('disconnected')
    if (sseClient) {
      sseClient.close()
    }

    sseClient = new SSEClient({
      onOperation(operation) {
        work.onHistory(operation)
        onOperation?.(operation)
        if (operation.status !== 'running') consoleStore.refreshRuntime()
      },
      onWorkSnapshot(event) {
        if (!work.snapshot(event) && work.invalidReason) sseClient?.connect()
      },
      onWorkUpsert(event) {
        if (
          work.upsert(event) &&
          ((event.run && event.run.status !== 'running') ||
            event.operations.some((op) => op.status !== 'running'))
        )
          consoleStore.refreshRuntime()
        if (work.invalidReason) sseClient?.connect()
      },
      onInvalidWork() {
        work.invalidate()
        sseClient?.connect()
      },
      onSnapshot(records) {
        work.legacySnapshotReceived()
        stackVersion++
        loading.value = false
        error.value = null
        stackMap.value = new Map(records.map((s) => [s.path, s]))
      },
      onUpsert(record) {
        stackVersion++
        const next = new Map(stackMap.value)
        next.set(record.path, record)
        stackMap.value = next
        consoleStore.refreshRuntime()
      },
      onDelete(path) {
        stackVersion++
        const next = new Map(stackMap.value)
        next.delete(path)
        stackMap.value = next
        consoleStore.refreshRuntime()
      },
      onRefreshStatus(snapshot) {
        refreshVersion++
        refreshStatus.value = snapshot as RefreshSnapshot
      },
      onUpdateSnapshot(progress) {
        isUpdating.value = false
        for (const item of progress) {
          const p = item
          const terminal =
            ['completed', 'cancelled'].includes(p.type) ||
            (p.source === 'reconcile' && ['stack_success', 'stack_error'].includes(p.type))
          if (!terminal) handleProgress(p)
        }
      },
      onUpdateProgress: handleProgress,
      onConnectionChange(state) {
        work.connectionChanged(state)
        connectionState.value = state
        if (state === 'connected') onConnected?.()
      },
    })

    sseClient.connect()
  }

  function handleProgress(progress: UpdateProgress) {
    // Older agents still gate actions on the updater cycle. Presentation uses
    // keyed work or operation stages, never these uncorrelated layer events.
    const terminal = progress.type === 'completed' || progress.type === 'cancelled'
    if (progress.source !== 'reconcile' && progress.cycle_id) {
      isUpdating.value = !terminal
    }
    if (progress.type === 'started') {
      isUpdating.value = true
    } else if (terminal) {
      // Legacy updater lifecycle is independent of individual stack outcomes.
      isUpdating.value = false
    }
  }

  function disconnectSSE() {
    sseClient?.close()
    sseClient = null
    useWorkStore().connectionChanged('disconnected')
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
    isUpdating,
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
