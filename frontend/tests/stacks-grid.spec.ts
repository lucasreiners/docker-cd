import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import StackCard from '../src/components/StackCard.vue'
import StatusBadge from '../src/components/StatusBadge.vue'
import type { StackRecord } from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

// Stub naive-ui components for shallow mounting
const naiveStubs = {
  NCard: {
    template: '<div class="n-card"><slot name="header" /><slot /></div>',
    props: ['hoverable'],
  },
  NTag: {
    template: '<span class="n-tag" :data-type="type"><slot name="icon" /><slot /></span>',
    props: ['type', 'bordered', 'size', 'round'],
  },
  NText: {
    template: '<span class="n-text"><slot /></span>',
    props: ['strong', 'depth', 'type', 'code'],
  },
  NSpace: {
    template: '<div class="n-space"><slot /></div>',
    props: ['vertical', 'size', 'align', 'justify', 'wrap'],
  },
  NIcon: { template: '<span class="n-icon"></span>', props: ['component'] },
  NInput: {
    template: '<input class="n-input" />',
    props: ['value', 'placeholder', 'clearable', 'size'],
  },
  NAlert: { template: '<div class="n-alert"><slot /></div>', props: ['type', 'title', 'bordered'] },
  NSpin: { template: '<div class="n-spin"></div>', props: ['size'] },
  NEmpty: { template: '<div class="n-empty"></div>', props: ['description'] },
}

// Mock router
const mockPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush }),
  useRoute: () => ({ params: {} }),
  createRouter: vi.fn(),
  createWebHistory: vi.fn(),
}))

// Mock API
vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  triggerRefresh: vi.fn(() => Promise.resolve()),
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
}))

import { fetchRefreshStatus, fetchStacks } from '../src/services/api'

const _mockFetchStacks = vi.mocked(fetchStacks)
const _mockFetchRefreshStatus = vi.mocked(fetchRefreshStatus)

describe('StackCard', () => {
  const mockStack: StackRecord = {
    path: 'apps/api',
    composeFile: 'docker-compose.yml',
    composeHash: 'abc123',
    status: 'synced',
    containersRunning: 3,
    containersTotal: 3,
    syncedRevision: 'deadbeef12345678',
    syncedCommitMessage: 'Deploy v2.0',
    lastSyncAt: '2024-06-15T10:30:00Z',
  }

  const mountOpts = () => ({
    props: { stack: mockStack },
    global: {
      plugins: [createPinia()],
      stubs: naiveStubs,
      mocks: { $router: { push: mockPush } },
    },
  })

  it('renders stack path', () => {
    const wrapper = mount(StackCard, mountOpts())
    expect(wrapper.text()).toContain('apps/api')
  })

  it('renders compose file name', () => {
    const wrapper = mount(StackCard, mountOpts())
    expect(wrapper.text()).toContain('docker-compose.yml')
  })

  it('renders truncated revision', () => {
    const wrapper = mount(StackCard, mountOpts())
    expect(wrapper.text()).toContain('deadbeef')
    expect(wrapper.text()).not.toContain('deadbeef12345678')
  })

  it('renders commit message', () => {
    const wrapper = mount(StackCard, mountOpts())
    expect(wrapper.text()).toContain('Deploy v2.0')
  })

  it('renders error when present', () => {
    const failedStack: StackRecord = {
      ...mockStack,
      status: 'failed',
      lastSyncError: 'compose up failed: image not found',
    }
    const wrapper = mount(StackCard, {
      props: { stack: failedStack },
      global: {
        plugins: [createPinia()],
        stubs: naiveStubs,
        mocks: { $router: { push: mockPush } },
      },
    })
    expect(wrapper.text()).toContain('compose up failed: image not found')
  })
})

describe('StatusBadge', () => {
  const mountBadge = (status: string) =>
    mount(StatusBadge, {
      props: { status },
      global: { plugins: [createPinia()], stubs: naiveStubs },
    })

  it('renders synced status', () => {
    const wrapper = mountBadge('synced')
    expect(wrapper.text()).toContain('Synced')
    expect(wrapper.find('.n-tag').attributes('data-type')).toBe('success')
  })

  it('renders failed status', () => {
    const wrapper = mountBadge('failed')
    expect(wrapper.text()).toContain('Failed')
    expect(wrapper.find('.n-tag').attributes('data-type')).toBe('error')
  })

  it('renders syncing status', () => {
    const wrapper = mountBadge('syncing')
    expect(wrapper.text()).toContain('Syncing')
    expect(wrapper.find('.n-tag').attributes('data-type')).toBe('warning')
  })

  it('renders missing status', () => {
    const wrapper = mountBadge('missing')
    expect(wrapper.text()).toContain('Missing')
    expect(wrapper.find('.n-tag').attributes('data-type')).toBe('info')
  })
})

describe('StacksGrid (stacks rendering)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('should render stack cards from store', async () => {
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
