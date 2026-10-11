import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Operation } from '../src/services/api'
import { requestJSON } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'

vi.mock('../src/services/api', () => ({ requestJSON: vi.fn() }))
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})
const operation: Operation = {
  id: '1',
  version: 1,
  kind: 'deployment',
  trigger: 'manual',
  stack: 'apps/web',
  status: 'running',
  message: 'Deploying',
  startedAt: '2026-10-11T12:00:00Z',
  steps: [],
}
it('does not let a stale HTTP history response overwrite a newer terminal SSE event', async () => {
  let resolve!: (value: unknown) => void
  vi.mocked(requestJSON).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r
    }),
  )
  const store = useConsoleStore()
  store.onOperation(operation)
  const loading = store.loadOperations()
  store.onOperation({
    ...operation,
    version: 3,
    status: 'completed',
    finishedAt: '2026-10-11T12:01:00Z',
  })
  resolve({ records: [{ ...operation, version: 2 }], nextCursor: '', storageError: '' })
  await loading
  expect(store.operationMap['1'].status).toBe('completed')
  expect(store.active).toHaveLength(0)
})
it('retains independent instance and runtime errors without erasing history', async () => {
  vi.mocked(requestJSON).mockRejectedValue(new Error('unavailable'))
  const store = useConsoleStore()
  store.onOperation(operation)
  await store.reload()
  expect(store.instanceError).toBe('unavailable')
  expect(store.runtimeFor('apps/web').error).toBe('unavailable')
  expect(store.operations).toHaveLength(1)
})
