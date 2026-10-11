import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { RuntimeState } from '../composables/useRuntimeOverview'
import { type Instance, type Operation, requestJSON } from '../services/api'

export interface RuntimeObservation extends RuntimeState {
  observedAt?: string
}
interface OperationPage {
  records: Operation[]
  nextCursor: string
  persistent: boolean
  storageError: string
}
export const useConsoleStore = defineStore('console', () => {
  const instance = ref<Instance | null>(null)
  const instanceError = ref('')
  const runtime = ref<Record<string, RuntimeObservation>>({})
  const runtimeError = ref('')
  const operationMap = ref<Record<string, Operation>>({})
  const operationsError = ref('')
  const nextCursor = ref('')
  const inspectedId = ref('')
  const collapsed = ref(localStorage.getItem('docker-cd-sidebar') === 'collapsed')
  const density = ref(localStorage.getItem('docker-cd-density') || 'comfortable')
  const operations = computed(() =>
    Object.values(operationMap.value).sort((a, b) => b.startedAt.localeCompare(a.startedAt)),
  )
  const active = computed(() =>
    operations.value.filter((operation) => operation.status === 'running'),
  )
  let runtimeRequest = 0
  let runtimeLoading = false
  function onOperation(operation: Operation) {
    const existing = operationMap.value[operation.id]
    if (existing && (existing.version || 0) > (operation.version || 0)) return
    if (existing?.finishedAt && !operation.finishedAt && !operation.version) return
    operationMap.value[operation.id] = operation
  }
  async function loadInstance() {
    try {
      instance.value = await requestJSON<Instance>('/api/instance')
      instanceError.value = ''
    } catch (error) {
      instanceError.value = error instanceof Error ? error.message : 'Instance unavailable'
    }
  }
  async function loadOperations(more = false) {
    try {
      const page = await requestJSON<OperationPage>(
        `/api/operations?limit=50${more && nextCursor.value ? `&cursor=${encodeURIComponent(nextCursor.value)}` : ''}`,
      )
      for (const operation of page.records) {
        onOperation(operation)
      }
      nextCursor.value = page.nextCursor
      operationsError.value = page.storageError
    } catch (error) {
      operationsError.value = error instanceof Error ? error.message : 'Operations unavailable'
    }
  }
  async function loadRuntime() {
    if (runtimeLoading) return
    runtimeLoading = true
    const version = ++runtimeRequest
    try {
      const next = await requestJSON<Record<string, RuntimeObservation>>('/api/runtime')
      if (version === runtimeRequest) {
        runtime.value = Object.fromEntries(
          Object.entries(next).map(([path, state]) => [path, { ...state, loading: false }]),
        )
        runtimeError.value = ''
      }
    } catch (error) {
      if (version === runtimeRequest)
        runtimeError.value = error instanceof Error ? error.message : 'Runtime unavailable'
    } finally {
      runtimeLoading = false
    }
  }
  function runtimeFor(path: string): RuntimeObservation {
    return runtimeError.value
      ? { containers: [], loading: false, error: runtimeError.value }
      : runtime.value[path] || { containers: [], loading: true, error: null }
  }
  async function inspect(id: string) {
    inspectedId.value = id
    try {
      const record = await requestJSON<Operation>(`/api/operations/${encodeURIComponent(id)}`)
      onOperation(record)
    } catch (error) {
      operationsError.value = error instanceof Error ? error.message : 'Operation unavailable'
    }
  }
  function setCollapsed(value: boolean) {
    collapsed.value = value
    localStorage.setItem('docker-cd-sidebar', value ? 'collapsed' : 'expanded')
  }
  function setDensity(value: string) {
    density.value = value
    localStorage.setItem('docker-cd-density', value)
  }
  async function reload() {
    await Promise.all([loadInstance(), loadOperations(), loadRuntime()])
  }
  return {
    instance,
    instanceError,
    runtime,
    runtimeError,
    operationMap,
    operations,
    active,
    operationsError,
    nextCursor,
    inspectedId,
    collapsed,
    density,
    onOperation,
    loadInstance,
    loadOperations,
    loadRuntime,
    runtimeFor,
    inspect,
    setCollapsed,
    setDensity,
    reload,
  }
})
