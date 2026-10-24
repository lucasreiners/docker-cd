// API client for Docker-CD backend.
// Uses window.__DOCKER_CD_CONFIG__ for runtime base URL (injected by entrypoint.sh)
// or falls back to relative paths (Vite dev proxy).

export interface StackRecord {
  desiredRevision?: string
  statusReason?: string
  requiresAcknowledgement?: boolean
  path: string
  composeFile: string
  composeHash: string
  status: 'missing' | 'syncing' | 'synced' | 'deleting' | 'failed'
  containersRunning?: number
  containersTotal?: number
  syncedRevision?: string
  syncedCommitMessage?: string
  syncedComposeHash?: string
  syncedAt?: string
  lastSyncAt?: string
  lastSyncStatus?: string
  lastSyncError?: string
}

export interface RepositoryDigest {
  repository: string
  digest: string
  kind: 'index' | 'manifest' | 'unknown'
}

export type ImageIdentityReason =
  | 'not_recorded'
  | 'image_missing'
  | 'container_replaced'
  | 'inspection_denied'
  | 'repository_ambiguous'
  | 'unsupported'
  | 'ownership_unverified'
  | 'inspection_failed'

interface ImageIdentityBase {
  observedAt: string
  source: 'container_image_inspect'
  imageId?: string
  reference: string
  platform?: { os: string; architecture: string; variant?: string }
  repositoryDigests: RepositoryDigest[]
  freshness: 'not_checked'
}

export type ImageIdentity = ImageIdentityBase &
  (
    | { status: 'available'; preferredRepositoryDigest: RepositoryDigest; reason?: never }
    | { status: 'ambiguous'; preferredRepositoryDigest?: never; reason: 'repository_ambiguous' }
    | {
        status: 'unavailable'
        preferredRepositoryDigest?: never
        reason: Exclude<ImageIdentityReason, 'repository_ambiguous'>
      }
  )

export interface ContainerInfo {
  id: string
  name: string
  service: string
  state: string
  health: string
  image: string
  ports?: string
  imageIdentity?: ImageIdentity
}

// Server observation times are not browser fetch times. Failed reads preserve
// the previous successful containers/observedAt; null means never observed.
export interface RuntimeObservation {
  containers: ContainerInfo[]
  error: string | null
  observedAt: string | null
  attemptedAt?: string
}

export interface RefreshSnapshot {
  revision: string
  commitMessage?: string
  ref: string
  refType: string
  refreshedAt: string
  refreshStatus: 'refreshing' | 'queued' | 'completed' | 'failed'
  refreshError?: string
  updatesBlocked: boolean
  blockedReason?: string
  localPath?: string
}

declare global {
  interface Window {
    __DOCKER_CD_CONFIG__?: {
      API_BASE_URL?: string
    }
  }
}

function getBaseURL(): string {
  return window.__DOCKER_CD_CONFIG__?.API_BASE_URL ?? ''
}

export function apiURL(path: string): string {
  return `${getBaseURL()}${path}`
}

export async function fetchStacks(): Promise<StackRecord[]> {
  const res = await fetch(`${getBaseURL()}/api/stacks`)
  if (!res.ok) {
    throw new Error(`Failed to fetch stacks: ${res.status}`)
  }
  return res.json()
}

export async function fetchRefreshStatus(): Promise<RefreshSnapshot> {
  const res = await fetch(`${getBaseURL()}/api/refresh-status`)
  if (!res.ok) {
    throw new Error(`Failed to fetch refresh status: ${res.status}`)
  }
  return res.json()
}

export async function triggerRefresh(): Promise<void> {
  const res = await fetch(`${getBaseURL()}/api/refresh`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new APIError(body.error || `Failed to trigger refresh: ${res.status}`, res.status)
  }
}

export async function triggerUpdate(): Promise<void> {
  const res = await fetch(`${getBaseURL()}/api/update`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new APIError(body.error || `Failed to trigger update: ${res.status}`, res.status)
  }
}

export class APIError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message)
  }
}

export type StackAction = 'update' | 'retry' | 'acknowledge'
export interface ReviewedStackAction {
  action: StackAction
  stack: string
  revision: string
}

export async function submitStackAction(
  body: ReviewedStackAction,
): Promise<{ operationId: string }> {
  const result = await requestJSON<{ operationId: string }>('/api/operations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!result || typeof result.operationId !== 'string' || !result.operationId) {
    throw new Error('Accepted action returned no operation identity')
  }
  return result
}

export async function requestJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${getBaseURL()}${path}`, options)
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new APIError(body.error || `Request failed (${response.status})`, response.status)
  }
  return response.json()
}

export interface Operation {
  version?: number
  id: string
  parentId?: string
  kind: string
  trigger: string
  stack?: string
  revision?: string
  status: string
  message: string
  startedAt: string
  finishedAt?: string
  steps: { at: string; phase: string; message: string }[]
}
export interface Instance {
  name: string
  version: string
  repository: {
    url: string
    ref: string
    deployDir: string
    pollInterval: string
    webhookConfigured: boolean
    credentialsConfigured: boolean
  }
  dockerEndpoint: string
  policies: { reconcile: boolean; remove: boolean; drift: string }
  schedule: { enabled: boolean; cron: string; timezone: string; nextRun: string }
  capabilities: {
    liveWorkVersion?: 2
    imageIdentity?: { version: 1; source: 'container_image_inspect' }
    logs?: boolean
    inspect: boolean
    apply: boolean
    imageUpdates: boolean
    history: boolean
    configuration: boolean
  }
  history: { persistent: boolean; error: string; retention: number }
  configurationSource: string
}

export function getEventsURL(): string {
  return `${getBaseURL()}/api/events`
}

export async function fetchContainers(stackPath: string): Promise<ContainerInfo[]> {
  const res = await fetch(`${getBaseURL()}/api/stacks/containers/${stackPath}`)
  if (!res.ok) {
    throw new Error(`Failed to fetch containers: ${res.status}`)
  }
  return res.json()
}
