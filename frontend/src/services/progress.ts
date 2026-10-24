// Legacy update.* payloads stay unchanged during v2 coexistence. They have no
// trustworthy run identity: never attach their layer bytes to v2 stack rows.
// Correlated work.snapshot/work.upsert wire types live in ./work.
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
