import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { buildPortURL, getLowestExternalPort, parsePortString } from '@/utils/portUtils'

describe('parsePortString', () => {
  test('parses single port mapping', () => {
    const result = parsePortString('8080:80/tcp')
    expect(result).toEqual([{ external: 8080, internal: 80, protocol: 'tcp' }])
  })

  test('parses multiple port mappings', () => {
    const result = parsePortString('8080:80/tcp, 8443:443/tcp, 9000:9000/tcp')
    expect(result).toEqual([
      { external: 8080, internal: 80, protocol: 'tcp' },
      { external: 8443, internal: 443, protocol: 'tcp' },
      { external: 9000, internal: 9000, protocol: 'tcp' },
    ])
  })

  test('handles port without external mapping', () => {
    const result = parsePortString('3306/tcp')
    expect(result).toEqual([{ external: null, internal: 3306, protocol: 'tcp' }])
  })

  test('handles mixed ports (with and without external mapping)', () => {
    const result = parsePortString('8080:80/tcp, 3306/tcp')
    expect(result).toEqual([
      { external: 8080, internal: 80, protocol: 'tcp' },
      { external: null, internal: 3306, protocol: 'tcp' },
    ])
  })

  test('handles UDP protocol', () => {
    const result = parsePortString('53:53/udp')
    expect(result).toEqual([{ external: 53, internal: 53, protocol: 'udp' }])
  })

  test('handles empty string', () => {
    const result = parsePortString('')
    expect(result).toEqual([])
  })

  test('handles undefined input', () => {
    const result = parsePortString(undefined as any)
    expect(result).toEqual([])
  })

  test('handles null input', () => {
    const result = parsePortString(null as any)
    expect(result).toEqual([])
  })

  test('handles whitespace-only string', () => {
    const result = parsePortString('   ')
    expect(result).toEqual([])
  })

  test('handles malformed strings gracefully', () => {
    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const result = parsePortString('invalid-port-string')
    expect(result).toEqual([])
    expect(consoleSpy).toHaveBeenCalled()

    consoleSpy.mockRestore()
  })

  test('skips malformed entries but keeps valid ones', () => {
    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const result = parsePortString('8080:80/tcp, invalid, 9000:9000/tcp')
    expect(result).toEqual([
      { external: 8080, internal: 80, protocol: 'tcp' },
      { external: 9000, internal: 9000, protocol: 'tcp' },
    ])

    consoleSpy.mockRestore()
  })

  test('handles ports with extra whitespace', () => {
    const result = parsePortString('  8080:80/tcp  ,  8443:443/tcp  ')
    expect(result).toEqual([
      { external: 8080, internal: 80, protocol: 'tcp' },
      { external: 8443, internal: 443, protocol: 'tcp' },
    ])
  })

  test('validates port number range (rejects invalid)', () => {
    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const result = parsePortString('99999:80/tcp')
    expect(result).toEqual([])
    expect(consoleSpy).toHaveBeenCalled()

    consoleSpy.mockRestore()
  })

  test('validates internal port number range', () => {
    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const result = parsePortString('8080:99999/tcp')
    expect(result).toEqual([])
    expect(consoleSpy).toHaveBeenCalled()

    consoleSpy.mockRestore()
  })

  test('normalizes protocol to lowercase', () => {
    const result = parsePortString('8080:80/TCP')
    expect(result).toEqual([{ external: 8080, internal: 80, protocol: 'tcp' }])
  })
})

describe('getLowestExternalPort', () => {
  test('returns lowest port from multiple mappings', () => {
    const result = getLowestExternalPort('9000:9000/tcp, 8080:80/tcp, 8443:443/tcp')
    expect(result).toBe(8080)
  })

  test('returns single port when only one mapping exists', () => {
    const result = getLowestExternalPort('8080:80/tcp')
    expect(result).toBe(8080)
  })

  test('returns null for ports without external mapping', () => {
    const result = getLowestExternalPort('3306/tcp')
    expect(result).toBe(null)
  })

  test('returns null for empty string', () => {
    const result = getLowestExternalPort('')
    expect(result).toBe(null)
  })

  test('returns null for undefined input', () => {
    const result = getLowestExternalPort(undefined as any)
    expect(result).toBe(null)
  })

  test('ignores ports without external mapping when finding lowest', () => {
    const result = getLowestExternalPort('9000:9000/tcp, 3306/tcp, 8080:80/tcp')
    expect(result).toBe(8080)
  })

  test('returns lowest port even with mixed protocols', () => {
    const result = getLowestExternalPort('8080:80/tcp, 53:53/udp, 9000:9000/tcp')
    expect(result).toBe(53)
  })
})

describe('buildPortURL', () => {
  let originalLocation: Location

  beforeEach(() => {
    originalLocation = window.location
  })

  afterEach(() => {
    // Restore original location
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
  })

  test('constructs URL with HTTP protocol', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'http:',
        hostname: 'localhost',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(8080)
    expect(result).toBe('http://localhost:8080')
  })

  test('constructs URL with HTTPS protocol', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'https:',
        hostname: 'prod-server.com',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(8443)
    expect(result).toBe('https://prod-server.com:8443')
  })

  test('uses correct hostname', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'http:',
        hostname: 'example.com',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(3000)
    expect(result).toBe('http://example.com:3000')
  })

  test('formats port correctly', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'http:',
        hostname: 'localhost',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(9000)
    expect(result).toBe('http://localhost:9000')
  })

  test('handles IP address as hostname', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'http:',
        hostname: '192.168.1.100',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(8080)
    expect(result).toBe('http://192.168.1.100:8080')
  })

  test('matches current page protocol to prevent mixed-content warnings', () => {
    Object.defineProperty(window, 'location', {
      value: {
        protocol: 'https:',
        hostname: 'secure.example.com',
      },
      writable: true,
      configurable: true,
    })

    const result = buildPortURL(8443)
    expect(result).toBe('https://secure.example.com:8443')
  })
})
