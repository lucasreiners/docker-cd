import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StackRecord } from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

// Mock the API module
vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  triggerRefresh: vi.fn(() => Promise.resolve()),
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
}))

import { fetchRefreshStatus, fetchStacks } from '../src/services/api'

const mockFetchStacks = vi.mocked(fetchStacks)
const mockFetchRefreshStatus = vi.mocked(fetchRefreshStatus)

describe('stacks store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('should start with empty state', () => {
    const store = useStacksStore()
    expect(store.stacks).toEqual([])
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
    expect(store.connectionState).toBe('disconnected')
  })

  it('should load initial stacks from API', async () => {
    const mockStacks: StackRecord[] = [
      { path: 'apps/api', composeFile: 'docker-compose.yml', composeHash: 'h1', status: 'synced' },
      { path: 'apps/web', composeFile: 'docker-compose.yml', composeHash: 'h2', status: 'syncing' },
    ]
    mockFetchStacks.mockResolvedValue(mockStacks)
    mockFetchRefreshStatus.mockResolvedValue({
      revision: 'abc123',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '2024-01-01T00:00:00Z',
      refreshStatus: 'completed',
    })

    const store = useStacksStore()
    await store.loadInitial()

    expect(store.stacks).toHaveLength(2)
    expect(store.stacks.map((s) => s.path)).toContain('apps/api')
    expect(store.stacks.map((s) => s.path)).toContain('apps/web')
    expect(store.refreshStatus?.revision).toBe('abc123')
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
  })

  it('should handle load error', async () => {
    mockFetchStacks.mockRejectedValue(new Error('Network error'))
    mockFetchRefreshStatus.mockRejectedValue(new Error('Network error'))

    const store = useStacksStore()
    await store.loadInitial()

    expect(store.error).toBe('Network error')
    expect(store.loading).toBe(false)
  })

  it('should filter by status', async () => {
    const mockStacks: StackRecord[] = [
      { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
      { path: 'apps/web', composeFile: 'dc.yml', composeHash: 'h2', status: 'failed' },
      { path: 'apps/db', composeFile: 'dc.yml', composeHash: 'h3', status: 'synced' },
    ]
    mockFetchStacks.mockResolvedValue(mockStacks)
    mockFetchRefreshStatus.mockResolvedValue({
      revision: 'abc',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '2024-01-01T00:00:00Z',
      refreshStatus: 'completed',
    })

    const store = useStacksStore()
    await store.loadInitial()

    store.setFilterStatus('synced')
    expect(store.filteredStacks).toHaveLength(2)
    expect(store.filteredStacks.every((s) => s.status === 'synced')).toBe(true)

    store.setFilterStatus('failed')
    expect(store.filteredStacks).toHaveLength(1)
    expect(store.filteredStacks[0].path).toBe('apps/web')

    store.setFilterStatus('')
    expect(store.filteredStacks).toHaveLength(3)
  })

  it('should search by path', async () => {
    const mockStacks: StackRecord[] = [
      { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
      { path: 'apps/web', composeFile: 'dc.yml', composeHash: 'h2', status: 'synced' },
      { path: 'infra/db', composeFile: 'dc.yml', composeHash: 'h3', status: 'synced' },
    ]
    mockFetchStacks.mockResolvedValue(mockStacks)
    mockFetchRefreshStatus.mockResolvedValue({
      revision: 'abc',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '2024-01-01T00:00:00Z',
      refreshStatus: 'completed',
    })

    const store = useStacksStore()
    await store.loadInitial()

    store.setSearchQuery('apps')
    expect(store.filteredStacks).toHaveLength(2)

    store.setSearchQuery('db')
    expect(store.filteredStacks).toHaveLength(1)
    expect(store.filteredStacks[0].path).toBe('infra/db')

    store.setSearchQuery('')
    expect(store.filteredStacks).toHaveLength(3)
  })

  it('should compute status counts', async () => {
    const mockStacks: StackRecord[] = [
      { path: 'a', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
      { path: 'b', composeFile: 'dc.yml', composeHash: 'h2', status: 'synced' },
      { path: 'c', composeFile: 'dc.yml', composeHash: 'h3', status: 'failed' },
      { path: 'd', composeFile: 'dc.yml', composeHash: 'h4', status: 'syncing' },
    ]
    mockFetchStacks.mockResolvedValue(mockStacks)
    mockFetchRefreshStatus.mockResolvedValue({
      revision: 'abc',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '2024-01-01T00:00:00Z',
      refreshStatus: 'completed',
    })

    const store = useStacksStore()
    await store.loadInitial()

    expect(store.statusCounts.synced).toBe(2)
    expect(store.statusCounts.failed).toBe(1)
    expect(store.statusCounts.syncing).toBe(1)
    expect(store.statusCounts.missing).toBe(0)
  })

  it('should sort filtered stacks by path', async () => {
    const mockStacks: StackRecord[] = [
      { path: 'c/app', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
      { path: 'a/app', composeFile: 'dc.yml', composeHash: 'h2', status: 'synced' },
      { path: 'b/app', composeFile: 'dc.yml', composeHash: 'h3', status: 'synced' },
    ]
    mockFetchStacks.mockResolvedValue(mockStacks)
    mockFetchRefreshStatus.mockResolvedValue({
      revision: 'abc',
      ref: 'main',
      refType: 'branch',
      refreshedAt: '2024-01-01T00:00:00Z',
      refreshStatus: 'completed',
    })

    const store = useStacksStore()
    await store.loadInitial()

    expect(store.filteredStacks.map((s) => s.path)).toEqual(['a/app', 'b/app', 'c/app'])
  })

  it('should handle SSE snapshot by replacing all stacks', () => {
    const store = useStacksStore()

    // Simulate what SSE onSnapshot does
    const records: StackRecord[] = [
      { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
    ]
    // Directly test the map update logic
    store.stackMap = new Map(records.map((s) => [s.path, s]))

    expect(store.stacks).toHaveLength(1)
    expect(store.stacks[0].path).toBe('apps/api')
  })

  it('should handle SSE upsert by adding/updating a stack', () => {
    const store = useStacksStore()

    // Set initial state
    store.stackMap = new Map([
      [
        'apps/api',
        { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' as const },
      ],
    ])

    // Simulate upsert
    const next = new Map(store.stackMap)
    next.set('apps/api', {
      path: 'apps/api',
      composeFile: 'dc.yml',
      composeHash: 'h2',
      status: 'syncing' as const,
    })
    store.stackMap = next

    expect(store.stacks).toHaveLength(1)
    expect(store.stacks[0].composeHash).toBe('h2')
    expect(store.stacks[0].status).toBe('syncing')
  })

  it('should handle SSE delete by removing a stack', () => {
    const store = useStacksStore()

    store.stackMap = new Map([
      [
        'apps/api',
        { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' as const },
      ],
      [
        'apps/web',
        { path: 'apps/web', composeFile: 'dc.yml', composeHash: 'h2', status: 'synced' as const },
      ],
    ])

    const next = new Map(store.stackMap)
    next.delete('apps/api')
    store.stackMap = next

    expect(store.stacks).toHaveLength(1)
    expect(store.stacks[0].path).toBe('apps/web')
  })

  it('should get a single stack by path', () => {
    const store = useStacksStore()

    store.stackMap = new Map([
      [
        'apps/api',
        { path: 'apps/api', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' as const },
      ],
    ])

    expect(store.getStack('apps/api')).toBeDefined()
    expect(store.getStack('apps/api')?.status).toBe('synced')
    expect(store.getStack('nonexistent')).toBeUndefined()
  })
})
