# Data Model: Stack UI Enhancements

**Feature**: 007-stack-ui-enhancements  
**Phase**: 1 - Design  
**Date**: 2026-03-01

## Overview

This feature is frontend-only and does not introduce new data entities. It enhances the presentation and interaction with existing data from the backend API. This document describes the data structures used within the frontend Vue components.

## Existing API Entities (No Changes)

### StackRecord

**Source**: Backend API `/stacks`  
**Usage**: Unchanged - consumed by StackCard and StacksGrid

```typescript
interface StackRecord {
  path: string                    // e.g., "my-app/production"
  composeFile: string             // e.g., "docker-compose.yml"
  composeHash: string             // SHA hash of compose file
  status: 'missing' | 'syncing' | 'synced' | 'deleting' | 'failed'
  containersRunning?: number      // Count of running containers
  containersTotal?: number        // Total count of containers
  syncedRevision?: string         // Git commit SHA
  syncedCommitMessage?: string    // Git commit message
  syncedComposeHash?: string      // Hash at last sync
  syncedAt?: string               // ISO timestamp
  lastSyncAt?: string             // ISO timestamp
  lastSyncStatus?: string         // Sync result
  lastSyncError?: string          // Error message if failed
}
```

**Notes**:
- Does not include container-level details (names, ports)
- StackCard will fetch additional container data via separate API call

---

### ContainerInfo

**Source**: Backend API `/stacks/{path}/containers`  
**Usage**: Enhanced for this feature - parsed to extract port mappings

```typescript
interface ContainerInfo {
  id: string          // Container ID (12 chars)
  name: string        // Container name - used for display and sorting
  service: string     // Docker Compose service name
  state: string       // running, exited, paused, restarting, dead, created
  health: string      // healthy, unhealthy, starting, none
  image: string       // Image name and tag
  ports?: string      // Comma-separated port mappings (see format below)
}
```

**Ports Field Format** (from backend):
- Format: `"8080:80/tcp, 8443:443/tcp"` or `"80/tcp"` (no external mapping)
- Comma-separated list of port mappings
- Each mapping: `[external]:[internal]/[protocol]` or `[internal]/[protocol]`
- Protocol: `tcp` or `udp`

**Example Values**:
```typescript
// Single port with mapping
ContainerInfo { ports: "8080:80/tcp" }

// Multiple ports
ContainerInfo { ports: "8080:80/tcp, 8443:443/tcp, 9000:9000/tcp" }

// No external mapping (internal only)
ContainerInfo { ports: "3306/tcp" }

// No ports
ContainerInfo { ports: undefined }
```

---

## Frontend Data Structures (New)

### PortMapping

**Purpose**: Structured representation of a parsed port mapping  
**Scope**: Frontend utility functions and component logic  
**Location**: `frontend/src/utils/portUtils.ts`

```typescript
interface PortMapping {
  external: number | null  // External (host) port, null if not exposed
  internal: number         // Internal (container) port
  protocol: string         // "tcp" or "udp"
}
```

**Example Parsing**:
```typescript
// Input: "8080:80/tcp, 8443:443/tcp"
// Output:
[
  { external: 8080, internal: 80, protocol: 'tcp' },
  { external: 8443, internal: 443, protocol: 'tcp' }
]

// Input: "3306/tcp"
// Output:
[
  { external: null, internal: 3306, protocol: 'tcp' }
]
```

**Validation Rules**:
- `external` and `internal` must be valid port numbers (1-65535)
- `protocol` must be lowercase ("tcp" or "udp")
- If `external` is null, port is not accessible from host

---

### ContainerWithPorts (Computed/Derived)

**Purpose**: Enhanced container data with parsed port mappings  
**Scope**: Component-level computed property  
**Location**: StackCard.vue, StackDetail.vue

```typescript
interface ContainerWithPorts extends ContainerInfo {
  parsedPorts: PortMapping[]       // Parsed port mappings
  lowestExternalPort: number | null // Lowest port number for grid link
  hasExternalPorts: boolean        // True if any external port exists
}
```

**Computation Logic**:
```typescript
const containersWithPorts = computed<ContainerWithPorts[]>(() => {
  return containers.value.map(c => {
    const parsedPorts = parsePortString(c.ports || '')
    const externalPorts = parsedPorts
      .filter(p => p.external !== null)
      .map(p => p.external as number)
    
    return {
      ...c,
      parsedPorts,
      lowestExternalPort: externalPorts.length > 0 
        ? Math.min(...externalPorts) 
        : null,
      hasExternalPorts: externalPorts.length > 0
    }
  })
})
```

---

## Component State Management

### StackCard Component

**New State** (in addition to existing props):
```typescript
const containers = ref<ContainerInfo[]>([])
const containersLoading = ref(false)
const containersError = ref<string | null>(null)
```

**Computed Properties**:
```typescript
const sortedContainers = computed<ContainerWithPorts[]>(() => {
  return containersWithPorts.value
    .sort((a, b) => a.name.localeCompare(b.name))
})
```

**Lifecycle**:
- `onMounted`: Fetch containers for stack
- `onUnmounted`: Cleanup if needed (cancel fetch)

---

### StackDetail Component

**Modified State**:
```typescript
// Existing containers ref remains
const containers = ref<ContainerInfo[]>([])

// New computed for port pills
const containerPortMappings = computed(() => {
  return containers.value.map(c => ({
    containerName: c.name,
    ports: parsePortString(c.ports || '')
  }))
})
```

**De-duplication Strategy**:
- Ensure ports are rendered once per container
- Use proper Vue :key on v-for to prevent duplicate DOM nodes
- Possible fix: Change from `v-for="c in containers"` to `v-for="(c, index) in containers" :key="c.id"`

---

### App-Level State (Document Title)

**Location**: App.vue or main.ts  
**Implementation**: Vue Router afterEach hook

```typescript
import { useRouter } from 'vue-router'

const router = useRouter()

router.afterEach(() => {
  const hostname = window.location.hostname || 'localhost'
  document.title = `${hostname} - Docker-CD`
})
```

**State**: None stored - computed dynamically on each route change

---

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         Backend API                          │
└───────────────┬─────────────────────────┬───────────────────┘
                │                         │
        GET /stacks              GET /stacks/{path}/containers
                │                         │
                v                         v
         ┌─────────────┐          ┌──────────────┐
         │ StackRecord │          │ContainerInfo │
         └──────┬──────┘          └──────┬───────┘
                │                         │
                │                         │ parsePortString()
                v                         v
         ┌─────────────┐          ┌──────────────┐
         │  StackCard  │          │PortMapping[] │
         │  Component  │          └──────┬───────┘
         └──────┬──────┘                 │
                │                         │
                │<────────────────────────┘
                │
                v
         ┌─────────────────────────┐
         │  Display:               │
         │  - Container names      │
         │  - ↗ underlined links   │
         │  - Lowest port link     │
         └─────────────────────────┘

         ┌─────────────────────────┐
         │   StackDetail Component │
         │   - Port pills (once)   │
         │   - Clickable pills     │
         │   - 8080:80/tcp format  │
         └─────────────────────────┘
```

---

## Data Validation & Error Handling

### Port String Parsing Errors

**Invalid Format**: Gracefully handle malformed port strings
```typescript
function parsePortString(ports: string): PortMapping[] {
  if (!ports || ports.trim() === '') return []
  
  try {
    // Parse logic with validation
    return parsed.filter(p => isValidPort(p))
  } catch (error) {
    console.warn('Failed to parse port string:', ports, error)
    return [] // Return empty array, don't crash UI
  }
}
```

**Validation Rules**:
- Port numbers must be 1-65535
- Protocol must be recognized string
- Malformed entries are skipped, not throw errors

---

### API Fetch Errors

**Container Fetch Failure**: Display error state in StackCard
```vue
<n-alert v-if="containersError" type="warning" size="small">
  Failed to load containers
</n-alert>
```

**Behavior**: Card remains functional, shows error, doesn't block other cards

---

## Performance Characteristics

### Memory Footprint
- **Per StackCard**: ~1-5 KB for ContainerInfo array (typical 2-5 containers)
- **Grid with 10 stacks**: ~10-50 KB total container data
- **Acceptable**: Well within browser memory limits

### Computational Complexity
- **Port parsing**: O(n) where n = number of port mappings (typically 1-5)
- **Container sorting**: O(n log n) where n = number of containers (typically 2-10)
- **Total per card**: O(n log n) - negligible for typical container counts

### Caching Strategy
- **Component-level**: Containers fetched once per StackCard mount
- **No global cache**: Each card independently fetches (acceptable for now)
- **Future optimization**: Add container data to StackRecord API response

---

## Relationships & Dependencies

```
StackRecord (API)
    └── contains: containersTotal, containersRunning (counts only)
    └── links to: ContainerInfo[] via API call

ContainerInfo (API)
    └── contains: ports (string)
    └── transforms to: PortMapping[] (parsed)

PortMapping (Frontend)
    └── used by: StackCard (link generation)
    └── used by: StackDetail (pill display)

Document Title (Browser)
    └── derived from: window.location.hostname
    └── updated by: Vue Router afterEach hook
```

---

## State Transitions

### StackCard Container Loading

```
Initial State
    │
    v
[Loading: true] ──API call──> [Success]
    │                              │
    │                              v
    │                     [Display containers]
    │
    └─────API error────> [Error State]
                              │
                              v
                    [Show error message]
```

### Port Link Click Flow

```
User clicks container name
    │
    v
Extract lowestExternalPort
    │
    v
Construct URL (protocol + hostname + port)
    │
    v
window.open(url, '_blank')
    │
    v
Browser opens new tab
```

---

## Data Invariants

1. **Container Name Uniqueness**: Container names within a stack are assumed unique (enforced by Docker Compose)
2. **Port Format Consistency**: Backend always provides ports in documented format
3. **Lowest Port Selection**: If multiple ports exist, Math.min() reliably selects lowest
4. **Protocol Matching**: window.location.protocol always returns 'http:' or 'https:'
5. **Hostname Availability**: window.location.hostname always available in browser context

---

## Migration & Compatibility

**No data migration required** - This feature:
- Uses existing API responses unchanged
- Adds no new backend storage
- Maintains backward compatibility with all existing data

**API Version**: No API version change needed - pure presentation layer enhancement
