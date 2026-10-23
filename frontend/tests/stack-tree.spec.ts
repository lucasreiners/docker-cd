import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import StackTree from '../src/components/StackTree.vue'
import type { ContainerInfo, StackRecord } from '../src/services/api'
import type { RuntimeView } from '../src/store/console'

const stack = (path = 'apps/web', extra: Partial<StackRecord> = {}): StackRecord => ({
  path,
  composeFile: 'compose.yaml',
  composeHash: 'hash',
  status: 'synced',
  ...extra,
})
const container = (id: string, extra: Partial<ContainerInfo> = {}): ContainerInfo => ({
  id,
  name: id,
  service: id,
  image: 'nginx:stable',
  state: 'running',
  health: 'healthy',
  ...extra,
})
const runtime = (
  containers: ContainerInfo[] = [],
  extra: Partial<RuntimeView> = {},
): RuntimeView => ({
  containers,
  loading: false,
  error: null,
  observedAt: '2026-10-23T12:00:00Z',
  stale: false,
  ...extra,
})
const wrappers: VueWrapper[] = []
beforeEach(() => {
  sessionStorage.clear()
  vi.useFakeTimers()
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.useRealTimers()
  vi.restoreAllMocks()
})
async function render(
  stacks = [stack()],
  observations: Record<string, RuntimeView> = {
    'apps/web': runtime([container('web-1'), container('proxy-1')]),
  },
  url = '/stacks',
  slots: Record<string, string> = {},
) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/stacks', component: { template: '<div />' } },
      { path: '/stacks/:path+', name: 'stack-detail', component: { template: '<div />' } },
    ],
  })
  await router.push(url)
  await router.isReady()
  const data = reactive(observations)
  const wrapper = mount(StackTree, {
    attachTo: document.body,
    props: {
      stacks,
      runtimeFor: (path) => data[path] ?? runtime([], { observedAt: null, stale: true }),
    },
    global: { plugins: [router] },
    slots,
  })
  wrappers.push(wrapper)
  await flushPromises()
  return { wrapper, data, router }
}
function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text().startsWith(text))
  if (!found) throw new Error(`Button missing: ${text}`)
  return found
}
async function search(wrapper: VueWrapper, value: string) {
  await wrapper.get('input[type="search"]').setValue(value)
  await vi.advanceTimersByTimeAsync(200)
  await flushPromises()
}
const ids = (wrapper: VueWrapper) =>
  wrapper.findAll('[data-container]').map((c) => c.attributes('data-container'))

describe('bounded operational inventory', () => {
  it('uses one native table with shared column headers and semantic stack rowgroups', async () => {
    const { wrapper } = await render()
    expect(wrapper.findAll('table')).toHaveLength(1)
    expect(wrapper.findAll('thead th[scope="col"]').map((cell) => cell.text())).toEqual([
      'Resource',
      'State',
      'Image',
      'Digest',
      'Actions',
    ])
    expect(wrapper.get('[data-stack]').element.tagName).toBe('TBODY')
    expect(wrapper.get('[data-stack] th[scope="row"]').text()).toBe('apps/web')
    for (const row of wrapper.findAll('[data-container]')) {
      expect(row.element.tagName).toBe('TR')
      expect(row.findAll('td')).toHaveLength(5)
      expect(row.find('.lucide-box').exists()).toBe(true)
    }
    expect(wrapper.find('[role="grid"], [role="treegrid"]').exists()).toBe(false)
    expect(wrapper.get('.inventory-stack-header .lucide-folder').attributes('aria-hidden')).toBe(
      'true',
    )
  })
  it('pairs deployment and runtime text with distinct status icons without claiming health from applied', async () => {
    const { wrapper } = await render(
      [
        stack('applied'),
        stack('approval', { requiresAcknowledgement: true }),
        stack('active', { status: 'syncing' }),
        stack('failure', { status: 'failed' }),
      ],
      {
        applied: runtime([
          container('bad', { health: 'unhealthy' }),
          container('stopped', { state: 'exited', health: 'none' }),
        ]),
      },
    )
    expect(
      wrapper
        .get('[data-stack="applied"] .inventory-stack-header .inventory-status--success')
        .text(),
    ).toBe('Applied')
    expect(wrapper.get('[data-container="bad"] .inventory-status--danger').text()).toBe(
      'Running · unhealthy',
    )
    expect(wrapper.get('[data-container="stopped"] .inventory-status--neutral').text()).toBe(
      'Exited · no health check',
    )
    expect(wrapper.get('[data-stack="approval"] .lucide-triangle-alert').exists()).toBe(true)
    expect(wrapper.get('[data-stack="active"] .inventory-spin').exists()).toBe(true)
    expect(wrapper.get('[data-stack="failure"] .inventory-status--danger').text()).toBe('Failed')
    for (const status of wrapper.findAll('.inventory-status'))
      expect(status.find('svg[aria-hidden="true"]').exists()).toBe(true)
  })
  it('shows two real children by default in exception-first order and keeps deployment independent', async () => {
    const { wrapper } = await render([stack('apps/web', { status: 'failed' })], {
      'apps/web': runtime([
        container('good'),
        container('starting', { health: 'starting' }),
        container('exited', { state: 'exited' }),
      ]),
    })
    expect(ids(wrapper)).toEqual(['exited', 'starting'])
    expect(wrapper.get('.inventory-state [aria-label="Deployment: Failed"]').text()).toBe('Failed')
    expect(wrapper.text()).toContain('2 / 3 running')
    expect(wrapper.get('button[aria-controls]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(1) // Display preference only, never resource selection.
  })
  it('does not interpret failed deployment or no health check as unhealthy runtime', async () => {
    const { wrapper } = await render(
      [stack('apps/web', { status: 'failed' }), stack('tools/neutral')],
      {
        'apps/web': runtime([container('healthy')]),
        'tools/neutral': runtime([container('neutral', { health: 'none' })]),
      },
    )
    await wrapper.get('.inventory-toolbar > label select').setValue('attention')
    await flushPromises()
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual([
      'apps/web',
    ])
    expect(wrapper.get('.inventory-state').attributes('title')).toContain('Deployment failed')
    expect(wrapper.text()).toContain('Running · healthy')
  })
  it('distinguishes empty observation, missing deployment and unavailable runtime', async () => {
    const { wrapper } = await render(
      [stack('empty'), stack('missing', { status: 'missing' }), stack('unknown')],
      {
        empty: runtime(),
        missing: runtime(),
        unknown: runtime([], { observedAt: null, error: 'Unavailable', stale: true }),
      },
    )
    expect(wrapper.get('[data-stack="empty"]').text()).toContain('No containers observed')
    expect(wrapper.get('[data-stack="missing"]').text()).toContain('Not deployed yet')
    expect(wrapper.get('[data-stack="unknown"]').text()).toContain('Runtime unavailable')
    expect(wrapper.get('[data-stack="unknown"]').text()).not.toContain('0 / 0 running')
    expect(wrapper.get('[data-stack="unknown"]').text()).not.toContain('No containers observed')
  })
  it('freezes preview order and focus through health changes, appending new containers', async () => {
    const { wrapper, data } = await render([stack()], {
      'apps/web': runtime([container('b', { state: 'exited' }), container('a'), container('c')]),
    })
    const focused = wrapper.get('[data-container="b"] button').element as HTMLButtonElement
    focused.focus()
    data['apps/web'] = runtime([
      container('a', { state: 'exited' }),
      container('b'),
      container('c'),
      container('new', { state: 'dead' }),
    ])
    await nextTick()
    expect(ids(wrapper)).toEqual(['b', 'a'])
    expect(document.activeElement).toBe(focused)
    await button(wrapper, 'Show all').trigger('click')
    expect(ids(wrapper)).toEqual(['b', 'a', 'c', 'new'])
  })
  it('recovers focus to the owning disclosure before a focused container vanishes', async () => {
    const { wrapper, data } = await render()
    ;(wrapper.get('[data-container="web-1"] button').element as HTMLButtonElement).focus()
    data['apps/web'] = runtime([container('proxy-1')])
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('button[aria-controls]').element)
  })
  it('restores full mode after hide/reopen and reveals in 20-child increments', async () => {
    const { wrapper } = await render([stack()], {
      'apps/web': runtime(
        Array.from({ length: 250 }, (_, i) => container(`c${String(i).padStart(3, '0')}`)),
      ),
    })
    expect(ids(wrapper)).toHaveLength(2)
    await button(wrapper, 'Show all 250').trigger('click')
    expect(ids(wrapper)).toHaveLength(20)
    await button(wrapper, 'Show 20 more').trigger('click')
    expect(ids(wrapper)).toHaveLength(40)
    await wrapper.get('button[aria-controls]').trigger('click')
    expect(ids(wrapper)).toHaveLength(0)
    await wrapper.get('button[aria-controls]').trigger('click')
    expect(ids(wrapper)).toHaveLength(40)
    await button(wrapper, 'Show fewer').trigger('click')
    expect(ids(wrapper)).toHaveLength(2)
    expect(document.activeElement).toBe(wrapper.get('button[aria-controls]').element)
  })
  it('bounds 250 stacks to 20 records and 40 children, with page-only disclosure', async () => {
    const stacks = Array.from({ length: 250 }, (_, i) =>
      stack(`group/${String(i).padStart(3, '0')}`),
    )
    const { wrapper } = await render(
      stacks.reverse(),
      Object.fromEntries(
        stacks.map((s) => [
          s.path,
          runtime([container('one'), container('two'), container('three')]),
        ]),
      ),
    )
    expect(wrapper.findAll('[data-stack]')).toHaveLength(20)
    expect(ids(wrapper)).toHaveLength(40)
    expect(wrapper.find('[data-stack]').attributes('data-stack')).toBe('group/000')
    await button(wrapper, 'Hide containers on this page').trigger('click')
    expect(ids(wrapper)).toHaveLength(0)
    await button(wrapper, 'Next').trigger('click')
    await flushPromises()
    expect(ids(wrapper)).toHaveLength(40)
    expect(wrapper.find('[data-stack]').attributes('data-stack')).toBe('group/020')
    await button(wrapper, 'Previous').trigger('click')
    await flushPromises()
    expect(ids(wrapper)).toHaveLength(0)
  })
  it('retains full-path duplicate leaves and accepts actions/work slots while collapsed', async () => {
    const { wrapper } = await render([stack('a/web'), stack('b/web')], {}, '/stacks', {
      actions: '<button>Stack action</button>',
      work: '<p>Applying Compose</p>',
    })
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual([
      'a/web',
      'b/web',
    ])
    expect(wrapper.get('[data-stack="a/web"] th[scope="row"] a').attributes('href')).toBe(
      '/stacks/a/web',
    )
    await button(wrapper, 'Hide containers on this page').trigger('click')
    expect(wrapper.findAll('button').filter((b) => b.text() === 'Stack action')).toHaveLength(2)
    expect(wrapper.text()).toContain('Applying Compose')
  })
})

describe('literal search and honest filters', () => {
  it('reveals matching children under a collapsed ancestor and restores collapse after clearing', async () => {
    const { wrapper } = await render([stack()], {
      'apps/web': runtime([
        container('needle-1'),
        container('needle-2'),
        container('needle-3'),
        container('other'),
      ]),
    })
    await wrapper.get('button[aria-controls]').trigger('click')
    await search(wrapper, 'NEEDLE')
    expect(ids(wrapper)).toEqual(['needle-1', 'needle-2'])
    expect(wrapper.text()).toContain('2 of 3 matching containers')
    await button(wrapper, 'Show matching containers').trigger('click')
    expect(ids(wrapper)).toHaveLength(3)
    await button(wrapper, 'Show all 4').trigger('click')
    expect(ids(wrapper)).toHaveLength(4)
    await search(wrapper, '')
    expect(ids(wrapper)).toHaveLength(0)
    expect(wrapper.get('button[aria-controls]').attributes('aria-expanded')).toBe('false')
  })
  it('searches full digests, service names and image references, not only truncated text', async () => {
    const c = container('actual', {
      service: 'unique-service',
      image: 'registry.test/team/nginx:v3',
      imageIdentity: {
        status: 'available',
        observedAt: '2026-10-23T12:00:00Z',
        source: 'container_image_inspect',
        reference: 'registry.test/team/nginx:v3',
        repositoryDigests: [
          {
            repository: 'registry.test/team/nginx',
            digest: `sha256:${'a'.repeat(64)}`,
            kind: 'unknown',
          },
        ],
        preferredRepositoryDigest: {
          repository: 'registry.test/team/nginx',
          digest: `sha256:${'a'.repeat(64)}`,
          kind: 'unknown',
        },
        freshness: 'not_checked',
      },
    })
    const { wrapper } = await render([stack()], { 'apps/web': runtime([c]) })
    for (const term of [
      'unique-service',
      'registry.test/team/nginx:v3',
      `sha256:${'a'.repeat(64)}`,
    ]) {
      await search(wrapper, term)
      expect(ids(wrapper)).toEqual(['actual'])
    }
    await search(wrapper, '[.*]')
    expect(wrapper.findAll('[data-stack]')).toHaveLength(0)
  })
  it('stack path matches keep normal previews rather than matching-only display', async () => {
    const { wrapper } = await render()
    await search(wrapper, 'apps/web')
    expect(ids(wrapper)).toHaveLength(2)
    expect(wrapper.text()).not.toContain('matching containers')
    await wrapper.get('button[aria-controls]').trigger('click')
    await search(wrapper, 'apps')
    expect(ids(wrapper)).toHaveLength(0)
  })
  it('filters runtime independently, never matching unknown to healthy', async () => {
    const { wrapper } = await render(
      [stack('failed', { status: 'failed' }), stack('stale'), stack('neutral')],
      {
        failed: runtime([container('healthy')]),
        stale: runtime([container('old')], { stale: true }),
        neutral: runtime([container('no-check', { health: '' })]),
      },
    )
    const runtimeSelect = wrapper
      .findAll('select')
      .find((s) => s.text().includes('No health checks'))
    if (!runtimeSelect) throw new Error('Runtime filter missing')
    await runtimeSelect.setValue('healthy')
    await flushPromises()
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual([
      'failed',
    ])
    await runtimeSelect.setValue('unknown')
    await flushPromises()
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual([
      'stale',
    ])
    await runtimeSelect.setValue('no-checks')
    await flushPromises()
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual([
      'neutral',
    ])
  })
  it('active filtering uses supplied authoritative stack membership only', async () => {
    const { wrapper } = await render([stack('one'), stack('two')])
    await wrapper.setProps({ activePaths: ['two'] })
    await wrapper.get('.inventory-toolbar > label select').setValue('active')
    await flushPromises()
    expect(wrapper.findAll('[data-stack]').map((r) => r.attributes('data-stack'))).toEqual(['two'])
  })
  it('preserves scoped logs and safe published-port links without child mutations', async () => {
    const { wrapper } = await render([stack()], {
      'apps/web': runtime([
        container('web', { ports: '8080:80/tcp, 8443:443/tcp' }),
        container('db', { ports: '5432/tcp' }),
      ]),
    })
    const link = wrapper.get('[data-container="web"] a[target="_blank"]')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(link.attributes('href')).toContain(':8080')
    expect(wrapper.get('[data-container="web"] a').attributes('href')).toContain(
      'tab=logs&container=web',
    )
    expect(wrapper.find('[data-container="db"] a[target="_blank"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-container] button').map((b) => b.text())).toEqual(['db', 'web'])
  })
})

describe('URL and session context', () => {
  it('restores keyboard focus to the stack navigation link on return', async () => {
    const { wrapper, router } = await render()
    const link = wrapper.get('[data-inventory-focus="stack-link:apps/web"]')
      .element as HTMLAnchorElement
    link.focus()
    expect(document.activeElement).toBe(link)
    await router.push('/stacks/apps/web')
    await flushPromises()
    const saved = Object.keys(sessionStorage).find((key) => key.startsWith('docker-cd-inventory:'))
    expect(saved).toBeDefined()
    expect(JSON.parse(sessionStorage.getItem(saved || '') || '{}').focus).toBe(
      'stack-link:apps/web',
    )
    wrapper.unmount()
    expect(JSON.parse(sessionStorage.getItem(saved || '') || '{}').focus).toBe(
      'stack-link:apps/web',
    )
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    const returned = await render()
    expect(document.activeElement).toBe(
      returned.wrapper.get('[data-inventory-focus="stack-link:apps/web"]').element,
    )
  })
  it('restores a dated visit scroll anchor and accepts prototype-like full stack paths', async () => {
    const scroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(640)
    const { wrapper, router } = await render([stack('constructor')], {
      constructor: runtime([container('real')]),
    })
    await wrapper.get('button[aria-controls]').trigger('click')
    await router.push('/stacks?q=elsewhere')
    await flushPromises()
    router.back()
    await flushPromises()
    expect(wrapper.get('[data-stack]').attributes('data-stack')).toBe('constructor')
    expect(ids(wrapper)).toHaveLength(0)
    expect(scroll).toHaveBeenCalledWith({ top: 640, behavior: 'instant' })
  })
  it('normalizes legacy filters and restores page/disclosure on Browser Back', async () => {
    const stacks = Array.from({ length: 42 }, (_, i) => stack(`s${String(i).padStart(2, '0')}`))
    const { wrapper, router } = await render(
      stacks,
      Object.fromEntries(stacks.map((s) => [s.path, runtime([container('actual')])])),
      '/stacks?page=2',
    )
    expect(wrapper.get('[data-stack]').attributes('data-stack')).toBe('s20')
    await button(wrapper, 'Hide containers on this page').trigger('click')
    await button(wrapper, 'Next').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.page).toBe('3')
    router.back()
    await flushPromises()
    expect(wrapper.get('[data-stack]').attributes('data-stack')).toBe('s20')
    expect(ids(wrapper)).toHaveLength(0)
    await router.push('/stacks?status=unchecked')
    await flushPromises()
    expect(
      wrapper
        .findAll('select')
        .some((select) => (select.element as HTMLSelectElement).value === 'no-checks'),
    ).toBe(true)
  })
  it('resets page for search/filter changes and restores focus and frozen order on return', async () => {
    const { wrapper, router, data } = await render([stack()], {
      'apps/web': runtime([container('b', { state: 'dead' }), container('a')]),
    })
    const focused = wrapper.get('[data-inventory-focus="disclosure:apps/web"]')
      .element as HTMLButtonElement
    focused.focus()
    await button(wrapper, 'Show container previews on this page').trigger('click')
    await router.push('/stacks?q=other&page=2')
    await flushPromises()
    data['apps/web'] = runtime([container('a', { state: 'dead' }), container('b')])
    router.back()
    await flushPromises()
    expect(ids(wrapper)).toEqual(['b', 'a'])
    expect(document.activeElement).toBe(
      wrapper.get('[data-inventory-focus="disclosure:apps/web"]').element,
    )
    await search(wrapper, 'apps')
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })
})
