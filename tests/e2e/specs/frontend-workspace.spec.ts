import { expect, test as base, type APIRequestContext, type Page, type Request, type TestInfo } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { operationalWorkspaceFixtures, workspaceBrowserFixture } from '../fixtures/frontend-data'
import type { WorkRun } from '../../../frontend/src/services/work'

const api = 'http://127.0.0.1:18082'
const catalogue = operationalWorkspaceFixtures()
const test = base.extend<{ expectedHttpErrors: string[] }>({
  expectedHttpErrors: async ({}, use) => { await use([]) },
})
async function control(request: APIRequestContext, path: string, data?: unknown) {
  const response = await request.post(`${api}/test/${path}`, { data })
  expect(response.ok()).toBeTruthy()
}
async function event(request: APIRequestContext, data: unknown, name = 'work.upsert') {
  await control(request, 'event', { event: name, data })
}
async function metrics(request: APIRequestContext) { return (await request.get(`${api}/test/metrics`)).json() }
const browserRuntime = new WeakMap<Page, { requests: number; inFlight: number; maxInFlight: number }>()
async function capture(page: Page, info: TestInfo, name: string) {
  await page.evaluate(() => document.fonts.ready)
  await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'instant' }))
  const path = info.outputPath(`${name}.png`)
  await page.screenshot({ path, fullPage: true, animations: 'disabled' })
  await info.attach(name, { path, contentType: 'image/png' })
}
async function evidence(info: TestInfo, name: string, value: unknown) {
  const path = info.outputPath(`${name}.json`)
  await writeFile(path, JSON.stringify(value, null, 2))
  await info.attach(name, { path, contentType: 'application/json' })
}
const workRegion = (page: Page) => page.getByRole('region', { name: 'Instance work', exact: true })
const runRegion = (page: Page, run: WorkRun) => page.getByRole('region', { name: `Progress for ${run.stack || 'instance'} run ${run.runId}`, exact: true })

test.beforeEach(async ({ request, page, expectedHttpErrors }) => {
  await control(request, 'reset')
  await control(request, 'configure', { v2: true })
  // Production injects this at container startup; Vite fixtures use same-origin API.
  await page.route('**/config.js', route => route.fulfill({ contentType: 'application/javascript', body: 'window.__DOCKER_CD_CONFIG__ = { API_BASE_URL: "" };' }))
  const runtime = { requests: 0, inFlight: 0, maxInFlight: 0 }
  const pendingRuntime = new Set<Request>()
  browserRuntime.set(page, runtime)
  page.on('request', request => {
    if (new URL(request.url()).pathname !== '/api/runtime') return
    pendingRuntime.add(request)
    runtime.requests++; runtime.inFlight++
    runtime.maxInFlight = Math.max(runtime.maxInFlight, runtime.inFlight)
  })
  const finishRuntime = (request: Request) => { if (pendingRuntime.delete(request)) runtime.inFlight-- }
  page.on('requestfinished', finishRuntime)
  page.on('requestfailed', finishRuntime)
  const pageErrors: string[] = [], consoleErrors: string[] = [], httpErrors: string[] = [], transportErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') consoleErrors.push(message.text()) })
  page.on('response', response => { if (response.status() >= 400) httpErrors.push(`${new URL(response.url()).pathname}:${response.status()}`) })
  page.on('requestfailed', request => {
    if (request.failure()?.errorText !== 'net::ERR_ABORTED') transportErrors.push(`transport:${new URL(request.url()).pathname}`)
  })
  // Every intentional failure must be declared by the test and asserted in the UI.
  diagnosticResults.set(page, () => {
    expect(pageErrors, 'uncaught browser errors').toEqual([])
    expect(httpErrors.filter(error => !expectedHttpErrors.includes(error)), 'unexpected HTTP errors').toEqual([])
    expect(transportErrors.filter(error => !expectedHttpErrors.includes(error)), 'unexpected transport errors').toEqual([])
    expect(consoleErrors.filter(error => !/^Failed to load resource:/.test(error)), 'application console errors').toEqual([])
    expect(consoleErrors.length, 'resource errors must have an expected failing response').toBeLessThanOrEqual(httpErrors.length + transportErrors.length)
  })
})
const diagnosticResults = new WeakMap<Page, () => void>()
for (const width of [1440, 320]) test(`native inventory table geometry and visual system ${width}`, async ({ page, request, context }, info) => {
  const state = workspaceBrowserFixture()
  const stacks = state.stacks.slice(0, 4).map((stack, index) => ({ ...stack, status: index === 0 ? 'failed' : index === 3 ? 'syncing' : 'synced', requiresAcknowledgement: index === 1 }))
  const source = state.containers['apps/immich']
  const containers = Object.fromEntries(stacks.map((stack, index) => [stack.path, index === 3 ? [] : [{ ...source[0], id: `visual-${index}`, name: ['web', 'worker', 'database'][index], health: ['healthy', 'unhealthy', 'none'][index], state: index === 2 ? 'exited' : 'running' }]]))
  await control(request, 'configure', { stacks, containers, work: catalogue.reconnect, operations: [], observedAt: new Date().toISOString() })
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width, height: 900 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/stacks')
  await expect(page.locator('table.inventory-table > tbody[data-stack]')).toHaveCount(4)
  await expect(page.locator('tr[data-container]')).toHaveCount(3)
  await expect(page.locator('table.inventory-table thead th')).toHaveText(['Resource', 'State', 'Image', 'Digest', 'Actions'])
  for (const cell of await page.locator('.inventory-stack-image').all()) await expect(cell).toBeEmpty()
  await page.evaluate(() => document.fonts.ready)
  const geometry = await page.locator('.inventory-table').evaluate(table => {
    const bounds = (element: Element) => { const rect = element.getBoundingClientRect(); return { x: rect.x, width: rect.width, height: rect.height } }
    const typography = (selector: string) => Array.from(table.querySelectorAll(selector)).map(element => { const style = getComputedStyle(element); return { family: style.fontFamily, size: style.fontSize, weight: style.fontWeight } })
    return {
      headers: Array.from(table.querySelectorAll('thead th')).map(bounds),
      stacks: Array.from(table.querySelectorAll('.inventory-stack-header')).map(row => ({ ...bounds(row), cells: Array.from(row.children).map(bounds) })),
      children: Array.from(table.querySelectorAll('tr[data-container]')).map(row => ({ ...bounds(row), cells: Array.from(row.children).map(bounds) })),
      names: typography('.inventory-name'), technical: typography('.inventory-revision code, .image-reference, .image-digest code'),
      colors: Array.from(table.querySelectorAll('.inventory-status')).map(element => ({ label: element.textContent, color: getComputedStyle(element).color, icon: !!element.querySelector('svg'), animation: getComputedStyle(element.querySelector('svg')!).animationName })),
      pageWidth: document.documentElement.scrollWidth, bodyHeight: document.body.scrollHeight,
    }
  })
  for (const row of [...geometry.stacks, ...geometry.children]) row.cells.forEach((cell, index) => {
    expect(Math.abs(cell.x - geometry.headers[index].x), 'native shared column left edges').toBeLessThanOrEqual(1)
    expect(Math.abs(cell.width - geometry.headers[index].width), 'native shared column widths').toBeLessThanOrEqual(1)
  })
  if (width === 1440) {
    for (const row of geometry.stacks) expect(row.height).toBeLessThanOrEqual(48)
    for (const row of geometry.children) expect(row.height).toBeLessThanOrEqual(44)
    expect(geometry.bodyHeight).toBeLessThanOrEqual(900)
  }
  expect(geometry.pageWidth).toBeLessThanOrEqual(width)
  for (const name of geometry.names) { expect(name.family).toContain('DM Sans'); expect(Number(name.weight)).toBeLessThanOrEqual(600); expect(name.size).toBe('13px') }
  for (const technical of geometry.technical) { expect(technical.family).toContain('IBM Plex Mono'); expect(technical.size).toBe('12px') }
  expect(geometry.colors.every(status => status.icon)).toBe(true)
  expect(new Set(geometry.colors.map(status => status.color)).size).toBeGreaterThanOrEqual(5)
  expect(geometry.colors.every(status => status.animation === 'none')).toBe(true)
  await expect(page.locator('.inventory-stack-header .inventory-status--danger')).toContainText('Failed')
  await expect(page.locator('.inventory-stack-header .inventory-status--warning')).toContainText('Approval required')
  await expect(page.locator('.inventory-stack-header .inventory-status--active')).toContainText('Applying')
  await expect(page.locator('.inventory-stack-header .inventory-status--success')).toContainText('Applied')
  await expect(page.locator('tr[data-container] .inventory-status--danger')).toContainText('unhealthy')
  await expect(page.locator('tr[data-container] .inventory-status--neutral')).toContainText('Exited')
  const copy = page.getByRole('button', { name: /Copy repository digest for/ }).first()
  await copy.click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(/^sha256:[a-f0-9]{64}$/)
  const directory = resolve('../../.lead/_adhoc/stacks-table-visual')
  await mkdir(directory, { recursive: true })
  await page.locator('.inventory-table-scroll').evaluate(element => { element.scrollLeft = 0 })
  await page.screenshot({ path: resolve(directory, `after-fixture-${width}.png`), fullPage: true, animations: 'disabled' })
  if (width === 320) {
    await page.locator('.inventory-table-scroll').evaluate(element => { element.scrollLeft = element.scrollWidth })
    const review = await page.getByRole('link', { name: 'Review change', exact: true }).boundingBox()
    expect(review?.x).toBeGreaterThanOrEqual(0)
    expect((review?.x || 0) + (review?.width || 0)).toBeLessThanOrEqual(width)
    expect(review?.height).toBeGreaterThanOrEqual(44)
    expect((await copy.boundingBox())?.width).toBeGreaterThanOrEqual(44)
    await page.screenshot({ path: resolve(directory, 'after-fixture-320-actions.png'), fullPage: true, animations: 'disabled' })
  }
  await writeFile(resolve(directory, `geometry-${width}.json`), JSON.stringify(geometry, null, 2))
  await evidence(info, 'native-table-geometry', geometry)
})
for (const width of [1440, 390]) test(`compact global work geometry ${width}`, async ({ page, request }, info) => {
  await page.setViewportSize({ width, height: 1000 })
  await page.goto('/stacks')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  await expect(page.locator('[data-container]').first()).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
  if (width === 1440) {
    const childHeights = await page.locator('tr[data-container]').evaluateAll(rows => rows.map(row => row.getBoundingClientRect().height))
    expect(childHeights.length).toBeGreaterThan(3)
    for (const height of childHeights) expect(height, 'adjacent container rows stay within the same 44px rhythm').toBeLessThanOrEqual(44)
  }
  const active = await workRegion(page).boundingBox()
  expect(active?.height, 'closed global work stays a summary, including last result').toBeLessThanOrEqual(width === 1440 ? 260 : 270)
  await expect(workRegion(page).locator('.workflow-list > li:visible')).toHaveCount(width === 1440 ? 2 : 1)
  const disclosure = workRegion(page).locator('summary:visible').first()
  if (width === 390) expect((await disclosure.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  await capture(page, info, `compact-active-${width}`)
  await disclosure.focus()
  await page.keyboard.press('Enter')
  await expect(workRegion(page).locator('details[open]')).toHaveCount(1)
  await expect(workRegion(page)).toContainText('128 B / unknown')
  const expanded = await workRegion(page).boundingBox()
  expect(expanded?.height).toBeGreaterThan(active?.height || 0)
  await capture(page, info, `compact-expanded-${width}`)
  await expect.poll(() => browserRuntime.get(page)?.inFlight).toBe(0)
  await control(request, 'configure', { work: catalogue.reconnect, operations: [] })
  await page.reload()
  await expect(workRegion(page)).toContainText('No work in progress')
  await page.evaluate(() => document.fonts.ready)
  const idle = await workRegion(page).boundingBox()
  expect(idle?.height, 'idle work is one quiet line, not a zero-work card').toBeLessThanOrEqual(40)
  await capture(page, info, `compact-idle-${width}`)
  await evidence(info, 'compact-geometry', { width, active, expanded, idle })
  await expect.poll(() => browserRuntime.get(page)?.inFlight).toBe(0)
})
test.afterEach(async ({ page }) => {
  diagnosticResults.get(page)?.()
  expect(browserRuntime.get(page)?.maxInFlight, 'browser runtime requests never overlap').toBeLessThanOrEqual(1)
})

test('A/E: authoritative independent roots, phase bytes, applying, reconnect and restart', async ({ page, request, expectedHttpErrors }, info) => {
  expectedHttpErrors.push('/api/operations/old-deployment:404', '/api/operations/global-update:404')
  await control(request, 'configure', { operations: catalogue.latest50 })
  await page.goto('/')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  const child = catalogue.snapshot.runs[0], independent = catalogue.snapshot.runs[1]
  const childPanel = runRegion(page, child).last(), globalPanel = runRegion(page, independent).first()
  await childPanel.locator('summary').click()
  await globalPanel.locator('summary').click()
  await expect(childPanel).toContainText('128 B / unknown')
  await expect(globalPanel).toContainText('128 B / unknown')
  await expect(childPanel.locator('progress')).toHaveCount(0)
  const navigationStart = await page.evaluate(() => performance.timeOrigin)
  const before = Date.now()
  await event(request, catalogue.bytes)
  await expect(childPanel).toContainText('256 B / unknown')
  const byteLatencyMs = Date.now() - before
  await expect(globalPanel).toContainText('128 B / unknown')
  const extracting = { ...catalogue.bytes, sequence: 14, run: { ...catalogue.bytes.run!, images: catalogue.bytes.run!.images.map(image => ({ ...image, layers: image.layers.map(layer => ({ ...layer, phase: 'extracting', status: 'Extracting', bytesCurrent: 32, bytesTotal: 1024 })) })) } }
  await event(request, extracting)
  await expect(childPanel).toContainText('extracting · Extracting')
  await expect(childPanel).toContainText('32 B / 1.0 KiB')
  await expect(childPanel.getByRole('progressbar')).toHaveAttribute('value', '32')
  // Byte ticks must not mutate any live region or replace the focused control.
  // Finish independent runtime bootstrap first: identity copy live regions are
  // mounted with observed children, not caused by byte ticks.
  const previewChildren = Object.values(workspaceBrowserFixture().containers).reduce((count, rows) => count + Math.min(2, rows.length), 0)
  await expect(page.locator('[data-container]')).toHaveCount(previewChildren)
  await childPanel.locator('summary').focus()
  const announcements = await page.locator('[role="status"]').allTextContents()
  for (let sequence = 15; sequence < 35; sequence++) await event(request, { ...extracting, sequence, run: { ...extracting.run, images: extracting.run.images.map(image => ({ ...image, layers: image.layers.map(layer => ({ ...layer, bytesCurrent: sequence * 2 })) })) } })
  await expect(childPanel).toContainText('68 B / 1.0 KiB')
  await expect(childPanel.locator('summary')).toBeFocused()
  expect(await page.locator('[role="status"]').allTextContents()).toEqual(announcements)
  const typingStart = Date.now()
  const search = page.getByRole('searchbox', { name: 'Search stacks, containers, images', exact: true })
  await Promise.all([
    search.fill('immich'),
    (async () => { for (let sequence = 35; sequence < 55; sequence++) await event(request, { ...extracting, sequence }) })(),
  ])
  await expect(search).toHaveValue('immich')
  await expect(search).toBeFocused()
  await expect(page.locator('[data-stack]')).toHaveCount(1)
  const searchRenderMs = Date.now() - typingStart
  const applying: WorkRun = { ...child, phase: 'applying', message: 'Images pulled; applying Compose', images: child.images.map(image => ({ ...image, done: true, status: 'completed', layerCount: 0, layers: [] })) }
  await event(request, { schemaVersion: 2, epoch: catalogue.snapshot.epoch, sequence: 55, operations: [], run: applying })
  await expect(childPanel).toContainText('Applying Compose')
  await expect(childPanel).toContainText('Images pulled — deployment may still be applying or pruning')
  await expect(childPanel).not.toContainText('Run completed')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(navigationStart)
  await capture(page, info, 'v2-independent-applying')
  await page.locator('.sidebar').getByRole('link', { name: 'Repository', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Source configuration', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Open instance work' }).click()
  await expect(page.getByRole('dialog').getByRole('heading', { name: 'Work in progress 2' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.goto('/stacks')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  // Snapshot on the next subscription is a replacement, not a merge with old bytes.
  await control(request, 'configure', { work: { ...catalogue.snapshot, sequence: 60, runs: [independent], operations: [catalogue.snapshot.operations[2]] } })
  await control(request, 'disconnect')
  await expect(page.getByTestId('connection-banner')).toContainText('Reconnecting')
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
  await expect(page.getByTestId('connection-banner')).toHaveText('Connected')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 1')
  await expect(page.locator('[data-stack="apps/immich"]')).toContainText('Outcome not confirmed')
  await control(request, 'configure', { work: catalogue.restart })
  await control(request, 'disconnect')
  await expect(page.getByTestId('connection-banner')).toContainText('Reconnecting')
  await expect(page.getByTestId('connection-banner')).toHaveText('Connected')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 0')
  await event(request, { ...catalogue.bytes, sequence: 999 })
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 0')
  await evidence(info, 'event-render-measurement', { byteLatencyMs, searchRenderMs, ...(await metrics(request)), browserRuntime: browserRuntime.get(page) })
})

test('B: global controls ignore filters; Git interruption and uncertain POST are explicit', async ({ page, request, expectedHttpErrors }, info) => {
  expectedHttpErrors.push('transport:/api/update')
  expectedHttpErrors.push('/api/operations/old-deployment:404', '/api/operations/global-update:404')
  await page.goto('/stacks?q=does-not-exist')
  await expect(page.locator('[data-stack]')).toHaveCount(0)
  await expect(workRegion(page)).toContainText('Outside current view · Open stack')
  await page.getByRole('button', { name: 'Check Git for changes…', exact: true }).click()
  const interrupt = page.getByRole('dialog', { name: 'Interrupt work and check Git', exact: true })
  await expect(interrupt).toContainText('Requests interruption; work may already have made changes.')
  await expect(interrupt.locator('li')).toHaveCount(2)
  await interrupt.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect((await metrics(request)).actions).toEqual([])
  await control(request, 'configure', { work: catalogue.reconnect })
  await control(request, 'disconnect')
  await expect(page.getByTestId('connection-banner')).toContainText('Reconnecting')
  await expect(page.getByTestId('connection-banner')).toHaveText('Connected')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 0')
  await page.getByRole('button', { name: 'Update all stacks…', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Update all stacks', exact: true })
  await expect(dialog).toContainText('regardless of inventory filters or the current page')
  await expect(dialog).toContainText('including images outside managed stacks')
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect((await metrics(request)).actions).toEqual([])
  // Transport failure after server acceptance: there is no safe automatic replay.
  await page.route('**/api/update', async route => { await route.fetch(); await route.abort('connectionreset') })
  await page.getByRole('button', { name: 'Update all stacks…', exact: true }).click()
  await dialog.getByRole('button', { name: 'Update all stacks and prune images', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Request outcome unknown; it may still be running' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
  expect((await metrics(request)).actions).toEqual([{ path: '/api/update', body: null }])
  await capture(page, info, 'filtered-global-uncertain')
  await page.getByRole('link', { name: 'View operations', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Request outcome unknown' })).toBeVisible()
  await page.getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Request outcome unknown' })).toHaveCount(0)
  expect((await metrics(request)).actions).toEqual([{ path: '/api/update', body: null }])
})

test('A: scoped inline review sends one revision-bound action and keeps accepted work distinct', async ({ page, request }) => {
  await control(request, 'configure', { work: catalogue.reconnect, holdActions: true })
  await page.goto('/stacks?q=traefik')
  const row = page.locator('[data-stack="infra/traefik"]')
  await row.getByRole('button', { name: 'Update images…', exact: true }).click()
  await expect(row).toContainText('No image prune.')
  await row.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect((await metrics(request)).actions).toEqual([])
  await row.getByRole('button', { name: 'Update images…', exact: true }).click()
  await row.getByRole('button', { name: 'Update infra/traefik', exact: true }).click()
  await expect(page.locator('.operation-drawer')).toContainText('Waiting for execution lock')
  await page.keyboard.press('Escape')
  await expect(row).not.toContainText('Request accepted; awaiting execution')
  await expect(page.getByRole('button', { name: 'Clear local request notice', exact: true })).toHaveCount(0)
  await expect(row.locator('.request-warning')).toHaveCount(0)
  await expect(row).toContainText('Waiting for execution lock')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 1')
  expect((await metrics(request)).actions).toEqual([{ path: '/api/operations', body: { action: 'update', stack: 'infra/traefik', revision: workspaceBrowserFixture().refresh.revision } }])
  await expect(row.getByRole('button', { name: 'Update images…', exact: true })).toBeDisabled()
  await expect(row).not.toContainText('Deployment completed')
})

test('C: loaded approval revision cannot drift or be silently retried after rejection', async ({ page, request, expectedHttpErrors }) => {
  expectedHttpErrors.push('/api/operations:409')
  const state = workspaceBrowserFixture(), reviewed = state.refresh.revision
  await page.route('**/api/stack/configuration?*', async route => { const response = await route.fetch(); await route.fulfill({ response, json: { ...await response.json(), revision: reviewed } }) })
  await page.goto('/stacks/apps/paperless')
  await expect(page.getByRole('button', { name: 'Update images…', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'Review change', exact: true }).click()
  await expect(page.locator('.source-code')).toContainText('2026.10')
  await control(request, 'configure', { refresh: { ...state.refresh, revision: 'new-review-required' } })
  await page.getByRole('button', { name: `Approve & deploy ${reviewed}`, exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Desired revision changed; review the stack again' })).toBeVisible()
  expect((await metrics(request)).actions).toEqual([{ path: '/api/operations', body: { action: 'acknowledge', stack: 'apps/paperless', revision: reviewed } }])
  await expect(page.locator('main')).not.toContainText('Stack action completed')
})

test('D: digest copy, ambiguous/local identities and selected identity survive replacement', async ({ page, request, context }, info) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.goto('/stacks')
  const row = page.locator('[data-stack="apps/immich"]')
  const trigger = row.getByRole('button', { name: 'immich-web', exact: true })
  await trigger.click()
  const inspector = page.locator('.container-inspector')
  const fullReference = `docker.io/library/postgres@sha256:${'a'.repeat(64)}`
  await expect(inspector).toContainText(fullReference)
  await expect(inspector).toContainText('Unknown (reported by local daemon)')
  await expect(inspector).toContainText('linux/arm64/v8')
  await expect(inspector).toContainText('Not checked')
  await inspector.getByRole('button', { name: 'Copy reference', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fullReference)
  const state = workspaceBrowserFixture()
  await control(request, 'configure', { containers: { ...state.containers, 'apps/immich': state.containers['apps/immich'].map(container => ({ ...container, id: `replacement-${container.id}`, imageIdentity: catalogue.identities.multipleRepositories })) } })
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Refresh runtime', exact: true }).click()
  await trigger.click()
  // Select replacement explicitly, then remove it without closing the inspector.
  await control(request, 'configure', { containers: { ...state.containers, 'apps/immich': [] } })
  await event(request, { record: { ...state.stacks[0], containersTotal: 0 } }, 'stack.upsert')
  await expect(inspector).toContainText('Container no longer observed')
  await expect(inspector).toContainText(fullReference)
  await inspector.getByRole('button', { name: 'Copy reference', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fullReference)
  await capture(page, info, 'retained-selected-digest')
  await page.keyboard.press('Escape')
  await expect(row.locator('[data-inventory-focus="stack:apps/immich"]')).toBeFocused()
  await page.locator('[data-stack="infra/monitoring"]').getByRole('button', { name: 'monitoring-web', exact: true }).click()
  await expect(inspector).toContainText('Ambiguous — no preferred repository digest')
  await expect(inspector.getByRole('button', { name: 'Copy reference', exact: true })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await control(request, 'configure', { containers: state.containers })
  await page.getByRole('button', { name: 'Refresh runtime', exact: true }).click()
  await row.getByRole('button', { name: 'Show all 3 containers', exact: true }).click()
  await row.getByRole('button', { name: 'immich-worker', exact: true }).first().click()
  await expect(inspector).toContainText('Digest unavailable')
  await expect(inspector.getByRole('button', { name: 'Copy reference', exact: true })).toHaveCount(0)
})

test('C: observed revision change invalidates an open review before any approval POST', async ({ page, request }) => {
  const state = workspaceBrowserFixture()
  await page.route('**/api/stack/configuration?*', async route => {
    const response = await route.fetch()
    await route.fulfill({ response, json: { ...await response.json(), revision: state.refresh.revision } })
  })
  await page.goto('/stacks/apps/paperless?tab=configuration')
  const approve = page.getByRole('button', { name: `Approve & deploy ${state.refresh.revision}`, exact: true })
  await expect(approve).toBeEnabled()
  await event(request, { record: { ...state.stacks[1], desiredRevision: 'new-observed-revision' } }, 'stack.upsert')
  await expect(approve).toBeDisabled()
  await expect(page.locator('.stack-actions')).toContainText('Desired revision changed; review the stack again')
  expect((await metrics(request)).actions).toEqual([])
})

test('runtime stale/failure retains children; history failure does not suppress live work', async ({ page, request, expectedHttpErrors }, info) => {
  expectedHttpErrors.push('/api/runtime:503', '/api/operations:503')
  await control(request, 'configure', { observedAt: new Date(Date.now() - 60000).toISOString(), historyStatus: 503 })
  await page.goto('/stacks')
  const row = page.locator('[data-stack="apps/immich"]')
  await expect(row).toContainText('Runtime observation stale')
  await expect(row.locator('[data-container]')).toHaveCount(2)
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  await control(request, 'configure', { runtimeStatus: 503 })
  await page.getByRole('button', { name: 'Refresh runtime', exact: true }).click()
  await expect(row).toContainText('Runtime unavailable')
  await expect(row).toContainText('Showing last known containers')
  await expect(row.locator('[data-container]')).toHaveCount(2)
  await page.goto('/operations')
  await expect(page.getByText('Fixture history unavailable', { exact: false })).toBeVisible()
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  await capture(page, info, 'history-unavailable-live-work')
})

test('snapshot barrier keeps mutations disabled and real visible polling never overlaps burst refreshes', async ({ page, request }, info) => {
  await control(request, 'configure', { work: null, runtimeDelay: 500 })
  const runtimeStarts: number[] = []
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/runtime') runtimeStarts.push(Date.now()) })
  await page.goto('/stacks')
  await expect(workRegion(page)).toContainText('Restoring authoritative work state')
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
  await event(request, catalogue.reconnect, 'work.snapshot')
  await expect(page.getByTestId('connection-banner')).toHaveText('Connected')
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeEnabled()
  await expect.poll(async () => (await metrics(request)).runtimeRequests).toBeGreaterThanOrEqual(2)
  await expect.poll(async () => (await metrics(request)).runtimeInFlight).toBe(0)
  const bootstrapRequests = (await metrics(request)).runtimeRequests
  await expect.poll(async () => (await metrics(request)).runtimeRequests, { timeout: 18000 }).toBeGreaterThan(bootstrapRequests)
  const state = workspaceBrowserFixture()
  for (let index = 0; index < 12; index++) await event(request, { record: state.stacks[0] }, 'stack.upsert')
  await expect.poll(async () => (await metrics(request)).runtimeInFlight).toBe(0)
  const counts = await metrics(request)
  expect(counts.runtimeMaxInFlight).toBe(1)
  const intervalsMs = runtimeStarts.slice(1).map((time, index) => time - runtimeStarts[index])
  expect(Math.max(...intervalsMs)).toBeGreaterThanOrEqual(14000)
  await evidence(info, 'real-polling-measurements', { bootstrapRequests, runtimeStarts, intervalsMs, ...counts, browserRuntime: browserRuntime.get(page) })
})

test('work beyond previews is reachable in pages of ten, including accepted and no-image work', async ({ page, request }) => {
  const operations = Array.from({ length: 23 }, (_, index) => ({ ...catalogue.operationOnly.operations[0], id: `waiting-${index}`, rootOperationId: `waiting-${index}`, stack: index % 2 ? 'tools/linkding' : undefined, message: `Waiting for execution ${index}` }))
  await control(request, 'configure', { work: { ...catalogue.snapshot, runs: [], operations }, operations: catalogue.latest50 })
  await page.goto('/stacks')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 23')
  await expect(workRegion(page).locator('.workflow-list > li')).toHaveCount(2)
  await workRegion(page).getByRole('button', { name: 'Show all work (23)', exact: true }).click()
  await expect(workRegion(page).locator('.workflow-list > li')).toHaveCount(10)
  await workRegion(page).getByRole('button', { name: 'Next work', exact: true }).click()
  await expect(workRegion(page)).toContainText('Page 2 of 3')
  await workRegion(page).getByRole('button', { name: 'Next work', exact: true }).click()
  await expect(workRegion(page).locator('.workflow-list > li')).toHaveCount(3)
  await expect(workRegion(page).getByRole('button', { name: 'Next work', exact: true })).toBeDisabled()
  await expect(workRegion(page).locator('progress')).toHaveCount(0)
})

test('pull terminal is not workflow completion; pruning, partial results and held details remain separate', async ({ page, request }, info) => {
  await page.goto('/stacks')
  const panel = runRegion(page, catalogue.snapshot.runs[1]).first()
  await panel.locator('summary').click()
  await event(request, catalogue.terminal)
  await expect(panel).toContainText('Run completed · Runtime observation pending')
  await expect(panel.locator('details')).toHaveAttribute('open', '')
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 2')
  await event(request, catalogue.pruning)
  await expect(workRegion(page)).toContainText('Pruning unused images')
  await expect(workRegion(page)).toContainText('2 stacks processed · 1 skipped · 1 failed')
  await expect(page.locator('body')).not.toContainText('100%')
  const finishedAt = new Date(Date.parse(catalogue.pruning.run!.startedAt) + 60000).toISOString()
  const result = { ...catalogue.snapshot.operations[2], version: 2, status: 'completed_with_errors', finishedAt, message: 'Completed with one failed stack and one skipped stack' }
  await event(request, { ...result, steps: [] }, 'operation.upsert')
  await event(request, {
    ...catalogue.pruning, sequence: 16,
    run: { ...catalogue.pruning.run, status: 'failed', phase: 'finished', finishedAt, message: 'Prune failed; earlier stack outcomes retained' },
    operations: [result],
  })
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 1')
  await expect(workRegion(page)).toContainText('Completed with one failed stack and one skipped stack')
  await expect(panel.locator('details')).toHaveAttribute('open', '')
  // Replayed older bytes cannot revive the completed pull run.
  await event(request, { ...catalogue.bytes, sequence: 15, run: catalogue.snapshot.runs[1] })
  await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 1')
  await capture(page, info, 'partial-result-held-pull-details')
})

for (const status of ['cancelled', 'interrupted'] as const) {
  test(`server ${status} is terminal evidence, never an inferred success`, async ({ page, request }) => {
    await page.goto('/stacks')
    const run = catalogue.snapshot.runs[0]
    const panel = runRegion(page, run).last()
    await panel.locator('summary').click()
    const finishedAt = new Date(Date.parse(run.startedAt) + 60000).toISOString()
    const operation = { ...catalogue.snapshot.operations[1], version: 2, status, finishedAt, message: `Deployment ${status} by server`, steps: [] }
    await event(request, operation, 'operation.upsert')
    await event(request, { ...catalogue.bytes, run: { ...run, status: 'cancelled', phase: 'finished', finishedAt, message: operation.message }, operations: [operation] })
    await expect(panel).toContainText('Cancelled by server')
    await expect(panel.locator('details')).toHaveAttribute('open', '')
    await expect(page.locator('[data-stack="apps/immich"]')).toContainText(status === 'interrupted' ? 'Interrupted by server' : 'Cancelled by server')
    await expect(workRegion(page).getByRole('heading')).toHaveText('Work in progress 1')
    await expect(panel).not.toContainText('Run completed')
  })
}

test('250 stacks/250 containers bound initial DOM, retain focus and Back context, coalesce observations', async ({ page, request }, info) => {
  const state = workspaceBrowserFixture()
  const stacks = Array.from({ length: 250 }, (_, index) => ({ ...state.stacks[0], path: `bench/nested/service-${String(index).padStart(3, '0')}` }))
  const children = Array.from({ length: 250 }, (_, index) => ({ ...state.containers['apps/immich'][0], id: `container-${index}`, name: `observed-${String(index).padStart(3, '0')}`, service: `service-${index}` }))
  const containers = Object.fromEntries(stacks.map((stack, index) => [stack.path, index === 0 ? children : children.slice(0, 2).map(container => ({ ...container, id: `${index}-${container.id}` }))]))
  await control(request, 'configure', { stacks, containers, runtimeDelay: 120, work: catalogue.reconnect })
  const start = Date.now()
  await page.goto('/stacks?q=bench&deployment=synced&group=directory')
  await expect(page.locator('[data-stack]')).toHaveCount(20)
  await expect(page.locator('[data-container]')).toHaveCount(40)
  const initialRenderMs = Date.now() - start
  const initialDomNodes = await page.locator('*').count()
  const row = page.locator(`[data-stack="${stacks[0].path}"]`)
  const focused = row.locator('[data-container]').first().getByRole('button').first()
  await focused.focus()
  const order = await row.locator('[data-container]').evaluateAll(nodes => nodes.map(node => node.getAttribute('data-container')))
  await control(request, 'configure', { containers: { ...containers, [stacks[0].path]: children.map((container, index) => ({ ...container, health: index === 1 ? 'unhealthy' : 'healthy' })) } })
  await event(request, { record: stacks[0] }, 'stack.upsert')
  await expect.poll(async () => (await metrics(request)).runtimeRequests).toBeGreaterThanOrEqual(2)
  await expect(row.locator('[data-container="container-1"]')).toContainText('unhealthy')
  await expect(focused).toBeFocused()
  expect(await row.locator('[data-container]').evaluateAll(nodes => nodes.map(node => node.getAttribute('data-container')))).toEqual(order)
  await row.getByRole('button', { name: 'Show all 250 containers', exact: true }).click()
  await expect(row.locator('[data-container]')).toHaveCount(20)
  await row.getByRole('button', { name: 'Show 20 more', exact: true }).click()
  await expect(row.locator('[data-container]')).toHaveCount(40)
  await page.getByRole('button', { name: 'Next', exact: true }).click()
  const returnRow = page.locator(`[data-stack="${stacks[30].path}"]`)
  await returnRow.getByRole('button', { name: `Hide containers for ${stacks[30].path}`, exact: true }).click()
  const link = returnRow.getByRole('link', { name: stacks[30].path, exact: true })
  await link.focus()
  const url = page.url(), scroll = await page.evaluate(() => scrollY)
  expect(scroll).toBeGreaterThan(0)
  await page.keyboard.press('Enter')
  await expect(page.getByRole('navigation', { name: 'Stack sections' })).toBeVisible()
  await page.goBack()
  await expect(page).toHaveURL(url)
  await expect(returnRow.getByRole('button', { name: `Show containers for ${stacks[30].path}`, exact: true })).toHaveAttribute('aria-expanded', 'false')
  await expect(link).toBeFocused()
  await expect.poll(() => page.evaluate(() => scrollY)).toBeCloseTo(scroll, 0)
  await page.getByRole('searchbox').fill('observed-249')
  await expect(page.locator('[data-stack]')).toHaveCount(1)
  await expect(page.locator('[data-container]')).toHaveCount(1)
  await page.getByRole('searchbox').fill('bench')
  await expect(row.locator('[data-container]')).toHaveCount(40)
  const counts = await metrics(request)
  expect(counts.runtimeMaxInFlight).toBe(1)
  await capture(page, info, '250-resource-child-search')
  await evidence(info, 'inventory-measurements', { initialRenderMs, initialDomNodes, returnScrollY: scroll, returnURL: url, ...counts, browserRuntime: browserRuntime.get(page) })
})

test('legacy nested links, log selection and full query context remain scoped', async ({ page, request }) => {
  const state = workspaceBrowserFixture()
  const path = 'sites/production/region-with-a-very-long-name/application-with-a-long-operational-name'
  const stack = { ...state.stacks[0], path }
  await control(request, 'configure', { stacks: [...state.stacks, stack], containers: { ...state.containers, [path]: state.containers['apps/immich'] } })
  await page.goto(`/stack/${path}?tab=logs&container=${state.containers['apps/immich'][0].id}&q=application&view=grid&group=directory&status=synced`)
  await expect(page).toHaveURL(new RegExp(`/stacks/${path}\\?`))
  await expect(page.getByRole('navigation', { name: 'Stack sections' }).getByRole('link', { name: 'Logs', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByLabel('Container log output')).toContainText('application started')
  const logRequest = page.waitForRequest(request => new URL(request.url()).pathname === '/api/container-logs')
  await page.getByRole('button', { name: 'Pause logs', exact: true }).click()
  await page.getByRole('button', { name: 'Follow logs', exact: true }).click()
  const params = new URL((await logRequest).url()).searchParams
  expect(params.get('stack')).toBe(path)
  expect(params.get('container')).toBe(state.containers['apps/immich'][0].id)
  await page.getByRole('link', { name: /All stacks/ }).click()
  await expect(page.getByRole('searchbox')).toHaveValue('application')
  await expect(page.locator('[data-stack]')).not.toHaveCount(0)
  await page.getByRole('searchbox').fill('a'.repeat(64))
  await expect(page.locator('[data-stack]')).toHaveCount(3)
  await expect(page.locator('[data-container]')).toHaveCount(3)
})

test('empty, unavailable capability, failed/healthy, terminal errors and expired operations stay honest', async ({ page, request, expectedHttpErrors }) => {
  expectedHttpErrors.push('/api/operations/old-refresh:404')
  const state = workspaceBrowserFixture()
  await control(request, 'configure', { work: catalogue.reconnect, stacks: [], containers: {} })
  await page.goto('/stacks')
  await expect(page.locator('main')).toContainText('No stacks discovered')
  await expect(workRegion(page)).toContainText('No work in progress')
  await control(request, 'configure', {
    stacks: [{ ...state.stacks[0], status: 'failed', lastSyncError: 'Compose failed after image pull' }],
    containers: state.containers,
    instance: { ...state.instance, capabilities: { ...state.instance.capabilities, imageUpdates: false } },
  })
  await page.reload()
  const row = page.locator('[data-stack="apps/immich"]')
  await expect(row).toContainText('Failed')
  await expect(row).toContainText('3 / 3 running')
  await expect(row).toContainText('healthy')
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
  await expect(page.getByText('Image updates disabled by instance configuration', { exact: false })).toBeVisible()
  const finishedAt = new Date(Date.parse(catalogue.snapshot.runs[0].startedAt) + 60000).toISOString()
  const run = { ...catalogue.snapshot.runs[0], status: 'failed', phase: 'finished', message: 'Compose failed after pull', finishedAt }
  const operation = { ...catalogue.snapshot.operations[1], status: 'failed', message: 'Compose failed after pull', version: 2, finishedAt }
  await event(request, { ...operation, steps: [] }, 'operation.upsert')
  await event(request, { ...catalogue.bytes, sequence: 21, run, operations: [operation] })
  await expect(row).toContainText('Deployment failed')
  await row.getByRole('button', { name: 'Result details', exact: true }).click()
  await expect(page.locator('.operation-drawer')).toContainText('Compose failed after pull')
  expectedHttpErrors.push('/api/operations/expired-id:404')
  await page.goto('/operations/expired-id')
  await expect(page.locator('main')).toContainText('Operation not found')
})

for (const theme of ['light', 'dark'] as const) for (const width of [1440, 768, 390, 320]) {
  test(`accessible reflow ${theme} ${width}: keyboard disclosure, drawer and live semantics`, async ({ page, request }, info) => {
    await page.setViewportSize({ width, height: 900 })
    await page.emulateMedia({ reducedMotion: 'reduce', colorScheme: theme })
    const state = workspaceBrowserFixture()
    const longPath = 'sites/production/region-with-an-exceptionally-long-name/application-with-a-long-name'
    await control(request, 'configure', { stacks: [...state.stacks, { ...state.stacks[0], path: longPath }], containers: { ...state.containers, [longPath]: state.containers['apps/immich'] } })
    await page.addInitScript(theme => localStorage.setItem('docker-cd-theme', theme), theme)
    await page.goto('/stacks')
    // Theme preference key is an implementation detail: use the real control if needed.
    if (await page.locator('html').getAttribute('data-theme') !== theme) await page.getByRole('button', { name: `Switch to ${theme} mode`, exact: true }).first().click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    const row = page.locator('[data-stack="apps/immich"]')
    const disclosure = row.getByRole('button', { name: 'Hide containers for apps/immich', exact: true })
    await disclosure.focus(); await page.keyboard.press('Enter')
    await expect(row.getByRole('button', { name: 'Show containers for apps/immich', exact: true })).toHaveAttribute('aria-expanded', 'false')
    await page.keyboard.press('Space')
    await expect(disclosure).toHaveAttribute('aria-expanded', 'true')
    await row.getByRole('button', { name: 'immich-web', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(page.getByRole('dialog', { name: 'Image inspection for immich-web in apps/immich', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.keyboard.press('Escape')
    await expect(row.getByRole('button', { name: 'immich-web', exact: true })).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(page.locator(':focus')).toBeVisible()
    await page.keyboard.press('Shift+Tab')
    await expect(row.getByRole('button', { name: 'immich-web', exact: true })).toBeFocused()
    await page.keyboard.press('ControlOrMeta+k')
    await expect(page.getByRole('textbox', { name: 'Search stacks and pages', exact: true })).toBeFocused()
    await page.keyboard.press('Escape')
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
    expect(await disclosure.evaluate(element => {
      const style = getComputedStyle(element)
      return { animation: style.animationName, transition: style.transitionDuration }
    })).toEqual({ animation: 'none', transition: '0s' })
    const client = await page.context().newCDPSession(page)
    const tree = await client.send('Accessibility.getFullAXTree')
    expect(tree.nodes.some(node => node.role?.value === 'button' && node.name?.value === 'Hide containers for apps/immich')).toBe(true)
    expect(tree.nodes.some(node => node.role?.value === 'status' && node.properties?.some(property => property.name === 'live' && property.value.value === 'polite'))).toBe(true)
    await evidence(info, 'accessibility-tree', tree)
    await capture(page, info, `${theme}-${width}`)
    expect((await metrics(request)).runtimeMaxInFlight).toBe(1)
  })
}
