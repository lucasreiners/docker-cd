import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { operationalWorkspaceFixtures } from '../../tests/e2e/fixtures/frontend-data'
import { useStackContainers } from '../src/composables/useStackContainers'
import type { ContainerInfo, RefreshSnapshot, StackRecord } from '../src/services/api'
import { fetchRefreshStatus, fetchStacks, requestJSON } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'
import { useStacksStore } from '../src/store/stacks'
import { useWorkStore } from '../src/store/work'

vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  fetchContainers: vi.fn(),
  requestJSON: vi.fn(),
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

  it('keeps correlated initial-sync work through applying without a scheduler cycle', () => {
    const store = useStacksStore()
    const work = useWorkStore()
    const f = operationalWorkspaceFixtures()
    work.setCapability(2)
    store.connectSSE()
    EventStream.current.onopen?.()
    EventStream.current.send('work.snapshot', f.snapshot)
    const run = f.snapshot.runs[0]
    if (!run) throw new Error('Missing run fixture')
    expect(work.activeRuns.find((r) => r.runId === run.runId)?.source).toBe('reconcile')
    EventStream.current.send('work.upsert', {
      ...f.bytes,
      run: { ...run, phase: 'applying', message: 'Applying Compose', imageCount: 0, images: [] },
    })
    expect(work.activeRuns.find((r) => r.runId === run.runId)?.phase).toBe('applying')
    expect(work.workflows).toHaveLength(2)
    expect(store.isUpdating).toBe(false)
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
    EventStream.current.send('update.progress', { type: 'started', cycle_id: '2' })
    EventStream.current.send('update.progress', {
      type: 'stack_success',
      cycle_id: '2',
      stack: stack.path,
    })
    expect(store.isUpdating).toBe(true)
    EventStream.current.send('update.progress', { type: 'completed', cycle_id: '2' })
    expect(store.isUpdating).toBe(false)
  })

  it('tracks layers independently and only marks the image complete at stream completion', () => {
    const store = useStacksStore()
    store.connectSSE()
    const f = operationalWorkspaceFixtures(),
      work = useWorkStore()
    work.setCapability(2)
    EventStream.current.onopen?.()
    EventStream.current.send('stack.snapshot', { records: [{ ...stack, status: 'syncing' }] })
    const run = structuredClone(f.snapshot.runs[0])
    if (!run) throw new Error('Missing run fixture')
    const image = run.images[0]
    const layer = image?.layers[0]
    if (!image || !layer) throw new Error('Missing image/layer fixture')
    image.layers = [
      {
        ...layer,
        id: 'a',
        status: 'Pull complete',
        done: true,
        bytesCurrent: 100,
        bytesTotal: 100,
      },
      { ...layer, id: 'b', bytesCurrent: 20, bytesTotal: 200 },
    ]
    image.layerCount = 2
    EventStream.current.send('work.snapshot', { ...f.snapshot, runs: [run, f.snapshot.runs[1]] })
    const currentImage = () => work.activeRuns.find((r) => r.runId === run.runId)?.images[0]
    expect(currentImage()?.layers).toHaveLength(2)
    expect(currentImage()?.layers[1]?.bytesCurrent).toBe(20)
    expect(currentImage()?.done).toBe(false)
    vi.advanceTimersByTime(15 * 60 * 1000)
    expect(store.getStack(stack.path)?.status).toBe('syncing')
    expect(currentImage()?.done).toBe(false)
    EventStream.current.send('work.upsert', {
      ...f.bytes,
      run: { ...run, phase: 'applying', images: [{ ...image, status: 'completed', done: true }] },
    })
    expect(currentImage()?.done).toBe(true)
    expect(work.activeRuns.find((r) => r.runId === run.runId)?.status).toBe('running')
    expect(
      work.activeRuns.find((r) => r.runId !== run.runId)?.images[0]?.layers[0]?.bytesCurrent,
    ).toBe(128)
  })

  it('handles legacy cancellation and keeps a new cycle active independently of reconcile completion', () => {
    const store = useStacksStore()
    const onOperation = vi.fn()
    store.connectSSE(onOperation)
    EventStream.current.send('update.progress', { type: 'started', cycle_id: '1' })
    EventStream.current.send('update.progress', { type: 'cancelled', cycle_id: '1' })
    expect(store.isUpdating).toBe(false)
    vi.advanceTimersByTime(2000)
    EventStream.current.send('update.progress', { type: 'started', cycle_id: '2' })
    vi.advanceTimersByTime(2000)
    EventStream.current.send('update.progress', {
      type: 'stack_success',
      source: 'reconcile',
      stack: stack.path,
    })
    expect(store.isUpdating).toBe(true)
    const operation = { ...operationalWorkspaceFixtures().snapshot.operations[1], steps: [] }
    EventStream.current.send('operation.upsert', operation)
    expect(onOperation).toHaveBeenCalledWith(operation)
    expect(useWorkStore().history[operation.id]?.trigger).toBe('webhook')
    expect(useWorkStore().activeRuns).toHaveLength(0)
  })

  it('consumes v2 work through the actual SSE store pipeline, restores after reconnect and refreshes runtime on terminal bursts', async () => {
    const f = operationalWorkspaceFixtures()
    const stacks = useStacksStore()
    const work = useWorkStore()
    const consoleStore = useConsoleStore()
    vi.mocked(requestJSON).mockResolvedValue({})
    work.setCapability(2)
    stacks.connectSSE(consoleStore.onOperation, () => consoleStore.refreshRuntime(true))
    const first = EventStream.current
    first.onopen?.()
    expect(work.mutationReady).toBe(false)
    first.send('work.snapshot', f.snapshot)
    expect(work.mutationReady).toBe(true)
    first.send('work.upsert', f.operationOnly)
    expect(work.workflows).toHaveLength(3)
    expect(work.activeRuns).toHaveLength(2)
    first.send('work.upsert', f.bytes)
    first.send('work.upsert', f.terminal)
    first.send('work.upsert', {
      ...f.operationOnly,
      sequence: 15,
      operations: f.operationOnly.operations.map((op) => ({
        ...op,
        version: 2,
        status: 'completed',
        finishedAt: '2026-10-23T12:00:05Z',
      })),
    })
    expect(work.workflows).toHaveLength(2)
    first.send('stack.upsert', { record: { ...stack, status: 'synced' } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(200)
    expect(requestJSON).toHaveBeenCalledWith('/api/runtime')
    expect(work.activeRuns).toHaveLength(1)
    expect(work.terminalRuns).toHaveLength(1)
    first.onerror?.()
    expect(work.mutationReady).toBe(false)
    await vi.advanceTimersByTimeAsync(1000)
    const second = EventStream.current
    second.onopen?.()
    expect(work.mutationReady).toBe(false)
    second.send('work.snapshot', f.reconnect)
    expect(work.activeRuns).toHaveLength(0)
    expect(work.mutationReady).toBe(true)
    first.send('work.snapshot', f.snapshot)
    first.send('work.upsert', { ...f.bytes, sequence: 999 })
    expect(work.activeRuns).toHaveLength(0)
    second.send('work.upsert', { ...f.bytes, sequence: 'malformed' })
    expect(work.mutationReady).toBe(false)
    expect(EventStream.current).not.toBe(second)
    EventStream.current.onopen?.()
    EventStream.current.send('work.snapshot', f.restart)
    expect(work.epoch).toBe(f.restart.epoch)
    expect(work.mutationReady).toBe(true)
    consoleStore.$dispose()
  })
})

it('serializes shared observations after sync even when counts stay equal, ignoring invalidated responses', async () => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  const scope = effectScope()
  const record = ref<StackRecord | undefined>(stack)
  let resolveOld!: (value: unknown) => void
  vi.mocked(requestJSON).mockReturnValueOnce(
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
  vi.mocked(requestJSON).mockResolvedValue({
    [stack.path]: { containers: [container], error: null, observedAt: '2026-10-23T12:00:00Z' },
  })
  record.value = { ...stack, status: 'synced' }
  await nextTick()
  await vi.advanceTimersByTimeAsync(200)
  expect(requestJSON).toHaveBeenCalledTimes(1)
  resolveOld({ [stack.path]: { containers: [], error: null, observedAt: '2026-10-23T11:00:00Z' } })
  await flushPromises()
  expect(state.containers.value[0]?.id).toBe('new')
  record.value = { ...record.value }
  await nextTick()
  await vi.advanceTimersByTimeAsync(200)
  expect(requestJSON).toHaveBeenCalledTimes(3)
  scope.stop()
  useConsoleStore().$dispose()
  vi.useRealTimers()
})
