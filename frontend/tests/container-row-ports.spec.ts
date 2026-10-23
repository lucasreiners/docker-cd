import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import StackTree from '../src/components/StackTree.vue'
import type { ContainerInfo, StackRecord } from '../src/services/api'
import * as api from '../src/services/api'
import { useConsoleStore } from '../src/store/console'

const stack: StackRecord = {
  path: 'apps/web',
  composeFile: 'compose.yaml',
  composeHash: 'hash',
  status: 'synced',
  containersRunning: 3,
  containersTotal: 3,
}
const container = (name: string, ports?: string): ContainerInfo => ({
  id: name,
  name,
  service: name,
  image: 'nginx',
  state: 'running',
  health: 'none',
  ports,
})
const wrappers: ReturnType<typeof mount>[] = []
beforeEach(() => {
  setActivePinia(createPinia())
  sessionStorage.clear()
})
async function render() {
  const consoleStore = useConsoleStore()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/stacks', component: { template: '<div/>' } },
      { path: '/stacks/:path+', name: 'stack-detail', component: { template: '<div/>' } },
    ],
  })
  await router.push('/stacks')
  const wrapper = mount(StackTree, {
    props: { stacks: [stack], runtimeFor: consoleStore.runtimeFor },
    global: {
      plugins: [router],
    },
  })
  wrappers.push(wrapper)
  await consoleStore.refreshRuntime(true)
  await flushPromises()
  return wrapper
}
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  useConsoleStore().$dispose()
  vi.restoreAllMocks()
})

describe('container links', () => {
  it('uses native external links with the lowest published port and keeps full ports in inspection', async () => {
    const fetch = vi.spyOn(api, 'requestJSON').mockResolvedValue({
      'apps/web': {
        containers: [
          container('web', '9000:9000/tcp, 8080:80/tcp, 8443:443/tcp'),
          container('database', '5432/tcp'),
          container('cache'),
        ],
        error: null,
        observedAt: '2026-10-23T12:00:00Z',
      },
    })
    const wrapper = await render()
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith('/api/runtime')
    const showAll = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Show all 3 containers')
    if (!showAll) throw new Error('Missing container disclosure')
    await showAll.trigger('click')
    expect(
      wrapper.findAll('[data-container]').map((item) => item.attributes('data-container')),
    ).toEqual(['cache', 'database', 'web'])
    const link = wrapper.get('.inventory-row-links a[target="_blank"]')
    expect(link.attributes('href')).toContain(':8080')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(wrapper.findAll('.inventory-row-links a[target="_blank"]')).toHaveLength(1)
    expect(wrapper.get('[data-container="database"]').find('a[target="_blank"]').exists()).toBe(
      false,
    )
    await wrapper.get('[data-container="database"] button').trigger('click')
    expect(wrapper.emitted('inspect')?.[0]?.slice(0, 2)).toEqual(['apps/web', 'database'])
    expect(wrapper.find('.inventory-ports').exists()).toBe(false)
    expect(wrapper.get('[data-container="cache"]').find('a[target="_blank"]').exists()).toBe(false)
  })

  it('shows unavailable runtime instead of claiming health when inspection fails', async () => {
    vi.spyOn(api, 'requestJSON').mockRejectedValue(new Error('Docker is unavailable'))
    const wrapper = await render()
    await flushPromises()
    expect(wrapper.text()).toContain('Runtime unavailable')
    expect(useConsoleStore().runtimeFor(stack.path).error).toContain('Docker is unavailable')
    expect(wrapper.text()).toContain('Unknown')
    expect(wrapper.text()).not.toContain('Health checks passing')
  })
})
