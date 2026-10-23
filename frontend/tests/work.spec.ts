import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { operationalWorkspaceFixtures } from '../../tests/e2e/fixtures/frontend-data'
import { APIError, requestJSON } from '../src/services/api'
import {
  isWorkSnapshot,
  isWorkUpsert,
  type WorkRun,
  type WorkRunningRun,
} from '../src/services/work'
import { useWorkStore, workRunKey } from '../src/store/work'

vi.mock('../src/services/api', async (original) => ({
  ...(await original<typeof import('../src/services/api')>()),
  requestJSON: vi.fn(),
}))
beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
  vi.useFakeTimers()
})
afterEach(() => {
  useWorkStore().$dispose()
  vi.useRealTimers()
})
const fixtures = () => structuredClone(operationalWorkspaceFixtures())
function boot() {
  const f = fixtures()
  const store = useWorkStore()
  store.setCapability(2)
  store.connectionChanged('connected')
  expect(store.mutationReady).toBe(false)
  expect(store.snapshot(f.snapshot)).toBe(true)
  return { store, f }
}
function terminal(run: WorkRunningRun): WorkRun {
  return { ...run, status: 'completed', phase: 'finished', finishedAt: run.updatedAt }
}

describe('ordered authoritative live work', () => {
  it('consumes frozen examples, isolates identical layers, preserves old active ancestors and counts roots', () => {
    const { store, f } = boot()
    expect(store.mutationReady).toBe(true)
    f.latest50.forEach(store.onHistory)
    expect(store.workflows).toHaveLength(2)
    expect(store.workflows.some((g) => g.root?.status === 'completed' && g.runs.length)).toBe(true)
    expect(store.activeRuns).toHaveLength(2)
    store.upsert(f.operationOnly)
    store.upsert(f.checkOnly)
    expect(store.workflows).toHaveLength(4)
    expect(store.activeRuns).toHaveLength(2)
    store.upsert(f.bytes)
    const current = JSON.parse(JSON.stringify(store.runs))
    expect(store.upsert(f.bytes)).toBe(false)
    expect(store.upsert({ ...f.bytes, sequence: 9 })).toBe(false)
    expect(store.runs).toEqual(current)
    expect(store.activeRuns.map((r) => r.images[0].layers[0].bytesCurrent).sort()).toEqual([
      128, 256,
    ])
    store.upsert(f.terminal)
    expect(store.terminalRuns[0].images).toEqual([])
    expect(store.workflows).toHaveLength(4)
    store.upsert(f.pruning)
    store.upsert(f.retry)
    expect(store.runsForStack(f.retry.run?.stack || '')).toContainEqual(f.retry.run)
    expect(store.activeRuns.some((r) => r.runId === f.terminal.run?.runId)).toBe(false)
    expect(store.workflows).toHaveLength(4)
  })

  it('requires reconnect barrier, clears empty snapshots without inventing success and resolves interrupted history', async () => {
    const { store, f } = boot()
    let resolve!: (value: unknown) => void
    vi.mocked(requestJSON).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r
        }),
    )
    store.connectionChanged('reconnecting')
    expect(store.mutationReady).toBe(false)
    store.connectionChanged('connected')
    expect(store.upsert(f.bytes)).toBe(false)
    expect(store.mutationReady).toBe(false)
    expect(store.snapshot(f.reconnect)).toBe(true)
    expect(store.activeRuns).toHaveLength(0)
    expect(store.workflows).toHaveLength(0)
    expect(store.terminalRuns).toHaveLength(0)
    expect(
      Object.values(store.missingRuns).every(
        (run) => run.status === 'unknown' && run.images.length === 0,
      ),
    ).toBe(true)
    expect(Object.values(store.outcomes).every((o) => o.state === 'loading')).toBe(true)
    resolve({ ...f.interruptedOperation, steps: [] })
    await flushPromises()
    expect(store.terminalRuns).toHaveLength(0)
    expect(store.mutationReady).toBe(true)
  })

  it('adopts epoch only from initial subscription snapshot and drops late old-epoch events', () => {
    const { store, f } = boot()
    expect(store.snapshot(f.restart)).toBe(false)
    expect(store.upsert({ ...f.bytes, epoch: 'unsolicited', sequence: 100 })).toBe(false)
    store.connectionChanged('reconnecting')
    store.connectionChanged('connected')
    vi.mocked(requestJSON).mockRejectedValue(new Error('offline'))
    expect(store.snapshot(f.restart)).toBe(true)
    expect(store.epoch).toBe(f.restart.epoch)
    expect(store.sequence).toBe(0)
    expect(store.upsert(f.bytes)).toBe(false)
    expect(store.activeRuns).toHaveLength(0)
  })

  it('never regresses a terminal run; retries use a new identity and replacements remove former layers', () => {
    const { store, f } = boot()
    store.upsert(f.terminal)
    store.upsert({ ...f.retry, sequence: 100, run: f.snapshot.runs[1] })
    expect(store.activeRuns.some((r) => r.runId === f.terminal.run?.runId)).toBe(false)
    store.upsert({ ...f.retry, sequence: 101 })
    const run = f.retry.run
    if (!run) throw new Error('Missing fixture retry')
    store.upsert({ ...f.retry, sequence: 102, run: { ...run, images: [], imageCount: 0 } })
    expect(store.runs[workRunKey(f.snapshot.epoch, run.runId)].images).toEqual([])
  })

  it('keys image attempts and layers beneath a run and treats extraction / applying as distinct from pull completion', () => {
    const { store, f } = boot()
    const run = f.snapshot.runs[0]
    const image = run.images[0]
    const next = {
      ...run,
      imageCount: 2,
      images: [
        { ...image, total: 2 },
        {
          ...image,
          pullAttemptId: 'second-invocation',
          position: 2,
          total: 2,
          layers: image.layers.map((layer) => ({ ...layer, bytesCurrent: 999 })),
        },
      ],
    }
    store.upsert({ ...f.bytes, run: next })
    const key = workRunKey(f.snapshot.epoch, run.runId)
    expect(Object.keys(store.details[key])).toHaveLength(2)
    expect(store.details[key][image.pullAttemptId].layers['shared-layer'].bytesCurrent).toBe(128)
    expect(store.details[key]['second-invocation'].layers['shared-layer'].bytesCurrent).toBe(999)
    const extracting = {
      ...next,
      images: [
        {
          ...next.images[0],
          layers: [{ ...image.layers[0], phase: 'extracting', bytesCurrent: 0, bytesTotal: null }],
        },
      ],
    }
    store.upsert({ ...f.bytes, sequence: 14, run: { ...extracting, imageCount: 1 } })
    expect(store.details[key][image.pullAttemptId].layers['shared-layer']).toMatchObject({
      phase: 'extracting',
      bytesCurrent: 0,
      bytesTotal: null,
    })
    store.upsert({
      ...f.bytes,
      sequence: 15,
      run: { ...run, phase: 'applying', images: [{ ...image, status: 'completed', done: true }] },
    })
    expect(store.runs[key].phase).toBe('applying')
    expect(store.runs[key].status).toBe('running')
    expect(store.terminalRuns).toHaveLength(0)
  })

  it('keeps 250 active records, and bounds terminal summaries at 50 / 60 seconds without retaining bytes', async () => {
    const { store, f } = boot()
    const sample = f.snapshot.runs[0]
    store.connectionChanged('reconnecting')
    const runs = Array.from({ length: 250 }, (_, i) => ({ ...sample, runId: `run-${i}` }))
    expect(store.snapshot({ ...f.snapshot, runs })).toBe(true)
    expect(store.activeRuns).toHaveLength(250)
    for (let i = 0; i < 250; i++)
      store.upsert({ ...f.terminal, sequence: 20 + i, run: terminal(runs[i]) })
    expect(store.activeRuns).toHaveLength(0)
    expect(store.terminalRuns).toHaveLength(50)
    expect(store.terminalRuns.every((r) => r.images.length === 0)).toBe(true)
    await vi.advanceTimersByTimeAsync(60000)
    expect(store.terminalRuns).toHaveLength(0)
  })

  it('separates missing ancestry / expired and failed history from authoritative active membership', async () => {
    const f = fixtures()
    const store = useWorkStore()
    vi.mocked(requestJSON)
      .mockRejectedValueOnce(new APIError('gone', 404))
      .mockRejectedValue(new Error('offline'))
    const child = f.snapshot.runs[0]
    store.snapshot({ ...f.snapshot, operations: [], runs: [child] })
    await flushPromises()
    expect(store.workflows).toHaveLength(1)
    expect(store.workflows[0].groupingUnavailable).toBe(true)
    expect(
      Object.values(store.outcomes)
        .map((o) => o.state)
        .sort(),
    ).toEqual(['failed', 'unavailable'])
    expect(store.activeRuns).toHaveLength(1)
    expect(store.history).toEqual({})
  })

  it('bounds fetched terminal history without evicting active ancestry, and never groups from guessed stack identity', () => {
    const { store, f } = boot()
    for (let i = 0; i < 250; i++) store.onHistory({ ...f.latest50[0], id: `history-${i}` })
    expect(Object.keys(store.history)).toHaveLength(50)
    expect(Object.keys(store.liveOperations)).toHaveLength(3)
    expect(store.workflows).toHaveLength(2)
    const child = f.snapshot.operations[1]
    store.upsert({
      ...f.operationOnly,
      operations: [
        { ...child, id: 'unknown-owner', rootOperationId: undefined, parentId: 'expired-parent' },
      ],
    })
    expect(store.workflows).toHaveLength(3)
    expect(store.workflows.find((group) => !group.rootOperationId)?.groupingUnavailable).toBe(true)
  })

  it('uses explicitly looked-up root metadata without creating active membership from history', async () => {
    const f = fixtures()
    const store = useWorkStore()
    const parent = f.snapshot.operations[0]
    vi.mocked(requestJSON).mockResolvedValue({ ...parent, steps: [] })
    const child = f.snapshot.runs[0]
    store.snapshot({ ...f.snapshot, operations: [f.snapshot.operations[1]], runs: [child] })
    expect(store.workflows[0].groupingUnavailable).toBe(true)
    await flushPromises()
    expect(store.outcomes[parent.id].state).toBe('resolved')
    expect(store.workflows).toHaveLength(1)
    expect(store.workflows[0].root?.id).toBe(parent.id)
    expect(store.workflows[0].root?.status).toBe('completed')
    expect(store.workflows[0].groupingUnavailable).toBe(false)
    expect(store.liveOperations[parent.id]).toBeUndefined()
  })

  it('rejects changed run ownership atomically and waits for a new snapshot after malformed upserts', () => {
    const { store, f } = boot()
    const previous = JSON.parse(JSON.stringify(store.runs))
    expect(store.upsert({ ...f.bytes, run: { ...f.bytes.run, stack: 'unrelated/stack' } })).toBe(
      false,
    )
    expect(store.runs).toEqual(previous)
    expect(store.mutationReady).toBe(false)
    expect(store.sequence).toBe(f.snapshot.sequence)
    store.connectionChanged('reconnecting')
    store.connectionChanged('connected')
    expect(store.snapshot(f.snapshot)).toBe(true)
    expect(store.mutationReady).toBe(true)
    expect(store.upsert({ ...f.bytes, run: null })).toBe(false)
    expect(store.runs).toEqual(previous)
  })

  it('uses only operation stages for legacy agents; readiness waits for restored state', () => {
    const store = useWorkStore()
    const f = fixtures()
    store.setCapability(undefined)
    store.connectionChanged('connected')
    expect(store.mutationReady).toBe(false)
    store.legacySnapshotReceived()
    expect(store.mutationReady).toBe(false)
    store.legacyHistoryReceived()
    expect(store.mutationReady).toBe(true)
    store.onHistory({ ...f.snapshot.operations[1], steps: [] })
    expect(store.detailUnavailableReason).toContain('unavailable')
    expect(store.runsForStack('anything')).toEqual([])
    store.connectionChanged('reconnecting')
    expect(store.mutationReady).toBe(false)
  })

  it('drops correlated byte caches when reconnect discovers a legacy agent, retaining unknown outcomes instead', async () => {
    const { store, f } = boot()
    vi.mocked(requestJSON).mockRejectedValue(new Error('History unavailable'))
    store.setCapability(undefined)
    expect(store.activeRuns).toHaveLength(0)
    expect(store.workflows).toHaveLength(0)
    expect(
      Object.values(store.missingRuns).every(
        (run) => run.status === 'unknown' && run.images.length === 0,
      ),
    ).toBe(true)
    await flushPromises()
    expect(Object.values(store.outcomes).every((outcome) => outcome.state === 'failed')).toBe(true)
    const operation = f.operationOnly.operations[0]
    store.onHistory({ ...operation, steps: [] })
    expect(store.legacyActiveOperations.map((op) => op.id)).toContain(operation.id)
    expect(store.operationStagesForStack(operation.stack || '')).toHaveLength(1)
    expect(store.runsForStack(operation.stack || '')).toEqual([])
  })
})

describe('untrusted work validation', () => {
  it('accepts every frozen event, truncation, operation-only events and known zero/unknown bytes', () => {
    const f = fixtures()
    for (const event of f.events)
      expect(
        event.event === 'work.snapshot' ? isWorkSnapshot(event.data) : isWorkUpsert(event.data),
      ).toBe(true)
    expect(isWorkSnapshot(f.truncated)).toBe(true)
  })
  it.each([
    (v: any) => {
      delete v.runs
    },
    (v: any) => {
      v.runs = null
    },
    (v: any) => {
      v.operations = [{}]
    },
    (v: any) => {
      v.sequence = -1
    },
    (v: any) => {
      v.sequence = Number.MAX_SAFE_INTEGER + 1
    },
    (v: any) => {
      v.schemaVersion = 3
    },
    (v: any) => {
      v.runs.push(v.runs[0])
    },
    (v: any) => {
      v.runs[0].images[0].layers.push(v.runs[0].images[0].layers[0])
    },
    (v: any) => {
      v.runs[0].images[0].layers[0].bytesTotal = -1
    },
    (v: any) => {
      v.runs[0].phase = 'deploy_complete'
    },
    (v: any) => {
      v.runs[0].imageCount = 0
    },
    (v: any) => {
      v.runs[0].updatedAt = 'not a date'
    },
    (v: any) => {
      v.runs[0].finishedAt = v.runs[0].startedAt
    },
    (v: any) => {
      v.runs[0].images[0].status = 'completed'
    },
    (v: any) => {
      v.runs[0].message = 'x'.repeat(1025)
    },
    (v: any) => {
      v.runs[0].rootOperationId = 'wrong-root'
    },
    (v: any) => {
      v.runs[0].images[0].layers = Array.from({ length: 201 }, (_, i) => ({
        ...v.runs[0].images[0].layers[0],
        id: `layer-${i}`,
      }))
      v.runs[0].images[0].layerCount = 201
    },
  ])('rejects malformed snapshot atomically and blocks mutations (%#)', (mutate) => {
    const { store, f } = boot()
    const previous = JSON.parse(JSON.stringify(store.runs))
    mutate(f.snapshot)
    expect(isWorkSnapshot(f.snapshot)).toBe(false)
    expect(store.snapshot(f.snapshot)).toBe(false)
    expect(store.runs).toEqual(previous)
    expect(store.mutationReady).toBe(false)
  })
})
