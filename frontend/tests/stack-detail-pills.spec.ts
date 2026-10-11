import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import StackDetail from '../src/pages/StackDetail.vue'
import * as api from '../src/services/api'
import { useStacksStore } from '../src/store/stacks'

afterEach(() => vi.restoreAllMocks())

it('renders external ports as links, internal ports as text, and a real details back link', async () => {
  const pinia = createPinia()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/stacks', component: { template: '<div />' } },
      { path: '/stack/:path+', name: 'stack-detail', component: StackDetail },
    ],
  })
  const store = useStacksStore(pinia)
  store.stackMap = new Map([
    [
      'apps/web',
      { path: 'apps/web', composeFile: 'compose.yaml', composeHash: 'hash', status: 'synced' },
    ],
  ])
  vi.spyOn(api, 'fetchContainers').mockResolvedValue([
    {
      id: 'abc123',
      name: 'web',
      service: 'web',
      image: 'nginx',
      state: 'running',
      health: 'none',
      ports: '8080:80/tcp, 8443:443/tcp, 5432/tcp',
    },
  ])
  await router.push('/stack/apps/web?q=web&view=grid&tab=containers')
  const wrapper = mount(StackDetail, {
    global: {
      plugins: [pinia, router],
      stubs: { StatusBadge: true, NTag: { template: '<span><slot /></span>' }, RelativeTime: true },
    },
  })
  await flushPromises()
  expect(wrapper.findAll('a.port-pill')).toHaveLength(2)
  expect(wrapper.get('a.port-pill').attributes('href')).toContain(':8080')
  expect(wrapper.get('a.port-pill').attributes('rel')).toBe('noopener noreferrer')
  expect(wrapper.get('.port-pill-internal').text()).toBe('5432/tcp')
  expect(wrapper.get('.port-pill-internal').element.tagName).toBe('SPAN')
  expect(wrapper.get('.back-link').attributes('href')).toBe('/stacks?q=web&view=grid')
  expect(wrapper.text()).toContain('No health check')
  wrapper.unmount()
})
