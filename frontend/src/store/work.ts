import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref } from 'vue'
import { type Operation, requestJSON } from '../services/api'
import type { ConnectionState } from '../services/sse'
import {
  isWorkSnapshot,
  isWorkUpsert,
  type WorkImage,
  type WorkLayer,
  type WorkOperation,
  type WorkRun,
} from '../services/work'

export interface WorkOutcome {
  operationId: string
  state: 'loading' | 'resolved' | 'unavailable' | 'failed' | 'unknown'
  operation?: Operation
}
export type MissingWorkRun = Omit<WorkRun, 'status' | 'phase' | 'images'> & {
  status: 'unknown'
  phase: 'unknown'
  images: []
}
export interface WorkflowGroup {
  key: string
  rootOperationId?: string
  root?: WorkOperation | Operation
  groupingUnavailable: boolean
  operations: WorkOperation[]
  runs: WorkRun[]
}
export const workRunKey = (epoch: string, runId: string) => JSON.stringify([epoch, runId])
const empty = <T>(): Record<string, T> => Object.create(null)
const dictionary = <T>(entries: [string, T][]) =>
  Object.assign(empty<T>(), Object.fromEntries(entries))
function validHistory(op: Operation) {
  return (
    op &&
    typeof op.id === 'string' &&
    !!op.id &&
    typeof op.status === 'string' &&
    !!op.status &&
    typeof op.kind === 'string' &&
    typeof op.trigger === 'string' &&
    typeof op.message === 'string' &&
    Number.isFinite(Date.parse(op.startedAt)) &&
    Array.isArray(op.steps) &&
    (op.version === undefined || (Number.isSafeInteger(op.version) && op.version > 0)) &&
    (op.status === 'running' ? !op.finishedAt : Number.isFinite(Date.parse(op.finishedAt || '')))
  )
}

export const useWorkStore = defineStore('work', () => {
  const epoch = ref('')
  const sequence = ref(-1)
  const connected = ref(false)
  const restored = ref(false)
  const supported = ref<boolean | null>(null)
  const invalidReason = ref('')
  const runs = ref<Record<string, WorkRun>>({})
  // Live ancestry is independent of the latest-history page and durable steps.
  const liveOperations = ref(empty<WorkOperation>())
  const history = ref(empty<Operation>())
  const outcomes = ref(empty<WorkOutcome>())
  const missingRuns = ref<Record<string, MissingWorkRun>>({})
  const terminals = ref<Record<string, { run: WorkRun; retainedAt: number }>>({})
  let generation = 0
  let awaitingSnapshot = true
  let legacySnapshot = false
  let legacyHistory = false
  const lookups = new Map<string, { generation: number; promise: Promise<void> }>()
  let expiry: ReturnType<typeof setTimeout> | undefined
  const mutationReady = computed(
    () => connected.value && restored.value && supported.value !== null,
  )
  const readinessReason = computed(() =>
    !connected.value
      ? 'Disconnected from agent'
      : !restored.value
        ? 'Restoring authoritative work state'
        : supported.value === null
          ? 'Loading agent capabilities'
          : '',
  )
  const detailUnavailableReason = computed(() =>
    supported.value === false ? 'Detailed live progress unavailable on this agent' : '',
  )
  const activeRuns = computed(() => Object.values(runs.value))
  const legacyActiveOperations = computed(() =>
    supported.value === false
      ? Object.values(history.value).filter(
          (op) =>
            op.status === 'running' &&
            (!outcomes.value[op.id] || outcomes.value[op.id].operation?.status === 'running'),
        )
      : [],
  )
  const details = computed(() =>
    Object.fromEntries(
      Object.entries(runs.value).map(([key, run]) => [
        key,
        Object.fromEntries(
          run.images.map(
            (image): [string, { image: WorkImage; layers: Record<string, WorkLayer> }] => [
              image.pullAttemptId,
              { image, layers: Object.fromEntries(image.layers.map((layer) => [layer.id, layer])) },
            ],
          ),
        ),
      ]),
    ),
  )
  const terminalRuns = computed(() => Object.values(terminals.value).map((v) => v.run))
  const workflows = computed<WorkflowGroup[]>(() => {
    const groups = new Map<string, WorkflowGroup>()
    function group(root: string | undefined, fallback: string) {
      const key = root || `unknown:${fallback}`
      let value = groups.get(key)
      if (!value) {
        const summary = root
          ? liveOperations.value[root] || outcomes.value[root]?.operation
          : undefined
        value = {
          key,
          rootOperationId: root,
          root: summary,
          groupingUnavailable: !root || !summary,
          operations: [],
          runs: [],
        }
        groups.set(key, value)
      }
      return value
    }
    const rootFor = (op: WorkOperation) =>
      op.rootOperationId ||
      activeRuns.value.find((run) => run.operationId === op.id)?.rootOperationId
    for (const op of Object.values(liveOperations.value)) {
      if (op.status === 'running') group(rootFor(op), op.id)
    }
    for (const run of activeRuns.value) group(run.rootOperationId, run.operationId).runs.push(run)
    for (const value of groups.values()) {
      value.operations = Object.values(liveOperations.value).filter((op) =>
        value.rootOperationId
          ? rootFor(op) === value.rootOperationId
          : value.key === `unknown:${op.id}`,
      )
    }
    return [...groups.values()]
  })
  function runsForStack(path: string) {
    return activeRuns.value.filter((r) => r.stack === path)
  }
  function operationStagesForStack(path: string) {
    return (
      supported.value === false ? legacyActiveOperations.value : Object.values(liveOperations.value)
    ).filter((op) => op.stack === path && op.status === 'running')
  }
  function setCapability(version: unknown) {
    supported.value = version === 2
    if (!supported.value && (activeRuns.value.length || Object.keys(liveOperations.value).length)) {
      const owners = new Set<string>()
      for (const op of Object.values(liveOperations.value))
        if (op.status === 'running') owners.add(op.id)
      for (const run of activeRuns.value) {
        rememberMissing(workRunKey(epoch.value, run.runId), run)
        owners.add(run.operationId)
      }
      runs.value = {}
      liveOperations.value = empty<WorkOperation>()
      for (const id of owners) void lookup(id)
    }
    if (supported.value && awaitingSnapshot) restored.value = false
    restoreLegacy()
  }
  function restoreLegacy() {
    if (supported.value === false)
      restored.value = connected.value && legacySnapshot && legacyHistory
  }
  function legacySnapshotReceived() {
    legacySnapshot = true
    restoreLegacy()
  }
  function legacyHistoryReceived() {
    legacyHistory = true
    restoreLegacy()
  }
  function connectionChanged(state: ConnectionState) {
    connected.value = state === 'connected'
    if (!connected.value) {
      generation++
      awaitingSnapshot = true
      restored.value = false
      legacySnapshot = false
      legacyHistory = false
      for (const [id, outcome] of Object.entries(outcomes.value)) {
        if (outcome.state === 'loading' || outcome.state === 'failed') delete outcomes.value[id]
      }
    } else restoreLegacy()
  }
  function invalidate() {
    restored.value = false
    invalidReason.value = 'Invalid live-work event; reconnect to restore state'
  }
  function onHistory(op: Operation) {
    if (!validHistory(op)) return
    const old = history.value[op.id]
    if (!old || (op.version || 0) >= (old.version || 0)) {
      history.value[op.id] = op
      const terminalIds = Object.values(history.value)
        .filter((record) => record.status !== 'running')
        .sort((a, b) => b.startedAt.localeCompare(a.startedAt))
        .map((record) => record.id)
      for (const id of terminalIds.slice(50)) delete history.value[id]
    }
  }
  function lookup(id: string): Promise<void> {
    const pending = lookups.get(id)
    if (pending?.generation === generation) return pending.promise
    const version = generation
    const entry = { generation, promise: Promise.resolve() }
    lookups.set(id, entry)
    outcomes.value[id] = { operationId: id, state: 'loading' }
    entry.promise = (async () => {
      try {
        const op = await requestJSON<Operation>(`/api/operations/${encodeURIComponent(id)}`)
        if (version !== generation) return
        if (!validHistory(op) || op.id !== id) throw new Error('Invalid operation response')
        onHistory(op)
        const cached = history.value[id]
        const current = cached && (cached.version || 0) > (op.version || 0) ? cached : op
        outcomes.value[id] = {
          operationId: id,
          state: current.status === 'running' ? 'unknown' : 'resolved',
          operation: current,
        }
      } catch (error) {
        if (version === generation)
          outcomes.value[id] = {
            operationId: id,
            state:
              error instanceof Error && 'status' in error && error.status === 404
                ? 'unavailable'
                : 'failed',
          }
      } finally {
        if (lookups.get(id) === entry) lookups.delete(id)
      }
      if (version === generation) boundOutcomes()
    })()
    return entry.promise
  }
  function boundOutcomes() {
    const needed = neededOutcomeIds()
    const ids = Object.keys(outcomes.value).filter((id) => !needed.has(id))
    for (const id of ids.slice(0, Math.max(0, ids.length - 50))) delete outcomes.value[id]
  }
  function pruneOperations() {
    const keep = new Set<string>()
    function mark(id?: string) {
      while (id && !keep.has(id)) {
        keep.add(id)
        id = liveOperations.value[id]?.parentId
      }
    }
    Object.values(liveOperations.value)
      .filter((o) => o.status === 'running')
      .forEach((o) => {
        mark(o.id)
        mark(o.rootOperationId)
      })
    activeRuns.value.forEach((r) => {
      mark(r.operationId)
      mark(r.rootOperationId)
    })
    liveOperations.value = dictionary(
      Object.entries(liveOperations.value).filter(([id]) => keep.has(id)),
    )
  }
  function applyOperations(operations: WorkOperation[]) {
    for (const op of operations) {
      const old = liveOperations.value[op.id]
      if (!old || op.version >= old.version) liveOperations.value[op.id] = op
    }
  }
  function retainTerminal(key: string, run: WorkRun) {
    delete missingRuns.value[key]
    // Terminal feedback contains no byte/image history.
    terminals.value[key] = {
      run: { ...run, images: [], detailsTruncated: run.detailsTruncated || run.images.length > 0 },
      retainedAt: Date.now(),
    }
    const keys = Object.keys(terminals.value)
    for (const old of keys.slice(0, Math.max(0, keys.length - 50))) delete terminals.value[old]
    scheduleExpiry()
  }
  function rememberMissing(key: string, run: WorkRun) {
    missingRuns.value[key] = {
      ...run,
      status: 'unknown',
      phase: 'unknown',
      images: [],
      message: 'No longer active; retrieving outcome',
      detailsTruncated: run.detailsTruncated || run.images.length > 0,
    }
    const keys = Object.keys(missingRuns.value)
    for (const old of keys.slice(0, Math.max(0, keys.length - 50))) delete missingRuns.value[old]
  }
  function scheduleExpiry() {
    if (expiry) clearTimeout(expiry)
    const oldest = Math.min(...Object.values(terminals.value).map((v) => v.retainedAt))
    if (!Number.isFinite(oldest)) return
    expiry = setTimeout(
      () => {
        for (const [key, value] of Object.entries(terminals.value))
          if (Date.now() - value.retainedAt >= 60000) delete terminals.value[key]
        scheduleExpiry()
      },
      Math.max(0, oldest + 60000 - Date.now()),
    )
  }
  function snapshot(value: unknown) {
    if (!isWorkSnapshot(value)) {
      invalidate()
      return false
    }
    if (!awaitingSnapshot) return false
    if (
      value.epoch === epoch.value &&
      (value.sequence < sequence.value ||
        value.runs.some((run) => terminals.value[workRunKey(value.epoch, run.runId)]))
    ) {
      invalidate()
      return false
    }
    const previousRuns = activeRuns.value
    const previousEpoch = epoch.value
    const previousOps = Object.values(liveOperations.value).filter((o) => o.status === 'running')
    const newEpoch = epoch.value !== value.epoch
    epoch.value = value.epoch
    sequence.value = value.sequence
    if (newEpoch) {
      terminals.value = {}
      outcomes.value = empty<WorkOutcome>()
    }
    runs.value = Object.fromEntries(
      value.runs.map((r) => [
        workRunKey(value.epoch, r.runId),
        JSON.parse(JSON.stringify(r)) as WorkRun,
      ]),
    )
    liveOperations.value = dictionary(value.operations.map((o) => [o.id, { ...o }]))
    awaitingSnapshot = false
    restored.value = true
    invalidReason.value = ''
    const missing = new Set(
      previousRuns
        .filter((r) => newEpoch || !runs.value[workRunKey(value.epoch, r.runId)])
        .map((r) => r.operationId),
    )
    for (const run of previousRuns) {
      const oldKey = workRunKey(previousEpoch, run.runId)
      if (newEpoch || !runs.value[oldKey]) rememberMissing(oldKey, run)
    }
    for (const key of Object.keys(runs.value)) delete missingRuns.value[key]
    const missingKeys = Object.keys(missingRuns.value)
    for (const key of missingKeys.slice(0, Math.max(0, missingKeys.length - 50)))
      delete missingRuns.value[key]
    previousOps
      .filter((o) => !liveOperations.value[o.id])
      .forEach((o) => {
        missing.add(o.id)
      })
    for (const id of missing) void lookup(id)
    recoverAncestors()
    boundOutcomes()
    return true
  }
  function neededOutcomeIds() {
    const ids = new Set<string>()
    activeRuns.value.forEach((r) => {
      ids.add(r.operationId)
      ids.add(r.rootOperationId)
    })
    Object.values(liveOperations.value).forEach((o) => {
      ids.add(o.id)
      if (o.parentId) ids.add(o.parentId)
      if (o.rootOperationId) ids.add(o.rootOperationId)
    })
    Object.values(missingRuns.value).forEach((run) => {
      ids.add(run.operationId)
    })
    return ids
  }
  function recoverAncestors() {
    const ids = neededOutcomeIds()
    for (const id of ids) if (!liveOperations.value[id] && !outcomes.value[id]) void lookup(id)
  }
  function upsert(value: unknown) {
    if (!isWorkUpsert(value)) {
      invalidate()
      return false
    }
    if (
      awaitingSnapshot ||
      !restored.value ||
      value.epoch !== epoch.value ||
      value.sequence <= sequence.value
    )
      return false
    if (value.run) {
      const previous = runs.value[workRunKey(value.epoch, value.run.runId)]
      const next = value.run
      const owner =
        value.operations.find((operation) => operation.id === next.operationId) ||
        liveOperations.value[next.operationId]
      if (
        owner &&
        ((owner.rootOperationId && owner.rootOperationId !== next.rootOperationId) ||
          owner.parentId !== next.parentOperationId)
      ) {
        invalidate()
        return false
      }
      if (
        previous &&
        (
          [
            'operationId',
            'rootOperationId',
            'parentOperationId',
            'source',
            'stack',
            'revision',
            'cycleId',
            'startedAt',
          ] as const
        ).some((key) => previous[key] !== next[key])
      ) {
        invalidate()
        return false
      }
      if (
        previous &&
        next.images.some((image) => {
          const old = previous.images.find(
            (attempt) => attempt.pullAttemptId === image.pullAttemptId,
          )
          return (
            old &&
            (old.reference !== image.reference ||
              (old.done && (old.status !== image.status || !image.done)))
          )
        })
      ) {
        invalidate()
        return false
      }
    }
    sequence.value = value.sequence
    applyOperations(value.operations)
    if (value.run) {
      const key = workRunKey(value.epoch, value.run.runId)
      if (!terminals.value[key]) {
        if (value.run.status === 'running')
          runs.value[key] = JSON.parse(JSON.stringify(value.run)) as WorkRun
        else {
          delete runs.value[key]
          retainTerminal(key, value.run)
        }
      }
    }
    pruneOperations()
    recoverAncestors()
    boundOutcomes()
    return true
  }
  onScopeDispose(() => {
    if (expiry) clearTimeout(expiry)
    generation++
  })
  return {
    epoch,
    sequence,
    connected,
    restored,
    supported,
    invalidReason,
    runs,
    liveOperations,
    history,
    outcomes,
    terminals,
    missingRuns,
    details,
    mutationReady,
    readinessReason,
    detailUnavailableReason,
    activeRuns,
    legacyActiveOperations,
    terminalRuns,
    workflows,
    legacySnapshotReceived,
    legacyHistoryReceived,
    runsForStack,
    operationStagesForStack,
    setCapability,
    connectionChanged,
    onHistory,
    lookup,
    snapshot,
    upsert,
    invalidate,
  }
})
