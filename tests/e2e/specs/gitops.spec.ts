import { createHmac } from 'node:crypto'
import { writeFile } from 'node:fs/promises'
import { expect, test, type APIRequestContext, type Locator, type Page, type TestInfo } from '@playwright/test'
import type { ContainerInfo, Operation } from '../../../frontend/src/services/api'
import type { WorkRun, WorkSnapshot, WorkUpsert } from '../../../frontend/src/services/work'

const api = 'http://127.0.0.1:18080'
const secret = 'integration-webhook-secret-not-a-real-credential'
const compose = (name: string) => `services:
  worker:
    image: busybox:1.38.0
    container_name: ${name}
    command: [sleep, '3600']
    stop_grace_period: 1s
    volumes: [data:/data]
volumes:
  data: {}
`

type WorkEvent =
  | { type: 'work.snapshot'; data: WorkSnapshot }
  | { type: 'work.upsert'; data: WorkUpsert }

declare global {
  interface Window {
    gitopsEvidence: { events: WorkEvent[]; streams: EventSource[]; documentMarker: string }
  }
}

async function control(request: APIRequestContext, path: string, data?: unknown) {
  const response = await request.post(`${api}/test/${path}`, { data })
  expect(response.ok(), `integration control ${path}`).toBeTruthy()
  return response
}

async function idle(request: APIRequestContext) {
  await expect.poll(async () => (await (await request.get(`${api}/test/idle`)).json()).idle).toBe(true)
}

async function commit(request: APIRequestContext, files: Record<string, string | null>, message: string) {
  await idle(request)
  const response = await control(request, 'commit', { files, message })
  return (await response.json()).revision as string
}

async function webhook(request: APIRequestContext, revision: string, valid = true) {
  const body = JSON.stringify({ ref: 'refs/heads/main', after: revision, repository: { full_name: 'test/fixture' } })
  return request.post(`${api}/api/webhook`, {
    data: body,
    headers: {
      'Content-Type': 'application/json',
      'X-GitHub-Event': 'push',
      'X-Hub-Signature-256': valid ? `sha256=${createHmac('sha256', secret).update(body).digest('hex')}` : 'sha256=invalid',
    },
  })
}

const stack = (page: Page, path: string) => page.locator(`.inventory-stack[data-stack="${path}"]`)
const layer = (progress: Locator, id: string) => progress.locator('.pull-layers > li').filter({ has: progress.page().getByText(id, { exact: true }) })

async function workRuns(page: Page): Promise<WorkRun[]> {
  return page.evaluate(() => window.gitopsEvidence.events.flatMap((event) => event.type === 'work.upsert' && event.data.run ? [event.data.run] : []))
}

async function observedRun(page: Page, match: Partial<Pick<WorkRun, 'source' | 'stack' | 'phase' | 'status' | 'runId'>>) {
  const matches = (run: WorkRun) => Object.entries(match).every(([key, value]) => run[key as keyof WorkRun] === value)
  await expect.poll(async () => (await workRuns(page)).some(matches), { message: `real work.upsert ${JSON.stringify(match)}` }).toBe(true)
  return (await workRuns(page)).find(matches)!
}

async function reconnect(page: Page) {
  const before = await page.evaluate(() => ({
    streams: window.gitopsEvidence.streams.length,
    snapshots: window.gitopsEvidence.events.filter((event) => event.type === 'work.snapshot').length,
  }))
  await page.evaluate(() => {
    const stream = window.gitopsEvidence.streams.at(-1)!
    stream.close()
    stream.dispatchEvent(new Event('error'))
  })
  await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
  await expect.poll(() => page.evaluate(() => window.gitopsEvidence.streams.length)).toBeGreaterThan(before.streams)
  await expect.poll(() => page.evaluate(() => window.gitopsEvidence.events.filter((event) => event.type === 'work.snapshot').length)).toBeGreaterThan(before.snapshots)
  return page.evaluate(() => {
    const events = window.gitopsEvidence.events.filter((event): event is Extract<WorkEvent, { type: 'work.snapshot' }> => event.type === 'work.snapshot')
    return events.at(-1)!.data
  })
}

async function containers(request: APIRequestContext, path = 'apps/web'): Promise<ContainerInfo[]> {
  const response = await request.get(`${api}/api/stacks/containers/${path}`)
  expect(response.ok()).toBe(true)
  return response.json()
}

async function attachJSON(info: TestInfo, name: string, value: unknown) {
  const path = info.outputPath(`${name}.json`)
  await writeFile(path, JSON.stringify(value, null, 2))
  await info.attach(name, { path, contentType: 'application/json' })
}

async function attachScreenshot(page: Page, info: TestInfo, name: string, fullPage = true) {
  const path = info.outputPath(`${name}.png`)
  if (fullPage) await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({ path, fullPage })
  await info.attach(name, { path, contentType: 'image/png' })
}

test('operational workspace follows real DinD work, reconnects, observations and safe actions', async ({ page, context, request }, testInfo) => {
  test.setTimeout(240_000)
  page.setDefaultTimeout(30_000)
  const errors: string[] = []
  const mutations: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (req) => {
    if (req.method() === 'POST' && new URL(req.url()).pathname.startsWith('/api/')) mutations.push(new URL(req.url()).pathname)
  })
  // Observe the actual browser EventSource. No HTTP/SSE responses or UI state
  // are mocked. Controlled Engine NDJSON bytes are simulated by SlowPull before
  // its actual private-DinD pull; these are not measured registry download sizes.
  await context.addInitScript(() => {
    const NativeEventSource = window.EventSource
    window.gitopsEvidence = { events: [], streams: [], documentMarker: '' }
    window.EventSource = class extends NativeEventSource {
      constructor(url: string | URL, init?: EventSourceInit) {
        super(url, init)
        window.gitopsEvidence.streams.push(this)
        this.addEventListener('work.snapshot', (event: MessageEvent<string>) => {
          window.gitopsEvidence.events.push({ type: 'work.snapshot', data: JSON.parse(event.data) as WorkSnapshot })
        })
        this.addEventListener('work.upsert', (event: MessageEvent<string>) => {
          window.gitopsEvidence.events.push({ type: 'work.upsert', data: JSON.parse(event.data) as WorkUpsert })
        })
      }
    }
  })

  try {
    await test.step('initial pull is inline, survives SPA navigation and restores the same run on reconnect', async () => {
      await control(request, 'pull/arm')
      await control(request, 'gate/arm')
      // Old grid URL remains consumable, but the rendered inventory is nested.
      await page.goto('/stacks?view=grid')
      await expect(page.getByRole('heading', { name: /^Stacks/ })).toBeVisible()
      await page.evaluate(() => { window.gitopsEvidence.documentMarker = 'same-document' })
      await expect(page.locator('.inventory-stack')).toHaveCount(0)
      await expect.poll(() => page.evaluate(() => window.gitopsEvidence.events.some((event) => event.type === 'work.snapshot' && event.data.schemaVersion === 2))).toBe(true)
      await control(request, 'start')
      const web = stack(page, 'apps/web')
      await expect(web.getByText('Applying', { exact: true })).toBeVisible()
      const run = await observedRun(page, { source: 'reconcile', stack: 'apps/web', phase: 'pulling', status: 'running' })
      const progress = web.locator(`[aria-label="Progress for apps/web run ${run.runId}"]`)
      await progress.locator('summary').click()
      await expect(layer(progress, 'layer-a')).toContainText('128.0 MiB')
      await expect(layer(progress, 'layer-b')).toContainText('256.0 MiB')
      await expect(page.locator('.p-drawer')).toHaveCount(0)
      await expect(page.locator('.active-work h2')).toHaveText('Work in progress 1')
      await attachScreenshot(page, testInfo, 'real-initial-inline-pull')

      // This is elapsed quiet-stream coverage, not a readiness sleep. Periodic
      // refresh must not interrupt the actual paused SDK stream.
      const until = Date.now() + Number(process.env.E2E_PULL_HOLD_MS || 5000)
      while (Date.now() < until) {
        await control(request, 'periodic')
        await expect(web.getByText('Applying', { exact: true })).toBeVisible()
        await expect(web.getByText('Failed', { exact: true })).toHaveCount(0)
        await page.waitForTimeout(1000)
      }
      await control(request, 'pull/advance')
      await expect(layer(progress, 'layer-a').getByRole('progressbar')).toHaveAttribute('value', String(32 * 1024 * 1024))
      await expect(layer(progress, 'layer-b').getByRole('progressbar')).toHaveAttribute('value', String(64 * 1024 * 1024))
      await page.locator('.sidebar').getByRole('link', { name: 'Repository', exact: true }).click()
      await expect(page).toHaveURL(/\/repository/)
      await page.locator('.sidebar').getByRole('link', { name: /^Stacks(?:\s+\d+)?$/ }).click()
      await progress.locator('summary').click()
      await expect(layer(progress, 'layer-a').getByRole('progressbar')).toHaveAttribute('value', String(32 * 1024 * 1024))
      const snapshot = await reconnect(page)
      const restored = snapshot.runs.find((candidate) => candidate.runId === run.runId)
      expect(restored?.operationId).toBe(run.operationId)
      expect(restored?.rootOperationId).toBe(run.rootOperationId)
      expect(restored?.images[0]?.layers.map((entry) => entry.id).sort()).toEqual(['layer-a', 'layer-b'])
      expect(restored?.images[0]?.layers.find((entry) => entry.id === 'layer-b')?.bytesCurrent).toBe(64 * 1024 * 1024)
      const byteRun = (await workRuns(page)).find((candidate) => candidate.runId === run.runId && candidate.images.length > 0)!
      expect(restored?.images[0]?.pullAttemptId).toBe(byteRun.images[0]!.pullAttemptId)
      await expect(layer(progress, 'layer-b').getByRole('progressbar')).toHaveAttribute('value', String(64 * 1024 * 1024))
      await control(request, 'pull/advance')
      await expect(layer(progress, 'layer-a')).toContainText('extracting')
      await expect(layer(progress, 'layer-b')).toContainText('downloading')
      await control(request, 'pull/advance')
      await expect(layer(progress, 'layer-a')).toContainText('Pull complete')
      await expect(layer(progress, 'layer-b')).toContainText('extracting')
      await control(request, 'pull/advance')
      await expect.poll(async () => (await (await request.get(`${api}/test/gate`)).json()).waiting).toBe(true)
      await expect(progress.getByText('Applying Compose', { exact: true })).toBeVisible()
      await expect(progress).toContainText('Images pulled — deployment may still be applying or pruning')
      expect((await workRuns(page)).some((candidate) => candidate.runId === run.runId && candidate.status === 'completed')).toBe(false)
      await expect(progress).not.toContainText('100%')
      await control(request, 'gate/release')
      await observedRun(page, { runId: run.runId, status: 'completed' })
      await expect(web.getByText('Applied', { exact: true })).toBeVisible()
      // Actual children are shown without a stack/detail/tab/disclosure click.
      await expect(web.getByRole('button', { name: 'web-v1', exact: true })).toBeVisible()
      await expect(stack(page, 'infra/cache').getByRole('button', { name: 'cache-v1', exact: true })).toBeVisible()
      await expect(web.locator('.inventory-container')).toHaveCount(1)
      await expect(web.locator('.inventory-container').getByRole('button', { name: /update|restart|delete|cancel/i })).toHaveCount(0)
      await idle(request)
      const terminalSnapshot = await reconnect(page)
      expect(terminalSnapshot.runs).toEqual([])
      await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeEnabled()
      await attachScreenshot(page, testInfo, 'real-nested-containers-before-disclosure')
    })

    await test.step('real running-image digest is inspectable and copies its full repository reference', async () => {
      const [running] = await containers(request)
      expect(running?.imageIdentity?.status).toBe('available')
      const preferred = running!.imageIdentity!.preferredRepositoryDigest!
      expect(preferred.digest).toMatch(/^sha256:[a-f0-9]{64}$/)
      const fullReference = `${preferred.repository}@${preferred.digest}`
      await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'http://127.0.0.1:18081' })
      await stack(page, 'apps/web').getByRole('button', { name: 'web-v1', exact: true }).click()
      const inspector = page.locator('#container-image-inspector')
      await expect(inspector).toBeVisible()
      await expect(inspector.getByText(fullReference, { exact: true })).toBeVisible()
      await expect(inspector).toContainText('Not checked')
      await expect(inspector).toContainText('Unknown (reported by local daemon)')
      await inspector.getByRole('button', { name: 'Copy reference', exact: true }).click()
      await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(fullReference)
      await inspector.getByText('Technical identifiers', { exact: true }).click()
      await expect(inspector.getByText(running!.id, { exact: true })).toBeVisible()
      await attachScreenshot(page, testInfo, 'real-image-inspection', false)
      await page.keyboard.press('Escape')
    })

    await test.step('signed webhooks discover nested resources and replace same-count observations without reload', async () => {
      let revision = await commit(request, {
        'apps/web/compose.yaml': compose('web-v2'),
        'deep/nested/worker/compose.yaml': compose('worker-v1'),
      }, 'web version two')
      expect((await webhook(request, revision, false)).status()).toBe(401)
      await expect(page.locator('.inventory-stack')).toHaveCount(2)
      await control(request, 'gate/arm')
      expect((await webhook(request, revision)).ok()).toBeTruthy()
      await expect(stack(page, 'apps/web').getByText('Applying', { exact: true })).toBeVisible()
      await reconnect(page)
      await expect(stack(page, 'apps/web')).toContainText('web-v1')
      await control(request, 'gate/release')
      await expect(stack(page, 'apps/web').getByRole('button', { name: 'web-v2', exact: true })).toBeVisible()
      await expect(stack(page, 'apps/web')).not.toContainText('web-v1')
      await expect(stack(page, 'deep/nested/worker').getByRole('button', { name: 'worker-v1', exact: true })).toBeVisible()
      await expect(page.locator('.inventory-stack')).toHaveCount(3)

      await stack(page, 'apps/web').getByRole('link', { name: 'apps/web', exact: true }).click()
      const sections = page.getByRole('navigation', { name: 'Stack sections' })
      await sections.getByRole('link', { name: 'Containers', exact: true }).click()
      const [before] = await containers(request)
      await expect(page.locator('.container-card')).toContainText('web-v2')
      const second = await context.newPage()
      try {
        await second.goto('/stacks')
        await expect(stack(second, 'apps/web')).toContainText('web-v2')
        revision = await commit(request, { 'apps/web/compose.yaml': compose('web-v3') }, 'web version three')
        expect((await webhook(request, revision)).ok()).toBeTruthy()
        // Stay on Containers throughout: no tab switch or manual refresh can
        // mask a missing shared-runtime terminal invalidation. Returning a tab
        // to the foreground is a real supported observation trigger.
        await page.bringToFront()
        await expect(page.locator('.container-card')).toHaveCount(1)
        await expect(page.locator('.container-card')).toContainText('web-v3')
        await expect(page.locator('.container-card')).not.toContainText('web-v2')
        await second.bringToFront()
        await expect(stack(second, 'apps/web')).toContainText('web-v3')
        const [after] = await containers(request)
        expect(after!.id).not.toBe(before!.id)
        await expect(stack(second, 'apps/web').locator(`[data-container="${after!.id}"]`)).toBeVisible()
      } finally { await second.close() }

      await sections.getByRole('link', { name: 'Configuration', exact: true }).click()
      await expect(page.locator('.source-code')).toContainText('web-v3')
      await page.getByRole('button', { name: 'Changes from applied', exact: true }).click()
      await expect(page.getByText('No configuration changes.', { exact: true })).toBeVisible()
      await sections.getByRole('link', { name: 'Activity', exact: true }).click()
      await expect(page.locator('.operations-table')).toContainText('Completed')
      await sections.getByRole('link', { name: 'Overview', exact: true }).click()
      const [current] = await containers(request)
      await page.getByRole('link', { name: 'View logs for web-v3' }).click()
      await expect(page).toHaveURL(new RegExp(`tab=logs.*container=${current!.id}`))
      await expect(page.getByLabel('Log container')).toHaveValue(current!.id)
      const scopedLog = await request.get(`${api}/api/container-logs?stack=apps/web&container=${current!.id}&tail=20`)
      expect(scopedLog.ok()).toBe(true)
      const [other] = await containers(request, 'infra/cache')
      expect((await request.get(`${api}/api/container-logs?stack=apps/web&container=${other!.id}&tail=20`)).ok()).toBe(false)
    })

    await test.step('real Compose failure publishes a failed v2 outcome and recovery clears the error', async () => {
      const sections = page.getByRole('navigation', { name: 'Stack sections' })
      await sections.getByRole('link', { name: 'Overview', exact: true }).click()
      const broken = 'services:\n  worker:\n    image: busybox:1.38.0\n    definitely_invalid_property: true\n'
      let revision = await commit(request, { 'apps/web/compose.yaml': broken }, 'broken configuration')
      expect((await webhook(request, revision)).ok()).toBeTruthy()
      await expect(page.getByRole('alert')).toContainText('compose up failed')
      const failed = await observedRun(page, { source: 'reconcile', stack: 'apps/web', status: 'failed' })
      expect(failed.finishedAt).toBeTruthy()
      expect((await workRuns(page)).some((run) => run.runId === failed.runId && run.status === 'completed')).toBe(false)
      revision = await commit(request, { 'apps/web/compose.yaml': compose('web-v4') }, 'fixed configuration')
      expect((await webhook(request, revision)).ok()).toBeTruthy()
      await expect(page.getByText('fixed configuration', { exact: true })).toBeVisible()
      await expect(page.getByRole('alert')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'web-v4', exact: true })).toBeVisible()
      await page.getByRole('link', { name: '← All stacks', exact: true }).click()
    })

    await test.step('UI Git interruption cancels a real targeted stream without a success shortcut', async () => {
      await idle(request)
      const previousRunIDs = new Set((await workRuns(page)).map((run) => run.runId))
      await control(request, 'pull/arm')
      const web = stack(page, 'apps/web')
      await web.getByRole('button', { name: 'Update images…', exact: true }).click()
      const confirmation = web.getByRole('group', { name: 'Confirm action for apps/web' })
      await expect(confirmation).toContainText('No image prune.')
      await confirmation.getByRole('button', { name: 'Update apps/web', exact: true }).click()
      // Ignore earlier deployment attempts sharing source/stack: this must be
      // the held run started by the reviewed UI action, not a historical match.
      const currentPull = (run: WorkRun) => !previousRunIDs.has(run.runId) && run.source === 'reconcile' && run.stack === 'apps/web' && run.status === 'running' && run.images.some((image) => image.layers.some((entry) => entry.id === 'layer-a'))
      await expect.poll(async () => (await workRuns(page)).some(currentPull)).toBe(true)
      const held = (await workRuns(page)).filter(currentPull).at(-1)!
      await expect(page.locator('.operation-drawer')).toBeVisible()
      await page.keyboard.press('Escape') // targeted accepted operation opens its existing inspector
      await expect(page.locator('.operation-drawer')).toHaveCount(0)
      await page.getByRole('button', { name: 'Check Git for changes…', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: 'Interrupt work and check Git' })
      await expect(dialog).toContainText('Requests interruption; work may already have made changes.')
      await expect(dialog).toContainText(held.rootOperationId)
      await dialog.getByRole('button', { name: 'Interrupt work and check Git', exact: true }).click()
      await observedRun(page, { runId: held.runId, status: 'cancelled' })
      expect((await workRuns(page)).some((candidate) => candidate.runId === held.runId && candidate.status === 'completed')).toBe(false)
      await idle(request)
      const snapshot = await reconnect(page)
      expect(snapshot.runs.some((candidate) => candidate.runId === held.runId)).toBe(false)
      await expect(web.getByRole('button', { name: 'web-v4', exact: true })).toBeVisible()
      await expect(web.getByText('Applied', { exact: true })).toBeVisible()
      await expect(page.locator('.active-work h2')).toHaveText('Work in progress 0')
    })

    await test.step('UI all-stack maintenance requires prune confirmation, skips failed stacks and records errors', async () => {
      const broken = 'services:\n  worker:\n    definitely_invalid_property: true\n'
      const revision = await commit(request, { 'zzz/failed/compose.yaml': broken }, 'discovered failure is skipped by image maintenance')
      expect((await webhook(request, revision)).ok()).toBeTruthy()
      await expect(stack(page, 'zzz/failed').getByText('Failed', { exact: true })).toBeVisible()
      await idle(request)
      await control(request, 'pull/arm?fail=true')
      const previousPosts = mutations.filter((path) => path === '/api/update').length
      await page.getByRole('button', { name: 'Update all stacks…', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: 'Update all stacks', exact: true })
      await expect(dialog).toContainText('Host-wide prune:')
      await expect(dialog).toContainText('regardless of inventory filters or the current page')
      expect(mutations.filter((path) => path === '/api/update')).toHaveLength(previousPosts)
      await attachScreenshot(page, testInfo, 'real-host-wide-prune-confirmation', false)
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
      expect(mutations.filter((path) => path === '/api/update')).toHaveLength(previousPosts)
      await page.getByRole('button', { name: 'Update all stacks…', exact: true }).click()
      await dialog.getByRole('button', { name: 'Update all stacks and prune images', exact: true }).click()
      const run = await observedRun(page, { source: 'scheduler', stack: 'apps/web', phase: 'pulling', status: 'running' })
      await expect(page.locator('.workspace-actions .request-warning')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeDisabled()
      const progress = stack(page, 'apps/web').locator(`[aria-label="Progress for apps/web run ${run.runId}"]`)
      await progress.locator('summary').click()
      await expect(layer(progress, 'layer-a')).toContainText('downloading')
      // The SDK reader samples same-phase byte callbacks every 500ms. Hold one
      // sampling interval before emitting the next controlled Engine tick; this
      // elapsed-stream coverage is not a readiness sleep or a fake UI event.
      await page.waitForTimeout(600)
      await control(request, 'pull/advance')
      await expect(layer(progress, 'layer-b').getByRole('progressbar')).toHaveAttribute('value', String(64 * 1024 * 1024))
      await control(request, 'pull/advance')
      await observedRun(page, { runId: run.runId, status: 'failed' })
      const prune = await observedRun(page, { source: 'scheduler', phase: 'pruning', status: 'running' })
      const terminal = await observedRun(page, { runId: prune.runId, status: 'completed' })
      expect(prune.rootOperationId).toBe(run.rootOperationId)
      expect(terminal.stackResults).toMatchObject({ processed: 3, skipped: 1, failed: 1 })
      expect(terminal.stackResults?.outcomes).toEqual(expect.arrayContaining([expect.objectContaining({ stack: 'zzz/failed', status: 'skipped' })]))
      await expect.poll(() => page.evaluate((id) => window.gitopsEvidence.events.some((event) => event.type === 'work.upsert' && event.data.operations.some((operation) => operation.id === id && operation.status === 'completed_with_errors')), run.rootOperationId)).toBe(true)
      await idle(request)
      const snapshot = await reconnect(page)
      expect(snapshot.runs).toEqual([])
      await expect(page.locator('.active-work')).toContainText('completed_with_errors')
      // Authoritative idle releases normal controls without browser-message cleanup.
      await expect(page.locator('.workspace-actions .request-warning')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Clear local request notice', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Update all stacks…', exact: true })).toBeEnabled()
      expect(mutations.filter((path) => path === '/api/update')).toHaveLength(previousPosts + 1)
    })

    await test.step('nested removals leave the inventory and reject stale stack inspection scopes', async () => {
      const revision = await commit(request, {
        'deep/nested/worker/compose.yaml': null,
        'zzz/failed/compose.yaml': null,
      }, 'remove worker and failed discovery')
      expect((await webhook(request, revision)).ok()).toBeTruthy()
      await expect(page.locator('.inventory-stack')).toHaveCount(2)
      await expect(stack(page, 'deep/nested/worker')).toHaveCount(0)
      await expect.poll(async () => (await request.get(`${api}/api/stacks/containers/deep/nested/worker`)).status()).toBe(404)
      await idle(request)
      expect(await page.evaluate(() => window.gitopsEvidence.documentMarker)).toBe('same-document')
      const instance = await (await request.get(`${api}/api/instance`)).json()
      expect(instance.history.persistent).toBe(true)
      const history: { records: Operation[] } = await (await request.get(`${api}/api/operations?limit=100`)).json()
      expect(history.records.some((operation) => operation.kind === 'deployment' && operation.parentId && operation.status === 'completed')).toBe(true)
      expect(history.records.some((operation) => operation.kind === 'image_update' && operation.status === 'completed_with_errors')).toBe(true)
      expect(history.records.some((operation) => operation.kind === 'webhook' && operation.status === 'failed' && operation.message.includes('Invalid webhook signature'))).toBe(true)
      const configuration = await (await request.get(`${api}/api/stack/configuration?path=apps/web`)).json()
      expect(configuration.applied.content).toBe(configuration.content)
      expect(configuration.applied.content).toContain('web-v4')
      const events = await page.evaluate(() => window.gitopsEvidence.events)
      let sequence = -1
      let epoch = ''
      const terminalStatuses = new Map<string, WorkRun['status']>()
      for (const event of events) {
        expect(event.data.schemaVersion).toBe(2)
        if (event.type === 'work.snapshot') {
          if (epoch) expect(event.data.epoch).toBe(epoch)
          expect(event.data.sequence).toBeGreaterThanOrEqual(sequence)
          epoch = event.data.epoch
          expect(event.data.runs.every((run) => run.status === 'running')).toBe(true)
        } else {
          expect(event.data.epoch).toBe(epoch)
          expect(event.data.sequence).toBeGreaterThan(sequence)
          const run = event.data.run
          if (run) {
            const previous = terminalStatuses.get(run.runId)
            if (previous) expect(run.status).toBe(previous)
            if (run.status !== 'running') terminalStatuses.set(run.runId, run.status)
          }
        }
        sequence = event.data.sequence
      }
      expect([...terminalStatuses.values()]).toEqual(expect.arrayContaining(['completed', 'failed', 'cancelled']))
      await attachJSON(testInfo, 'real-lifecycle-operation-history', history)
      await attachScreenshot(page, testInfo, 'real-final-nested-inventory')
      expect(errors).toEqual([])
    })
  } finally {
    await attachJSON(testInfo, 'real-work-v2-sse-events', await page.evaluate(() => window.gitopsEvidence?.events || []).catch(() => []))
    await attachJSON(testInfo, 'real-browser-mutation-requests', mutations)
    // Release deterministic fixtures even after an assertion failure so the
    // test server can shut down promptly without waiting for the test timeout.
    await control(request, 'gate/release').catch(() => undefined)
    for (let i = 0; i < 4; i++) await control(request, 'pull/advance').catch(() => undefined)
  }
})
