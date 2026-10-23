// V2 contracts and fail-closed validation at the SSE boundary.
// Authority: .lead/_adhoc/operational-workspace-contracts.md
import type { Operation } from './api'

export type WorkStatus = 'running' | 'completed' | 'failed' | 'cancelled'
export type WorkPhase =
  | 'waiting'
  | 'checking_repository'
  | 'pulling'
  | 'applying'
  | 'pruning'
  | 'finished'

export interface WorkLayer {
  id: string
  phase: 'waiting' | 'downloading' | 'extracting' | 'finished' | 'unknown'
  status: string
  bytesCurrent?: number | null
  bytesTotal?: number | null
  done: boolean
  updatedAt: string
}

export interface WorkImage {
  pullAttemptId: string
  reference: string
  status: WorkStatus
  done: boolean
  position?: number
  total?: number | null
  layerCount: number
  layers: WorkLayer[]
  detailsTruncated: boolean
}

export interface WorkStackOutcome {
  stack: string
  status: 'completed' | 'skipped' | 'failed'
  operationId?: string
  message: string
}

export interface WorkStackResults {
  processed: number
  skipped: number
  failed: number
  outcomeCount: number
  outcomes: WorkStackOutcome[]
  detailsTruncated: boolean
}

interface WorkRunBase {
  runId: string
  operationId: string
  rootOperationId: string
  parentOperationId?: string
  source: 'scheduler' | 'reconcile' | 'refresh'
  stack?: string
  revision?: string
  cycleId?: string
  startedAt: string
  updatedAt: string
  message: string
  errorCode?: string
  imageCount: number
  images: WorkImage[]
  stackResults?: WorkStackResults
  detailsTruncated: boolean
}

export type WorkRunningRun = WorkRunBase & {
  status: 'running'
  phase: Exclude<WorkPhase, 'finished'>
  finishedAt?: never
}

export type WorkTerminalRun = WorkRunBase & {
  status: Exclude<WorkStatus, 'running'>
  phase: 'finished'
  finishedAt: string
}

export type WorkRun = WorkRunningRun | WorkTerminalRun

// Complete live summary, deliberately excluding durable steps/layer history.
// Missing rootOperationId means ancestry is unavailable, not "this is a root".
export interface WorkOperation extends Omit<Operation, 'version' | 'steps'> {
  version: number
  rootOperationId?: string
}

interface WorkEnvelope {
  schemaVersion: 2
  epoch: string
  sequence: number
  operations: WorkOperation[]
}

export interface WorkSnapshot extends WorkEnvelope {
  runs: WorkRunningRun[]
  detailsTruncated: boolean
}

export interface WorkUpsert extends WorkEnvelope {
  run?: WorkRun
}

export type WorkEvent =
  | { event: 'work.snapshot'; data: WorkSnapshot }
  | { event: 'work.upsert'; data: WorkUpsert }

type RecordValue = Record<string, unknown>
const object = (v: unknown): v is RecordValue => !!v && typeof v === 'object' && !Array.isArray(v)
const text = (v: unknown): v is string => typeof v === 'string'
const id = (v: unknown): v is string => text(v) && v.length > 0
const count = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0
const time = (v: unknown): v is string =>
  text(v) && /^\d{4}-\d\d-\d\dT/.test(v) && Number.isFinite(Date.parse(v))
const optional = (v: RecordValue, keys: string[]) =>
  keys.every((key) => v[key] === undefined || id(v[key]))
const unique = <T, K extends keyof T>(values: T[], key: K) =>
  new Set(values.map((v) => v[key])).size === values.length
const statuses = ['running', 'completed', 'failed', 'cancelled']
const encoder = new TextEncoder()
const displayText = (v: unknown) => text(v) && encoder.encode(v).length <= 1024

function operation(v: unknown): v is WorkOperation {
  return (
    object(v) &&
    id(v.id) &&
    count(v.version) &&
    v.version > 0 &&
    id(v.kind) &&
    id(v.trigger) &&
    id(v.status) &&
    displayText(v.message) &&
    time(v.startedAt) &&
    optional(v, ['parentId', 'rootOperationId', 'stack', 'revision']) &&
    v.parentId !== v.id &&
    (v.status === 'running'
      ? v.finishedAt === undefined
      : time(v.finishedAt) && Date.parse(v.finishedAt) >= Date.parse(v.startedAt))
  )
}

function layer(v: unknown): v is WorkLayer {
  return (
    object(v) &&
    id(v.id) &&
    ['waiting', 'downloading', 'extracting', 'finished', 'unknown'].includes(String(v.phase)) &&
    displayText(v.status) &&
    typeof v.done === 'boolean' &&
    time(v.updatedAt) &&
    ['bytesCurrent', 'bytesTotal'].every((key) => v[key] == null || count(v[key]))
  )
}

function image(v: unknown): v is WorkImage {
  if (
    !object(v) ||
    !id(v.pullAttemptId) ||
    !id(v.reference) ||
    !statuses.includes(String(v.status)) ||
    v.done !== (v.status !== 'running') ||
    typeof v.detailsTruncated !== 'boolean' ||
    !count(v.layerCount) ||
    !Array.isArray(v.layers) ||
    v.layers.length > 200 ||
    !v.layers.every(layer) ||
    !unique(v.layers, 'id') ||
    v.layerCount < v.layers.length ||
    (!v.detailsTruncated && v.layerCount !== v.layers.length)
  )
    return false
  return (
    (v.position === undefined || (count(v.position) && v.position > 0)) &&
    (v.total == null ||
      (count(v.total) && (v.position === undefined || (v.position as number) <= v.total)))
  )
}

function run(v: unknown): v is WorkRun {
  if (
    !object(v) ||
    !id(v.runId) ||
    !id(v.operationId) ||
    !id(v.rootOperationId) ||
    !optional(v, ['parentOperationId', 'stack', 'revision', 'cycleId', 'errorCode']) ||
    !['scheduler', 'reconcile', 'refresh'].includes(String(v.source)) ||
    !statuses.includes(String(v.status)) ||
    !time(v.startedAt) ||
    !time(v.updatedAt) ||
    Date.parse(v.updatedAt) < Date.parse(v.startedAt) ||
    !displayText(v.message) ||
    typeof v.detailsTruncated !== 'boolean' ||
    !count(v.imageCount) ||
    !Array.isArray(v.images) ||
    v.images.length > 32 ||
    !v.images.every(image) ||
    !unique(v.images, 'pullAttemptId') ||
    v.imageCount < v.images.length ||
    (!v.detailsTruncated &&
      (v.imageCount !== v.images.length || v.images.some((i) => i.detailsTruncated)))
  )
    return false
  if (v.status === 'running') {
    if (
      !['waiting', 'checking_repository', 'pulling', 'applying', 'pruning'].includes(
        String(v.phase),
      ) ||
      v.finishedAt !== undefined
    )
      return false
  } else if (
    v.phase !== 'finished' ||
    !time(v.finishedAt) ||
    Date.parse(v.finishedAt) < Date.parse(v.startedAt)
  )
    return false
  if (v.stackResults !== undefined) {
    const s = v.stackResults
    if (
      !object(s) ||
      !['processed', 'skipped', 'failed', 'outcomeCount'].every((k) => count(s[k])) ||
      typeof s.detailsTruncated !== 'boolean' ||
      !Array.isArray(s.outcomes) ||
      s.outcomes.length > 100 ||
      (s.failed as number) > (s.processed as number) ||
      s.outcomeCount !== (s.processed as number) + (s.skipped as number) ||
      (s.outcomeCount as number) < s.outcomes.length ||
      (!s.detailsTruncated && s.outcomeCount !== s.outcomes.length) ||
      (s.detailsTruncated && !v.detailsTruncated) ||
      !s.outcomes.every(
        (o: unknown) =>
          object(o) &&
          id(o.stack) &&
          ['completed', 'skipped', 'failed'].includes(String(o.status)) &&
          displayText(o.message) &&
          optional(o, ['operationId']),
      )
    )
      return false
  }
  return true
}

export function isWorkSnapshot(v: unknown): v is WorkSnapshot {
  return (
    envelope(v) &&
    Array.isArray(v.runs) &&
    v.runs.every((r) => run(r) && r.status === 'running') &&
    unique(v.runs, 'runId') &&
    typeof v.detailsTruncated === 'boolean' &&
    (v.detailsTruncated || !v.runs.some((r) => r.detailsTruncated)) &&
    detailBudget(v.runs) &&
    ownership(v.operations, v.runs)
  )
}

export function isWorkUpsert(v: unknown): v is WorkUpsert {
  return (
    envelope(v) &&
    (v.run === undefined
      ? v.operations.length > 0
      : run(v.run) && detailBudget([v.run]) && ownership(v.operations, [v.run]))
  )
}

function envelope(v: unknown): v is RecordValue & { operations: WorkOperation[] } {
  return (
    object(v) &&
    v.schemaVersion === 2 &&
    id(v.epoch) &&
    count(v.sequence) &&
    Array.isArray(v.operations) &&
    v.operations.every(operation) &&
    unique(v.operations, 'id')
  )
}

function detailBudget(runs: WorkRun[]) {
  return (
    runs.reduce(
      (n, r) =>
        n +
        encoder.encode(JSON.stringify(r.images)).length +
        (r.stackResults ? encoder.encode(JSON.stringify(r.stackResults.outcomes)).length : 0),
      0,
    ) <= 2097152
  )
}

function ownership(operations: WorkOperation[], runs: WorkRun[]) {
  const byId = new Map(operations.map((op) => [op.id, op]))
  return runs.every((run) => {
    const owner = byId.get(run.operationId)
    return (
      !owner ||
      ((!owner.rootOperationId || owner.rootOperationId === run.rootOperationId) &&
        owner.parentId === run.parentOperationId)
    )
  })
}
