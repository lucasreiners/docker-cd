import type { ContainerInfo, RefreshSnapshot, StackRecord, Instance, Operation } from '../../../frontend/src/services/api'

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
