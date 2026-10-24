import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import StackTree from '../src/components/StackTree.vue'
import StatusBadge from '../src/components/StatusBadge.vue'
import type { StackRecord } from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

const inventoryStubs = {
  RouterLink: { template: '<a><slot /></a>', props: ['to'] },
}

// Mock router
const mockPush = vi.fn()
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRouter: () => ({ push: mockPush }),
  useRoute: () => ({ params: {}, query: {} }),
  createRouter: vi.fn(),
  createWebHistory: vi.fn(),
}))

describe('StackTree migrated inventory fields', () => {
  const mockStack: StackRecord = {
    path: 'apps/api',
    composeFile: 'docker-compose.yml',
    composeHash: 'abc123',
    status: 'synced',
    containersRunning: 3,
    containersTotal: 3,
    syncedRevision: 'deadbeef12345678',
    desiredRevision: 'cafebabe12345678',
    syncedCommitMessage: 'Deploy v2.0',
    lastSyncAt: '2024-06-15T10:30:00Z',
  }

  const mountOpts = () => ({
    props: {
      stacks: [mockStack],
      runtimeFor: () => ({
        containers: [
          {
            id: 'one',
            name: 'api-1',
            service: 'api',
            image: 'nginx',
            state: 'running',
            health: 'none',
          },
          {
            id: 'two',
            name: 'api-2',
            service: 'api',
            image: 'nginx',
            state: 'exited',
            health: 'none',
          },
        ],
        loading: false,
        error: null,
        observedAt: new Date().toISOString(),
      }),
    },
    global: {
      plugins: [createPinia()],
      stubs: inventoryStubs,
      mocks: { $router: { push: mockPush } },
    },
  })

  it('renders stack path', () => {
    const wrapper = mount(StackTree, mountOpts())
    expect(wrapper.text()).toContain('apps/api')
  })

  it('renders actual observed running counts rather than stale deployment counts', () => {
    const wrapper = mount(StackTree, mountOpts())
    expect(wrapper.text()).toContain('1 / 2 running')
    expect(wrapper.text()).not.toContain('3 / 3 running')
  })

  it('keeps the Image column empty for stacks and populated for containers', () => {
    const wrapper = mount(StackTree, mountOpts())
    const stackRow = wrapper.get('.inventory-stack-header')
    expect(stackRow.get('.inventory-stack-image').text()).toBe('')
    expect(stackRow.get('.inventory-stack-image').attributes('title')).toBeUndefined()
    expect(stackRow.text()).not.toContain('cafebabe')
    expect(stackRow.text()).not.toContain('deadbeef')
    const containers = wrapper.findAll('[data-container]')
    expect(containers).toHaveLength(2)
    for (const container of containers) {
      expect(container.findAll('td')[2]?.text()).toBe('nginx')
    }
  })
})

describe('StatusBadge', () => {
  const mountBadge = (status: string) =>
    mount(StatusBadge, {
      props: { status },
      global: { plugins: [createPinia()], stubs: inventoryStubs },
    })

  it('renders synced status', () => {
    const wrapper = mountBadge('synced')
    expect(wrapper.text()).toContain('In sync')
    expect(wrapper.classes()).toContain('status-success')
  })

  it('renders failed status', () => {
    const wrapper = mountBadge('failed')
    expect(wrapper.text()).toContain('Failed')
    expect(wrapper.classes()).toContain('status-error')
  })

  it('renders syncing status', () => {
    const wrapper = mountBadge('syncing')
    expect(wrapper.text()).toContain('Deploying')
    expect(wrapper.classes()).toContain('status-info')
  })

  it('renders missing status', () => {
    const wrapper = mountBadge('missing')
    expect(wrapper.text()).toContain('Not deployed')
    expect(wrapper.classes()).toContain('status-neutral')
  })
})

describe('StacksGrid (stacks rendering)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('exposes all stacks for the inventory', async () => {
    const stacks: StackRecord[] = [
      {
        path: 'apps/api',
        composeFile: 'dc.yml',
        composeHash: 'h1',
        status: 'synced',
        containersRunning: 2,
        containersTotal: 2,
      },
      {
        path: 'apps/web',
        composeFile: 'dc.yml',
        composeHash: 'h2',
        status: 'failed',
        containersRunning: 0,
        containersTotal: 1,
      },
    ]

    // Pre-populate store
    const store = useStacksStore()
    store.stackMap = new Map(stacks.map((s) => [s.path, s]))
    store.loading = false
    store.error = null

    // Verify store state is accessible
    expect(store.stacks).toHaveLength(2)
    expect(store.filteredStacks).toHaveLength(2)
    expect(store.filteredStacks.map((s) => s.path)).toContain('apps/api')
    expect(store.filteredStacks.map((s) => s.path)).toContain('apps/web')
  })

  it('should show filtered stacks when filter applied', async () => {
    const stacks: StackRecord[] = [
      {
        path: 'apps/api',
        composeFile: 'dc.yml',
        composeHash: 'h1',
        status: 'synced',
        containersRunning: 1,
        containersTotal: 1,
      },
      {
        path: 'apps/web',
        composeFile: 'dc.yml',
        composeHash: 'h2',
        status: 'failed',
        containersRunning: 0,
        containersTotal: 1,
      },
      {
        path: 'apps/db',
        composeFile: 'dc.yml',
        composeHash: 'h3',
        status: 'synced',
        containersRunning: 1,
        containersTotal: 1,
      },
    ]

    const store = useStacksStore()
    store.stackMap = new Map(stacks.map((s) => [s.path, s]))

    store.setFilterStatus('failed')
    expect(store.filteredStacks).toHaveLength(1)
    expect(store.filteredStacks[0].path).toBe('apps/web')
  })
})
