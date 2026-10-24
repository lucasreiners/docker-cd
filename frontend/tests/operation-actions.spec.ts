import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fixtureData, operationalWorkspaceFixtures } from '../../tests/e2e/fixtures/frontend-data'
import StackActions from '../src/components/StackActions.vue'
import WorkspaceActions from '../src/components/WorkspaceActions.vue'
import { useOperationActions } from '../src/composables/useOperationActions'
import type { StackRecord } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'
import { useStacksStore } from '../src/store/stacks'
import { useWorkStore } from '../src/store/work'

const fetchMock = vi.fn<typeof fetch>()
const stack = {
  path: 'nested/web',
  status: 'synced',
  composeFile: 'compose.yaml',
  composeHash: 'hash',
  desiredRevision: 'reviewed-full-revision',
} satisfies StackRecord
const operation = {
  id: 'accepted-id',
  kind: 'action',
  trigger: 'update',
  stack: stack.path,
  status: 'running',
  message: 'Waiting',
  startedAt: '2026-10-23T12:00:00Z',
  steps: [],
}
const stubs = {
  RouterLink: { props: ['to'], template: '<a><slot /></a>' },
  Dialog: {
    props: ['visible', 'header'],
    template:
      '<section v-if="visible" role="dialog" :aria-label="header"><slot /><slot name="footer" /></section>',
  },
}
let wrappers: VueWrapper[] = []
function render(component: typeof WorkspaceActions | typeof StackActions, props = {}) {
  const wrapper = mount(component, { props, attachTo: document.body, global: { stubs } })
  wrappers.push(wrapper)
  return wrapper
}
function button(wrapper: VueWrapper, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === label)
  if (!found) throw new Error(`Missing button: ${label}`)
  return found
}
const posts = () => fetchMock.mock.calls.filter(([, options]) => options?.method === 'POST')
beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const path = String(input)
    return new Response(
      JSON.stringify(
        path === '/api/operations'
          ? { operationId: operation.id }
          : path.endsWith(operation.id)
            ? operation
            : path === '/api/runtime'
              ? { stacks: {} }
              : { records: [] },
      ),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  })
  const consoleStore = useConsoleStore()
  consoleStore.instance = fixtureData().instance
  consoleStore.instance.capabilities.apply = true
  consoleStore.instance.capabilities.imageUpdates = true
  consoleStore.instance.policies.reconcile = true
  const stacks = useStacksStore()
  stacks.stackMap = new Map([[stack.path, { ...stack }]])
  stacks.refreshStatus = {
    revision: stack.desiredRevision,
    ref: 'main',
    refType: 'branch',
    refreshedAt: '',
    refreshStatus: 'completed',
    updatesBlocked: false,
  }
  const work = useWorkStore()
  work.supported = true
  work.connected = true
  work.restored = true
})
afterEach(() => {
  wrappers.forEach((w) => {
    w.unmount()
  })
  wrappers = []
  vi.unstubAllGlobals()
})

describe('workspace action scope and confirmation', () => {
  it('allows an interrupting Git check after the refresh parent completes while its child runs', async () => {
    const work = useWorkStore()
    const { snapshot } = operationalWorkspaceFixtures()
    work.snapshot(snapshot)
    const actions = useOperationActions()
    expect(actions.workspaceReason('refresh')).toBe('')
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    expect(wrapper.get('[role="dialog"]').attributes('aria-label')).toBe(
      'Interrupt work and check Git',
    )
    expect(wrapper.text()).toContain('Requests interruption; work may already have made changes')
    expect(posts()).toHaveLength(0)
    work.liveOperations = {
      ...work.liveOperations,
      'old-refresh': { ...snapshot.operations[0], status: 'running', finishedAt: undefined },
    }
    expect(actions.workspaceReason('refresh')).toBe('A Git check is already in progress')
  })
  it('always confirms all-stack host-wide prune; dismiss sends nothing and filters never enter POST', async () => {
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Update all stacks…').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain(
      'including images outside managed stacks',
    )
    expect(wrapper.text()).toContain('regardless of inventory filters')
    await button(wrapper, 'Cancel').trigger('click')
    expect(posts()).toHaveLength(0)
    await button(wrapper, 'Update all stacks…').trigger('click')
    await button(wrapper, 'Update all stacks and prune images').trigger('click')
    await flushPromises()
    expect(posts()).toEqual([['/api/update', { method: 'POST' }]])
    expect(wrapper.find('.request-warning').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Request accepted')
    expect(wrapper.text()).not.toContain('Clear local request notice')
    expect(button(wrapper, 'Update all stacks…').attributes('disabled')).toBeUndefined()
    // An unrelated server workflow must not be used to manufacture correlation.
    useConsoleStore().onOperation({ ...operation, id: 'unrelated', kind: 'image_update' })
    expect(useOperationActions().notice('workspace:update')?.operationId).toBeUndefined()
  })
  it('confirms idle Git policy inline, focuses confirmation, and cancel restores trigger', async () => {
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('May deploy/remove stacks according to policy')
    expect(document.activeElement).toBe(button(wrapper, 'Check Git and apply policy').element)
    await button(wrapper, 'Cancel').trigger('click')
    expect(posts()).toHaveLength(0)
    expect(document.activeElement).toBe(button(wrapper, 'Check Git for changes…').element)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    await button(wrapper, 'Check Git and apply policy').trigger('click')
    await flushPromises()
    expect(posts()).toEqual([['/api/refresh', { method: 'POST' }]])
  })
  it('names authoritative workflow roots, including completed roots with active children', async () => {
    const work = useWorkStore()
    work.liveOperations = {
      root: {
        ...operation,
        id: 'root',
        rootOperationId: 'root',
        kind: 'image_update',
        version: 1,
        status: 'completed',
        finishedAt: '2026-10-23T12:01:00Z',
      },
      child: { ...operation, id: 'child', rootOperationId: 'root', parentId: 'root', version: 1 },
    }
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('image_update · nested/web · root')
    expect(wrapper.findAll('li')).toHaveLength(1)
    expect(wrapper.text()).toContain('Requests interruption; work may already have made changes')
    await button(wrapper, 'Interrupt work and check Git').trigger('click')
    await flushPromises()
    expect(posts()).toEqual([['/api/refresh', { method: 'POST' }]])
  })
  it('upgrades an open idle check to interruption confirmation if work appears', async () => {
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    useWorkStore().liveOperations = {
      root: { ...operation, id: 'root', rootOperationId: 'root', version: 1 },
    }
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Check Git and apply policy')
    await button(wrapper, 'Cancel').trigger('click')
    expect(posts()).toHaveLength(0)
  })
  it('uses legacy active operations only for older agents', async () => {
    const work = useWorkStore()
    work.supported = false
    work.onHistory(operation)
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Check Git for changes…').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain(operation.id)
  })
  it('keeps runtime refresh read-only while disconnected and explains disabled capabilities', async () => {
    const wrapper = render(WorkspaceActions)
    useWorkStore().connected = false
    await flushPromises()
    expect(button(wrapper, 'Update all stacks…').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Disconnected from agent')
    await button(wrapper, 'Refresh runtime').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.map(([path]) => path)).toContain('/api/runtime')
    expect(posts()).toHaveLength(0)
    useWorkStore().connected = true
    const instance = useConsoleStore().instance
    if (!instance) throw new Error('Missing instance fixture')
    instance.capabilities.imageUpdates = false
    await flushPromises()
    expect(wrapper.text()).toContain('Image updates disabled by instance configuration')
  })
})

describe('workspace request outcomes', () => {
  it.each(['update', 'refresh'] as const)(
    'releases acknowledged %s bookkeeping without claiming completion; server work and reconnect still gate repeat requests',
    async (action) => {
      const actions = useOperationActions()
      const work = useWorkStore()
      const wrapper = render(WorkspaceActions)
      await actions.submitWorkspace(action)
      expect(actions.notice(`workspace:${action}`)).toBeUndefined()
      work.liveOperations = {
        unrelated: {
          ...operation,
          id: 'unrelated',
          kind: action === 'update' ? 'image_update' : 'refresh',
          rootOperationId: 'unrelated',
          version: 1,
        },
      }
      expect(actions.workspaceReason(action)).not.toBe('')
      expect(actions.notice(`workspace:${action}`)).toBeUndefined()
      work.connectionChanged('reconnecting')
      wrapper.unmount()
      const navigated = render(WorkspaceActions)
      expect(actions.workspaceReason(action)).toBe('Disconnected from agent')
      work.connectionChanged('connected')
      expect(actions.workspaceReason(action)).toBe('Restoring authoritative work state')
      expect(
        work.snapshot({
          schemaVersion: 2,
          epoch: 'restored',
          sequence: 1,
          operations: [],
          runs: [],
          detailsTruncated: false,
        }),
      ).toBe(true)
      await flushPromises()
      expect(actions.workspaceReason(action)).toBe('')
      expect(navigated.find('.request-warning').exists()).toBe(false)
      await actions.submitWorkspace(action)
      expect(posts()).toHaveLength(2)
    },
  )
  it('blocks simultaneous global and stack POSTs across navigation until acknowledgement', async () => {
    let resolve!: (value: Response) => void
    fetchMock.mockReturnValueOnce(
      new Promise((r) => {
        resolve = r
      }),
    )
    const actions = useOperationActions()
    const first = actions.submitWorkspace('update')
    const wrapper = render(WorkspaceActions)
    expect(button(wrapper, 'Update all stacks…').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.request-warning').exists()).toBe(false)
    wrapper.unmount()
    render(WorkspaceActions)
    await actions.submitWorkspace('update')
    await actions.submitWorkspace('refresh')
    await actions.submitStack({
      action: 'update',
      stack: stack.path,
      revision: stack.desiredRevision,
    })
    expect(posts()).toHaveLength(1)
    resolve(new Response('{}'))
    await first
    expect(actions.workspaceReason('update')).toBe('')
    expect(actions.notice('workspace:update')).toBeUndefined()
  })
  it('rechecks readiness and capability when an already-open prune modal is confirmed', async () => {
    const wrapper = render(WorkspaceActions)
    await button(wrapper, 'Update all stacks…').trigger('click')
    useWorkStore().connected = false
    await flushPromises()
    expect(
      button(wrapper, 'Update all stacks and prune images').attributes('disabled'),
    ).toBeDefined()
    await button(wrapper, 'Update all stacks and prune images').trigger('click')
    expect(posts()).toHaveLength(0)
    useWorkStore().connected = true
    const instance = useConsoleStore().instance
    if (!instance) throw new Error('Missing instance fixture')
    instance.capabilities.imageUpdates = false
    await flushPromises()
    expect(
      button(wrapper, 'Update all stacks and prune images').attributes('disabled'),
    ).toBeDefined()
    await button(wrapper, 'Cancel').trigger('click')
    expect(posts()).toHaveLength(0)
  })
  it.each(['update', 'refresh'] as const)(
    'preserves global %s POST uncertainty without guessing a later operation or replaying',
    async (action) => {
      fetchMock.mockResolvedValueOnce(
        new Response(JSON.stringify({ error: 'Gateway timeout' }), { status: 502 }),
      )
      const actions = useOperationActions()
      await actions.submitWorkspace(action)
      await flushPromises()
      expect(actions.notice(`workspace:${action}`)?.phase).toBe('uncertain')
      useConsoleStore().onOperation({
        ...operation,
        id: 'not-correlated',
        kind: action === 'update' ? 'image_update' : 'refresh',
      })
      expect(actions.notice(`workspace:${action}`)?.operationId).toBeUndefined()
      await actions.submitWorkspace(action)
      expect(posts()).toHaveLength(1)
      expect(posts()[0]).toEqual([
        action === 'update' ? '/api/update' : '/api/refresh',
        { method: 'POST' },
      ])
      const wrapper = render(WorkspaceActions)
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('it may still be running')
      await button(wrapper, 'Dismiss').trigger('click')
      expect(actions.notice(`workspace:${action}`)).toBeUndefined()
      expect(posts()).toHaveLength(1)
    },
  )
  it('preserves an explicit rejected global response without reporting acceptance', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: 'Update cycle already running' }), { status: 409 }),
    )
    const actions = useOperationActions()
    await actions.submitWorkspace('update')
    expect(actions.notice('workspace:update')).toEqual({
      phase: 'failed',
      message: 'Update cycle already running',
    })
    expect(posts()).toHaveLength(1)
    const wrapper = render(WorkspaceActions)
    expect(wrapper.get('[role="alert"]').text()).toContain('Update cycle already running')
  })
})

describe('reviewed stack actions', () => {
  it.each(['update', 'retry'] as const)(
    'posts exact %s path and frozen revision, opens only returned operation',
    async (action) => {
      const record = {
        ...stack,
        status: action === 'retry' ? ('failed' as const) : ('synced' as const),
      }
      useStacksStore().stackMap.set(record.path, record)
      const wrapper = render(StackActions, { stack: record })
      await button(wrapper, action === 'retry' ? 'Retry deployment…' : 'Update images…').trigger(
        'click',
      )
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      expect(wrapper.text()).toContain('No image prune')
      await button(wrapper, `${action === 'retry' ? 'Retry' : 'Update'} ${stack.path}`).trigger(
        'click',
      )
      await flushPromises()
      expect(posts()).toHaveLength(1)
      expect(posts()[0]?.[0]).toBe('/api/operations')
      expect(JSON.parse(String(posts()[0]?.[1]?.body))).toEqual({
        action,
        stack: stack.path,
        revision: stack.desiredRevision,
      })
      expect(fetchMock.mock.calls.some(([url]) => url === '/api/operations/accepted-id')).toBe(true)
      expect(useOperationActions().notice(`stack:${stack.path}`)?.phase).toBe('accepted')
      expect(wrapper.find('.request-warning').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('Request accepted')
      expect(
        button(wrapper, action === 'retry' ? 'Retry deployment…' : 'Update images…').attributes(
          'disabled',
        ),
      ).toBeDefined()
      useConsoleStore().onOperation({
        ...operation,
        version: 2,
        steps: [
          { at: operation.startedAt, phase: 'executing', message: 'Executing stack deployment' },
        ],
      })
      await flushPromises()
      expect(useOperationActions().notice(`stack:${stack.path}`)?.phase).toBe('executing')
      expect(wrapper.find('.request-warning').exists()).toBe(false)
      useConsoleStore().onOperation({
        ...operation,
        version: 3,
        status: 'completed',
        finishedAt: '2026-10-23T12:01:00Z',
      })
      await flushPromises()
      expect(useOperationActions().notice(`stack:${stack.path}`)?.phase).toBe('completed')
      expect(wrapper.find('.request-warning').exists()).toBe(false)
      expect(
        button(wrapper, action === 'retry' ? 'Retry deployment…' : 'Update images…').attributes(
          'disabled',
        ),
      ).toBeUndefined()
    },
  )
  it('does not send on inline dismiss or silently adopt a newer revision while confirmation is open', async () => {
    const wrapper = render(StackActions, { stack })
    await button(wrapper, 'Update images…').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(posts()).toHaveLength(0)
    await button(wrapper, 'Update images…').trigger('click')
    const newer = { ...stack, desiredRevision: 'new-revision' }
    useStacksStore().stackMap.set(stack.path, newer)
    await wrapper.setProps({ stack: newer })
    expect(wrapper.text()).toContain('Desired revision changed; review the stack again')
    expect(button(wrapper, `Update ${stack.path}`).attributes('disabled')).toBeDefined()
    await button(wrapper, `Update ${stack.path}`).trigger('click')
    expect(posts()).toHaveLength(0)
  })
  it('requires loaded comparison, explicit approval and exact loaded revision; rejects drift', async () => {
    const record = { ...stack, requiresAcknowledgement: true }
    useStacksStore().stackMap.set(stack.path, record)
    const wrapper = render(StackActions, { stack: record })
    expect(wrapper.text()).toContain('Review change')
    expect(wrapper.text()).not.toContain('Update images…')
    expect(wrapper.text()).not.toContain('Approve & deploy')
    await wrapper.setProps({ reviewedRevision: stack.desiredRevision })
    await button(wrapper, `Approve & deploy ${stack.desiredRevision}`).trigger('click')
    await flushPromises()
    expect(JSON.parse(String(posts()[0]?.[1]?.body))).toEqual({
      action: 'acknowledge',
      stack: stack.path,
      revision: stack.desiredRevision,
    })
  })
  it('disables loaded approval after desired revision changes and never sends at the new revision', async () => {
    const record = { ...stack, requiresAcknowledgement: true }
    const wrapper = render(StackActions, { stack: record, reviewedRevision: stack.desiredRevision })
    const newer = { ...record, desiredRevision: 'new-revision' }
    useStacksStore().stackMap.set(stack.path, newer)
    await wrapper.setProps({ stack: newer })
    expect(
      button(wrapper, `Approve & deploy ${stack.desiredRevision}`).attributes('disabled'),
    ).toBeDefined()
    expect(wrapper.text()).toContain('Desired revision changed')
    expect(posts()).toHaveLength(0)
  })
  it('prevents duplicate submissions across surfaces while pending', async () => {
    let resolve!: (value: Response) => void
    fetchMock.mockReturnValueOnce(
      new Promise((r) => {
        resolve = r
      }),
    )
    const actions = useOperationActions()
    const body = { action: 'update' as const, stack: stack.path, revision: stack.desiredRevision }
    const first = actions.submitStack(body)
    await actions.submitStack(body)
    expect(posts()).toHaveLength(1)
    resolve(new Response(JSON.stringify({ operationId: 'accepted-id' })))
    await first
  })
  it('preserves a rejected approval as failed and requires source review again, never completed', async () => {
    const record = { ...stack, requiresAcknowledgement: true }
    useStacksStore().stackMap.set(stack.path, record)
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: 'Desired revision changed; review the stack again' }), {
        status: 409,
      }),
    )
    const wrapper = render(StackActions, { stack: record, reviewedRevision: stack.desiredRevision })
    await button(wrapper, `Approve & deploy ${stack.desiredRevision}`).trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Approval rejected; load the source comparison again')
    expect(wrapper.text()).not.toContain('completed')
    expect(posts()).toHaveLength(1)
    expect(
      button(wrapper, `Approve & deploy ${stack.desiredRevision}`).attributes('disabled'),
    ).toBeDefined()
  })
  it('retains authoritative under-lock rejection after acceptance, rather than completing or retrying the new revision', async () => {
    const actions = useOperationActions()
    await actions.submitStack({
      action: 'acknowledge',
      stack: stack.path,
      revision: stack.desiredRevision,
    })
    useConsoleStore().onOperation({
      ...operation,
      version: 2,
      status: 'failed',
      message: 'Desired revision changed; review the stack again',
      finishedAt: '2026-10-23T12:01:00Z',
    })
    expect(actions.notice(`stack:${stack.path}`)?.phase).toBe('failed')
    expect(actions.notice(`stack:${stack.path}`)?.message).toContain('Desired revision changed')
    expect(posts()).toHaveLength(1)
  })
  it('shows a late approval failure and keeps source-review protection after dismissing the warning', async () => {
    const record = { ...stack, requiresAcknowledgement: true }
    useStacksStore().stackMap.set(stack.path, record)
    const wrapper = render(StackActions, { stack: record, reviewedRevision: stack.desiredRevision })
    const label = `Approve & deploy ${stack.desiredRevision}`
    await button(wrapper, label).trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(button(wrapper, label).attributes('disabled')).toBeDefined()
    // Acknowledgement is not deployment success; only this exact ID can resolve it.
    useConsoleStore().onOperation({
      ...operation,
      version: 2,
      status: 'failed',
      message: 'Desired revision changed; review the stack again',
      finishedAt: '2026-10-23T12:01:00Z',
    })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Desired revision changed')
    expect(
      wrapper
        .get('[role="alert"]')
        .findAll('button')
        .map((control) => control.text()),
    ).toEqual(['Dismiss'])
    expect(wrapper.get('[role="alert"]').get('a').text()).toBe('View operations')
    expect(wrapper.text()).toContain('Approval rejected; load the source comparison again')
    await button(wrapper, 'Dismiss').trigger('click')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(button(wrapper, label).attributes('disabled')).toBeDefined()
    expect(posts()).toHaveLength(1)
  })
  it('does not replay POST after network uncertainty and only checks read-only history', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Network lost'))
    const actions = useOperationActions()
    const body = { action: 'retry' as const, stack: stack.path, revision: stack.desiredRevision }
    await actions.submitStack(body)
    await flushPromises()
    expect(actions.notice(`stack:${stack.path}`)?.phase).toBe('uncertain')
    expect(actions.notice(`stack:${stack.path}`)?.message).toContain(
      'Request outcome unknown; it may still be running',
    )
    await actions.submitStack(body)
    expect(posts()).toHaveLength(1)
    expect(
      fetchMock.mock.calls.some(
        ([url, init]) => String(url).startsWith('/api/operations?') && !init?.method,
      ),
    ).toBe(true)
    const wrapper = render(StackActions, { stack })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Review operations before trying again')
    useConsoleStore().onOperation({
      ...operation,
      id: 'unrelated',
      version: 2,
      status: 'completed',
      finishedAt: '2026-10-23T12:01:00Z',
    })
    await flushPromises()
    expect(actions.notice(`stack:${stack.path}`)?.phase).toBe('uncertain')
    expect(button(wrapper, 'Update images…').attributes('disabled')).toBeDefined()
    await button(wrapper, 'Dismiss').trigger('click')
    expect(actions.notice(`stack:${stack.path}`)).toBeUndefined()
    expect(button(wrapper, 'Update images…').attributes('disabled')).toBeUndefined()
    expect(posts()).toHaveLength(1)
  })
  it('guards disabled reconciliation, updates, blocked refresh and restoring state', async () => {
    const actions = useOperationActions()
    const instance = useConsoleStore().instance
    if (!instance) throw new Error('Missing instance fixture')
    instance.capabilities.apply = false
    expect(actions.stackReason(stack, 'retry')).toContain('Reconciliation disabled')
    instance.capabilities.apply = true
    instance.capabilities.imageUpdates = false
    expect(actions.stackReason(stack, 'update')).toContain('Image updates disabled')
    instance.capabilities.imageUpdates = true
    const refresh = useStacksStore().refreshStatus
    if (!refresh) throw new Error('Missing refresh fixture')
    refresh.updatesBlocked = true
    expect(actions.stackReason(stack, 'retry')).toContain('Repository refresh must complete')
    useWorkStore().restored = false
    expect(actions.stackReason(stack, 'retry')).toBe('Restoring authoritative work state')
    await actions.submitStack({
      action: 'retry',
      stack: stack.path,
      revision: stack.desiredRevision,
    })
    expect(posts()).toHaveLength(0)
  })
})
