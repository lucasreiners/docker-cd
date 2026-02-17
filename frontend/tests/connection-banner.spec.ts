import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import ConnectionBanner from '../src/components/ConnectionBanner.vue'
import { useStacksStore } from '../src/store/stacks'

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
  it('does not show banner when disconnected and never connected', () => {
    setActivePinia(createPinia())
    const wrapper = mount(ConnectionBanner, {
      global: {
        plugins: [createPinia()],
        stubs: naiveStubs,
      },
    })
    expect(wrapper.find('[data-testid="connection-banner"]').exists()).toBe(false)
  })

  it('does not show banner when connected', () => {
    setActivePinia(createPinia())
    const store = useStacksStore()
    store.connectionState = 'connected'

    const wrapper = mount(ConnectionBanner, {
      global: {
        stubs: naiveStubs,
      },
    })
    expect(wrapper.find('[data-testid="connection-banner"]').exists()).toBe(false)
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
