// SSE client wrapper with auto-reconnect and typed event handling.

import type { Operation, RefreshSnapshot, StackRecord } from './api'
import { getEventsURL } from './api'
import type { UpdateProgress } from './progress'
import { isWorkSnapshot, isWorkUpsert, type WorkSnapshot, type WorkUpsert } from './work'

export type ConnectionState = 'connected' | 'reconnecting' | 'disconnected'

export interface SSECallbacks {
  onWorkSnapshot?: (snapshot: WorkSnapshot) => void
  onWorkUpsert?: (upsert: WorkUpsert) => void
  onInvalidWork?: () => void
  onOperation?: (operation: Operation) => void
  onSnapshot?: (records: StackRecord[]) => void
  onUpsert?: (record: StackRecord) => void
  onDelete?: (path: string) => void
  onRefreshStatus?: (snapshot: RefreshSnapshot) => void
  onUpdateProgress?: (progress: UpdateProgress) => void
  onUpdateSnapshot?: (progress: UpdateProgress[]) => void
  onConnectionChange?: (state: ConnectionState) => void
}

export class SSEClient {
  private es: EventSource | null = null
  private callbacks: SSECallbacks
  private retryCount = 0
  private maxRetries = 50
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private _state: ConnectionState = 'disconnected'
  private generation = 0

  constructor(callbacks: SSECallbacks) {
    this.callbacks = callbacks
  }

  get state(): ConnectionState {
    return this._state
  }

  private setState(state: ConnectionState) {
    if (this._state !== state) {
      this._state = state
      this.callbacks.onConnectionChange?.(state)
    }
  }

  connect(): void {
    this.close()

    const url = getEventsURL()
    this.es = new EventSource(url)
    const es = this.es
    const generation = this.generation
    for (const name of ['work.snapshot', 'work.upsert'] as const) {
      es.addEventListener(name, (event: MessageEvent) => {
        if (generation !== this.generation || this.es !== es) return
        try {
          const value: unknown = JSON.parse(event.data)
          if (name === 'work.snapshot' && isWorkSnapshot(value))
            this.callbacks.onWorkSnapshot?.(value)
          else if (name === 'work.upsert' && isWorkUpsert(value))
            this.callbacks.onWorkUpsert?.(value)
          else this.callbacks.onInvalidWork?.()
        } catch {
          this.callbacks.onInvalidWork?.()
        }
      })
    }

    this.es.onopen = () => {
      if (this.es !== es) return
      this.retryCount = 0
      this.setState('connected')
    }

    this.es.addEventListener('stack.snapshot', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onSnapshot?.(data.records ?? [])
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('stack.upsert', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onUpsert?.(data.record)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('stack.delete', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onDelete?.(data.path)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('refresh.status', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onRefreshStatus?.(data)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('update.progress', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onUpdateProgress?.(data)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('update.snapshot', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        this.callbacks.onUpdateSnapshot?.(JSON.parse(e.data).progress ?? [])
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('operation.upsert', (e: MessageEvent) => {
      if (this.es !== es) return
      try {
        this.callbacks.onOperation?.(JSON.parse(e.data))
      } catch {
        /* Discard malformed events. */
      }
    })

    this.es.onerror = () => {
      if (this.es !== es) return
      this.es?.close()
      this.es = null

      if (this.retryCount < this.maxRetries) {
        this.setState('reconnecting')
        const delay = Math.min(1000 * 2 ** this.retryCount, 30000)
        this.retryCount++
        this.retryTimer = setTimeout(() => this.connect(), delay)
      } else {
        this.setState('disconnected')
      }
    }
  }

  close(): void {
    this.generation++
    if (this.retryTimer) {
      clearTimeout(this.retryTimer)
      this.retryTimer = null
    }
    if (this.es) {
      this.es.close()
      this.es = null
    }
    this.setState('disconnected')
  }
}
