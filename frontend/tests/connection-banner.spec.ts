import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import ConnectionBanner from '../src/components/ConnectionBanner.vue'
import { useStacksStore } from '../src/store/stacks'
import { useWorkStore } from '../src/store/work'

// Stub naive-ui components
const naiveStubs = {
  NAlert: {
    template:
      '<div class="n-alert" :data-type="type" data-testid="connection-banner"><slot /></div>',
    props: ['type', 'bordered'],
  },
}

// Mock API
vi.mock('../src/services/api', () => ({
  fetchStacks: vi.fn(),
  fetchRefreshStatus: vi.fn(),
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
}))

describe('ConnectionBanner', () => {
  it('shows connecting state before the first connection', () => {
    setActivePinia(createPinia())
    const wrapper = mount(ConnectionBanner, {
      global: {
        plugins: [createPinia()],
        stubs: naiveStubs,
      },
    })
    expect(wrapper.get('[role="status"]').text()).toContain('Connecting')
  })

  it('shows connected only after authoritative work recovery', async () => {
    setActivePinia(createPinia())
    const store = useStacksStore()
    store.connectionState = 'connected'

    const wrapper = mount(ConnectionBanner, {
      global: {
        stubs: naiveStubs,
      },
    })
    expect(wrapper.get('[role="status"]').text()).toBe('Restoring work state…')
    useWorkStore().restored = true
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toBe('Connected')
  })

  it('shows reconnecting banner', async () => {
    setActivePinia(createPinia())
    const store = useStacksStore()

    // Simulate: was connected, now reconnecting
    store.connectionState = 'connected'

    const wrapper = mount(ConnectionBanner, {
      global: {
        stubs: naiveStubs,
      },
    })

    // Transition to reconnecting
    store.connectionState = 'reconnecting'
    await wrapper.vm.$nextTick()

    const banner = wrapper.find('[data-testid="connection-banner"]')
    expect(banner.exists()).toBe(true)
    expect(wrapper.text()).toContain('Reconnecting')
  })

  it('shows disconnected banner after having been connected', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useStacksStore()

    const wrapper = mount(ConnectionBanner, {
      global: {
        plugins: [pinia],
        stubs: naiveStubs,
      },
    })

    // Simulate connect
    store.connectionState = 'connected'
    await flushPromises()
    await wrapper.vm.$nextTick()

    // Simulate disconnect
    store.connectionState = 'disconnected'
    await flushPromises()
    await wrapper.vm.$nextTick()

    const banner = wrapper.find('[data-testid="connection-banner"]')
    expect(banner.exists()).toBe(true)
    expect(wrapper.text()).toContain('Disconnected')
  })
})
