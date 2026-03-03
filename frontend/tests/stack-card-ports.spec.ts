import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import StackCard from '@/components/StackCard.vue'
import type { ContainerInfo, StackRecord } from '@/services/api'
import * as api from '@/services/api'

// Mock Vue Router
const mockRouterPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockRouterPush,
  }),
}))

describe('StackCard with containers', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockRouterPush.mockClear()
    vi.clearAllMocks()
  })

  const mockStack: StackRecord = {
    path: 'test-stack/production',
    composeFile: 'docker-compose.yml',
    composeHash: 'abc123',
    status: 'synced',
    containersRunning: 2,
    containersTotal: 2,
    syncedRevision: 'abc123def456',
    syncedCommitMessage: 'Test commit message',
    lastSyncAt: '2026-03-01T12:00:00Z',
  }

  const mockContainersWithPorts: ContainerInfo[] = [
    {
      id: 'container-1',
      name: 'web-server',
      service: 'web',
      state: 'running',
      health: 'healthy',
      image: 'nginx:latest',
      ports: '8080:80/tcp, 8443:443/tcp',
    },
    {
      id: 'container-2',
      name: 'api-server',
      service: 'api',
      state: 'running',
      health: 'healthy',
      image: 'node:18',
      ports: '3000:3000/tcp',
    },
    {
      id: 'container-3',
      name: 'database',
      service: 'db',
      state: 'running',
      health: 'healthy',
      image: 'postgres:14',
      ports: '3306/tcp', // No external port mapping
    },
  ]

  test('fetches containers on mount', async () => {
    const fetchContainersSpy = vi.spyOn(api, 'fetchContainers').mockResolvedValue([])

    mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(fetchContainersSpy).toHaveBeenCalledWith(mockStack.path)
    })
  })

  test('displays containers alphabetically', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue(mockContainersWithPorts)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    // Wait for containers to load
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('api-server')
    })

    // Check order: api-server, database, web-server (alphabetical)
    const containerItems = wrapper.findAll('.container-item')
    expect(containerItems.length).toBe(3)
    expect(containerItems[0].text()).toContain('api-server')
    expect(containerItems[1].text()).toContain('database')
    expect(containerItems[2].text()).toContain('web-server')
  })

  test('shows arrow and underline for containers with ports', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue(mockContainersWithPorts)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('web-server')
    })

    // Container with ports should have link styling
    const webServerLink = wrapper.find('.container-link')
    expect(webServerLink.exists()).toBe(true)
    expect(webServerLink.text()).toContain('↗')
    expect(webServerLink.find('.underlined').exists()).toBe(true)
  })

  test('shows plain text for containers without ports', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue(mockContainersWithPorts)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('database')
    })

    // Find the database container item
    const containerItems = wrapper.findAll('.container-item')
    const databaseItem = containerItems.find((item) => item.text().includes('database'))

    expect(databaseItem).toBeDefined()
    expect(databaseItem?.find('.container-text').exists()).toBe(true)
    expect(databaseItem?.find('.container-link').exists()).toBe(false)
    expect(databaseItem?.text()).not.toContain('↗')
  })

  test('clicking container with single port opens correct URL', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue([
      {
        id: 'container-1',
        name: 'web-server',
        service: 'web',
        state: 'running',
        health: 'healthy',
        image: 'nginx:latest',
        ports: '8080:80/tcp',
      },
    ])

    const windowOpenSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('web-server')
    })

    const link = wrapper.find('.container-link')
    await link.trigger('click')

    expect(windowOpenSpy).toHaveBeenCalledWith(
      expect.stringContaining(':8080'),
      '_blank',
      'noopener,noreferrer',
    )

    windowOpenSpy.mockRestore()
  })

  test('clicking container with multiple ports opens lowest port', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue([
      {
        id: 'container-1',
        name: 'web-server',
        service: 'web',
        state: 'running',
        health: 'healthy',
        image: 'nginx:latest',
        ports: '9000:9000/tcp, 8080:80/tcp, 8443:443/tcp',
      },
    ])

    const windowOpenSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('web-server')
    })

    const link = wrapper.find('.container-link')
    await link.trigger('click')

    expect(windowOpenSpy).toHaveBeenCalledWith(
      expect.stringContaining(':8080'), // Lowest port
      '_blank',
      'noopener,noreferrer',
    )

    windowOpenSpy.mockRestore()
  })

  test('clicking container link does not trigger card navigation', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue([
      {
        id: 'container-1',
        name: 'web-server',
        service: 'web',
        state: 'running',
        health: 'healthy',
        image: 'nginx:latest',
        ports: '8080:80/tcp',
      },
    ])

    vi.spyOn(window, 'open').mockImplementation(() => null)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('web-server')
    })

    const link = wrapper.find('.container-link')
    await link.trigger('click')

    // Router push should not have been called (event propagation stopped)
    expect(mockRouterPush).not.toHaveBeenCalled()
  })

  test('displays loading state while fetching containers', async () => {
    vi.spyOn(api, 'fetchContainers').mockImplementation(() => new Promise(() => {}))

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Loading containers')
    })
  })

  test('displays error state when container fetch fails', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.spyOn(api, 'fetchContainers').mockRejectedValue(new Error('Network error'))

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Failed to load containers')
    })

    consoleSpy.mockRestore()
  })

  test('handles containers with mixed port configurations', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue([
      {
        id: 'container-1',
        name: 'web',
        service: 'web',
        state: 'running',
        health: 'healthy',
        image: 'nginx:latest',
        ports: '8080:80/tcp',
      },
      {
        id: 'container-2',
        name: 'db',
        service: 'db',
        state: 'running',
        health: 'healthy',
        image: 'postgres:14',
        ports: '5432/tcp', // No external port
      },
      {
        id: 'container-3',
        name: 'cache',
        service: 'cache',
        state: 'running',
        health: 'healthy',
        image: 'redis:7',
        ports: undefined, // No ports at all
      },
    ])

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('web')
    })

    const containerItems = wrapper.findAll('.container-item')
    expect(containerItems.length).toBe(3)

    // 'web' should have link (external port)
    const webItem = containerItems.find((item) => item.text().includes('web'))
    expect(webItem?.find('.container-link').exists()).toBe(true)

    // 'db' and 'cache' should be plain text
    const dbItem = containerItems.find((item) => item.text().includes('db'))
    expect(dbItem?.find('.container-text').exists()).toBe(true)

    const cacheItem = containerItems.find((item) => item.text().includes('cache'))
    expect(cacheItem?.find('.container-text').exists()).toBe(true)
  })

  test('clicking container without external port does nothing', async () => {
    vi.spyOn(api, 'fetchContainers').mockResolvedValue([
      {
        id: 'container-1',
        name: 'database',
        service: 'db',
        state: 'running',
        health: 'healthy',
        image: 'postgres:14',
        ports: '5432/tcp',
      },
    ])

    const windowOpenSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    const wrapper = mount(StackCard, {
      props: { stack: mockStack },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('database')
    })

    // Should show plain text, not a link
    const containerText = wrapper.find('.container-text')
    expect(containerText.exists()).toBe(true)

    // No window.open should be called
    expect(windowOpenSpy).not.toHaveBeenCalled()

    windowOpenSpy.mockRestore()
  })
})
