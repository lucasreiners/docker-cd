// Wire fields shared by reconciliation and scheduled/manual image updates.
export interface UpdateProgress {
  type:
    | 'started'
    | 'completed'
    | 'cancelled'
    | 'pruning'
    | 'stack_progress'
    | 'stack_sync_started'
    | 'stack_deploying'
    | 'stack_success'
    | 'stack_error'
    | 'image_pull_progress'
  source?: string
  cycle_id?: string
  stack?: string
  message?: string
  error?: string
  errors?: number
  timestamp?: string
  current?: number
  total?: number
  image?: string
  status?: string
  progress?: string
  layerId?: string
  bytesCurrent?: number
  bytesTotal?: number
  done?: boolean
}

export interface ActivityEntry {
  id: number
  at: string
  message: string
  stack?: string
  tone: 'success' | 'error' | 'info'
}
