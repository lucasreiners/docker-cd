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

export interface ContainerInfo {
  id: string
  name: string
  service: string
  state: string
  health: string
  image: string
  ports?: string
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
    throw new Error(`Failed to trigger refresh: ${res.status}`)
  }
}

export async function triggerUpdate(): Promise<void> {
  const res = await fetch(`${getBaseURL()}/api/update`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Failed to trigger update: ${res.status}`)
  }
}

export async function requestJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${getBaseURL()}${path}`, options)
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error || `Request failed (${response.status})`)
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
