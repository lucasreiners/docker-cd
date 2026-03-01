# Research: Stack UI Enhancements

**Feature**: 007-stack-ui-enhancements  
**Phase**: 0 - Research & Technical Decisions  
**Date**: 2026-03-01

## Overview

This document captures all technical research and decisions made during the planning phase for the Stack UI Enhancements feature. All clarifications from the specification process have been resolved.

## Technical Decisions

### 1. Port String Parsing Strategy

**Decision**: Create utility functions to parse port strings in format "8080:80/tcp, 8443:443/tcp"

**Rationale**: 
- Backend provides ports as comma-separated string in ContainerInfo API
- Need to extract individual port mappings for pill display and link generation
- Need to identify lowest external port number for grid view links

**Implementation Approach**:
```typescript
// Parse "8080:80/tcp, 8443:443/tcp" into structured array
interface PortMapping {
  external: number
  internal: number
  protocol: string
}

function parsePortString(ports: string): PortMapping[]
function getLowestExternalPort(ports: string): number | null
function buildPortURL(protocol: string, hostname: string, port: number): string
```

**Alternatives Considered**:
- Regex-based parsing: Rejected - harder to maintain and test
- Modify backend API to return structured port data: Rejected - violates constraint of frontend-only changes

---

### 2. Container Fetching for Grid View

**Decision**: Fetch container details for each stack when displaying grid, use existing fetchContainers API

**Rationale**:
- StackRecord doesn't include container names/ports, only counts
- Need container-level details to show names and port mappings in cards
- Existing API endpoint `/stacks/{path}/containers` provides required data

**Implementation Approach**:
- Enhance StackCard component to fetch containers on mount
- Use reactive loading state to show skeleton/spinner during fetch
- Cache results in component to avoid repeated API calls

**Performance Considerations**:
- Grid view may have 5-10 stacks → 5-10 API calls
- Each API call is lightweight (container metadata only)
- Backend already computes this data via `docker compose ps`
- Consider future optimization: add containers array to StackRecord API response

**Alternatives Considered**:
- Add containers to StackRecord: Rejected - requires backend changes, outside scope
- SSE events for container updates: Already implemented, no changes needed

---

### 3. Link Icon Styling

**Decision**: Use underline text decoration + external link arrow (↗) character as prefix

**Rationale**:
- User specified: "underline AND arrow inline as a prefix"
- Arrow character (U+2197) widely supported across browsers
- Underline provides clear visual affordance for clickability
- No additional icon library needed (Naive UI icons not required)

**Implementation**:
```vue
<template>
  <a v-if="hasPortMappings" class="container-link">
    ↗ <span class="underlined">{{ containerName }}</span>
  </a>
  <span v-else>{{ containerName }}</span>
</template>

<style scoped>
.container-link {
  text-decoration: none;
  cursor: pointer;
}
.container-link .underlined {
  text-decoration: underline;
}
</style>
```

**Alternatives Considered**:
- Icon after name: Rejected - user specified prefix
- Naive UI icon component: Rejected - adds dependency, character is simpler

---

### 4. Protocol Matching for Port Links

**Decision**: Use window.location.protocol to match current page protocol (HTTP/HTTPS)

**Rationale**:
- User confirmed: "match the protocol of the current page"
- Prevents mixed-content browser warnings
- Assumes container services support same protocol as Docker-CD

**Implementation**:
```typescript
function buildPortURL(port: number): string {
  const protocol = window.location.protocol.replace(':', '')
  const hostname = window.location.hostname
  return `${protocol}://${hostname}:${port}`
}
```

**Alternatives Considered**:
- Always use HTTP: Rejected - causes warnings on HTTPS pages
- Protocol detection per container: Rejected - not feasible without probing

---

### 5. Browser Tab Title Management

**Decision**: Use Vue Router's `afterEach` hook to update document.title on route changes

**Rationale**:
- Need consistent hostname in title across all pages
- Vue Router provides lifecycle hooks for global operations
- Format: `"${window.location.hostname} - Docker-CD"`

**Implementation**:
```typescript
// In main.ts or App.vue
router.afterEach(() => {
  const hostname = window.location.hostname || 'localhost'
  document.title = `${hostname} - Docker-CD`
})
```

**Alternatives Considered**:
- Individual title meta tags per route: Rejected - duplicates hostname logic
- Vue plugin for title management: Rejected - overkill for simple requirement

---

### 6. Port Pills on Details Page

**Decision**: Use Naive UI's `n-tag` component with clickable handler

**Rationale**:
- Consistent with existing badge/pill usage (StatusBadge, container counts)
- Native hover/focus states built-in
- Easy to style as clickable with cursor pointer

**Implementation**:
```vue
<n-tag
  v-for="port in parsedPorts"
  :key="port.external"
  :bordered="false"
  size="small"
  round
  class="port-pill"
  style="cursor: pointer"
  @click="openPort(port.external)"
>
  {{ port.external }}:{{ port.internal }}/{{ port.protocol }}
</n-tag>
```

**Alternatives Considered**:
- Custom button elements: Rejected - reinvents Naive UI styling
- n-button component: Rejected - too prominent, not visually "pill-like"

---

### 7. Duplicate Port Display Fix

**Decision**: Investigate and fix root cause in StackDetail.vue rendering logic

**Rationale**:
- User noticed "all the port mappings are shown twice somehow"
- Likely caused by duplicate iteration or incorrect v-for key
- Fix requires code inspection during implementation phase

**Research Required During Implementation**:
1. Check if ports string contains duplicates from backend
2. Verify v-for iteration logic doesn't duplicate elements
3. Check if multiple components render same port data

**Expected Root Cause**:
- Most likely: v-for without proper :key causing Vue to duplicate DOM nodes
- Also possible: port parsing creates duplicate entries

---

### 8. Uniform Card Height in Grid Layout

**Decision**: Use CSS Grid with `grid-auto-rows: 1fr` for each row

**Rationale**:
- CSS Grid automatically equalizes heights within a row
- Cards grow vertically to show all containers (no truncation)
- Standard responsive grid behavior

**Implementation**:
```css
.stacks-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  grid-auto-rows: 1fr;
}

.stack-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
```

**Alternatives Considered**:
- JavaScript height calculation: Rejected - CSS Grid handles this natively
- Fixed card heights with scrolling: Rejected - user wants all containers visible

---

### 9. Container Ordering in Cards

**Decision**: Sort containers alphabetically by container name

**Rationale**:
- User confirmed: "alphabetical order by container name"
- Provides predictable, consistent positioning
- Easy to implement with Array.sort()

**Implementation**:
```typescript
const sortedContainers = computed(() => {
  return [...containers.value].sort((a, b) => 
    a.name.localeCompare(b.name)
  )
})
```

---

### 10. Stopped Container Link Behavior

**Decision**: Keep links enabled for stopped containers, let browser show connection error

**Rationale**:
- User specified: "still show enabled links even if the container is stopped"
- Clicking will attempt connection - browser handles failure naturally
- Avoids complexity of state-dependent UI changes

**Implementation**: No special handling needed - all containers with ports get links regardless of state

---

## Testing Strategy

### Unit Tests
- **Port parsing utilities** (`tests/port-utils.spec.ts`)
  - Parse valid port strings: "8080:80/tcp", "8080:80/tcp, 443:443/tcp"
  - Handle edge cases: empty string, malformed ports, no external port
  - Lowest port selection: multiple ports, single port, no ports
  - URL construction: HTTP/HTTPS protocol matching, hostname extraction

### Component Tests
- **StackCard with containers** (`tests/stack-card-ports.spec.ts`)
  - Displays container names alphabetically
  - Shows arrow + underline for containers with ports
  - Plain text for containers without ports
  - Click opens correct URL in new tab/window
  - Multiple port bindings open lowest port
  - Card height adjusts to container count

- **Stack Detail pills** (`tests/stack-detail-pills.spec.ts`)
  - No duplicate port mappings displayed
  - Ports shown as clickable pills in "8080:80/tcp" format
  - Click handler opens port URL
  - Pills use Naive UI styling consistently

- **Document title** (`tests/app.spec.ts` or integration test)
  - Title shows hostname on initial load
  - Title persists across route changes
  - Format: "[hostname] - Docker-CD"

### Integration Tests (optional, if time permits)
- Full user flow: grid view → click container link → opens port
- Full user flow: details page → click pill → opens port

---

## API Contract Verification

### Existing API Endpoints Used

**GET /stacks**
- Returns: `StackRecord[]` with containersRunning, containersTotal
- No changes needed

**GET /stacks/{path}/containers**
- Returns: `ContainerInfo[]` with ports string
- No changes needed

### Data Format Assumptions

**ContainerInfo.Ports format**: "8080:80/tcp, 8443:443/tcp"
- Comma-separated list
- Each entry: `[external]:[internal]/[protocol]` or `[internal]/[protocol]`
- Protocols: tcp, udp (only tcp useful for browser links)

**Verification Required**: Confirm backend always formats ports consistently (integration test will validate)

---

## Browser Compatibility

**Target Browsers**: Modern evergreen browsers
- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

**Features Used**:
- CSS Grid (widely supported since 2017)
- window.location API (universal)
- Unicode character ↗ (U+2197) - universal
- window.open() for new tabs - universal

**No polyfills required** - all features natively supported

---

## Performance Considerations

### Grid View Container Fetching
- **Current**: Each StackCard makes individual API call
- **Load**: Grid with 10 stacks = 10 parallel API calls
- **Impact**: Acceptable - each response is ~1-5KB, <50ms latency
- **Future Optimization** (out of scope): Batch API or include in StackRecord

### Port Parsing Overhead
- **Per Card**: Parse port string once, cache result
- **Complexity**: O(n) where n = number of ports
- **Typical**: 1-5 ports per container = negligible
- **No optimization needed**

### CSS Grid Layout Performance
- **Reflow**: Occurs on window resize and card content changes
- **Impact**: Modern browsers handle Grid layout efficiently
- **No optimization needed** for typical grid sizes (<50 cards)

---

## Development Milestones

1. **Phase 0 Complete**: ✅ Research documented
2. **Phase 1**: Create data model, contracts, quickstart [NEXT]
3. **Phase 2**: Generate task breakdown [NOT IN THIS COMMAND]
4. **Implementation**: Execute tasks (separate from /speckit.plan)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Backend port string format inconsistency | Low | Medium | Add robust parsing with error handling, unit tests |
| Container fetch API performance on large grids | Low | Low | Already acceptable per testing, document future optimization path |
| Browser tab title conflicts with router | Low | Low | Use Vue Router afterEach hook, well-documented pattern |
| Duplicate ports bug hard to find | Medium | Low | Code inspection + tests will identify, likely simple fix |

**Overall Risk**: Low - straightforward frontend changes with clear requirements

---

## Open Questions

**None** - All clarifications resolved during specification phase:
- ✅ Link icon style (arrow + underline prefix)
- ✅ Protocol handling (match current page)
- ✅ Container ordering (alphabetical)
- ✅ Port pill format (show both external:internal)
- ✅ Stopped container behavior (links remain enabled)

**Ready to proceed to Phase 1 (Design)**
