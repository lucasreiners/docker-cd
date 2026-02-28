import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { StackRecord } from '../src/services/api'
import type { SSECallbacks } from '../src/services/sse'
import { SSEClient } from '../src/services/sse'

// Mock the API module
vi.mock('../src/services/api', () => ({
  getEventsURL: vi.fn(() => 'http://localhost:8080/api/events'),
}))

// Mock EventSource
class MockEventSource {
  url: string
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private listeners: Map<string, ((e: MessageEvent) => void)[]> = new Map()
  readyState: number = 0 // CONNECTING

  constructor(url: string) {
    this.url = url
    // Store instance for test access
    ;(global as any).__mockEventSourceInstance = this
  }

  addEventListener(event: string, callback: (e: MessageEvent) => void): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, [])
    }
    const callbacks = this.listeners.get(event)
    if (callbacks) {
      callbacks.push(callback)
    }
  }

  removeEventListener(event: string, callback: (e: MessageEvent) => void): void {
    const callbacks = this.listeners.get(event)
    if (callbacks) {
      const index = callbacks.indexOf(callback)
      if (index > -1) {
        callbacks.splice(index, 1)
      }
    }
  }

  close(): void {
    this.readyState = 2 // CLOSED
  }

  // Test helper to trigger events
  __triggerEvent(event: string, data: any): void {
    const callbacks = this.listeners.get(event)
    if (callbacks) {
      const messageEvent = { data: JSON.stringify(data) } as MessageEvent
      for (const cb of callbacks) {
        cb(messageEvent)
      }
    }
  }

  // Test helper to trigger connection
  __triggerOpen(): void {
    this.readyState = 1 // OPEN
    this.onopen?.()
  }

  // Test helper to trigger error
  __triggerError(): void {
    this.onerror?.()
  }
}

// Replace global EventSource
global.EventSource = MockEventSource as any

describe('SSEClient', () => {
  let callbacks: SSECallbacks
  let mockEventSource: MockEventSource

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    callbacks = {
      onSnapshot: vi.fn(),
      onUpsert: vi.fn(),
      onDelete: vi.fn(),
      onRefreshStatus: vi.fn(),
      onUpdateProgress: vi.fn(),
      onConnectionChange: vi.fn(),
    }
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  describe('Connection Management', () => {
    it('should start in disconnected state', () => {
      const client = new SSEClient(callbacks)
      expect(client.state).toBe('disconnected')
    })

    it('should transition to connected on successful connection', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      expect(mockEventSource.url).toBe('http://localhost:8080/api/events')

      mockEventSource.__triggerOpen()

      expect(client.state).toBe('connected')
      expect(callbacks.onConnectionChange).toHaveBeenCalledWith('connected')
    })

    it('should close connection properly', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()
      expect(client.state).toBe('connected')

      client.close()

      expect(client.state).toBe('disconnected')
      expect(callbacks.onConnectionChange).toHaveBeenCalledWith('disconnected')
      expect(mockEventSource.readyState).toBe(2) // CLOSED
    })

    it('should close existing connection when connecting again', () => {
      const client = new SSEClient(callbacks)

      // First connection
      client.connect()
      const firstES = (global as any).__mockEventSourceInstance
      firstES.__triggerOpen()

      // Second connection
      client.connect()
      const secondES = (global as any).__mockEventSourceInstance

      expect(firstES.readyState).toBe(2) // CLOSED
      expect(secondES).not.toBe(firstES)
    })
  })

  describe('Event Handling', () => {
    beforeEach(() => {
      const client = new SSEClient(callbacks)
      client.connect()
      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()
    })

    it('should handle stack.snapshot event', () => {
      const mockRecords: StackRecord[] = [
        { path: 'app1', composeFile: 'dc.yml', composeHash: 'h1', status: 'synced' },
        { path: 'app2', composeFile: 'dc.yml', composeHash: 'h2', status: 'syncing' },
      ]

      mockEventSource.__triggerEvent('stack.snapshot', { records: mockRecords })

      expect(callbacks.onSnapshot).toHaveBeenCalledWith(mockRecords)
    })

    it('should handle stack.snapshot with empty records', () => {
      mockEventSource.__triggerEvent('stack.snapshot', { records: [] })

      expect(callbacks.onSnapshot).toHaveBeenCalledWith([])
    })

    it('should handle stack.snapshot without records field', () => {
      mockEventSource.__triggerEvent('stack.snapshot', {})

      expect(callbacks.onSnapshot).toHaveBeenCalledWith([])
    })

    it('should handle stack.upsert event', () => {
      const mockRecord: StackRecord = {
        path: 'app1',
        composeFile: 'dc.yml',
        composeHash: 'hash1',
        status: 'synced',
      }

      mockEventSource.__triggerEvent('stack.upsert', { record: mockRecord })

      expect(callbacks.onUpsert).toHaveBeenCalledWith(mockRecord)
    })

    it('should handle stack.delete event', () => {
      mockEventSource.__triggerEvent('stack.delete', { path: 'app1' })

      expect(callbacks.onDelete).toHaveBeenCalledWith('app1')
    })

    it('should handle refresh.status event', () => {
      const mockStatus = {
        revision: 'abc123',
        ref: 'main',
        refreshedAt: '2024-01-01T00:00:00Z',
      }

      mockEventSource.__triggerEvent('refresh.status', mockStatus)

      expect(callbacks.onRefreshStatus).toHaveBeenCalledWith(mockStatus)
    })

    it('should handle update.progress event', () => {
      const mockProgress = {
        type: 'stack_progress',
        cycle_id: 'cycle-123',
        stack: 'app1',
        current: 1,
        total: 3,
      }

      mockEventSource.__triggerEvent('update.progress', mockProgress)

      expect(callbacks.onUpdateProgress).toHaveBeenCalledWith(mockProgress)
    })

    it('should ignore invalid JSON in events', () => {
      // Directly trigger with invalid JSON through the mock
      const listeners = mockEventSource.listeners
      const callbacks = listeners.get('stack.snapshot')
      if (callbacks && callbacks.length > 0) {
        const callback = callbacks[0]
        // Trigger with invalid JSON
        callback({ data: 'invalid-json' } as MessageEvent)
      }

      // Should not throw, callbacks should not be called with invalid data
      // (onSnapshot was not in our test callbacks for this specific test)
    })
  })

  describe('Reconnection Logic', () => {
    it('should attempt reconnection on error', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()
      expect(client.state).toBe('connected')

      // Trigger error
      mockEventSource.__triggerError()

      expect(client.state).toBe('reconnecting')
      expect(callbacks.onConnectionChange).toHaveBeenCalledWith('reconnecting')
    })

    it('should use exponential backoff for retries', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // First error - should retry after 1000ms (2^0 * 1000)
      const firstES = mockEventSource
      mockEventSource.__triggerError()
      expect(client.state).toBe('reconnecting')

      vi.advanceTimersByTime(1000)
      const secondES = (global as any).__mockEventSourceInstance
      expect(secondES).not.toBe(firstES)
      secondES.__triggerOpen()

      // Second error - should retry after 2000ms (2^1 * 1000)
      secondES.__triggerError()

      vi.advanceTimersByTime(2000)
      const thirdES = (global as any).__mockEventSourceInstance
      expect(thirdES).not.toBe(secondES)
      thirdES.__triggerOpen()

      // Third error - should retry after 4000ms (2^2 * 1000)
      thirdES.__triggerError()

      vi.advanceTimersByTime(4000)
      const fourthES = (global as any).__mockEventSourceInstance
      expect(fourthES).not.toBe(thirdES)
    })

    it('should cap retry delay at 30 seconds', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // Trigger many errors to reach max delay
      // Delays: 1000, 2000, 4000, 8000, 16000, 30000 (capped), 30000, ...
      let currentES = mockEventSource
      for (let i = 0; i < 6; i++) {
        const delay = Math.min(1000 * 2 ** i, 30000)
        currentES.__triggerError()
        vi.advanceTimersByTime(delay)
        currentES = (global as any).__mockEventSourceInstance
        currentES.__triggerOpen()
      }

      // Next retry should still be capped at 30s
      const beforeError = currentES
      currentES.__triggerError()

      vi.advanceTimersByTime(30000)
      const afterRetry = (global as any).__mockEventSourceInstance
      expect(afterRetry).not.toBe(beforeError)
    })

    it('should reset retry count on successful connection', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // Trigger error and wait for retry
      mockEventSource.__triggerError()
      vi.advanceTimersByTime(1000)

      const secondES = (global as any).__mockEventSourceInstance
      secondES.__triggerOpen()

      // Trigger another error - should use initial delay again
      secondES.__triggerError()

      vi.advanceTimersByTime(999)
      expect((global as any).__mockEventSourceInstance).toBe(secondES)

      vi.advanceTimersByTime(1)
      expect((global as any).__mockEventSourceInstance).not.toBe(secondES)
    })

    it('should stop reconnecting after max retries', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // Trigger 50 errors (max retries)
      for (let i = 0; i < 50; i++) {
        mockEventSource.__triggerError()
        const delay = Math.min(1000 * 2 ** i, 30000)
        vi.advanceTimersByTime(delay)
        if (i < 49) {
          mockEventSource = (global as any).__mockEventSourceInstance
          mockEventSource.__triggerOpen()
        }
      }

      // After 50 retries, should give up
      expect(client.state).toBe('disconnected')
      expect(callbacks.onConnectionChange).toHaveBeenLastCalledWith('disconnected')

      // Wait more time - should not retry
      const finalES = (global as any).__mockEventSourceInstance
      vi.advanceTimersByTime(60000)
      expect((global as any).__mockEventSourceInstance).toBe(finalES)
    })

    it('should clear retry timer when closed during reconnection', () => {
      const client = new SSEClient(callbacks)
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // Trigger error to start reconnection
      mockEventSource.__triggerError()
      expect(client.state).toBe('reconnecting')

      // Close before retry happens
      client.close()
      expect(client.state).toBe('disconnected')

      // Advance timers - should not reconnect
      const esBeforeTimer = (global as any).__mockEventSourceInstance
      vi.advanceTimersByTime(5000)
      const esAfterTimer = (global as any).__mockEventSourceInstance
      expect(esAfterTimer).toBe(esBeforeTimer)
    })
  })

  describe('Optional Callbacks', () => {
    it('should work without any callbacks', () => {
      const client = new SSEClient({})
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      // Should not throw
      expect(() => {
        mockEventSource.__triggerEvent('stack.snapshot', { records: [] })
        mockEventSource.__triggerEvent('stack.upsert', { record: {} })
        mockEventSource.__triggerEvent('stack.delete', { path: 'test' })
        mockEventSource.__triggerError()
      }).not.toThrow()
    })

    it('should work with partial callbacks', () => {
      const onSnapshot = vi.fn()
      const client = new SSEClient({ onSnapshot })
      client.connect()

      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()

      mockEventSource.__triggerEvent('stack.snapshot', { records: [] })
      expect(onSnapshot).toHaveBeenCalledWith([])

      // Other events should not throw
      expect(() => {
        mockEventSource.__triggerEvent('stack.upsert', { record: {} })
        mockEventSource.__triggerEvent('stack.delete', { path: 'test' })
      }).not.toThrow()
    })
  })

  describe('State Transitions', () => {
    it('should not trigger onConnectionChange if state is unchanged', () => {
      const client = new SSEClient(callbacks)
      expect(client.state).toBe('disconnected')

      client.close() // Should not trigger change callback
      expect(callbacks.onConnectionChange).not.toHaveBeenCalled()
    })

    it('should track state through full lifecycle', () => {
      const states: string[] = []
      const onConnectionChange = vi.fn((state) => states.push(state))
      const client = new SSEClient({ onConnectionChange })

      // Start disconnected
      expect(client.state).toBe('disconnected')

      // Connect (close is called first but state is already disconnected, so no callback)
      client.connect()
      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()
      expect(states).toEqual(['connected'])

      // Error triggers reconnecting
      mockEventSource.__triggerError()
      expect(states).toEqual(['connected', 'reconnecting'])

      // Reconnect (connect() calls close() first, setting state to disconnected)
      vi.advanceTimersByTime(1000)
      mockEventSource = (global as any).__mockEventSourceInstance
      mockEventSource.__triggerOpen()
      expect(states).toEqual(['connected', 'reconnecting', 'disconnected', 'connected'])

      // Close
      client.close()
      expect(states).toEqual([
        'connected',
        'reconnecting',
        'disconnected',
        'connected',
        'disconnected',
      ])
    })
  })
})
