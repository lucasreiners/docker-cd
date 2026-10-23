import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { fixtureData, operationalWorkspaceFixtures } from '../../tests/e2e/fixtures/frontend-data'
import ActiveWork from '../src/components/ActiveWork.vue'
import ContainerLogs from '../src/components/ContainerLogs.vue'
import DeploymentProgress from '../src/components/DeploymentProgress.vue'
import OperationDetails from '../src/components/OperationDetails.vue'
import StackWork from '../src/components/StackWork.vue'
import OperationsPage from '../src/pages/OperationsPage.vue'
import StacksGrid from '../src/pages/StacksGrid.vue'
import appRouter from '../src/router'
import { useConsoleStore } from '../src/store/console'
import { useStacksStore } from '../src/store/stacks'
import { useWorkStore } from '../src/store/work'

const fetchMock = vi.fn<typeof fetch>()
const wrappers: VueWrapper[] = []
const stubs = {
  Dialog: {
    props: ['visible', 'header'],
    template:
      '<section v-if="visible" role="dialog" :aria-label="header"><slot/><slot name="footer"/></section>',
  },
  Drawer: { props: ['visible'], template: '<section v-if="visible"><slot/></section>' },
}
beforeEach(() => {
  setActivePinia(createPinia())
  sessionStorage.clear()
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockReset()
  fetchMock.mockImplementation(async () => Response.json({ records: [], nextCursor: '' }))
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  useWorkStore().$dispose()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})
async function render(component: object, props = {}, url = '/stacks') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/stacks' },
      { path: '/stacks', name: 'stacks', component: StacksGrid },
      { path: '/stacks/:path+', name: 'stack-detail', component: { template: '<div/>' } },
      { path: '/operations/:id', component: { template: '<div/>' } },
      { path: '/operations', component: OperationsPage },
      { path: '/repository', component: { template: '<div/>' } },
      { path: '/system', component: { template: '<div/>' } },
    ],
  })
  await router.push(url)
  const wrapper = mount(component, {
    props,
    attachTo: document.body,
    global: { plugins: [router], stubs },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}
function boot() {
  const work = useWorkStore(),
    fixtures = operationalWorkspaceFixtures()
  work.setCapability(2)
  work.connectionChanged('connected')
  work.snapshot(fixtures.snapshot)
  return { work, fixtures }
}
function button(wrapper: VueWrapper, label: string) {
  const node = wrapper.findAll('button').find((node) => node.text().startsWith(label))
  if (!node) throw new Error(`Missing button: ${label}`)
  return node
}

describe('operational workspace composition', () => {
  it('uses compact mode only for global work and keeps verbose messages behind explicit disclosure', async () => {
    const { fixtures } = boot()
    const { wrapper } = await render(ActiveWork)
    const progress = wrapper.findComponent(DeploymentProgress)
    expect(progress.props('compact')).toBe(true)
    expect(progress.get('[role="status"]').classes()).toContain('inventory-sr-only')
    expect(wrapper.findAll('.workflow-summary p')).toHaveLength(0)
    expect(progress.text()).not.toContain('Pulling imagePulling image')
    expect(progress.findAll('p').some((node) => node.text() === 'Pulling image')).toBe(false)
    const details = progress.get('details')
    ;(details.element as HTMLDetailsElement).open = true
    await details.trigger('toggle')
    expect(progress.findAll('p').some((node) => node.text() === 'Pulling image')).toBe(true)
    const inline = await render(DeploymentProgress, { run: fixtures.snapshot.runs[0] })
    expect(inline.wrapper.get('[role="status"]').classes()).not.toContain('inventory-sr-only')
    expect(inline.wrapper.text()).toContain('Pulling image')
  })

  it('keeps global failure messages and result access visible beyond the success feedback timer', async () => {
    vi.useFakeTimers()
    const { work, fixtures } = boot()
    const failed = {
      ...fixtures.snapshot.operations[2],
      status: 'failed',
      message: 'Prune denied; earlier outcomes retained',
      finishedAt: new Date().toISOString(),
      steps: [],
    }
    work.onHistory(failed)
    work.connectionChanged('reconnecting')
    work.connectionChanged('connected')
    work.snapshot(fixtures.reconnect)
    const { wrapper } = await render(ActiveWork)
    await vi.advanceTimersByTimeAsync(70000)
    expect(wrapper.text()).toContain('Last result')
    expect(wrapper.text()).toContain('Prune denied; earlier outcomes retained')
    expect(button(wrapper, 'Result details').exists()).toBe(true)
  })

  it('redirects the real root and legacy nested routes without losing query-backed return context', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: appRouter.options.routes,
    })
    await router.push('/?q=monitoring&view=grid&page=2')
    expect(router.currentRoute.value.path).toBe('/stacks')
    expect(router.currentRoute.value.query).toEqual({ q: 'monitoring', view: 'grid', page: '2' })
    await router.push('/stack/apps/a%20b/nested?q=search&tab=logs&container=two')
    expect(router.currentRoute.value.name).toBe('stack-detail')
    expect(router.currentRoute.value.params.path).toEqual(['apps', 'a b', 'nested'])
    expect(router.currentRoute.value.query).toEqual({ q: 'search', tab: 'logs', container: 'two' })
  })
  it('integrates real child previews, global actions above search, old URL normalization and pinned inspector data', async () => {
    const data = fixtureData(),
      stacks = useStacksStore(),
      consoleStore = useConsoleStore()
    stacks.loading = false
    stacks.stackMap = new Map(data.stacks.map((stack) => [stack.path, stack]))
    consoleStore.instance = data.instance
    for (const [path, containers] of Object.entries(data.containers))
      consoleStore.runtime[path] = {
        containers,
        loading: false,
        error: null,
        observedAt: new Date().toISOString(),
      }
    boot()
    const { wrapper, router } = await render(StacksGrid, {}, '/stacks?view=grid&status=unchecked')
    expect(wrapper.find('.stack-grid').exists()).toBe(false)
    expect(wrapper.findAll('[data-container]').length).toBeGreaterThan(0)
    expect(router.currentRoute.value.query.view).toBeUndefined()
    expect(wrapper.html().indexOf('Update all stacks…')).toBeLessThan(
      wrapper.html().indexOf('inventory-search'),
    )
    expect(
      wrapper.findAll('button').filter((node) => node.text() === 'Refresh runtime'),
    ).toHaveLength(1)
    const target = wrapper.find('[data-container] button')
    await target.trigger('click')
    expect(consoleStore.inspectedContainer?.container.id).toBe(
      target.element.closest('[data-container]')?.getAttribute('data-container'),
    )
    expect(wrapper.text()).toContain('Registry freshness')
    expect(wrapper.text()).toContain('Not checked')
  })

  it('counts root workflows once and keeps off-view targets visible regardless of inventory filters', async () => {
    const { work, fixtures } = boot()
    const { wrapper } = await render(ActiveWork, { visiblePaths: [] })
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(2)
    expect(wrapper.text()).toContain('Show all work (2)')
    expect(wrapper.text()).toContain('Outside current view')
    work.upsert(fixtures.bytes)
    await flushPromises()
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(2)
    for (const details of wrapper.findAll('details')) {
      ;(details.element as HTMLDetailsElement).open = true
      await details.trigger('toggle')
    }
    expect(wrapper.text()).toContain('256 B / unknown')
  })

  it('bounds all-work pages at ten and preserves every independent root', async () => {
    const { work, fixtures } = boot()
    const template = fixtures.snapshot.operations[0]
    if (!template) throw new Error('Missing operation fixture')
    work.liveOperations = Object.fromEntries(
      Array.from({ length: 23 }, (_, i) => {
        const id = `root-${i}`
        return [
          id,
          {
            ...template,
            id,
            parentId: undefined,
            rootOperationId: id,
            status: 'running',
            finishedAt: undefined,
          },
        ]
      }),
    )
    work.runs = {}
    const { wrapper } = await render(ActiveWork)
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(2)
    await button(wrapper, 'Show all work').trigger('click')
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(10)
    await button(wrapper, 'Next work').trigger('click')
    await button(wrapper, 'Next work').trigger('click')
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(3)
  })

  it('keeps an explicitly opened pull panel mounted when its run ends while the parent continues', async () => {
    const { work, fixtures } = boot()
    const { wrapper } = await render(ActiveWork)
    const runId = fixtures.terminal.run?.runId
    const progress = wrapper
      .findAllComponents(DeploymentProgress)
      .find((node) => node.props('run')?.runId === runId)
    if (!progress) throw new Error('Missing correlated run')
    const details = progress.get('details')
    ;(details.element as HTMLDetailsElement).open = true
    await details.trigger('toggle')
    work.upsert(fixtures.terminal)
    await flushPromises()
    const retained = wrapper
      .findAllComponents(DeploymentProgress)
      .find((node) => node.props('run')?.runId === runId)
    expect(retained?.get('details').element).toBe(details.element)
    expect((details.element as HTMLDetailsElement).open).toBe(true)
    expect(retained?.props('run')?.status).toBe('completed')
    expect(wrapper.text()).toContain('Show all work (2)')
  })

  it('invalidates old-epoch bytes without closing an open pull panel or inventing completion', async () => {
    const { work, fixtures } = boot()
    const { wrapper } = await render(ActiveWork)
    const details = wrapper.get('details')
    ;(details.element as HTMLDetailsElement).open = true
    await details.trigger('toggle')
    expect(wrapper.text()).toContain('128 B / unknown')
    work.connectionChanged('reconnecting')
    work.connectionChanged('connected')
    work.snapshot(fixtures.restart)
    await flushPromises()
    expect(wrapper.findAll('details').some((node) => node.element === details.element)).toBe(true)
    expect((details.element as HTMLDetailsElement).open).toBe(true)
    expect(wrapper.text()).toContain('Previous live details invalidated')
    expect(wrapper.text()).not.toContain('128 B / unknown')
    expect(wrapper.text()).not.toContain('Workflow completed')
  })

  it('shows only phase-local byte progress and never treats complete pulls as deployment success', async () => {
    const run = structuredClone(operationalWorkspaceFixtures().snapshot.runs[0])
    if (!run) throw new Error('Missing run fixture')
    run.phase = 'applying'
    run.images.forEach((image) => {
      image.done = true
      image.status = 'completed'
    })
    const { wrapper } = await render(DeploymentProgress, { run })
    ;(wrapper.get('details').element as HTMLDetailsElement).open = true
    await wrapper.get('details').trigger('toggle')
    expect(wrapper.text()).toContain('Applying Compose')
    expect(wrapper.text()).toContain('Images pulled — deployment may still be applying or pruning')
    expect(wrapper.text()).not.toContain('100%')
    const unknown = {
      ...run,
      images: run.images.map((image) => ({
        ...image,
        layers: image.layers.map((layer) => ({ ...layer, bytesTotal: null })),
      })),
    }
    await wrapper.setProps({ run: unknown })
    expect(wrapper.find('progress').exists()).toBe(false)
    expect(wrapper.text()).toContain('unknown')
  })

  it('retains failure and changes successful feedback after ten seconds without inferring outcomes', async () => {
    vi.useFakeTimers()
    const consoleStore = useConsoleStore(),
      work = useWorkStore()
    const op = {
      id: 'failed',
      kind: 'deployment',
      trigger: 'manual',
      stack: 'apps/web',
      status: 'failed',
      message: 'Registry denied',
      startedAt: new Date().toISOString(),
      finishedAt: new Date().toISOString(),
      steps: [],
    }
    consoleStore.onOperation(op)
    const { wrapper } = await render(StackWork, { stack: 'apps/web' })
    await vi.advanceTimersByTimeAsync(70000)
    expect(wrapper.text()).toContain('Deployment failed')
    expect(wrapper.text()).toContain('Registry denied')
    const success = {
      ...op,
      id: 'success',
      status: 'completed',
      startedAt: new Date().toISOString(),
      finishedAt: new Date().toISOString(),
      message: 'Applied',
    }
    consoleStore.onOperation(success)
    work.onHistory(success)
    await flushPromises()
    expect(wrapper.text()).not.toContain('Last result')
    await vi.advanceTimersByTimeAsync(10000)
    expect(wrapper.text()).toContain('Last result')
    expect(wrapper.text()).toContain('Runtime observation is independent')
  })

  it('uses explicit legacy stage fallback, not singleton bytes', async () => {
    const work = useWorkStore()
    work.setCapability(undefined)
    work.history = {
      root: {
        id: 'root',
        kind: 'action',
        trigger: 'manual',
        status: 'completed',
        startedAt: '2026-10-23T10:00:00Z',
        finishedAt: '2026-10-23T10:00:01Z',
        message: 'Accepted',
        steps: [],
      },
      child: {
        id: 'child',
        parentId: 'root',
        kind: 'deployment',
        trigger: 'action',
        stack: 'web',
        status: 'running',
        startedAt: '2026-10-23T10:00:01Z',
        message: 'Applying Compose',
        steps: [],
      },
    }
    const { wrapper } = await render(ActiveWork)
    expect(wrapper.findAll('.workflow-list > li')).toHaveLength(1)
    expect(wrapper.text()).toContain('Applying Compose')
    expect(wrapper.text()).toContain('Detailed live progress unavailable')
    expect(wrapper.find('progress').exists()).toBe(false)
  })
})

describe('investigation links and complete history scope', () => {
  it('deduplicates drawer lookup and keeps an individual expired record separate from history health', async () => {
    let release: ((response: Response) => void) | undefined
    fetchMock.mockImplementation(async (input) =>
      String(input).endsWith('/operations/expired')
        ? new Promise<Response>((resolve) => {
            release = resolve
          })
        : Response.json({ records: [] }),
    )
    const consoleStore = useConsoleStore()
    const pending = consoleStore.inspect('expired')
    const { wrapper } = await render(OperationDetails, { id: 'expired' })
    expect(
      fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/operations/expired')),
    ).toHaveLength(1)
    release?.(Response.json({ error: 'Operation not found' }, { status: 404 }))
    await pending
    await flushPromises()
    expect(wrapper.text()).toContain('Operation not found or expired')
    expect(consoleStore.operationsError).toBe('')
  })
  it('loads direct operations independently of latest fifty and pages related children by parent', async () => {
    const op = {
      id: 'old',
      kind: 'action',
      trigger: 'manual',
      status: 'completed',
      startedAt: '2026-10-23T10:00:00Z',
      finishedAt: '2026-10-23T10:00:01Z',
      message: 'Old result',
      steps: [],
    }
    fetchMock.mockImplementation(async (input) =>
      String(input).includes('/api/operations/old')
        ? Response.json(op)
        : Response.json({
            records: [{ ...op, id: 'child', parentId: 'old' }],
            nextCursor: String(input).includes('cursor=') ? '' : 'second',
          }),
    )
    const { wrapper } = await render(OperationDetails, { id: 'old' })
    expect(wrapper.text()).toContain('Old result')
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('parentId=old'))).toBe(
      true,
    )
    await button(wrapper, 'Load more related').trigger('click')
    await flushPromises()
    expect(
      fetchMock.mock.calls.some(
        ([input]) =>
          String(input).includes('parentId=old') && String(input).includes('cursor=second'),
      ),
    ).toBe(true)
    expect(wrapper.text()).toContain('All retained direct children loaded')
  })

  it('queries server filters and cursors rather than filtering the latest page', async () => {
    fetchMock.mockImplementation(async (input) =>
      Response.json({ records: [], nextCursor: String(input).includes('cursor=') ? '' : 'older' }),
    )
    const { wrapper, router } = await render(
      OperationsPage,
      {},
      '/operations?kind=deployment&stack=old%2Fstack',
    )
    expect(
      fetchMock.mock.calls.some(
        ([input]) =>
          String(input).includes('kind=deployment') && String(input).includes('stack=old%2Fstack'),
      ),
    ).toBe(true)
    await button(wrapper, 'Load older operations').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('cursor=older'))).toBe(
      true,
    )
    await router.push('/operations?status=running')
    await flushPromises()
    expect(String(fetchMock.mock.calls.at(-1)?.[0])).toContain('status=running')
    expect(String(fetchMock.mock.calls.at(-1)?.[0])).not.toContain('cursor=')
  })

  it.each(['success', 'error'])(
    'ignores an older same-filter history %s after a newer refresh',
    async (outcome) => {
      const responses: Array<(response: Response) => void> = []
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => responses.push(resolve)),
      )
      const { wrapper } = await render(OperationsPage, {}, '/operations')
      const page = wrapper.vm as unknown as { loadHistory(append: boolean): Promise<void> }
      const newer = page.loadHistory(false)
      const operation = {
        id: 'newer-operation',
        kind: 'deployment',
        trigger: 'manual',
        status: 'completed',
        startedAt: '2026-10-23T10:00:00Z',
        finishedAt: '2026-10-23T10:00:01Z',
        message: 'New result',
        steps: [],
      }
      responses[1]?.(Response.json({ records: [operation], nextCursor: 'newer-cursor' }))
      await newer
      await flushPromises()
      responses[0]?.(
        outcome === 'success'
          ? Response.json({
              records: [{ ...operation, id: 'obsolete-operation' }],
              nextCursor: 'obsolete-cursor',
            })
          : Response.json({ error: 'Obsolete failure' }, { status: 503 }),
      )
      await flushPromises()
      expect(wrapper.find('[data-operation-id="newer-operation"]').exists()).toBe(true)
      expect(wrapper.find('[data-operation-id="obsolete-operation"]').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('Obsolete failure')
      expect(button(wrapper, 'Refresh history').attributes('disabled')).toBeUndefined()
      const next = page.loadHistory(true)
      expect(String(fetchMock.mock.calls.at(-1)?.[0])).toContain('cursor=newer-cursor')
      responses[2]?.(Response.json({ records: [], nextCursor: '' }))
      await next
    },
  )

  it('keeps history loading while the latest same-filter request is still pending', async () => {
    const responses: Array<(response: Response) => void> = []
    fetchMock.mockImplementation(() => new Promise<Response>((resolve) => responses.push(resolve)))
    const { wrapper } = await render(OperationsPage, {}, '/operations')
    const page = wrapper.vm as unknown as { loadHistory(append: boolean): Promise<void> }
    const newer = page.loadHistory(false)
    responses[0]?.(Response.json({ records: [], nextCursor: 'obsolete-cursor' }))
    await flushPromises()
    expect(button(wrapper, 'Refresh history').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Loading history…')
    responses[1]?.(Response.json({ records: [], nextCursor: '' }))
    await newer
    await flushPromises()
    expect(button(wrapper, 'Refresh history').attributes('disabled')).toBeUndefined()
  })

  it('selects the exact scoped log query and does not switch when its container disappears', async () => {
    const containers = ['one', 'two'].map((id) => ({
      id,
      name: id,
      service: id,
      image: 'nginx',
      state: 'running',
      health: 'none',
    }))
    fetchMock.mockImplementation(async () => new Response('selected output'))
    const { wrapper } = await render(
      ContainerLogs,
      { stackPath: 'nested/web', containers },
      '/stacks/nested/web?tab=logs&container=two',
    )
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('stack=nested%2Fweb&container=two')
    expect(wrapper.get('select').element.value).toBe('two')
    const requests = fetchMock.mock.calls.length
    await wrapper.setProps({ containers: [containers[0]] })
    expect(wrapper.get('select').element.value).toBe('two')
    expect(wrapper.text()).toContain('choose another container explicitly')
    expect(fetchMock.mock.calls).toHaveLength(requests)
    expect(button(wrapper, 'Follow logs').attributes('disabled')).toBeDefined()
  })
})
