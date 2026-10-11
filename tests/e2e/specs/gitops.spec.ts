import { createHmac } from 'node:crypto'
import { expect, test, type APIRequestContext } from '@playwright/test'

const api = 'http://127.0.0.1:18080'
const secret = 'integration-webhook-secret-not-a-real-credential'
const compose = (name: string) => `services:
  worker:
    image: busybox:1.37.0
    container_name: ${name}
    command: [sleep, '3600']
    stop_grace_period: 1s
    volumes: [data:/data]
volumes:
  data: {}
`

async function control(request: APIRequestContext, path: string, data?: unknown) {
  const response = await request.post(`${api}/test/${path}`, { data })
  expect(response.ok()).toBeTruthy()
  return response
}

async function commit(request: APIRequestContext, files: Record<string, string | null>, message: string) {
  await expect.poll(async () => (await (await request.get(`${api}/test/idle`)).json()).idle).toBe(true)
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

test('GitOps lifecycle updates cards and details live through real SSE and DinD', async ({ page, context, request }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  // Track real SSE messages for short-lived transitions; no responses are mocked.
  await page.addInitScript(() => {
    const NativeEventSource = window.EventSource
    ;(window as any).events = []
    ;(window as any).streams = []
    window.EventSource = class extends NativeEventSource {
      constructor(url: string | URL, init?: EventSourceInit) {
        super(url, init)
        ;(window as any).streams.push(this)
        for (const type of ['update.progress', 'stack.upsert', 'refresh.status']) {
          this.addEventListener(type, (event: MessageEvent) => {
            ;(window as any).events.push({ type, data: JSON.parse(event.data) })
          })
        }
      }
    }
  })

  await test.step('initial discovery progresses from empty to syncing to populated cards', async () => {
    await control(request, 'pull/arm')
    await control(request, 'gate/arm')
    await page.goto('/')
    await expect(page.getByText('Queued', { exact: true })).toBeVisible()
    await expect(page.locator('.stack-card')).toHaveCount(0)
    await control(request, 'start')
    const web = page.locator('.stack-card').filter({ hasText: 'apps/web' })
    await expect(web.getByText('Syncing', { exact: true })).toBeVisible()
    const a = page.locator('[data-layer-id="layer-a"]')
    const b = page.locator('[data-layer-id="layer-b"]')
    await expect(a).toContainText('Downloading')
    await expect(b).toContainText('Downloading')
    await expect(a).toContainText('128.0 MiB')
    await expect(b).toContainText('256.0 MiB')
    // Deliberately hold a quiet stream, including beyond proxy idle timeouts in
    // the long-pull CI job. This is elapsed-time coverage, not a readiness sleep.
    const hold = Number(process.env.E2E_PULL_HOLD_MS || 5000)
    const until = Date.now() + hold
    while (Date.now() < until) {
      await control(request, 'periodic')
      await expect(web.getByText('Syncing', { exact: true })).toBeVisible()
      await expect(web.locator('.error-section')).toHaveCount(0)
      await expect(page.locator('.stack-card--failed')).toHaveCount(0)
      await page.waitForTimeout(1000)
    }
    await control(request, 'pull/advance')
    await expect(a).toHaveAttribute('data-bytes', String(32 * 1024 * 1024))
    await expect(b).toHaveAttribute('data-bytes', String(64 * 1024 * 1024))
    // Reconnection must restore BOTH layers, not just the latest event.
    await page.evaluate(() => {
      const stream = (window as any).streams.at(-1) as EventSource
      stream.close(); stream.dispatchEvent(new Event('error'))
    })
    await expect.poll(() => page.evaluate(() => (window as any).streams.length)).toBeGreaterThan(1)
    await expect(a).toHaveAttribute('data-bytes', String(32 * 1024 * 1024))
    await expect(b).toHaveAttribute('data-bytes', String(64 * 1024 * 1024))
    await control(request, 'pull/advance')
    await expect(a).toContainText('Extracting')
    await expect(b).toContainText('Downloading')
    await control(request, 'pull/advance')
    await expect(a).toContainText('Pull complete')
    await expect(b).toContainText('Extracting')
    await expect(page.locator('[data-image="busybox:1.37.0"]')).toHaveAttribute('data-done', 'false')
    await expect(web.getByText('Syncing', { exact: true })).toBeVisible()
    await control(request, 'pull/advance')
    await expect.poll(async () => (await (await request.get(`${api}/test/gate`)).json()).waiting).toBe(true)
    await control(request, 'gate/release')
    await expect(web.getByText('Synced', { exact: true })).toBeVisible()
    await expect(web).toContainText('web-v1')
    await expect(page.locator('.stack-card').filter({ hasText: 'infra/cache' })).toContainText('cache-v1')
    await expect(page.locator('.stack-card--synced')).toHaveCount(2)
    await expect(page.locator('.pull-progress-section')).toHaveCount(0)
    expect(await page.evaluate(() => (window as any).events.some((e: any) => e.type === 'stack.upsert' && e.data.record.status === 'failed'))).toBe(false)
  })

  // This marker must survive every update; a hidden page reload would erase it.
  await page.evaluate(() => { (window as any).noReloadMarker = 'same-document' })
  await test.step('signed push discovers a nested stack and updates existing card containers', async () => {
    const revision = await commit(request, {
      'apps/web/compose.yaml': compose('web-v2'),
      'deep/nested/worker/compose.yaml': compose('worker-v1'),
    }, 'web version two')
    expect((await webhook(request, revision, false)).status()).toBe(401)
    await expect(page.locator('.stack-card')).toHaveCount(2)
    await control(request, 'gate/arm')
    expect((await webhook(request, revision)).ok()).toBeTruthy()
    const web = page.locator('.stack-card').filter({ hasText: 'apps/web' })
    await expect(web.getByText('Syncing', { exact: true })).toBeVisible()

    // Reconnect this tab mid-deploy, verifying bootstrap without navigating.
    await page.evaluate(() => {
      const stream = (window as any).streams.at(-1) as EventSource
      stream.close()
      stream.dispatchEvent(new Event('error'))
    })
    await expect.poll(() => page.evaluate(() => (window as any).streams.length)).toBeGreaterThan(1)
    await expect(web.getByText('Syncing', { exact: true })).toBeVisible()
    await control(request, 'gate/release')
    await expect(web).toContainText('web-v2')
    await expect(web).not.toContainText('web-v1')
    await expect(page.locator('.stack-card--synced')).toHaveCount(3)
    await expect(page.locator('.stack-card').filter({ hasText: 'deep/nested/worker' })).toContainText('worker-v1')
  })

  await test.step('open details and a second tab stay current after same-count redeployment', async () => {
    await page.locator('.stack-card').filter({ hasText: 'apps/web' }).click()
    const before = await (await request.get(`${api}/api/stacks/containers/apps/web`)).json()
    await expect(page.getByText(before[0].id, { exact: true })).toBeVisible()
    const second = await context.newPage()
    await second.goto('/')
    const revision = await commit(request, { 'apps/web/compose.yaml': compose('web-v3') }, 'web version three')
    expect((await webhook(request, revision)).ok()).toBeTruthy()
    await expect(page.getByText('web version three', { exact: true })).toBeVisible()
    await expect(page.getByText(before[0].id, { exact: true })).toHaveCount(0)
    await expect.poll(async () => {
      const containers = await (await request.get(`${api}/api/stacks/containers/apps/web`)).json()
      return containers[0]?.id
    }).not.toBe(before[0].id)
    const after = await (await request.get(`${api}/api/stacks/containers/apps/web`)).json()
    await expect(page.getByText(after[0].id, { exact: true })).toBeVisible()
    await expect(second.locator('.stack-card').filter({ hasText: 'apps/web' })).toContainText('web-v3')
    await second.close()
  })

  await test.step('failure and recovery replace stale errors without reloading', async () => {
    const broken = 'services:\n  worker:\n    image: busybox:1.37.0\n    definitely_invalid_property: true\n'
    let revision = await commit(request, { 'apps/web/compose.yaml': broken }, 'broken configuration')
    expect((await webhook(request, revision)).ok()).toBeTruthy()
    await expect(page.getByText('Failed', { exact: true }).first()).toBeVisible()
    await expect(page.getByText('compose up failed', { exact: false })).toBeVisible()
    revision = await commit(request, { 'apps/web/compose.yaml': compose('web-v4') }, 'fixed configuration')
    expect((await webhook(request, revision)).ok()).toBeTruthy()
    await expect(page.getByText('fixed configuration', { exact: true })).toBeVisible()
    await expect(page.getByText('compose up failed', { exact: false })).toHaveCount(0)
  })

  await test.step('manual image update finishes and deleted stacks leave the UI', async () => {
    // SPA navigation preserves the document marker.
    await page.locator('.n-page-header__back').click()
    await control(request, 'pull/arm?fail=true')
    await page.getByRole('button', { name: 'Update Stacks' }).click()
    await expect(page.locator('[data-layer-id="layer-a"]')).toContainText('Downloading')
    await page.waitForTimeout(600)
    await control(request, 'pull/advance')
    await expect(page.locator('[data-layer-id="layer-b"]')).toHaveAttribute('data-bytes', String(64 * 1024 * 1024))
    await control(request, 'pull/advance')
    await expect.poll(() => page.evaluate(() => (window as any).events.some((e: any) => e.type === 'update.progress' && e.data.type === 'stack_error' && e.data.error.includes('registry connection interrupted')))).toBe(true)
    await expect.poll(() => page.evaluate(() => (window as any).events.some((e: any) => e.type === 'update.progress' && e.data.type === 'completed'))).toBe(true)
    await expect(page.getByRole('button', { name: 'Update Stacks' })).toBeEnabled()
    await expect(page.locator('.pull-progress-section')).toHaveCount(0)
    const revision = await commit(request, { 'deep/nested/worker/compose.yaml': null }, 'remove worker')
    expect((await webhook(request, revision)).ok()).toBeTruthy()
    await expect(page.locator('.stack-card')).toHaveCount(2)
    await expect(page.locator('.stack-card').filter({ hasText: 'deep/nested/worker' })).toHaveCount(0)
    await expect.poll(async () => (await (await request.get(`${api}/api/stacks/containers/deep/nested/worker`)).json()).length).toBe(0)
    await expect.poll(async () => (await (await request.get(`${api}/test/idle`)).json()).idle).toBe(true)
    expect(await page.evaluate(() => (window as any).noReloadMarker)).toBe('same-document')
    expect(errors).toEqual([])
  })
})
