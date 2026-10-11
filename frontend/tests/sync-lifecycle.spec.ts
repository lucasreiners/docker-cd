import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useStackContainers } from '../src/composables/useStackContainers'
import type { ContainerInfo, RefreshSnapshot, StackRecord } from '../src/services/api'
import { fetchContainers, fetchRefreshStatus, fetchStacks } from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  fetchContainers: vi.fn(),
  getEventsURL: () => '/api/events',
}))

class EventStream {
  static current: EventStream
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  listeners = new Map<string, (event: { data: string }) => void>()
  constructor() {
    EventStream.current = this
  }
  addEventListener(type: string, listener: (event: { data: string }) => void) {
    this.listeners.set(type, listener)
  }
  close() {}
  send(type: string, data: unknown) {
    this.listeners.get(type)?.({ data: JSON.stringify(data) })
  }
}

const stack: StackRecord = {
  path: 'apps/web',
  composeFile: 'compose.yaml',
  composeHash: 'h1',
  status: 'missing',
}
const refresh: RefreshSnapshot = {
  revision: 'new',
  ref: 'main',
  refType: 'branch',
  refreshedAt: '',
  refreshStatus: 'completed',
}

describe('live sync state', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', EventStream)
  })
  afterEach(() => {
    useStacksStore().disconnectSSE()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('keeps live initial-sync events when an older HTTP bootstrap finishes later', async () => {
    let resolveStacks!: (records: StackRecord[]) => void
    vi.mocked(fetchStacks).mockReturnValue(
      new Promise((resolve) => {
        resolveStacks = resolve
      }),
    )
    vi.mocked(fetchRefreshStatus).mockResolvedValue({
      ...refresh,
      revision: 'old',
      refreshStatus: 'refreshing',
    })
    const store = useStacksStore()
    const loading = store.loadInitial()
    store.connectSSE()
    EventStream.current.send('stack.snapshot', { records: [stack] })
    EventStream.current.send('stack.upsert', {
      record: { ...stack, status: 'synced', containersTotal: 1 },
    })
    EventStream.current.send('refresh.status', refresh)
    resolveStacks([])
    await loading
    expect(store.getStack(stack.path)?.status).toBe('synced')
    expect(store.refreshStatus?.revision).toBe('new')
  })

  it('recovers from a startup HTTP error when SSE becomes available', async () => {
    vi.mocked(fetchStacks).mockRejectedValue(new Error('starting'))
    vi.mocked(fetchRefreshStatus).mockResolvedValue(refresh)
    const store = useStacksStore()
    await store.loadInitial()
    store.connectSSE()
    EventStream.current.send('stack.snapshot', { records: [stack] })
    expect(store.error).toBeNull()
    expect(store.stacks).toHaveLength(1)
  })

  it('clears initial-sync pulls on deployment and completion without a scheduler cycle', () => {
    const store = useStacksStore()
    store.connectSSE()
    const send = (data: object) =>
      EventStream.current.send('update.progress', {
        source: 'reconcile',
        stack: stack.path,
        ...data,
      })
    send({ type: 'stack_sync_started', message: 'Syncing stack' })
    send({
      type: 'image_pull_progress',
      image: 'nginx',
      status: 'Download complete',
      current: 1,
      total: 1,
    })
    expect(store.pullProgress?.images[0].done).toBe(false)
    expect(store.updateProgress.message).toBe('Syncing stack')
    send({ type: 'stack_deploying', message: 'Deploying stack' })
    expect(store.pullProgress).toBeNull()
    send({ type: 'stack_success', message: 'Synced stack' })
    vi.advanceTimersByTime(3000)
    expect(store.updateProgress).toBeNull()
  })

  it('restores an in-flight update and clears a missed completion on reconnect', () => {
    const store = useStacksStore()
    store.connectSSE()
    EventStream.current.send('update.snapshot', {
      progress: [{ type: 'pruning', cycle_id: '1', message: 'Pruning' }],
    })
    expect(store.isUpdating).toBe(true)
    EventStream.current.send('update.snapshot', {
      progress: [{ type: 'completed', cycle_id: '1' }],
    })
    expect(store.isUpdating).toBe(false)
    expect(store.updateProgress).toBeNull()
  })

  it('tracks layers independently and only marks the image complete at stream completion', () => {
    const store = useStacksStore()
    store.connectSSE()
    EventStream.current.send('stack.snapshot', { records: [{ ...stack, status: 'syncing' }] })
    const send = (data: object) =>
      EventStream.current.send('update.progress', {
        type: 'image_pull_progress',
        source: 'reconcile',
        stack: stack.path,
        image: 'large-image',
        current: 1,
        total: 1,
        ...data,
      })
    send({ layerId: 'a', status: 'Downloading', bytesCurrent: 10, bytesTotal: 100 })
    send({ layerId: 'b', status: 'Downloading', bytesCurrent: 20, bytesTotal: 200 })
    send({ layerId: 'a', status: 'Pull complete' })
    expect(store.pullProgress?.images[0].layers).toHaveLength(2)
    expect(store.pullProgress?.images[0].layers[1].bytesCurrent).toBe(20)
    expect(store.pullProgress?.images[0].done).toBe(false)
    vi.advanceTimersByTime(15 * 60 * 1000)
    expect(store.getStack(stack.path)?.status).toBe('syncing')
    expect(store.pullProgress?.images[0].done).toBe(false)
    send({ status: 'Pull complete', done: true })
    expect(store.pullProgress?.images[0].done).toBe(true)
  })

  it('handles cancellation and prevents an old completion timer clearing a newer sync', () => {
    const store = useStacksStore()
    store.connectSSE()
    EventStream.current.send('update.progress', { type: 'started', cycle_id: '1' })
    EventStream.current.send('update.progress', { type: 'cancelled', cycle_id: '1' })
    expect(store.isUpdating).toBe(false)
    vi.advanceTimersByTime(2000)
    EventStream.current.send('update.progress', { type: 'started', cycle_id: '2' })
    vi.advanceTimersByTime(2000)
    expect(store.updateProgress.cycle_id).toBe('2')
  })
})

it('refreshes containers after sync even when counts stay equal, ignoring stale responses', async () => {
  const scope = effectScope()
  const record = ref<StackRecord | undefined>(stack)
  let resolveOld!: (containers: ContainerInfo[]) => void
  vi.mocked(fetchContainers).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveOld = resolve
    }),
  )
  const state = scope.run(() => useStackContainers(() => record.value))
  if (!state) throw new Error('scope did not start')
  const container: ContainerInfo = {
    id: 'new',
    name: 'web',
    service: 'web',
    image: 'nginx:new',
    state: 'running',
    health: 'none',
  }
  vi.mocked(fetchContainers).mockResolvedValue([container])
  record.value = { ...stack, status: 'synced' }
  await nextTick()
  await Promise.resolve()
  expect(state.containers.value[0]?.id).toBe('new')
  resolveOld([])
  await Promise.resolve()
  expect(state.containers.value[0]?.id).toBe('new')
  record.value = { ...record.value }
  await nextTick()
  expect(fetchContainers).toHaveBeenCalledTimes(3)
  scope.stop()
})
