/**
 * Port mapping utility functions for parsing and handling Docker port strings
 * @module portUtils
 */

/**
 * Represents a parsed Docker port mapping
 */
export interface PortMapping {
  external: number | null // Host port, null if not exposed
  internal: number // Container port
  protocol: string // "tcp" or "udp"
}

/**
 * Parse Docker port string into structured array of port mappings
 *
 * @param ports - String in format "8080:80/tcp, 8443:443/tcp" or empty/undefined
 * @returns Array of structured PortMapping objects
 *
 * @example
 * parsePortString("8080:80/tcp")
 * // Returns: [{ external: 8080, internal: 80, protocol: "tcp" }]
 *
 * parsePortString("8080:80/tcp, 8443:443/tcp")
 * // Returns: [
 * //   { external: 8080, internal: 80, protocol: "tcp" },
 * //   { external: 8443, internal: 443, protocol: "tcp" }
 * // ]
 *
 * parsePortString("3306/tcp")
 * // Returns: [{ external: null, internal: 3306, protocol: "tcp" }]
 */
export function parsePortString(ports: string): PortMapping[] {
  if (!ports || typeof ports !== 'string' || ports.trim() === '') {
    return []
  }

  const result: PortMapping[] = []
  const portEntries = ports.split(',').map((p) => p.trim())

  for (const entry of portEntries) {
    try {
      // Match patterns: "8080:80/tcp" or "80/tcp"
      const match = entry.match(/^(?:(\d+):)?(\d+)\/(\w+)$/)

      if (!match) {
        console.warn(`[portUtils] Skipping malformed port entry: "${entry}"`)
        continue
      }

      const external = match[1] ? parseInt(match[1], 10) : null
      const internal = parseInt(match[2], 10)
      const protocol = match[3].toLowerCase()

      // Validate port numbers (1-65535)
      if (external !== null && (external < 1 || external > 65535)) {
        console.warn(`[portUtils] Invalid external port number: ${external}`)
        continue
      }
      if (internal < 1 || internal > 65535) {
        console.warn(`[portUtils] Invalid internal port number: ${internal}`)
        continue
      }

      result.push({
        external,
        internal,
        protocol,
      })
    } catch (error) {
      console.warn(`[portUtils] Error parsing port entry "${entry}":`, error)
    }
  }

  // Deduplicate port mappings based on external:internal/protocol combination
  const seen = new Set<string>()
  const deduplicated: PortMapping[] = []

  for (const port of result) {
    const key = `${port.external}:${port.internal}/${port.protocol}`
    if (!seen.has(key)) {
      seen.add(key)
      deduplicated.push(port)
    }
  }

  return deduplicated
}

/**
 * Extract lowest external port number from port string
 *
 * @param ports - String in format "8080:80/tcp, 8443:443/tcp" or empty/undefined
 * @returns Lowest external port number, or null if no external ports
 *
 * @example
 * getLowestExternalPort("8080:80/tcp, 8443:443/tcp")
 * // Returns: 8080
 *
 * getLowestExternalPort("9000:9000/tcp, 8080:80/tcp")
 * // Returns: 8080
 *
 * getLowestExternalPort("3306/tcp")
 * // Returns: null (no external port)
 */
export function getLowestExternalPort(ports: string): number | null {
  const parsedPorts = parsePortString(ports)
  const externalPorts = parsedPorts
    .filter((p) => p.external !== null)
    .map((p) => p.external as number)

  if (externalPorts.length === 0) {
    return null
  }

  return Math.min(...externalPorts)
}

/**
 * Construct full URL to access container port from browser
 * Matches current page protocol (HTTP/HTTPS) and hostname
 *
 * @param port - Port number (1-65535)
 * @returns Full URL string "http://hostname:port" or "https://hostname:port"
 *
 * @example
 * // Assuming current URL is http://localhost:3000
 * buildPortURL(8080)
 * // Returns: "http://localhost:8080"
 *
 * // Assuming current URL is https://prod-server.com
 * buildPortURL(8443)
 * // Returns: "https://prod-server.com:8443"
 */
export function buildPortURL(port: number): string {
  const protocol = window.location.protocol.replace(':', '')
  const hostname = window.location.hostname
  return `${protocol}://${hostname}:${port}`
}
