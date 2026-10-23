import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref } from 'vue'
import type { RuntimeState } from '../composables/useRuntimeOverview'
import {
  type ContainerInfo,
  type ImageIdentity,
  type Instance,
  type Operation,
  type RuntimeObservation as RuntimeDTO,
  requestJSON,
} from '../services/api'
import { useWorkStore } from './work'

export interface RuntimeObservation extends RuntimeState {
  observedAt?: string | null
  attemptedAt?: string
  stale?: boolean
}
export interface RuntimeView extends RuntimeObservation {
  observedAt: string | null
  stale: boolean
}
export interface ContainerInspectionSelection {
  stack: string
  id: string
  // Detached selection snapshot: identity and observation provenance never follow polling.
  container: ContainerInfo
  // Actual successful server listing timestamp at selection; never browser fetch time.
  // Image inspection provenance remains in container.imageIdentity.observedAt/source.
  observedAt: string | null
  attemptedAt?: string
}
export interface ContainerInspectionIdentityComparison {
  currentStatus: ImageIdentity['status'] | 'not_reported' | 'container_removed'
  currentUnavailable: boolean
  referenceChanged: boolean
  // Metadata difference, not a claim that the immutable image contents changed.
  // Observation timestamps alone do not constitute an identity change.
  metadataChanged: boolean
}

function identityMetadata(identity: ImageIdentity | undefined): string {
  if (!identity) return ''
  const digest = (value: ImageIdentity['repositoryDigests'][number]) => [
    value.repository,
    value.digest,
    value.kind,
  ]
  return JSON.stringify({
    status: identity.status,
    source: identity.source,
    imageId: identity.imageId,
    reference: identity.reference,
    platform: identity.platform && [
      identity.platform.os,
      identity.platform.architecture,
      identity.platform.variant,
    ],
    repositoryDigests: identity.repositoryDigests
      .map((value) => JSON.stringify(digest(value)))
      .sort(),
    preferredRepositoryDigest:
      identity.preferredRepositoryDigest && digest(identity.preferredRepositoryDigest),
    reason: identity.reason,
    freshness: identity.freshness,
  })
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
  const runtime = ref<Record<string, RuntimeObservation>>(Object.create(null))
  const runtimeError = ref('')
  const runtimeLoading = ref(false)
  const runtimeNow = ref(Date.now())
  const inspectedContainer = ref<ContainerInspectionSelection | null>(null)
  // Raw latest same-ID observation, for comparison/status only. Identity display
  // uses inspectedContainerCurrent or the anchored inspectedContainer.container.
  const inspectedContainerLatest = computed(() => {
    const selected = inspectedContainer.value
    return selected
      ? (runtime.value[selected.stack]?.containers.find((row) => row.id === selected.id) ?? null)
      : null
  })
  const inspectedContainerRemoved = computed(() => {
    return !!inspectedContainer.value && !inspectedContainerLatest.value
  })
  // Compatibility display view: operational state/health may update, but the
  // selected image reference and dated immutable identity require explicit reselection.
  const inspectedContainerCurrent = computed(() => {
    const selected = inspectedContainer.value
    if (!selected) return null
    const current = {
      ...(inspectedContainerLatest.value || selected.container),
      image: selected.container.image,
    }
    if (selected.container.imageIdentity) current.imageIdentity = selected.container.imageIdentity
    else delete current.imageIdentity
    return current
  })
  // Latest listing provenance/error/staleness is separate from the selected
  // observation above; do not date the retained identity with this timestamp.
  const inspectedContainerRuntime = computed(() =>
    inspectedContainer.value ? runtimeFor(inspectedContainer.value.stack) : null,
  )
  const inspectedContainerIdentityComparison =
    computed<ContainerInspectionIdentityComparison | null>(() => {
      const selected = inspectedContainer.value
      if (!selected) return null
      const latest = inspectedContainerLatest.value
      return {
        currentStatus: latest
          ? (latest.imageIdentity?.status ?? 'not_reported')
          : 'container_removed',
        currentUnavailable: !!latest && latest.imageIdentity?.status !== 'available',
        referenceChanged:
          !!latest &&
          (latest.image !== selected.container.image ||
            (!!latest.imageIdentity &&
              !!selected.container.imageIdentity &&
              latest.imageIdentity.reference !== selected.container.imageIdentity.reference)),
        metadataChanged:
          !!latest &&
          identityMetadata(latest.imageIdentity) !==
            identityMetadata(selected.container.imageIdentity),
      }
    })
  const operationMap = ref<Record<string, Operation>>(Object.create(null))
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
  let flight: Promise<void> | null = null
  let queued = false
  let polling = false
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let refreshTimer: ReturnType<typeof setTimeout> | undefined
  let nextPoll = 0
  let onPoll: (() => void) | undefined
  let instanceRequest = 0
  function onOperation(operation: Operation) {
    const existing = operationMap.value[operation.id]
    if (existing && (existing.version || 0) > (operation.version || 0)) return
    if (existing?.finishedAt && !operation.finishedAt && !operation.version) return
    operationMap.value[operation.id] = operation
    useWorkStore().onHistory(operation)
  }
  async function loadInstance() {
    const version = ++instanceRequest
    try {
      const next = await requestJSON<Instance>('/api/instance')
      if (version !== instanceRequest) return
      if (!next?.capabilities) throw new Error('Invalid instance response')
      instance.value = next
      useWorkStore().setCapability(next.capabilities.liveWorkVersion)
      instanceError.value = ''
    } catch (error) {
      if (version === instanceRequest)
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
      if (!more) useWorkStore().legacyHistoryReceived()
    } catch (error) {
      operationsError.value = error instanceof Error ? error.message : 'Operations unavailable'
    }
  }
  function loadRuntime(): Promise<void> {
    if (flight) return flight
    queued = false
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = undefined
    }
    runtimeLoading.value = true
    const version = ++runtimeRequest
    flight = (async () => {
      try {
        const next = await requestJSON<Record<string, RuntimeDTO>>('/api/runtime')
        if (
          !next ||
          typeof next !== 'object' ||
          Array.isArray(next) ||
          Object.values(next).some(
            (v) =>
              !v ||
              !Array.isArray(v.containers) ||
              v.containers.some(
                (row) =>
                  !row ||
                  ['id', 'name', 'service', 'state', 'health', 'image'].some(
                    (key) => typeof row[key as keyof ContainerInfo] !== 'string',
                  ),
              ) ||
              (v.observedAt != null &&
                (typeof v.observedAt !== 'string' || !Number.isFinite(Date.parse(v.observedAt)))) ||
              (v.attemptedAt !== undefined &&
                (typeof v.attemptedAt !== 'string' ||
                  !Number.isFinite(Date.parse(v.attemptedAt)))) ||
              (v.error != null && typeof v.error !== 'string'),
          )
        )
          throw new Error('Invalid runtime response')
        if (version !== runtimeRequest) return
        const merged: Record<string, RuntimeObservation> = Object.create(null)
        for (const [path, state] of Object.entries(next)) {
          const old = runtime.value[path]
          // Do not let server-cache reuse or partial failures erase a known success.
          const older =
            old &&
            (old.observedAt
              ? !state.observedAt ||
                Date.parse(state.observedAt) < Date.parse(old.observedAt) ||
                (!!state.error && state.observedAt === old.observedAt)
              : !!state.error)
          merged[path] = {
            ...state,
            loading: false,
            containers: older ? old.containers : state.containers,
            observedAt: older ? old.observedAt : (state.observedAt ?? null),
          }
        }
        runtime.value = merged
        runtimeError.value = ''
      } catch (error) {
        if (version === runtimeRequest)
          runtimeError.value = error instanceof Error ? error.message : 'Runtime unavailable'
      } finally {
        runtimeLoading.value = false
        runtimeNow.value = Date.now()
        flight = null
        if (queued && document.visibilityState !== 'hidden') {
          queued = false
          void loadRuntime()
        }
        scheduleTick()
      }
    })()
    return flight
  }
  function runtimeFor(path: string): RuntimeView {
    const state = runtime.value[path]
    const stamp = state?.observedAt ? Date.parse(state.observedAt) : NaN
    return {
      ...(state || { containers: [], loading: !runtimeError.value, error: null }),
      observedAt: state?.observedAt ?? null,
      error: runtimeError.value || state?.error || null,
      stale: !Number.isFinite(stamp) || runtimeNow.value - stamp >= 45000,
    }
  }
  function ensureRuntime(path: string) {
    if (document.visibilityState === 'hidden') {
      queued = true
      return
    }
    if (!runtime.value[path]) void loadRuntime()
  }
  function refreshRuntime(immediate = false) {
    if (refreshTimer) clearTimeout(refreshTimer)
    queued = false
    // Invalidate in-flight observations before a deployment/stack change can race them.
    if (flight) runtimeRequest++
    if (document.visibilityState === 'hidden') {
      queued = true
      return
    }
    const refresh = () => {
      refreshTimer = undefined
      if (document.visibilityState === 'hidden') {
        queued = true
        return
      }
      if (flight) {
        queued = true
        return
      }
      queued = false
      void loadRuntime()
    }
    if (immediate) refresh()
    else refreshTimer = setTimeout(refresh, 200)
  }
  function scheduleTick() {
    if (pollTimer) clearTimeout(pollTimer)
    if (!polling || document.visibilityState === 'hidden') return
    const now = Date.now()
    const staleTimes = Object.values(runtime.value)
      .map((s) => (s.observedAt ? Date.parse(s.observedAt) + 45000 : NaN))
      .filter((t) => Number.isFinite(t) && t > now)
    const at = Math.min(nextPoll, ...staleTimes)
    pollTimer = setTimeout(
      () => {
        runtimeNow.value = Date.now()
        if (runtimeNow.value >= nextPoll) {
          nextPoll = Date.now() + 15000
          void loadRuntime()
          onPoll?.()
        }
        scheduleTick()
      },
      Math.max(1, at - now),
    )
  }
  function visibilityChanged() {
    if (pollTimer) clearTimeout(pollTimer)
    if (document.visibilityState !== 'hidden') {
      runtimeNow.value = Date.now()
      nextPoll = Date.now() + 15000
      refreshRuntime(true)
      scheduleTick()
    }
  }
  function startRuntimePolling(onVisiblePoll?: () => void) {
    if (polling) return
    polling = true
    onPoll = onVisiblePoll
    nextPoll = Date.now() + 15000
    document.addEventListener('visibilitychange', visibilityChanged)
    if (document.visibilityState !== 'hidden') void loadRuntime()
    scheduleTick()
  }
  function stopRuntimePolling() {
    polling = false
    if (pollTimer) clearTimeout(pollTimer)
    if (refreshTimer) clearTimeout(refreshTimer)
    document.removeEventListener('visibilitychange', visibilityChanged)
    runtimeRequest++
    instanceRequest++
    queued = false
    onPoll = undefined
  }
  function inspectContainer(stack: string, id: string) {
    const container = runtime.value[stack]?.containers.find((row) => row.id === id)
    if (container)
      inspectedContainer.value = {
        stack,
        id,
        container: JSON.parse(JSON.stringify(container)) as ContainerInfo,
        observedAt: runtime.value[stack]?.observedAt ?? null,
        attemptedAt: runtime.value[stack]?.attemptedAt,
      }
  }
  onScopeDispose(stopRuntimePolling)
  async function inspect(id: string) {
    inspectedId.value = id
    // The shared inspector and direct route share the same in-flight lookup.
    // A missing individual record is not a failure of the whole history feed.
    const work = useWorkStore()
    await work.lookup(id)
    const record = work.outcomes[id]?.operation
    if (record) onOperation(record)
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
    runtimeLoading,
    runtimeNow,
    inspectedContainer,
    inspectedContainerRemoved,
    inspectedContainerCurrent,
    inspectedContainerLatest,
    inspectedContainerRuntime,
    inspectedContainerIdentityComparison,
    inspectContainer,
    ensureRuntime,
    refreshRuntime,
    startRuntimePolling,
    stopRuntimePolling,
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
