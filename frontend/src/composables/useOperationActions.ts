import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  APIError,
  type ReviewedStackAction,
  type StackRecord,
  submitStackAction,
  triggerRefresh,
  triggerUpdate,
} from '../services/api'
import { useConsoleStore } from '../store/console'
import { useStacksStore } from '../store/stacks'
import { useWorkStore } from '../store/work'

export type ActionPhase =
  | 'pending'
  | 'accepted'
  | 'executing'
  | 'completed'
  | 'failed'
  | 'uncertain'
export interface ActionNotice {
  phase: ActionPhase
  message: string
  operationId?: string
}
export const stackActionKey = (path: string) => `stack:${path}`

// Shared across every mounted action surface: navigation cannot replay a pending POST.
export const useOperationActions = defineStore('operation-actions', () => {
  const consoleStore = useConsoleStore()
  const stacks = useStacksStore()
  const work = useWorkStore()
  const notices = ref<Record<string, ActionNotice>>({})
  const activeWorkflows = computed(() =>
    work.supported === false
      ? work.legacyActiveOperations.map((op) => ({
          key: op.id,
          label: `${op.kind}${op.stack ? ` · ${op.stack}` : ''} · ${op.id}`,
        }))
      : work.workflows.map((group) => ({
          key: group.key,
          label: `${group.root?.kind || 'Workflow'}${group.root?.stack ? ` · ${group.root.stack}` : ''} · ${group.rootOperationId || group.key}`,
        })),
  )
  function notice(key: string): ActionNotice | undefined {
    const local = notices.value[key]
    if (!local?.operationId) return local
    const id = local.operationId
    const operation = [work.liveOperations[id], work.history[id], consoleStore.operationMap[id]]
      .filter((value) => !!value)
      .sort((a, b) => (b.version || 0) - (a.version || 0))[0]
    if (!operation) return local
    if (operation.status === 'running') {
      const durable = work.history[id] || consoleStore.operationMap[id]
      const started =
        work.activeRuns.some(
          (run) =>
            (run.operationId === id || run.rootOperationId === id) && run.phase !== 'waiting',
        ) ||
        durable?.steps.some((step) => ['executing', 'pulling', 'deploying'].includes(step.phase))
      return started
        ? { ...local, phase: 'executing', message: `Executing · ${id}` }
        : { ...local, phase: 'accepted', message: `Request accepted; awaiting execution · ${id}` }
    }
    return {
      ...local,
      phase: operation.status === 'completed' ? 'completed' : 'failed',
      message: `${operation.status === 'completed' ? 'Deployment completed · Runtime observation pending' : operation.message || operation.status} · ${id}`,
    }
  }
  function duplicateReason(key: string) {
    const state = notice(key)?.phase
    return state && ['pending', 'accepted', 'executing', 'uncertain'].includes(state)
      ? state === 'uncertain'
        ? 'Outcome unknown. Inspect operations before making another request.'
        : 'A request is already pending or awaiting execution.'
      : ''
  }
  function readinessReason() {
    return !work.mutationReady ? work.readinessReason : ''
  }
  function pendingReason() {
    return Object.values(notices.value).some((value) => value.phase === 'pending')
      ? 'A mutation request is pending'
      : ''
  }
  function deploymentReason() {
    return stacks.refreshStatus?.updatesBlocked ||
      stacks.refreshStatus?.refreshStatus !== 'completed'
      ? stacks.refreshStatus?.blockedReason || 'Repository refresh must complete before deployment'
      : ''
  }
  function workspaceReason(action: 'update' | 'refresh') {
    const base = readinessReason() || duplicateReason(`workspace:${action}`) || pendingReason()
    if (base) return base
    if (!consoleStore.instance) return 'Loading instance configuration'
    if (action === 'refresh')
      return ['refreshing', 'queued'].includes(stacks.refreshStatus?.refreshStatus || '') ||
        (work.supported === false
          ? work.legacyActiveOperations.some((op) => op.kind === 'refresh')
          : work.workflows.some(
              (group) =>
                (group.root?.kind === 'refresh' && group.root.status === 'running') ||
                group.operations.some((op) => op.kind === 'refresh' && op.status === 'running'),
            ))
        ? 'A Git check is already in progress'
        : ''
    return !consoleStore.instance.capabilities.imageUpdates
      ? 'Image updates disabled by instance configuration'
      : deploymentReason() || (activeWorkflows.value.length ? 'Work is already in progress' : '')
  }
  function stackReason(stack: StackRecord, action: ReviewedStackAction['action']) {
    return (
      readinessReason() ||
      duplicateReason(stackActionKey(stack.path)) ||
      pendingReason() ||
      (!consoleStore.instance?.capabilities.apply || !consoleStore.instance.policies.reconcile
        ? 'Reconciliation disabled by instance configuration'
        : '') ||
      (action === 'update' && !consoleStore.instance?.capabilities.imageUpdates
        ? 'Image updates disabled by instance configuration'
        : '') ||
      deploymentReason() ||
      (stack.requiresAcknowledgement && action !== 'acknowledge'
        ? 'Approval required; review the change first'
        : '') ||
      (stack.status === 'syncing' ||
      stack.status === 'deleting' ||
      work.operationStagesForStack(stack.path).length ||
      work.runsForStack(stack.path).length
        ? 'Work is already in progress for this stack'
        : '')
    )
  }
  async function send(
    key: string,
    submit: (() => Promise<{ operationId: string }>) | (() => Promise<void>),
  ) {
    notices.value[key] = { phase: 'pending', message: 'Request pending…' }
    try {
      const result = await submit()
      // Global endpoints acknowledge receipt without identifying an operation.
      // Release only the POST guard, not a fabricated operation outcome. Server
      // work/readiness remains authoritative; never match later work by kind/time.
      if (!result?.operationId) {
        delete notices.value[key]
        return
      }
      notices.value[key] = {
        phase: 'accepted',
        message: 'Request accepted; awaiting execution',
        operationId: result.operationId,
      }
      if (result?.operationId) await consoleStore.inspect(result.operationId).catch(() => undefined)
    } catch (error) {
      const rejected = error instanceof APIError && error.status < 500
      notices.value[key] = {
        phase: rejected ? 'failed' : 'uncertain',
        message: rejected
          ? error.message
          : 'Request outcome unknown; it may still be running. Review operations before trying again.',
      }
      if (!rejected) void consoleStore.loadOperations().catch(() => undefined)
    }
  }
  async function submitWorkspace(action: 'update' | 'refresh') {
    if (workspaceReason(action)) return
    await send(`workspace:${action}`, action === 'update' ? triggerUpdate : triggerRefresh)
  }
  async function submitStack(body: ReviewedStackAction) {
    const stack = stacks.getStack(body.stack)
    if (!stack || stackReason(stack, body.action)) return
    const current = stack.desiredRevision || stacks.refreshStatus?.revision
    if (!body.revision || current !== body.revision) {
      notices.value[stackActionKey(body.stack)] = {
        phase: 'failed',
        message: 'Desired revision changed; review the stack again',
      }
      return
    }
    const frozen = { ...body }
    await send(stackActionKey(frozen.stack), () => submitStackAction(frozen))
  }
  function dismissNotice(key: string) {
    if (['failed', 'uncertain'].includes(notice(key)?.phase || '')) delete notices.value[key]
  }
  return {
    activeWorkflows,
    notice,
    workspaceReason,
    stackReason,
    submitWorkspace,
    submitStack,
    dismissNotice,
  }
})
