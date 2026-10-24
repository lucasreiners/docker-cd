import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useRuntimeOverview } from '../src/composables/useRuntimeOverview'
import { useStackContainers } from '../src/composables/useStackContainers'
import { type ContainerInfo, requestJSON, type StackRecord } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'
import { needsAttention, runtimeHealth } from '../src/utils/stackState'

vi.mock('../src/services/api', () => ({ requestJSON: vi.fn() }))
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})
afterEach(() => useConsoleStore().$dispose())
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

it('shares a single global observation with detail, avoids per-stack fetches and removes absent scopes', async () => {
  vi.mocked(requestJSON).mockResolvedValue({
    [stack.path]: { containers: [container], error: null, observedAt: '2026-10-23T12:00:00Z' },
  })
  const records = ref([stack])
  const scope = effectScope()
  const runtime = scope.run(() => useRuntimeOverview(() => records.value))
  if (!runtime) throw new Error('Runtime scope did not start')
  const detail = scope.run(() => useStackContainers(() => records.value[0]))
  if (!detail) throw new Error('Detail scope did not start')
  await flushPromises()
  expect(detail.containers.value).toEqual(runtime.value.get(stack.path)?.containers)
  records.value = [{ ...stack, syncedRevision: 'new' }]
  await flushPromises()
  expect(runtime.value.get(stack.path)?.containers[0].id).toBe('new')
  records.value = [...records.value]
  await nextTick()
  expect(requestJSON).toHaveBeenCalledTimes(1)
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
