import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ContainerInspector from '../src/components/ContainerInspector.vue'
import ImageIdentity from '../src/components/ImageIdentity.vue'
import type { ContainerInfo, ImageIdentity as Identity } from '../src/services/api'
import { useConsoleStore } from '../src/store/console'

const digest = `sha256:${'abcdef0123456789'.repeat(4)}`
const reference = `docker.io/library/nginx@${digest}`
const identity = (): Identity => ({
  status: 'available',
  observedAt: '2026-10-23T12:00:00Z',
  source: 'container_image_inspect',
  imageId: 'sha256:local-secret-id',
  reference: 'nginx:stable',
  platform: { os: 'linux', architecture: 'arm64', variant: 'v8' },
  repositoryDigests: [{ repository: 'docker.io/library/nginx', digest, kind: 'unknown' }],
  preferredRepositoryDigest: { repository: 'docker.io/library/nginx', digest, kind: 'unknown' },
  freshness: 'not_checked',
})
const container = (): ContainerInfo => ({
  id: 'container-exact-id',
  name: 'web-1',
  service: 'web',
  image: 'nginx:stable',
  state: 'running',
  health: 'none',
  imageIdentity: identity(),
})
const wrappers: VueWrapper[] = []
let write: ReturnType<typeof vi.fn>
beforeEach(() => {
  setActivePinia(createPinia())
  write = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText: write } })
})
afterEach(() => {
  for (const w of wrappers.splice(0)) w.unmount()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
function renderIdentity(value?: Identity) {
  const w = mount(ImageIdentity, { props: { image: 'nginx:stable', identity: value } })
  wrappers.push(w)
  return w
}
function renderInspector() {
  const store = useConsoleStore()
  store.runtime['apps/web'] = {
    containers: [container()],
    observedAt: '2026-10-23T11:59:59Z',
    loading: false,
    error: null,
  }
  store.inspectContainer('apps/web', 'container-exact-id')
  const trigger = document.createElement('button')
  document.body.append(trigger)
  trigger.focus()
  const w = mount(ContainerInspector, {
    props: {
      selection: store.inspectedContainer,
      current: store.inspectedContainerCurrent,
      comparison: store.inspectedContainerIdentityComparison,
      runtime: store.inspectedContainerRuntime,
      removed: store.inspectedContainerRemoved,
      trigger,
    },
    global: {
      plugins: [PrimeVue],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        Drawer: {
          props: ['visible', 'header'],
          template: '<section v-if="visible"><h2>{{ header }}</h2><slot /></section>',
        },
      },
    },
  })
  wrappers.push(w)
  return { w, store, trigger }
}
describe('repository image identity', () => {
  it('splits reference and digest into table cells without losing exact copying or accessible unavailable reasons', async () => {
    const referenceCell = mount(ImageIdentity, {
      props: { image: 'nginx:stable', identity: identity(), part: 'reference' },
    })
    const digestCell = mount(ImageIdentity, {
      props: { image: 'nginx:stable', identity: identity(), part: 'digest' },
    })
    wrappers.push(referenceCell, digestCell)
    expect(referenceCell.text()).toBe('nginx:stable')
    expect(referenceCell.find('button').exists()).toBe(false)
    expect(digestCell.text()).not.toContain('nginx:stable')
    expect(digestCell.get('code').attributes('aria-label')).toBe(digest)
    expect(digestCell.get('button').attributes('aria-label')).toBe(
      'Copy repository digest for nginx:stable',
    )
    expect(digestCell.get('.lucide-copy').attributes('aria-hidden')).toBe('true')
    await digestCell.get('button').trigger('click')
    expect(write).toHaveBeenCalledWith(digest)
    await digestCell.setProps({
      identity: { ...identity(), status: 'ambiguous', preferredRepositoryDigest: undefined },
    })
    expect(digestCell.get('[aria-label="Repository digest ambiguous"]').text()).toBe('—')
    expect(digestCell.find('button').exists()).toBe(false)
    await digestCell.setProps({ identity: undefined })
    expect(digestCell.get('[aria-label="Digest unavailable"]').text()).toBe('—')
  })
  it('copies the full digest, exposes full accessible text, and labels it as repository identity', async () => {
    const w = renderIdentity(identity())
    expect(w.text()).toContain('Repository digest')
    expect(w.get('code').attributes('aria-label')).toBe(digest)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(write).toHaveBeenCalledWith(digest)
    expect(w.get('[role="status"]').text()).toBe('Full repository digest copied')
    expect(w.text()).not.toContain('local-secret-id')
  })
  it('never substitutes a local image or container ID for unavailable or ambiguous digest', () => {
    for (const value of [
      undefined,
      {
        ...identity(),
        status: 'unavailable',
        reason: 'not_recorded',
        preferredRepositoryDigest: undefined,
      } as Identity,
      {
        ...identity(),
        status: 'ambiguous',
        reason: 'repository_ambiguous',
        preferredRepositoryDigest: undefined,
      } as Identity,
    ]) {
      const w = renderIdentity(value)
      expect(w.find('button').exists()).toBe(false)
      expect(w.text()).not.toContain('local-secret-id')
      expect(w.text()).toMatch(/Digest unavailable|Repository digest ambiguous/)
    }
  })
  it('announces copy failure without replacing displayed identity', async () => {
    write.mockRejectedValue(new Error('Denied'))
    const w = renderIdentity(identity())
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('[role="status"]').text()).toContain('Copy unavailable')
    expect(w.get('code').attributes('aria-label')).toBe(digest)
  })
})
describe('pinned image inspection', () => {
  it('uses the modal drawer keyboard contract, closes on Escape and restores a removed trigger to its stack', async () => {
    const { store, trigger } = renderInspector()
    const heading = document.createElement('h3')
    heading.tabIndex = -1
    heading.dataset.inventoryFocus = 'stack:apps/web'
    document.body.append(heading)
    const w = mount(ContainerInspector, {
      attachTo: document.body,
      props: {
        selection: store.inspectedContainer,
        current: store.inspectedContainerCurrent,
        trigger,
      },
      global: {
        plugins: [PrimeVue],
        stubs: {
          transition: false,
          RouterLink: { template: '<a href="/stacks/apps/web"><slot /></a>' },
        },
      },
    })
    wrappers.push(w)
    await flushPromises()
    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    expect(dialog?.getAttribute('aria-label')).toContain('web-1 in apps/web')
    document.dispatchEvent(new KeyboardEvent('keydown', { code: 'Escape', key: 'Escape' }))
    expect(w.emitted('close')).toHaveLength(1)
    trigger.remove()
    await w.setProps({ selection: null, current: null })
    await vi.waitFor(() => expect(document.querySelector('[role="dialog"]')).toBeNull())
    await vi.waitFor(() => expect(document.activeElement).toBe(heading))
  })
  it('shows kind/platform/provenance/not-checked freshness with technical IDs in disclosure only', async () => {
    const { w } = renderInspector()
    expect(w.text()).toContain('linux/arm64/v8')
    expect(w.text()).toContain('Unknown (reported by local daemon)')
    expect(w.text()).toContain('Local container → immutable image inspection')
    expect(w.text()).toContain('Selected runtime observation2026-10-23T11:59:59Z')
    expect(w.text()).toContain('Image inspected2026-10-23T12:00:00Z')
    expect(w.text()).toContain('Registry freshnessNot checked')
    const technical = w.findAll('details').find((d) => d.text().includes('Technical identifiers'))
    if (!technical) throw new Error('Technical identifiers disclosure missing')
    expect(technical.attributes('open')).toBeUndefined()
    expect(technical.text()).toContain('Local image IDsha256:local-secret-id')
    expect(technical.text()).toContain('container-exact-id')
    await w
      .findAll('button')
      .find((b) => b.text() === 'Copy reference')
      ?.trigger('click')
    await flushPromises()
    expect(write).toHaveBeenCalledWith(reference)
  })
  it('uses anchored selected metadata and explicit same-ID degradation flags, with live health', async () => {
    const { w, store } = renderInspector()
    store.runtime['apps/web'] = {
      containers: [
        {
          ...container(),
          image: 'nginx:moved',
          health: 'unhealthy',
          imageIdentity: {
            ...identity(),
            status: 'unavailable',
            preferredRepositoryDigest: undefined,
            reason: 'inspection_failed',
          } as Identity,
        },
      ],
      observedAt: '2026-10-23T12:01:00Z',
      loading: false,
      error: null,
    }
    await w.setProps({
      current: store.inspectedContainerCurrent,
      comparison: store.inspectedContainerIdentityComparison,
      runtime: store.inspectedContainerRuntime,
    })
    expect(w.text()).toContain('Current image metadata is unavailable')
    expect(w.text()).toContain('reported image reference has changed')
    expect(w.text()).toContain('Runtime: running · unhealthy')
    expect(w.text()).toContain(reference)
    expect(w.text()).not.toContain('nginx:moved')
  })
  it('retains the dated identity after removal rather than switching to a same-name replacement', async () => {
    const { w, store } = renderInspector()
    store.runtime['apps/web'] = {
      containers: [{ ...container(), id: 'replacement', image: 'new:tag' }],
      observedAt: '2026-10-23T12:02:00Z',
      loading: false,
      error: null,
    }
    await w.setProps({
      current: store.inspectedContainerCurrent,
      comparison: store.inspectedContainerIdentityComparison,
      removed: store.inspectedContainerRemoved,
    })
    expect(w.text()).toContain('Container no longer observed')
    expect(w.text()).toContain(reference)
    expect(w.text()).not.toContain('new:tag')
  })
  it('offers all ambiguous digests without guessing a preferred reference or digest kind', async () => {
    const { w, store } = renderInspector()
    const next = {
      ...identity(),
      status: 'ambiguous',
      reason: 'repository_ambiguous',
      preferredRepositoryDigest: undefined,
      repositoryDigests: [
        ...identity().repositoryDigests,
        { repository: 'registry.example/nginx', digest: 'sha256:other', kind: 'manifest' },
      ],
    } as Identity
    const selected = store.runtime['apps/web']?.containers[0]
    if (!selected) throw new Error('Fixture container missing')
    selected.imageIdentity = next
    store.inspectContainer('apps/web', 'container-exact-id')
    await w.setProps({
      selection: store.inspectedContainer,
      current: store.inspectedContainerCurrent,
    })
    expect(w.text()).toContain('Ambiguous — no preferred repository digest')
    expect(w.text()).toContain('Show other reported digests (2)')
    expect(w.findAll('button').some((b) => b.text() === 'Copy reference')).toBe(false)
    expect(w.text()).toContain('Unknown (reported by local daemon)')
  })
})
