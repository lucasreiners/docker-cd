import type { ContainerInfo, RefreshSnapshot, StackRecord, Instance, Operation, ImageIdentity, RepositoryDigest } from '../../../frontend/src/services/api'
import type { WorkEvent, WorkOperation, WorkRun, WorkRunningRun, WorkSnapshot, WorkUpsert } from '../../../frontend/src/services/work'

export function fixtureData() {
  const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()
  const definitions = [
    ['apps/immich', 'synced', 'Upgrade photo library', 3, 3],
    ['apps/paperless', 'synced', 'Configure document ingestion', 2, 2],
    ['infra/monitoring', 'synced', 'Update metrics retention', 2, 2],
    ['infra/traefik', 'synced', 'Add internal routes', 1, 1],
    ['media/jellyfin', 'failed', 'Update media server', 0, 1],
    ['tools/linkding', 'synced', 'Deploy bookmark service', 1, 1],
  ] as const
  const stacks: StackRecord[] = definitions.map(([path, status, message, running, total], i) => ({
    path, status, desiredRevision: 'a81bc9271234567890', composeFile: 'compose.yaml', composeHash: `desired-${i}`,
    syncedComposeHash: `applied-${i}`, syncedRevision: `a81bc92${i}1234567890`,
    syncedCommitMessage: message, containersRunning: running, containersTotal: total,
    syncedAt: ago(12 + i * 20), lastSyncAt: ago(i + 1), lastSyncStatus: status,
    ...(status === 'failed' ? { lastSyncError: 'compose up failed: registry denied access to jellyfin/server:latest' } : {}),
  }))
  const containers: Record<string, ContainerInfo[]> = Object.fromEntries(stacks.map((s) => [s.path,
    Array.from({ length: s.containersTotal ?? 0 }, (_, i) => ({
      id: `${s.path.replaceAll('/', '-')}-${i}-container-id`,
      name: `${s.path.split('/').at(-1)}-${i === 0 ? 'web' : 'worker'}`,
      service: i === 0 ? 'web' : `worker-${i}`,
      image: i === 0 ? 'ghcr.io/example/application:2026.10' : 'postgres:17',
      state: s.status === 'failed' ? 'exited' : 'running',
      health: s.path === 'infra/monitoring' && i === 0 ? 'unhealthy' : s.path === 'tools/linkding' ? 'none' : 'healthy',
      ports: i === 0 ? '8088:80/tcp, 8443:443/tcp' : '5432/tcp',
    })),
  ]))
  const refresh: RefreshSnapshot = {
    revision: 'a81bc9271234567890', ref: 'main', refType: 'branch',
    commitMessage: 'Update home-server services', refreshedAt: ago(1),
    refreshStatus: 'completed', updatesBlocked: false,
  }
  stacks[1] = {...stacks[1], status:'failed', requiresAcknowledgement:true, statusReason:'Compose configuration changed since the last approved deployment'}
  const instance:Instance={name:'home-server',version:'0.2.0-dev',repository:{url:'https://github.com/example/homelab',ref:'main',deployDir:'stacks',pollInterval:'5m',webhookConfigured:true,credentialsConfigured:true},dockerEndpoint:'/var/run/docker.sock',policies:{reconcile:true,remove:false,drift:'flag'},schedule:{enabled:true,cron:'0 3 * * *',timezone:'UTC',nextRun:new Date(Date.now()+3600000).toISOString()},capabilities:{inspect:true,apply:true,imageUpdates:true,history:true,configuration:true},history:{persistent:false,error:'',retention:1000},configurationSource:'environment'}
  const operations:Operation[]=[
    {id:'op-deploy',parentId:'op-refresh',kind:'deployment',trigger:'webhook',stack:'media/jellyfin',revision:refresh.revision,status:'failed',message:'Registry denied access to jellyfin/server:latest',startedAt:ago(5),finishedAt:ago(4),steps:[{at:ago(5),phase:'pulling',message:'Pulling jellyfin/server:latest'},{at:ago(4),phase:'failed',message:'Registry denied access'}]},
    {id:'op-refresh',kind:'refresh',trigger:'webhook',revision:refresh.revision,status:'completed',message:'Repository checked at a81bc927',startedAt:ago(6),finishedAt:ago(5),steps:[{at:ago(5),phase:'completed',message:'2 changed stacks discovered'}]},
    {id:'op-images',kind:'image_update',trigger:'schedule',status:'completed_with_errors',message:'Image update completed with 1 error',startedAt:ago(240),finishedAt:ago(237),steps:[{at:ago(238),phase:'stack',message:'Updated infra/traefik'},{at:ago(237),phase:'completed_with_errors',message:'media/jellyfin: registry denied access'}]},
  ]
  instance.capabilities.logs=true
  operations.push({id:'op-webhook',kind:'webhook',trigger:'github',status:'failed',message:'Invalid webhook signature · delivery fixture-42 · HTTP 401',startedAt:ago(12),finishedAt:ago(12),steps:[]})
  return { stacks, containers, refresh, instance, operations }
}

// Opt-in catalogue: legacy fixtures remain available for compatibility coverage.
export function operationalWorkspaceFixtures() {
  const at = (seconds: number) => new Date(Date.UTC(2026, 9, 23, 12, 0, seconds)).toISOString()
  const epoch = 'fixture-agent-a'
  const parent: WorkOperation = {
    id: 'old-refresh', rootOperationId: 'old-refresh', version: 2,
    kind: 'refresh', trigger: 'webhook', status: 'completed',
    message: 'Repository checked; child continues', startedAt: at(-3600), finishedAt: at(-3590),
  }
  const child: WorkOperation & { rootOperationId: string } = {
    id: 'old-deployment', parentId: parent.id, rootOperationId: parent.id, version: 1,
    kind: 'deployment', trigger: 'webhook', stack: 'apps/immich', status: 'running',
    message: 'Pulling image', startedAt: at(-3580),
  }
  const independent: WorkOperation & { rootOperationId: string } = {
    id: 'global-update', rootOperationId: 'global-update', version: 1,
    kind: 'image_update', trigger: 'schedule', status: 'running',
    message: 'Pulling stack image', startedAt: at(-30),
  }
  const pull = (runId: string, owner: WorkOperation & { rootOperationId: string }): WorkRunningRun => ({
    runId, operationId: owner.id, rootOperationId: owner.rootOperationId,
    ...(owner.parentId ? { parentOperationId: owner.parentId } : {}),
    source: owner.id === independent.id ? 'scheduler' : 'reconcile',
    stack: owner.stack ?? 'infra/monitoring', revision: 'a81bc927',
    cycleId: 'reusable-context-not-an-identity', status: 'running', phase: 'pulling',
    startedAt: at(-20), updatedAt: at(0), message: 'Pulling image',
    imageCount: 1, detailsTruncated: false,
    images: [{
      pullAttemptId: `${runId}-pull-1`, reference: 'docker.io/library/postgres:17',
      status: 'running', done: false, position: 1, total: 1, layerCount: 1,
      detailsTruncated: false,
      layers: [{ id: 'shared-layer', phase: 'downloading', status: 'Downloading',
        bytesCurrent: 128, bytesTotal: null, done: false, updatedAt: at(0) }],
    }],
  })
  const childRun = pull('run-deployment-1', child)
  const schedulerRun = pull('run-stack-pull-1', independent)
  const snapshot: WorkSnapshot = {
    schemaVersion: 2, epoch, sequence: 10, runs: [childRun, schedulerRun],
    operations: [parent, child, independent], detailsTruncated: false,
  }
  const waiting: WorkOperation = {
    id: 'accepted-action', rootOperationId: 'accepted-action', version: 1,
    kind: 'action', trigger: 'retry', stack: 'tools/linkding', revision: 'a81bc927',
    status: 'running', message: 'Waiting for execution lock', startedAt: at(1),
  }
  const checking: WorkOperation = {
    id: 'check-only', rootOperationId: 'check-only', version: 1,
    kind: 'refresh', trigger: 'manual', status: 'running',
    message: 'Checking repository; no image work', startedAt: at(2),
  }
  const operationOnly: WorkUpsert = { schemaVersion: 2, epoch, sequence: 11, operations: [waiting] }
  const checkOnly: WorkUpsert = { schemaVersion: 2, epoch, sequence: 12, operations: [checking] }
  const bytes: WorkUpsert = {
    schemaVersion: 2, epoch, sequence: 13, operations: [],
    run: { ...childRun, updatedAt: at(3), images: childRun.images.map(image => ({
      ...image, layers: image.layers.map(layer => ({ ...layer, bytesCurrent: 256, updatedAt: at(3) })),
    })) },
  }
  const completedPull: WorkRun = {
    ...schedulerRun, status: 'completed', phase: 'finished', finishedAt: at(4),
    updatedAt: at(4), message: 'Stack pull complete; workflow continues',
    images: schedulerRun.images.map(image => ({
      ...image, status: 'completed', done: true,
      layers: image.layers.map(layer => ({ ...layer, phase: 'finished', status: 'Pull complete', done: true, updatedAt: at(4) })),
    })),
  }
  const terminal: WorkUpsert = { schemaVersion: 2, epoch, sequence: 14, run: completedPull, operations: [] }
  const prune: WorkRun = {
    runId: 'run-prune-1', operationId: independent.id, rootOperationId: independent.id,
    source: 'scheduler', status: 'running', phase: 'pruning', startedAt: at(5),
    updatedAt: at(5), message: 'Pruning unused images host-wide',
    imageCount: 0, images: [], detailsTruncated: false,
    stackResults: { processed: 2, skipped: 1, failed: 1, outcomeCount: 3, detailsTruncated: false,
      outcomes: [
        { stack: 'infra/monitoring', status: 'completed', message: 'Updated' },
        { stack: 'apps/paperless', status: 'skipped', message: 'Approval required' },
        { stack: 'media/jellyfin', status: 'failed', message: 'Pull failed' },
      ] },
  }
  const pruning: WorkUpsert = { schemaVersion: 2, epoch, sequence: 15, run: prune, operations: [] }
  const retry: WorkUpsert = {
    schemaVersion: 2, epoch, sequence: 16, operations: [],
    run: pull('run-stack-pull-2', independent),
  }
  const reconnect: WorkSnapshot = {
    schemaVersion: 2, epoch, sequence: 20, runs: [], operations: [], detailsTruncated: false,
  }
  const restart: WorkSnapshot = {
    schemaVersion: 2, epoch: 'fixture-agent-b', sequence: 0,
    runs: [], operations: [], detailsTruncated: false,
  }
  const truncated: WorkSnapshot = {
    ...snapshot, sequence: 21, detailsTruncated: true,
    runs: [{ ...childRun, imageCount: 40, images: [], detailsTruncated: true }, schedulerRun],
  }
  const events: WorkEvent[] = [
    { event: 'work.snapshot', data: snapshot },
    ...[operationOnly, checkOnly, bytes, terminal, pruning, retry].map(data => ({ event: 'work.upsert' as const, data })),
    { event: 'work.snapshot', data: reconnect },
    { event: 'work.snapshot', data: restart },
  ]
  const digest = (repository: string, value: string): RepositoryDigest => ({
    repository, digest: `sha256:${value.repeat(64)}`, kind: 'unknown',
  })
  const preferred = digest('docker.io/library/postgres', 'a')
  const baseIdentity: ImageIdentity = {
    status: 'available', observedAt: at(0), source: 'container_image_inspect',
    imageId: `sha256:${'1'.repeat(64)}`, reference: 'postgres:17',
    platform: { os: 'linux', architecture: 'arm64', variant: 'v8' },
    repositoryDigests: [preferred], preferredRepositoryDigest: preferred, freshness: 'not_checked',
  }
  const { preferredRepositoryDigest: _preferred, ...withoutPreferred } = baseIdentity
  const { imageId: _imageId, platform: _platform, ...withoutLocalMetadata } = withoutPreferred
  const identities: Record<string, ImageIdentity> = {
    uniqueUnknownKind: baseIdentity,
    multipleRepositories: { ...baseIdentity, repositoryDigests: [preferred, digest('ghcr.io/example/postgres', 'b')] },
    localBuild: { ...withoutPreferred, status: 'unavailable', reference: 'local/app:dev', repositoryDigests: [], reason: 'not_recorded' },
    ambiguous: { ...withoutPreferred, status: 'ambiguous', repositoryDigests: [preferred, digest(preferred.repository, 'c')], reason: 'repository_ambiguous' },
    noRepositoryMatch: { ...withoutPreferred, status: 'unavailable', repositoryDigests: [digest('ghcr.io/example/mirror', 'b')], reason: 'not_recorded' },
    missingImage: { ...withoutLocalMetadata, status: 'unavailable', repositoryDigests: [], reason: 'image_missing' },
    replacedContainer: { ...withoutLocalMetadata, status: 'unavailable', repositoryDigests: [], reason: 'container_replaced' },
    denied: { ...withoutLocalMetadata, status: 'unavailable', repositoryDigests: [], reason: 'inspection_denied' },
    unsupported: { ...withoutLocalMetadata, status: 'unavailable', repositoryDigests: [], reason: 'unsupported' },
    unowned: { ...withoutLocalMetadata, status: 'unavailable', repositoryDigests: [], reason: 'ownership_unverified' },
  }
  const latest50: Operation[] = Array.from({ length: 50 }, (_, i) => ({
    id: `recent-${i}`, version: 2, kind: 'refresh', trigger: 'poll', status: 'completed',
    message: 'No changes', startedAt: at(-i - 1), finishedAt: at(-i), steps: [],
  }))
  return {
    snapshot, operationOnly, checkOnly, bytes, terminal, pruning, retry, reconnect, restart, truncated,
    events, identities, latest50,
    capabilities: { liveWorkVersion: 2, imageIdentity: { version: 1, source: 'container_image_inspect' } } satisfies Pick<Instance['capabilities'], 'liveWorkVersion' | 'imageIdentity'>,
    interruptedOperation: { ...child, version: 2, status: 'interrupted', finishedAt: at(30), message: 'Agent restarted before this operation finished' } satisfies WorkOperation,
  }
}

/** Browser-ready v2 state, deliberately independent of retained history pages. */
export function workspaceBrowserFixture() {
  const state = fixtureData()
  const catalogue = operationalWorkspaceFixtures()
  state.instance.capabilities = { ...state.instance.capabilities, ...catalogue.capabilities }
  state.containers['apps/immich'] = state.containers['apps/immich'].map((container, index) => ({
    ...container,
    imageIdentity: index === 0 ? catalogue.identities.uniqueUnknownKind : catalogue.identities.localBuild,
  }))
  state.containers['infra/monitoring'][0].imageIdentity = catalogue.identities.ambiguous
  return { ...state, work: catalogue.snapshot }
}
