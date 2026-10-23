import { afterEach, expect, it, vi } from 'vitest'
import { APIError, requestJSON } from '../src/services/api'

afterEach(() => vi.unstubAllGlobals())

it('preserves HTTP status so expired operation outcomes are distinguishable from lookup failure', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: 'Operation expired' }),
    }),
  )
  await expect(requestJSON('/api/operations/expired')).rejects.toMatchObject({
    status: 404,
    message: 'Operation expired',
  })
  await expect(requestJSON('/api/operations/expired')).rejects.toBeInstanceOf(APIError)
})

it('retains sanitized fallback errors for a non-JSON error response', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => {
        throw new SyntaxError('not JSON')
      },
    }),
  )
  await expect(requestJSON('/api/runtime')).rejects.toMatchObject({
    status: 503,
    message: 'Request failed (503)',
  })
})
