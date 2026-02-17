// SSE client wrapper with auto-reconnect and typed event handling.

import type { StackRecord } from './api'
import { getEventsURL } from './api'

export type ConnectionState = 'connected' | 'reconnecting' | 'disconnected'

export interface SSECallbacks {
  onSnapshot?: (records: StackRecord[]) => void
  onUpsert?: (record: StackRecord) => void
  onDelete?: (path: string) => void
  onRefreshStatus?: (snapshot: unknown) => void
  onConnectionChange?: (state: ConnectionState) => void
}

export class SSEClient {
  private es: EventSource | null = null
  private callbacks: SSECallbacks
  private retryCount = 0
  private maxRetries = 50
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private _state: ConnectionState = 'disconnected'

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

    this.es.onopen = () => {
      this.retryCount = 0
      this.setState('connected')
    }

    this.es.addEventListener('stack.snapshot', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onSnapshot?.(data.records ?? [])
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('stack.upsert', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onUpsert?.(data.record)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('stack.delete', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onDelete?.(data.path)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.addEventListener('refresh.status', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        this.callbacks.onRefreshStatus?.(data)
      } catch {
        // Ignore parse errors
      }
    })

    this.es.onerror = () => {
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
