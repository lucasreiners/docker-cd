import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { type ContainerInfo, type ImageIdentity, requestJSON } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'

vi.mock('../src/services/api', () => ({ requestJSON: vi.fn() }))
const container = {
  id: 'original',
  name: 'web',
  service: 'web',
  image: 'nginx',
  state: 'running',
  health: 'none',
}
const response = (observedAt: string | null, error: string | null = null) => ({
  'apps/web': { containers: [container], observedAt, error },
})
const selectedAt = '2026-10-23T12:00:00Z'
const latestAt = '2026-10-23T12:00:15Z'
const selectedIdentity: ImageIdentity = {
  status: 'available',
  source: 'container_image_inspect',
  observedAt: selectedAt,
  reference: 'nginx:stable',
  imageId: `sha256:${'a'.repeat(64)}`,
  repositoryDigests: [
    { repository: 'docker.io/library/nginx', digest: `sha256:${'b'.repeat(64)}`, kind: 'unknown' },
  ],
  preferredRepositoryDigest: {
    repository: 'docker.io/library/nginx',
    digest: `sha256:${'b'.repeat(64)}`,
    kind: 'unknown',
  },
  platform: { os: 'linux', architecture: 'amd64' },
  freshness: 'not_checked',
}
async function observe(row: ContainerInfo, observedAt: string) {
  vi.mocked(requestJSON).mockResolvedValueOnce({
    'apps/web': { containers: [row], observedAt, attemptedAt: observedAt, error: null },
  })
  await useConsoleStore().loadRuntime()
}
async function selectIdentity() {
  await observe(
    { ...container, image: 'nginx:stable', imageIdentity: structuredClone(selectedIdentity) },
    selectedAt,
  )
  const store = useConsoleStore()
  store.inspectContainer('apps/web', container.id)
  return store
}
let visible = true
beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-23T12:00:00Z'))
  visible = true
  vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() =>
    visible ? 'visible' : 'hidden',
  )
})
afterEach(() => {
  useConsoleStore().$dispose()
  vi.useRealTimers()
  vi.restoreAllMocks()
})
function visibility(value: boolean) {
  visible = value
  document.dispatchEvent(new Event('visibilitychange'))
}

it('owns one visible 15-second poll, pauses hidden, refreshes immediately on return and marks cached server timestamps stale at 45 seconds', async () => {
  vi.mocked(requestJSON).mockResolvedValue(response('2026-10-23T12:00:00Z'))
  const store = useConsoleStore()
  const visiblePoll = vi.fn()
  store.startRuntimePolling(visiblePoll)
  store.startRuntimePolling()
  await flushPromises()
  expect(requestJSON).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(15000)
  expect(requestJSON).toHaveBeenCalledTimes(2)
  expect(visiblePoll).toHaveBeenCalledTimes(1)
  expect(store.runtimeFor('apps/web').stale).toBe(false)
  await vi.advanceTimersByTimeAsync(30000)
  expect(store.runtimeFor('apps/web').observedAt).toBe('2026-10-23T12:00:00Z')
  expect(store.runtimeFor('apps/web').stale).toBe(true)
  visibility(false)
  await vi.advanceTimersByTimeAsync(60000)
  expect(requestJSON).toHaveBeenCalledTimes(4)
  expect(visiblePoll).toHaveBeenCalledTimes(3)
  visibility(true)
  await flushPromises()
  expect(requestJSON).toHaveBeenCalledTimes(5)
  store.stopRuntimePolling()
  await vi.advanceTimersByTimeAsync(60000)
  expect(requestJSON).toHaveBeenCalledTimes(5)
})

it('preserves successful rows, provenance and independent partial / transport failures', async () => {
  const store = useConsoleStore()
  vi.mocked(requestJSON).mockResolvedValueOnce(response('2026-10-23T12:00:00Z'))
  await store.loadRuntime()
  vi.mocked(requestJSON).mockResolvedValueOnce({
    'apps/web': { containers: [], observedAt: null, error: 'Docker unavailable' },
  })
  await store.loadRuntime()
  expect(store.runtimeFor('apps/web')).toMatchObject({
    containers: [container],
    observedAt: '2026-10-23T12:00:00Z',
    error: 'Docker unavailable',
  })
  vi.mocked(requestJSON).mockRejectedValueOnce(new Error('Network unavailable'))
  await store.loadRuntime()
  expect(store.runtimeFor('apps/web').containers).toEqual([container])
  expect(store.runtimeFor('apps/web').error).toBe('Network unavailable')
  vi.mocked(requestJSON).mockResolvedValueOnce({
    'apps/web': { containers: [], observedAt: '2026-10-23T12:00:00Z', error: 'Listing failed' },
  })
  await store.loadRuntime()
  expect(store.runtimeFor('apps/web').containers).toEqual([container])
  vi.mocked(requestJSON).mockResolvedValueOnce(response(null))
  await store.loadRuntime()
  expect(store.runtimeFor('apps/web').observedAt).toBe('2026-10-23T12:00:00Z')
})

it('coalesces in-flight reads, ignores invalidated older polls and debounces terminal / stack bursts without overlap', async () => {
  const store = useConsoleStore()
  let finish!: (value: unknown) => void
  vi.mocked(requestJSON).mockReturnValueOnce(
    new Promise((resolve) => {
      finish = resolve
    }),
  )
  const first = store.loadRuntime()
  const second = store.loadRuntime()
  expect(requestJSON).toHaveBeenCalledTimes(1)
  store.refreshRuntime()
  store.refreshRuntime()
  vi.mocked(requestJSON).mockResolvedValue(response('2026-10-23T12:00:01Z'))
  await vi.advanceTimersByTimeAsync(200)
  expect(requestJSON).toHaveBeenCalledTimes(1)
  finish(response('2026-10-23T12:00:00Z'))
  await Promise.all([first, second])
  await flushPromises()
  expect(requestJSON).toHaveBeenCalledTimes(2)
  expect(store.runtimeFor('apps/web').observedAt).toBe('2026-10-23T12:00:01Z')
  store.refreshRuntime(true) // reconnect
  await flushPromises()
  expect(requestJSON).toHaveBeenCalledTimes(3)
})

it('retains the inspected ID after replacement and clears removed stack scopes only on authoritative success', async () => {
  const store = useConsoleStore()
  vi.mocked(requestJSON).mockResolvedValueOnce(response('2026-10-23T12:00:00Z'))
  await store.loadRuntime()
  store.inspectContainer('apps/web', container.id)
  vi.mocked(requestJSON).mockResolvedValueOnce({
    'apps/web': {
      containers: [{ ...container, id: 'replacement' }],
      observedAt: '2026-10-23T12:00:01Z',
      error: null,
    },
  })
  await store.loadRuntime()
  expect(store.inspectedContainerRemoved).toBe(true)
  expect(store.inspectedContainerCurrent?.id).toBe('original')
  expect(store.inspectedContainerLatest).toBeNull()
  expect(store.inspectedContainerIdentityComparison?.currentStatus).toBe('container_removed')
  expect(store.inspectedContainer?.id).toBe('original')
  vi.mocked(requestJSON).mockResolvedValueOnce({})
  await store.loadRuntime()
  expect(store.runtime).toEqual({})
  expect(store.inspectedContainerCurrent?.id).toBe('original')
})

it.each([
  undefined,
  {
    status: 'unavailable',
    source: 'container_image_inspect',
    observedAt: latestAt,
    reference: 'nginx:stable',
    repositoryDigests: [],
    reason: 'inspection_failed',
    freshness: 'not_checked',
  } satisfies ImageIdentity,
  {
    status: 'ambiguous',
    source: 'container_image_inspect',
    observedAt: latestAt,
    reference: 'nginx:stable',
    repositoryDigests: [],
    reason: 'repository_ambiguous',
    freshness: 'not_checked',
  } satisfies ImageIdentity,
])(
  'retains selected same-ID identity and provenance when current enrichment degrades (%#)',
  async (imageIdentity) => {
    const store = await selectIdentity()
    await observe(
      { ...container, image: 'nginx:stable', imageIdentity, health: 'unhealthy', state: 'exited' },
      latestAt,
    )
    expect(store.inspectedContainerCurrent).toMatchObject({
      id: container.id,
      image: 'nginx:stable',
      imageIdentity: selectedIdentity,
      health: 'unhealthy',
      state: 'exited',
    })
    expect(store.inspectedContainer?.observedAt).toBe(selectedAt)
    expect(store.inspectedContainer?.attemptedAt).toBe(selectedAt)
    expect(store.inspectedContainerCurrent?.imageIdentity?.observedAt).toBe(selectedAt)
    expect(store.inspectedContainerLatest?.imageIdentity).toEqual(imageIdentity)
    expect(store.inspectedContainerRuntime?.observedAt).toBe(latestAt)
    expect(store.inspectedContainerRemoved).toBe(false)
    expect(store.inspectedContainerIdentityComparison).toEqual({
      currentStatus: imageIdentity?.status ?? 'not_reported',
      currentUnavailable: true,
      referenceChanged: false,
      metadataChanged: true,
    })
  },
)

it('retains the selected mutable reference until explicit same-ID reselection', async () => {
  const store = await selectIdentity()
  await observe(
    {
      ...container,
      image: 'nginx:latest',
      imageIdentity: { ...selectedIdentity, observedAt: latestAt },
    },
    latestAt,
  )
  expect(store.inspectedContainerCurrent?.image).toBe('nginx:stable')
  expect(store.inspectedContainerIdentityComparison).toMatchObject({
    referenceChanged: true,
    metadataChanged: false,
  })
  const nextIdentity = { ...selectedIdentity, reference: 'nginx:latest', observedAt: latestAt }
  await observe({ ...container, image: 'nginx:latest', imageIdentity: nextIdentity }, latestAt)
  expect(store.inspectedContainerCurrent?.image).toBe('nginx:stable')
  expect(store.inspectedContainerCurrent?.imageIdentity).toEqual(selectedIdentity)
  expect(store.inspectedContainerIdentityComparison).toMatchObject({
    referenceChanged: true,
    metadataChanged: true,
    currentUnavailable: false,
  })
  expect(store.inspectedContainerRemoved).toBe(false)
  store.inspectContainer('apps/web', container.id)
  expect(store.inspectedContainerCurrent?.image).toBe('nginx:latest')
  expect(store.inspectedContainerCurrent?.imageIdentity).toEqual(nextIdentity)
  expect(store.inspectedContainer?.observedAt).toBe(latestAt)
  expect(store.inspectedContainerIdentityComparison).toMatchObject({
    referenceChanged: false,
    metadataChanged: false,
  })
})

it('retains selected immutable image/digests/platform when same-ID identity metadata changes', async () => {
  const store = await selectIdentity()
  const digest = {
    repository: 'docker.io/library/nginx',
    digest: `sha256:${'c'.repeat(64)}`,
    kind: 'manifest' as const,
  }
  const nextIdentity: ImageIdentity = {
    ...selectedIdentity,
    observedAt: latestAt,
    imageId: `sha256:${'d'.repeat(64)}`,
    repositoryDigests: [digest],
    preferredRepositoryDigest: digest,
    platform: { os: 'linux', architecture: 'arm64' },
  }
  await observe({ ...container, image: 'nginx:stable', imageIdentity: nextIdentity }, latestAt)
  expect(store.inspectedContainerCurrent?.imageIdentity).toEqual(selectedIdentity)
  expect(store.inspectedContainerLatest?.imageIdentity).toEqual(nextIdentity)
  expect(store.inspectedContainerIdentityComparison).toEqual({
    currentStatus: 'available',
    currentUnavailable: false,
    referenceChanged: false,
    metadataChanged: true,
  })
  expect(store.inspectedContainerRemoved).toBe(false)
})

it('retains dated selection provenance without reporting timestamp-only refreshes as identity changes', async () => {
  const store = await selectIdentity()
  await observe(
    {
      ...container,
      image: 'nginx:stable',
      imageIdentity: { ...selectedIdentity, observedAt: latestAt },
    },
    latestAt,
  )
  expect(store.inspectedContainerIdentityComparison).toMatchObject({
    currentUnavailable: false,
    metadataChanged: false,
    referenceChanged: false,
  })
  expect(store.inspectedContainerCurrent?.imageIdentity?.observedAt).toBe(selectedAt)
  expect(store.inspectedContainer?.observedAt).toBe(selectedAt)
  expect(store.inspectedContainerLatest?.imageIdentity?.observedAt).toBe(latestAt)
})

it('detaches nested selected identity metadata from runtime cache mutations and does not enrich an initially unknown selection automatically', async () => {
  const store = await selectIdentity()
  const row = store.runtime['apps/web'].containers[0]
  if (!row.imageIdentity) throw new Error('Missing identity fixture')
  row.imageIdentity.repositoryDigests[0].digest = 'sha256:changed-in-cache'
  expect(store.inspectedContainerCurrent?.imageIdentity?.repositoryDigests[0].digest).toBe(
    `sha256:${'b'.repeat(64)}`,
  )
  await observe(container, latestAt)
  store.inspectContainer('apps/web', container.id)
  await observe({ ...container, imageIdentity: selectedIdentity }, '2026-10-23T12:00:30Z')
  expect(store.inspectedContainerCurrent?.imageIdentity).toBeUndefined()
  expect(store.inspectedContainerLatest?.imageIdentity?.status).toBe('available')
  expect(store.inspectedContainerIdentityComparison).toMatchObject({
    currentUnavailable: false,
    metadataChanged: true,
  })
  store.inspectedContainer = null
  expect(store.inspectedContainerCurrent).toBeNull()
  expect(store.inspectedContainerLatest).toBeNull()
  expect(store.inspectedContainerRuntime).toBeNull()
  expect(store.inspectedContainerIdentityComparison).toBeNull()
  expect(store.inspectedContainerRemoved).toBe(false)
})

it('debounces again when new terminal events arrive while a queued refresh waits behind a slow request', async () => {
  const store = useConsoleStore()
  let finish!: (value: unknown) => void
  vi.mocked(requestJSON).mockReturnValueOnce(
    new Promise((resolve) => {
      finish = resolve
    }),
  )
  const pending = store.loadRuntime()
  store.refreshRuntime()
  await vi.advanceTimersByTimeAsync(250)
  store.refreshRuntime()
  vi.mocked(requestJSON).mockResolvedValue(response('2026-10-23T12:00:01Z'))
  finish(response('2026-10-23T12:00:00Z'))
  await pending
  expect(requestJSON).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(199)
  expect(requestJSON).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(requestJSON).toHaveBeenCalledTimes(2)
})

it('ignores responses after polling disposal, and malformed runtime never erases known rows', async () => {
  const store = useConsoleStore()
  vi.mocked(requestJSON).mockResolvedValueOnce(response('2026-10-23T12:00:00Z'))
  await store.loadRuntime()
  vi.mocked(requestJSON).mockResolvedValueOnce({ 'apps/web': null })
  await store.loadRuntime()
  expect(store.runtimeFor('apps/web').containers).toEqual([container])
  expect(store.runtimeError).toBe('Invalid runtime response')
  let finish!: (value: unknown) => void
  vi.mocked(requestJSON).mockReturnValueOnce(
    new Promise((r) => {
      finish = r
    }),
  )
  const pending = store.loadRuntime()
  store.stopRuntimePolling()
  finish({})
  await pending
  expect(store.runtimeFor('apps/web').containers).toEqual([container])
})
