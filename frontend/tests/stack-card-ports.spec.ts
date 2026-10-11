import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import StackCard from '../src/components/StackCard.vue'
import type { ContainerInfo, StackRecord } from '../src/services/api'
import * as api from '../src/services/api'

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
function render() {
  const wrapper = mount(StackCard, {
    props: { stack },
    global: {
      stubs: { RouterLink: { template: '<a><slot /></a>' }, StatusBadge: true, RelativeTime: true },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.restoreAllMocks()
})

describe('container links', () => {
  it('uses native external links with the lowest published port and leaves internal ports as text', async () => {
    const fetch = vi
      .spyOn(api, 'fetchContainers')
      .mockResolvedValue([
        container('web', '9000:9000/tcp, 8080:80/tcp, 8443:443/tcp'),
        container('database', '5432/tcp'),
        container('cache'),
      ])
    const wrapper = render()
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith('apps/web')
    expect(wrapper.findAll('.container-item').map((item) => item.text())).toEqual([
      'cache',
      'database',
      'web↗',
    ])
    const link = wrapper.get('a.container-link')
    expect(link.attributes('href')).toContain(':8080')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(wrapper.findAll('.container-link')).toHaveLength(1)
    expect(wrapper.findAll('.container-text')).toHaveLength(2)
  })

  it('shows unavailable runtime instead of claiming health when inspection fails', async () => {
    vi.spyOn(api, 'fetchContainers').mockRejectedValue(new Error('Docker is unavailable'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('Runtime unavailable')
    expect(wrapper.text()).toContain('Failed to load containers')
    expect(wrapper.text()).not.toContain('Health checks passing')
  })
})
