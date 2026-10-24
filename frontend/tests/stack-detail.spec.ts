import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useRoute } from 'vue-router'
import { fixtureData } from '../../tests/e2e/fixtures/frontend-data'
import StackDetail from '../src/pages/StackDetail.vue'
import type { StackRecord } from '../src/services/api'
import { requestJSON } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'
import { useStacksStore } from '../src/store/stacks'
import { useWorkStore } from '../src/store/work'

// Stub naive-ui components
const naiveStubs = {
  RouterLink: { template: '<a><slot /></a>', props: ['to'] },
  NCard: { template: '<div class="n-card"><slot /></div>' },
  NPageHeader: {
    template: '<div class="n-page-header"><slot name="title" /><slot name="extra" /><slot /></div>',
    props: ['onBack'],
  },
  NText: {
    template: '<span class="n-text"><slot /></span>',
    props: ['strong', 'depth', 'type', 'code'],
  },
  NAlert: { template: '<div class="n-alert"><slot /></div>', props: ['type', 'title'] },
  NDescriptions: {
    template: '<div class="n-descriptions"><slot /></div>',
    props: ['column', 'labelPlacement', 'bordered'],
  },
  NDescriptionsItem: {
    template: '<div class="n-descriptions-item" :data-label="label"><slot /></div>',
    props: ['label'],
  },
  NTag: {
    template: '<span class="n-tag" :data-type="type"><slot name="icon" /><slot /></span>',
    props: ['type', 'bordered', 'size', 'round'],
  },
  NIcon: { template: '<span class="n-icon"></span>', props: ['component'] },
}

// Mock vue-router
vi.mock('vue-router', () => ({
  useRoute: vi.fn(() => ({
    params: { path: ['apps', 'api'] },
    query: {},
  })),
  useRouter: vi.fn(() => ({
    push: vi.fn(),
  })),
}))

// Mock API
vi.mock('../src/services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../src/services/api')>()),
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  fetchContainers: vi.fn(() => Promise.resolve([])),
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
  requestJSON: vi.fn(),
}))

describe('StackDetail', () => {
  it('grants approval only after source comparison loads and retains the loaded revision when desired state changes', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const route = reactive({ params: { path: ['apps', 'api'] }, query: { tab: 'configuration' } })
    vi.mocked(useRoute).mockReturnValueOnce(route as unknown as ReturnType<typeof useRoute>)
    const record: StackRecord = {
      path: 'apps/api',
      status: 'failed',
      requiresAcknowledgement: true,
      desiredRevision: 'reviewed-revision',
      composeFile: 'compose.yaml',
      composeHash: 'new',
    }
    useStacksStore().stackMap = new Map([[record.path, record]])
    useStacksStore().refreshStatus = {
      revision: 'reviewed-revision',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '',
      refreshStatus: 'completed',
      updatesBlocked: false,
    }
    useConsoleStore().instance = fixtureData().instance
    const work = useWorkStore()
    work.connected = true
    work.restored = true
    work.supported = true
    let resolve!: (value: unknown) => void
    vi.mocked(requestJSON).mockImplementation((url) =>
      url.startsWith('/api/stack/configuration')
        ? new Promise((r) => {
            resolve = r
          })
        : Promise.resolve({ stacks: {} }),
    )
    const wrapper = mount(StackDetail, {
      global: { plugins: [pinia], stubs: { ...naiveStubs, Drawer: true } },
    })
    expect(wrapper.text()).toContain('Load the source comparison before approving')
    expect(wrapper.findAll('button').some((b) => b.text().startsWith('Approve & deploy'))).toBe(
      false,
    )
    resolve({
      revision: 'reviewed-revision',
      content: 'services: {}',
      applied: { revision: 'applied', content: 'services: {}' },
    })
    await flushPromises()
    const approval = wrapper
      .findAll('button')
      .find((b) => b.text() === 'Approve & deploy reviewed-revision')
    expect(approval).toBeDefined()
    useStacksStore().stackMap.set(record.path, {
      ...record,
      desiredRevision: 'new-unreviewed-revision',
    })
    await flushPromises()
    expect(approval?.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Desired revision changed; review the stack again')
    expect(wrapper.text()).not.toContain('Approve & deploy new-unreviewed-revision')
    wrapper.unmount()
  })
  it('renders stack details and the applied commit message when stack exists', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useStacksStore()
    const stack: StackRecord = {
      path: 'apps/api',
      composeFile: 'docker-compose.yml',
      composeHash: 'abc123def456',
      status: 'synced',
      containersRunning: 3,
      containersTotal: 3,
      syncedRevision: 'deadbeef12345678',
      syncedCommitMessage: 'Deploy v2.0 to production',
      syncedComposeHash: 'abc123def456',
      syncedAt: '2024-06-15T10:30:00Z',
      lastSyncAt: '2024-06-15T10:30:00Z',
      lastSyncStatus: 'synced',
    }
    store.stackMap = new Map([[stack.path, stack]])

    const wrapper = mount(StackDetail, {
      global: {
        plugins: [pinia],
        stubs: naiveStubs,
        mocks: {
          $router: { push: vi.fn() },
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('apps/api')
    expect(wrapper.text()).toContain('Deploy v2.0 to production')
    wrapper.unmount()
  })

  it('renders not found message when stack missing', () => {
    setActivePinia(createPinia())

    const wrapper = mount(StackDetail, {
      global: {
        plugins: [createPinia()],
        stubs: naiveStubs,
        mocks: {
          $router: { push: vi.fn() },
        },
      },
    })

    expect(wrapper.text()).toContain('was not found')
  })

  it('renders error details for failed stack', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useStacksStore()
    const stack: StackRecord = {
      path: 'apps/api',
      composeFile: 'docker-compose.yml',
      composeHash: 'abc123',
      status: 'failed',
      containersRunning: 0,
      containersTotal: 2,
      lastSyncAt: '2024-06-15T10:30:00Z',
      lastSyncStatus: 'failed',
      lastSyncError: 'compose up failed: no such image',
    }
    store.stackMap = new Map([[stack.path, stack]])

    const wrapper = mount(StackDetail, {
      global: {
        plugins: [pinia],
        stubs: naiveStubs,
        mocks: {
          $router: { push: vi.fn() },
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('apps/api')
    expect(wrapper.text()).toContain('compose up failed: no such image')
    wrapper.unmount()
  })
})
