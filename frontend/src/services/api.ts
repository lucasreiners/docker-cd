// API client for Docker-CD backend.
// Uses window.__DOCKER_CD_CONFIG__ for runtime base URL (injected by entrypoint.sh)
// or falls back to relative paths (Vite dev proxy).

export interface StackRecord {
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
