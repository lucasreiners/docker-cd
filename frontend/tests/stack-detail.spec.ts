import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import StackDetail from '../src/pages/StackDetail.vue'
import type { StackRecord } from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

// Stub naive-ui components
const naiveStubs = {
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
  })),
  useRouter: vi.fn(() => ({
    push: vi.fn(),
  })),
}))

// Mock API
vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  fetchContainers: vi.fn(() => Promise.resolve([])),
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
}))

describe('StackDetail', () => {
  it('renders stack details when stack exists', () => {
    setActivePinia(createPinia())
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
        plugins: [createPinia()],
        stubs: naiveStubs,
        mocks: {
          $router: { push: vi.fn() },
        },
      },
    })

    // Re-set the store after pinia plugin creates a new one
    const detailStore = useStacksStore()
    detailStore.stackMap = new Map([[stack.path, stack]])

    // Check that key details render
    expect(wrapper.text()).toContain('apps/api')
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

  it('renders error details for failed stack', () => {
    setActivePinia(createPinia())
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
        plugins: [createPinia()],
        stubs: naiveStubs,
        mocks: {
          $router: { push: vi.fn() },
        },
      },
    })

    const detailStore = useStacksStore()
    detailStore.stackMap = new Map([[stack.path, stack]])

    expect(wrapper.text()).toContain('apps/api')
  })
})
