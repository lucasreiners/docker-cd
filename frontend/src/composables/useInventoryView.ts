import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { type LocationQuery, routeLocationKey, routerKey } from 'vue-router'
import type { ContainerInfo, StackRecord } from '../services/api'
import type { RuntimeView } from '../store/console'

export type InventoryQuickFilter = 'all' | 'attention' | 'active'
export type InventoryRuntimeFilter = 'all' | 'healthy' | 'attention' | 'no-checks' | 'unknown'
export type InventoryDeploymentFilter = 'all' | StackRecord['status'] | 'approval'
export interface InventoryDisclosure {
  mode: 'hidden' | 'preview' | 'full'
  lastOpen: 'preview' | 'full'
  limit: number
}
interface SavedVisit {
  disclosure: Record<string, InventoryDisclosure>
  order: Record<string, string[]>
  scroll: number
  focus: string
  anchor?: string
}
export interface InventoryEntry {
  stack: StackRecord
  runtime: RuntimeView
  reason: string
  childOnly: boolean
  matches: ContainerInfo[]
}
const compare = (a: string, b: string) => a.localeCompare(b)
export function containerPriority(container: ContainerInfo): number {
  return container.state !== 'running'
    ? 0
    : ['unhealthy', 'starting'].includes(container.health)
      ? 1
      : 2
}
export function compareContainers(a: ContainerInfo, b: ContainerInfo): number {
  return (
    containerPriority(a) - containerPriority(b) ||
    compare(a.service, b.service) ||
    compare(a.name, b.name) ||
    compare(a.id, b.id)
  )
}
export function containerMatches(container: ContainerInfo, search: string): boolean {
  return [
    container.name,
    container.service,
    container.image,
    ...(container.imageIdentity?.repositoryDigests.flatMap((digest) => [
      digest.digest,
      `${digest.repository}@${digest.digest}`,
    ]) ?? []),
  ].some((value) => value.toLowerCase().includes(search))
}
export function runtimeUnknown(runtime: RuntimeView): boolean {
  return !!runtime.error || runtime.stale || !runtime.observedAt || runtime.loading
}
export function inventoryAttention(stack: StackRecord, runtime: RuntimeView): string {
  const reasons: string[] = []
  if (stack.requiresAcknowledgement) reasons.push('Approval required')
  if (stack.status === 'failed') reasons.push('Deployment failed')
  if (stack.status === 'missing') reasons.push('Not deployed yet')
  if (runtimeUnknown(runtime)) reasons.push('Runtime unknown')
  else if (
    runtime.containers.some(
      (c) => c.state !== 'running' || ['unhealthy', 'starting'].includes(c.health),
    )
  )
    reasons.push('Runtime needs attention')
  return reasons.join(' · ')
}
function matchesRuntime(runtime: RuntimeView, filter: InventoryRuntimeFilter): boolean {
  if (filter === 'all') return true
  if (filter === 'unknown') return runtimeUnknown(runtime)
  if (runtimeUnknown(runtime)) return false
  const rows = runtime.containers
  if (filter === 'no-checks') return rows.some((c) => !c.health || c.health === 'none')
  if (filter === 'attention') return rows.some((c) => containerPriority(c) < 2)
  return rows.length > 0 && rows.every((c) => c.state === 'running' && c.health === 'healthy')
}
const initialDisclosure = (): InventoryDisclosure => ({
  mode: 'preview',
  lastOpen: 'preview',
  limit: 20,
})
const first = (query: LocationQuery, key: string) =>
  String((Array.isArray(query[key]) ? query[key]?.[0] : query[key]) || '')

/** UI context only. Data and observation scheduling remain owned by the console/work stores. */
export function useInventoryView(options: {
  stacks: () => StackRecord[]
  runtimeFor: (path: string) => RuntimeView
  activePaths: () => readonly string[]
  syncHistory?: boolean
}) {
  const router = inject(routerKey, undefined)
  const route = inject(routeLocationKey, undefined)
  const inventoryPath = route?.path
  const searchInput = ref('')
  const search = ref('')
  const quick = ref<InventoryQuickFilter>('all')
  const deployment = ref<InventoryDeploymentFilter>('all')
  const runtime = ref<InventoryRuntimeFilter>('all')
  const group = ref(false)
  const page = ref(1)
  const disclosure = reactive<Record<string, InventoryDisclosure>>(Object.create(null))
  const matchingDisclosure = reactive<Record<string, InventoryDisclosure & { all?: boolean }>>(
    Object.create(null),
  )
  const order = reactive<Record<string, string[]>>(Object.create(null))
  let debounce: ReturnType<typeof setTimeout> | undefined
  let readingQuery = false
  const historyEnabled = !!router && !!route && options.syncHistory !== false
  const context = computed(() =>
    JSON.stringify([
      search.value,
      quick.value,
      deployment.value,
      runtime.value,
      group.value,
      page.value,
    ]),
  )
  const storageKey = (value = context.value) => `docker-cd-inventory:/stacks:${value}`
  function save(value = context.value) {
    const focused = document.activeElement?.getAttribute('data-inventory-focus') || ''
    const saved: SavedVisit = {
      disclosure: { ...disclosure },
      order: { ...order },
      scroll: window.scrollY,
      focus: focused,
      anchor: document.activeElement?.closest('[data-stack]')?.getAttribute('data-stack') || '',
    }
    try {
      sessionStorage.setItem(storageKey(value), JSON.stringify(saved))
    } catch {
      /* Session storage can be disabled. */
    }
  }
  async function restore() {
    for (const key of Object.keys(order)) delete order[key]
    for (const key of Object.keys(matchingDisclosure)) delete matchingDisclosure[key]
    let saved: SavedVisit | undefined
    try {
      saved = JSON.parse(sessionStorage.getItem(storageKey()) || 'null') ?? undefined
    } catch {
      /* Ignore invalid/disabled storage. */
    }
    if (!saved || typeof saved !== 'object') {
      seedOrder(visible.value)
      return
    }
    for (const key of Object.keys(disclosure)) delete disclosure[key]
    for (const [path, value] of Object.entries(saved.disclosure ?? {})) {
      if (
        value &&
        ['hidden', 'preview', 'full'].includes(value.mode) &&
        ['preview', 'full'].includes(value.lastOpen)
      ) {
        disclosure[path] = {
          ...value,
          limit: Math.max(20, Math.min(10000, Number(value.limit) || 20)),
        }
      }
    }
    for (const [path, value] of Object.entries(saved.order ?? {}))
      if (Array.isArray(value) && value.every((id) => typeof id === 'string')) order[path] = value
    seedOrder(visible.value)
    await nextTick()
    if (saved.focus) {
      const targets = Array.from(document.querySelectorAll<HTMLElement>('[data-inventory-focus]'))
      const target =
        targets.find((node) => node.dataset.inventoryFocus === saved.focus) ||
        targets.find((node) => node.dataset.inventoryFocus === `stack:${saved.anchor}`) ||
        targets.find((node) => node.dataset.inventoryFocus === 'inventory-search')
      target?.focus({ preventScroll: true })
    }
    if (Number.isFinite(saved.scroll) && saved.scroll > 0)
      window.scrollTo({ top: saved.scroll, behavior: 'instant' })
  }
  const entries = computed<InventoryEntry[]>(() =>
    options
      .stacks()
      .slice()
      .sort((a, b) => compare(a.path, b.path))
      .flatMap((stack) => {
        const state = options.runtimeFor(stack.path)
        const reason = inventoryAttention(stack, state)
        if (quick.value === 'attention' && !reason) return []
        if (quick.value === 'active' && !options.activePaths().includes(stack.path)) return []
        if (
          deployment.value === 'approval'
            ? !stack.requiresAcknowledgement
            : deployment.value !== 'all' && stack.status !== deployment.value
        )
          return []
        if (!matchesRuntime(state, runtime.value)) return []
        const stackMatch = !search.value || stack.path.toLowerCase().includes(search.value)
        const matches = search.value
          ? state.containers.filter((c) => containerMatches(c, search.value))
          : []
        if (!stackMatch && !matches.length) return []
        return [{ stack, runtime: state, reason, childOnly: !stackMatch, matches }]
      }),
  )
  const pageCount = computed(() => Math.max(1, Math.ceil(entries.value.length / 20)))
  const currentPage = computed(() => Math.min(page.value, pageCount.value))
  const visible = computed(() =>
    entries.value.slice((currentPage.value - 1) * 20, currentPage.value * 20),
  )
  function seedOrder(rows: InventoryEntry[]) {
    for (const row of rows) {
      if (!row.runtime.observedAt) continue
      const ids = order[row.stack.path] ?? []
      const seen = new Set(ids)
      const added = row.runtime.containers
        .filter((c) => !seen.has(c.id))
        .sort(compareContainers)
        .map((c) => c.id)
      if (!order[row.stack.path] || added.length) order[row.stack.path] = [...ids, ...added]
    }
  }
  watch(visible, seedOrder, { immediate: true })
  function stateFor(entry: InventoryEntry) {
    if (entry.childOnly) return matchingDisclosure[entry.stack.path] ?? initialDisclosure()
    return disclosure[entry.stack.path] ?? initialDisclosure()
  }
  function setState(entry: InventoryEntry, next: InventoryDisclosure & { all?: boolean }) {
    if (entry.childOnly) matchingDisclosure[entry.stack.path] = next
    else disclosure[entry.stack.path] = next
    save()
  }
  function children(entry: InventoryEntry): ContainerInfo[] {
    const state = stateFor(entry)
    if (state.mode === 'hidden') return []
    const rows =
      entry.childOnly && !matchingDisclosure[entry.stack.path]?.all
        ? entry.matches
        : entry.runtime.containers
    const ranks = new Map((order[entry.stack.path] ?? []).map((id, index) => [id, index]))
    return rows
      .slice()
      .sort(
        (a, b) =>
          (ranks.get(a.id) ?? Infinity) - (ranks.get(b.id) ?? Infinity) || compareContainers(a, b),
      )
      .slice(0, state.mode === 'preview' ? 2 : state.limit)
  }
  function toggle(entry: InventoryEntry) {
    const state = stateFor(entry)
    setState(entry, {
      ...state,
      mode: state.mode === 'hidden' ? state.lastOpen : 'hidden',
      lastOpen: state.mode === 'hidden' ? state.lastOpen : state.mode,
    })
  }
  function reveal(entry: InventoryEntry, all = false) {
    const state = stateFor(entry)
    const exitsMatching = entry.childOnly && all && !matchingDisclosure[entry.stack.path]?.all
    setState(entry, {
      ...state,
      mode: 'full',
      lastOpen: 'full',
      limit: state.mode === 'full' && !exitsMatching ? state.limit + 20 : 20,
      all: all || matchingDisclosure[entry.stack.path]?.all,
    })
  }
  function fewer(entry: InventoryEntry) {
    setState(entry, { ...stateFor(entry), mode: 'preview', lastOpen: 'preview', limit: 20 })
  }
  function pageDisclosure(hide: boolean) {
    for (const entry of visible.value)
      setState(entry, {
        ...stateFor(entry),
        mode: hide ? 'hidden' : 'preview',
        lastOpen: hide ? stateFor(entry).lastOpen : 'preview',
      })
  }
  function clearFilters() {
    clearTimeout(debounce)
    searchInput.value = ''
    search.value = ''
    quick.value = 'all'
    deployment.value = 'all'
    runtime.value = 'all'
    page.value = 1
  }
  function writeQuery(replace = false) {
    if (!historyEnabled || readingQuery || route.path !== inventoryPath) return
    const query = { ...route.query }
    for (const key of ['q', 'filter', 'deployment', 'runtime', 'group', 'page', 'status', 'view'])
      delete query[key]
    if (search.value) query.q = search.value
    if (quick.value !== 'all') query.filter = quick.value
    if (deployment.value !== 'all') query.deployment = deployment.value
    if (runtime.value !== 'all') query.runtime = runtime.value
    if (group.value) query.group = 'directory'
    if (page.value > 1) query.page = String(page.value)
    const normalized = (value: LocationQuery) =>
      JSON.stringify(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)))
    if (normalized(query) !== normalized(route.query))
      void router[replace ? 'replace' : 'push']({ query })
  }
  function readQuery() {
    if (!historyEnabled || route.path !== inventoryPath) return
    readingQuery = true
    const query = route.query
    search.value = first(query, 'q').trim().toLowerCase()
    searchInput.value = first(query, 'q')
    const filter =
      first(query, 'filter') || (first(query, 'status') === 'attention' ? 'attention' : '')
    quick.value = filter === 'attention' || filter === 'active' ? filter : 'all'
    const dep =
      first(query, 'deployment') ||
      (['syncing', 'failed', 'missing', 'synced', 'deleting'].includes(first(query, 'status'))
        ? first(query, 'status')
        : '')
    deployment.value = ['missing', 'syncing', 'synced', 'deleting', 'failed', 'approval'].includes(
      dep,
    )
      ? (dep as InventoryDeploymentFilter)
      : 'all'
    const rt =
      first(query, 'runtime') || (first(query, 'status') === 'unchecked' ? 'no-checks' : '')
    runtime.value = ['healthy', 'attention', 'no-checks', 'unknown'].includes(rt)
      ? (rt as InventoryRuntimeFilter)
      : 'all'
    group.value = ['directory', 'path'].includes(first(query, 'group'))
    page.value = Math.max(1, Math.floor(Number(first(query, 'page')) || 1))
    readingQuery = false
  }
  if (historyEnabled) readQuery()
  watch(searchInput, () => {
    clearTimeout(debounce)
    debounce = setTimeout(() => {
      search.value = searchInput.value.trim().toLowerCase()
    }, 200)
  })
  // A filter/display change must not navigate using the previous debounced
  // query and overwrite the operator's still-pending search input.
  watch(
    [quick, deployment, runtime, group],
    () => {
      if (readingQuery) return
      clearTimeout(debounce)
      search.value = searchInput.value.trim().toLowerCase()
    },
    { flush: 'sync' },
  )
  watch(
    [search, quick, deployment, runtime, group],
    () => {
      if (readingQuery) return
      page.value = 1
      for (const key of Object.keys(matchingDisclosure)) delete matchingDisclosure[key]
    },
    { flush: 'sync' },
  )
  watch(context, async (_value, previous) => {
    save(previous)
    writeQuery()
    await restore()
  })
  if (historyEnabled) watch(() => route.fullPath, readQuery)
  const saveOnLeave = () => save()
  const removeGuard = historyEnabled
    ? router.beforeEach((to, from) => {
        if (to.path !== from.path) save()
        return true
      })
    : undefined
  onMounted(() => {
    writeQuery(true)
    void restore()
    window.addEventListener('pagehide', saveOnLeave)
  })
  onBeforeUnmount(() => {
    // The route guard captured the outgoing inventory before focus/scroll moved.
    // Teardown on the destination route must not overwrite that return context.
    if (!historyEnabled || route.path === inventoryPath) save()
    clearTimeout(debounce)
    window.removeEventListener('pagehide', saveOnLeave)
    removeGuard?.()
  })
  return {
    searchInput,
    search,
    quick,
    deployment,
    runtime,
    group,
    page,
    pageCount,
    currentPage,
    entries,
    visible,
    stateFor,
    children,
    toggle,
    reveal,
    fewer,
    pageDisclosure,
    clearFilters,
    matchingDisclosure,
    save,
  }
}
