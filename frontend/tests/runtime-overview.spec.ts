import { flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useRuntimeOverview } from '../src/composables/useRuntimeOverview'
import { type ContainerInfo, fetchContainers, type StackRecord } from '../src/services/api'
import { needsAttention, runtimeHealth } from '../src/utils/stackState'

vi.mock('../src/services/api', () => ({ fetchContainers: vi.fn() }))
afterEach(() => vi.clearAllMocks())
const stack: StackRecord = {
  path: 'apps/web',
  status: 'synced',
  composeFile: 'compose.yaml',
  composeHash: 'h1',
  containersRunning: 1,
  containersTotal: 1,
}
const container: ContainerInfo = {
  id: 'new',
  name: 'web',
  service: 'web',
  image: 'nginx',
  state: 'running',
  health: 'healthy',
}

it('invalidates only changed stacks and ignores inspection responses after redeployment or removal', async () => {
  let resolveOld!: (value: ContainerInfo[]) => void
  vi.mocked(fetchContainers).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveOld = resolve
    }),
  )
  const records = ref([stack])
  const scope = effectScope()
  const runtime = scope.run(() => useRuntimeOverview(() => records.value))
  if (!runtime) throw new Error('Runtime scope did not start')
  vi.mocked(fetchContainers).mockResolvedValue([container])
  records.value = [{ ...stack, syncedRevision: 'new' }]
  await flushPromises()
  expect(runtime.value.get(stack.path)?.containers[0].id).toBe('new')
  resolveOld([{ ...container, id: 'stale' }])
  await flushPromises()
  expect(runtime.value.get(stack.path)?.containers[0].id).toBe('new')
  records.value = [...records.value]
  await nextTick()
  expect(fetchContainers).toHaveBeenCalledTimes(2)
  records.value = []
  await nextTick()
  expect(runtime.value.has(stack.path)).toBe(false)
  scope.stop()
})

it('finds unhealthy in-sync stacks without treating running counts as a health check', () => {
  const state = { containers: [{ ...container, health: 'unhealthy' }], loading: false, error: null }
  expect(needsAttention(stack, state)).toBe(true)
  expect(runtimeHealth(state)).toEqual({ label: '1 unhealthy', tone: 'text-error' })
  expect(runtimeHealth({ ...state, containers: [{ ...container, health: 'none' }] }).label).toBe(
    'No health checks',
  )
  expect(runtimeHealth({ ...state, containers: [{ ...container, state: 'exited' }] }).label).toBe(
    '1 not running',
  )
  expect(runtimeHealth({ ...state, error: 'Docker unavailable' }).label).toBe('Runtime unavailable')
  expect(needsAttention(stack, { ...state, error: 'Docker unavailable', containers: [] })).toBe(
    true,
  )
})
