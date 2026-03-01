# Component API Contracts: Stack UI Enhancements

**Feature**: 007-stack-ui-enhancements  
**Phase**: 1 - Design  
**Date**: 2026-03-01

## Overview

This document defines the public interfaces (props, events, and composable signatures) for components and utilities modified or created in this feature. These contracts serve as the implementation guide and test specification.

---

## Utility Functions

### portUtils.ts

Location: `frontend/src/utils/portUtils.ts`

#### parsePortString

**Purpose**: Parse Docker port string into structured array of port mappings

**Signature**:
```typescript
function parsePortString(ports: string): PortMapping[]

interface PortMapping {
  external: number | null  // Host port, null if not exposed
  internal: number         // Container port
  protocol: string         // "tcp" or "udp"
}
```

**Contract**:
- **Input**: String in format `"8080:80/tcp, 8443:443/tcp"` or empty/undefined
- **Output**: Array of structured PortMapping objects
- **Error Handling**: Returns empty array for invalid input, logs warning, never throws
- **Validation**: Filters out malformed entries, keeps valid ones

**Examples**:
```typescript
parsePortString("8080:80/tcp")
// Returns: [{ external: 8080, internal: 80, protocol: "tcp" }]

parsePortString("8080:80/tcp, 8443:443/tcp")
// Returns: [
//   { external: 8080, internal: 80, protocol: "tcp" },
//   { external: 8443, internal: 443, protocol: "tcp" }
// ]

parsePortString("3306/tcp")
// Returns: [{ external: null, internal: 3306, protocol: "tcp" }]

parsePortString("")
// Returns: []

parsePortString("invalid-port-string")
// Returns: [] (logs warning)
```

**Test Coverage**:
- ✅ Valid single port with mapping
- ✅ Valid multiple ports
- ✅ Port without external mapping
- ✅ Empty string
- ✅ Undefined input
- ✅ Malformed strings

---

#### getLowestExternalPort

**Purpose**: Extract lowest external port number from port string

**Signature**:
```typescript
function getLowestExternalPort(ports: string): number | null
```

**Contract**:
- **Input**: String in format `"8080:80/tcp, 8443:443/tcp"` or empty/undefined
- **Output**: Lowest external port number, or null if no external ports
- **Logic**: Parses ports, filters for external only, returns Math.min()

**Examples**:
```typescript
getLowestExternalPort("8080:80/tcp, 8443:443/tcp")
// Returns: 8080

getLowestExternalPort("9000:9000/tcp, 8080:80/tcp")
// Returns: 8080

getLowestExternalPort("3306/tcp")
// Returns: null (no external port)

getLowestExternalPort("")
// Returns: null
```

**Test Coverage**:
- ✅ Multiple ports returns lowest
- ✅ Single port returns that port
- ✅ No external ports returns null
- ✅ Empty string returns null

---

#### buildPortURL

**Purpose**: Construct full URL to access container port from browser

**Signature**:
```typescript
function buildPortURL(port: number): string
```

**Contract**:
- **Input**: Port number (1-65535)
- **Output**: Full URL string `"http://hostname:port"` or `"https://hostname:port"`
- **Protocol**: Matches current page protocol (window.location.protocol)
- **Hostname**: Uses current page hostname (window.location.hostname)

**Examples**:
```typescript
// Assuming current URL is http://localhost:3000
buildPortURL(8080)
// Returns: "http://localhost:8080"

// Assuming current URL is https://prod-server.com
buildPortURL(8443)
// Returns: "https://prod-server.com:8443"
```

**Test Coverage**:
- ✅ Matches HTTP protocol
- ✅ Matches HTTPS protocol
- ✅ Uses correct hostname
- ✅ Formats port correctly

---

## Component Interfaces

### StackCard.vue (Modified)

Location: `frontend/src/components/StackCard.vue`

#### Props

**Existing** (unchanged):
```typescript
interface Props {
  stack: StackRecord  // Stack data from API
}
```

**No new props added**

#### New Internal State

```typescript
const containers = ref<ContainerInfo[]>([])
const containersLoading = ref(false)
const containersError = ref<string | null>(null)
```

#### New Computed Properties

```typescript
const sortedContainers = computed<ContainerWithPorts[]>(() => {
  // Returns containers sorted alphabetically by name
  // Each container enhanced with parsed ports and lowest port
})
```

#### New Methods

```typescript
function handleContainerClick(container: ContainerWithPorts): void {
  // Opens lowest external port in new browser tab/window
  // Does nothing if container has no external ports
}
```

#### Template Contract

**Display Rules**:
1. For each container in alphabetical order by name:
   - If `hasExternalPorts === true`: 
     - Show `↗` character prefix
     - Show container name with underline
     - Make clickable (opens lowest port)
   - If `hasExternalPorts === false`:
     - Show container name as plain text
     - No arrow, no underline, not clickable

2. Card must grow vertically to fit all containers
3. Loading state shows spinner/skeleton
4. Error state shows warning message

**Example Template Structure**:
```vue
<template>
  <n-card>
    <!-- Existing header content -->
    
    <div class="containers-list">
      <n-spin v-if="containersLoading" />
      <n-text v-else-if="containersError" type="warning">
        Failed to load containers
      </n-text>
      <div v-else>
        <div v-for="c in sortedContainers" :key="c.id">
          <a 
            v-if="c.hasExternalPorts"
            @click.prevent="handleContainerClick(c)"
            class="container-link"
          >
            ↗ <span class="underlined">{{ c.name }}</span>
          </a>
          <span v-else>{{ c.name }}</span>
        </div>
      </div>
    </div>
  </n-card>
</template>
```

#### Test Coverage
- ✅ Fetches containers on mount
- ✅ Sorts containers alphabetically
- ✅ Shows arrow + underline for linkable containers
- ✅ Shows plain text for non-linkable containers
- ✅ Click opens correct port URL
- ✅ Handles loading state
- ✅ Handles error state

---

### StackDetail.vue (Modified)

Location: `frontend/src/pages/StackDetail.vue`

#### Props

**Existing** (unchanged via route params):
```typescript
const stackPath = computed(() => route.params.path)
```

**No new props**

#### Modified Template Logic

**Ports Display Contract**:
1. Parse each container's ports string
2. Display each port mapping as separate n-tag pill
3. Format: `"8080:80/tcp"` (external:internal/protocol)
4. Each pill is clickable
5. **No duplicates**: Ensure proper Vue :key usage

**Example Template Structure**:
```vue
<template>
  <div v-for="container in containers" :key="container.id">
    <n-text strong>{{ container.name }}</n-text>
    
    <div v-if="container.ports" class="ports-container">
      <n-tag
        v-for="port in parsePortString(container.ports)"
        :key="`${container.id}-${port.external}-${port.internal}`"
        size="small"
        round
        :bordered="false"
        class="port-pill"
        @click="openPort(port.external)"
      >
        {{ port.external }}:{{ port.internal }}/{{ port.protocol }}
      </n-tag>
    </div>
  </div>
</template>
```

#### New Methods

```typescript
function openPort(port: number | null): void {
  // Opens port URL in new tab/window if port is not null
  if (port !== null) {
    const url = buildPortURL(port)
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}
```

#### Duplicate Fix Strategy

**Problem**: "all the port mappings are shown twice somehow"

**Solution Approach**:
1. Verify v-for :key is unique (use container.id + port numbers)
2. Check if containers array has duplicates (should not)
3. Ensure parsePortString not called multiple times per render

**Key Best Practice**:
```vue
<!-- CORRECT: Unique key per pill -->
<n-tag
  v-for="port in ports"
  :key="`${container.id}-${port.external}-${port.internal}`"
>

<!-- WRONG: Would cause duplicates -->
<n-tag v-for="port in ports" :key="port.external">
```

#### Test Coverage
- ✅ Ports displayed once per container (no duplicates)
- ✅ Port pills formatted correctly
- ✅ Pills are clickable
- ✅ Click opens correct URL
- ✅ Handles containers with no ports gracefully

---

### App.vue (Modified for Title)

Location: `frontend/src/App.vue`

#### New Setup Logic

```typescript
import { useRouter } from 'vue-router'
import { onMounted } from 'vue'

const router = useRouter()

function updateDocumentTitle(): void {
  const hostname = window.location.hostname || 'localhost'
  document.title = `${hostname} - Docker-CD`
}

// Update on route changes
router.afterEach(() => {
  updateDocumentTitle()
})

// Update on initial mount
onMounted(() => {
  updateDocumentTitle()
})
```

#### Contract
- **Title Format**: `"${hostname} - Docker-CD"`
- **Update Timing**: On initial mount and after every route change
- **Hostname Source**: `window.location.hostname`
- **Fallback**: Uses "localhost" if hostname is empty

#### Test Coverage
- ✅ Title set on mount
- ✅ Title updates on route change
- ✅ Title includes hostname
- ✅ Format matches specification

---

## CSS Contracts

### StacksGrid.vue Layout

Location: `frontend/src/pages/StacksGrid.vue`

#### Grid Layout Contract

```css
.stacks-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  grid-auto-rows: 1fr; /* Key: uniform row height */
}
```

**Requirements**:
- Cards in same row have equal height
- Cards grow vertically to fit content (no fixed height)
- Responsive grid with minimum 300px card width
- 16px gap between cards

---

### Container Link Styling

Location: `frontend/src/components/StackCard.vue`

#### Link Style Contract

```css
.container-link {
  cursor: pointer;
  text-decoration: none;
  color: inherit;
}

.container-link:hover {
  opacity: 0.8;
}

.container-link .underlined {
  text-decoration: underline;
}
```

**Requirements**:
- Underline only on container name, not arrow
- Cursor changes to pointer on hover
- Hover effect for visual feedback
- Arrow character (↗) uses same font as text

---

### Port Pill Styling

Location: `frontend/src/pages/StackDetail.vue`

#### Pill Style Contract

```css
.port-pill {
  cursor: pointer;
  margin-right: 8px;
  margin-bottom: 4px;
}

.port-pill:hover {
  opacity: 0.8;
}
```

**Requirements**:
- Pills use Naive UI n-tag component styling
- Cursor indicates clickability
- Spacing between pills (margin)
- Hover feedback

---

## Event Handling Contracts

### Click Events

#### Container Name Click (StackCard)

**Event**: `@click` on container link
**Handler**: Opens lowest external port in new tab
**Behavior**:
```typescript
window.open(buildPortURL(lowestPort), '_blank', 'noopener,noreferrer')
```

**Security**: Uses `noopener,noreferrer` to prevent tab-nabbing

---

#### Port Pill Click (StackDetail)

**Event**: `@click` on port pill
**Handler**: Opens specific port in new tab
**Behavior**:
```typescript
window.open(buildPortURL(port.external), '_blank', 'noopener,noreferrer')
```

**Edge Case**: If `port.external === null`, click does nothing (no external port to open)

---

## API Integration Contracts

### Container Fetching (StackCard)

**API Call**: `fetchContainers(stackPath: string): Promise<ContainerInfo[]>`

**Integration**:
```typescript
onMounted(async () => {
  containersLoading.value = true
  try {
    containers.value = await fetchContainers(props.stack.path)
  } catch (error) {
    containersError.value = 'Failed to load containers'
  } finally {
    containersLoading.value = false
  }
})
```

**Contract**:
- Called once on component mount
- Loading state managed with ref
- Errors caught and displayed, don't crash component
- Success updates containers ref

---

## Type Safety Contracts

### TypeScript Strict Mode

All new code must:
- ✅ Pass TypeScript strict type checking
- ✅ Have explicit return types on exported functions
- ✅ Use proper interface definitions
- ✅ Avoid `any` types (use `unknown` if needed)

### Example:
```typescript
// ✅ GOOD: Explicit types
function parsePortString(ports: string): PortMapping[] {
  // ...
}

// ❌ BAD: Implicit any
function parsePortString(ports) {
  // ...
}
```

---

## Testing Contracts

### Unit Test Requirements

**Port Utilities**:
- Must test all documented examples
- Must test edge cases (empty, null, malformed)
- Must verify error handling doesn't throw

**Format**:
```typescript
describe('parsePortString', () => {
  test('parses single port mapping', () => {
    const result = parsePortString('8080:80/tcp')
    expect(result).toEqual([
      { external: 8080, internal: 80, protocol: 'tcp' }
    ])
  })
  
  // ... more tests
})
```

---

### Component Test Requirements

**StackCard**:
- Must mock fetchContainers API call
- Must verify sorted order
- Must verify link styling
- Must verify click behavior

**StackDetail**:
- Must verify no duplicate pills
- Must verify pill formatting
- Must verify click behavior

**Format**:
```typescript
import { mount } from '@vue/test-utils'
import StackCard from '@/components/StackCard.vue'

describe('StackCard port links', () => {
  test('displays containers alphabetically', async () => {
    const wrapper = mount(StackCard, {
      props: { stack: mockStack }
    })
    // ... assertions
  })
})
```

---

## Browser Compatibility Contracts

**Minimum Requirements**:
- Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- CSS Grid support (universal in modern browsers)
- window.open() support (universal)
- Unicode character support (universal)

**No Polyfills Required**: All features are natively supported in target browsers

---

## Performance Contracts

**StackCard Container Fetching**:
- Must not block other cards from rendering
- Must handle fetch errors gracefully
- Acceptable: 10 parallel API calls for 10-card grid

**Port Parsing**:
- Must complete in <1ms for typical port strings
- Must not cause visible UI lag

**Grid Layout**:
- Must not cause layout thrashing on window resize
- CSS Grid handles reflow efficiently

---

## Security Contracts

**XSS Prevention**:
- All user-generated content (container names, port strings) rendered via Vue's automatic escaping
- No `v-html` used
- No direct DOM manipulation with user data

**Tab-Nabbing Prevention**:
- All `window.open()` calls use `'noopener,noreferrer'`
- Prevents opened tabs from accessing window.opener

**HTTPS Mixed Content**:
- Port links match current page protocol
- Prevents mixed-content browser warnings

---

## Accessibility Contracts

**Keyboard Navigation**:
- Container links must be keyboard-accessible (native <a> element)
- Port pills must be keyboard-accessible (n-tag with @click has tabindex)

**Screen Reader Support**:
- Container names have semantic meaning
- Link purpose is clear from context
- No visual-only indicators

**Focus Management**:
- Clicking links doesn't lose focus context
- New tabs open without disrupting current page focus

---

## Documentation Contracts

**Code Comments**:
- All utility functions have JSDoc comments
- Complex logic has inline explanations
- Regex patterns have comments explaining format

**Example**:
```typescript
/**
 * Parses a Docker port string into structured port mappings.
 * 
 * @param ports - Comma-separated port string (e.g., "8080:80/tcp, 443:443/tcp")
 * @returns Array of parsed port mappings, empty array if invalid
 * @example
 * parsePortString("8080:80/tcp") 
 * // Returns: [{ external: 8080, internal: 80, protocol: "tcp" }]
 */
function parsePortString(ports: string): PortMapping[] {
  // ...
}
```

---

## Version Compatibility

**Vue**: 3.5.28 (Composition API required)  
**Naive UI**: 2.43.2 (n-tag, n-card components)  
**TypeScript**: 5.9+ (strict mode)  
**Vite**: 7.3.1 (build tool)

**No Breaking Changes**: This feature is additive, maintains backward compatibility with existing components
