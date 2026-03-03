# Quickstart Guide: Stack UI Enhancements

**Feature**: 007-stack-ui-enhancements  
**For**: Developers implementing this feature  
**Date**: 2026-03-01

## Overview

This guide provides a step-by-step implementation path for the Stack UI Enhancements feature. Follow these phases in order for a systematic, testable rollout.

---

## Prerequisites

Before starting implementation:

- [ ] Read [spec.md](../spec.md) - Understand requirements and user stories
- [ ] Read [research.md](../research.md) - Understand technical decisions
- [ ] Read [data-model.md](../data-model.md) - Understand data structures
- [ ] Read [contracts/component-apis.md](../contracts/component-apis.md) - Understand interfaces
- [ ] Ensure development environment is ready:
  ```bash
  cd frontend
  bun install
  bun run dev  # Starts Vite dev server
  ```
- [ ] Verify tests pass:
  ```bash
  bun run test
  bun run lint
  ```

---

## Implementation Phases

### Phase 1: Port Utilities (Foundation)

**Goal**: Create port parsing utilities with full test coverage

**Files to Create**:
1. `frontend/src/utils/portUtils.ts`
2. `frontend/tests/port-utils.spec.ts`

**Implementation Steps**:

1. **Create portUtils.ts**:
   ```typescript
   // frontend/src/utils/portUtils.ts
   export interface PortMapping {
     external: number | null
     internal: number
     protocol: string
   }

   export function parsePortString(ports: string): PortMapping[] {
     // TODO: Implement parsing logic
     // Parse "8080:80/tcp, 8443:443/tcp" format
     // Handle edge cases: empty, malformed, no external port
   }

   export function getLowestExternalPort(ports: string): number | null {
     // TODO: Extract lowest external port
   }

   export function buildPortURL(port: number): string {
     // TODO: Construct http://hostname:port or https://hostname:port
     // Use window.location.protocol and window.location.hostname
   }
   ```

2. **Create port-utils.spec.ts**:
   ```typescript
   // frontend/tests/port-utils.spec.ts
   import { describe, test, expect } from 'vitest'
   import { parsePortString, getLowestExternalPort, buildPortURL } from '@/utils/portUtils'

   describe('parsePortString', () => {
     test('parses single port mapping', () => {
       const result = parsePortString('8080:80/tcp')
       expect(result).toEqual([
         { external: 8080, internal: 80, protocol: 'tcp' }
       ])
     })

     test('parses multiple port mappings', () => {
       // TODO: Add test
     })

     test('handles port without external mapping', () => {
       // TODO: Add test
     })

     test('handles empty string', () => {
       // TODO: Add test
     })

     test('handles malformed input gracefully', () => {
       // TODO: Add test
     })
   })

   describe('getLowestExternalPort', () => {
     test('returns lowest port from multiple', () => {
       // TODO: Add test
     })

     test('returns null for no external ports', () => {
       // TODO: Add test
     })
   })

   describe('buildPortURL', () => {
     test('constructs URL with current protocol and hostname', () => {
       // TODO: Add test (may need to mock window.location)
     })
   })
   ```

3. **Run tests**:
   ```bash
   bun run test port-utils.spec.ts
   ```

**Validation**:
- [ ] All tests pass
- [ ] Test coverage >90% for portUtils.ts
- [ ] Linting passes: `bun run lint`

**Estimated Time**: 2-3 hours

---

### Phase 2: StackCard Container Display

**Goal**: Display containers in grid cards with clickable port links

**Files to Modify**:
1. `frontend/src/components/StackCard.vue`

**Files to Create**:
1. `frontend/tests/stack-card-ports.spec.ts`

**Implementation Steps**:

1. **Update StackCard.vue**:
   - Add state for containers, loading, error
   - Add onMounted hook to fetch containers via `fetchContainers(stack.path)`
   - Add computed property for sorted containers (alphabetical)
   - Enhance computed to add parsed ports and lowest port
   - Update template to display container list
   - Add link styling for containers with ports (arrow + underline)
   - Add click handler to open lowest port

2. **Add CSS for container list**:
   ```css
   .containers-list {
     margin-top: 12px;
     padding-top: 12px;
     border-top: 1px solid var(--border-color);
   }

   .container-link {
     cursor: pointer;
     text-decoration: none;
     color: inherit;
     display: block;
     padding: 4px 0;
   }

   .container-link:hover {
     opacity: 0.8;
   }

   .container-link .underlined {
     text-decoration: underline;
   }
   ```

3. **Create stack-card-ports.spec.ts**:
   ```typescript
   import { mount } from '@vue/test-utils'
   import { describe, test, expect, vi } from 'vitest'
   import StackCard from '@/components/StackCard.vue'
   import * as api from '@/services/api'

   describe('StackCard with containers', () => {
     test('fetches containers on mount', async () => {
       // TODO: Mock fetchContainers
       // TODO: Mount component
       // TODO: Verify API called
     })

     test('displays containers alphabetically', async () => {
       // TODO: Test sorting
     })

     test('shows arrow and underline for containers with ports', async () => {
       // TODO: Test link styling
     })

     test('shows plain text for containers without ports', async () => {
       // TODO: Test non-link display
     })

     test('clicking container opens port URL', async () => {
       // TODO: Mock window.open
       // TODO: Trigger click
       // TODO: Verify URL opened
     })
   })
   ```

4. **Run tests**:
   ```bash
   bun run test stack-card-ports.spec.ts
   ```

**Validation**:
- [ ] Containers display in cards
- [ ] Containers sorted alphabetically
- [ ] Arrow + underline appear for linkable containers
- [ ] Click opens correct port
- [ ] Tests pass
- [ ] Manual testing: view grid, see containers, click links

**Estimated Time**: 3-4 hours

---

### Phase 3: Grid Layout Uniform Height

**Goal**: Ensure cards in same row have uniform height

**Files to Modify**:
1. `frontend/src/pages/StacksGrid.vue`

**Implementation Steps**:

1. **Update StacksGrid.vue CSS**:
   ```css
   .stacks-grid {
     display: grid;
     grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
     gap: 16px;
     grid-auto-rows: 1fr; /* ADD THIS LINE */
   }
   ```

2. **Update StackCard.vue wrapper**:
   ```css
   .stack-card {
     display: flex;
     flex-direction: column;
     height: 100%; /* Ensure card fills grid cell */
   }

   .card-content {
     flex: 1; /* Content area grows to fill space */
   }
   ```

**Validation**:
- [ ] Cards in same row have equal height
- [ ] Cards with more containers are taller
- [ ] Layout responds smoothly to window resize
- [ ] Manual testing: view grid with varied container counts

**Estimated Time**: 1 hour

---

### Phase 4: Browser Tab Title

**Goal**: Show hostname in browser tab title on all pages

**Files to Modify**:
1. `frontend/src/App.vue` or `frontend/src/main.ts`

**Implementation Steps**:

1. **Add title update logic to App.vue**:
   ```typescript
   <script setup lang="ts">
   import { useRouter } from 'vue-router'
   import { onMounted } from 'vue'

   const router = useRouter()

   function updateDocumentTitle(): void {
     const hostname = window.location.hostname || 'localhost'
     document.title = `${hostname} - Docker-CD`
   }

   router.afterEach(() => {
     updateDocumentTitle()
   })

   onMounted(() => {
     updateDocumentTitle()
   })
   </script>
   ```

2. **Alternative: main.ts approach**:
   ```typescript
   // In frontend/src/main.ts after router setup
   router.afterEach(() => {
     const hostname = window.location.hostname || 'localhost'
     document.title = `${hostname} - Docker-CD`
   })
   ```

**Validation**:
- [ ] Title shows hostname on load
- [ ] Title updates on route change
- [ ] Title format matches specification
- [ ] Manual testing: navigate between pages, check tab title

**Estimated Time**: 30 minutes

---

### Phase 5: StackDetail Port Pills

**Goal**: Fix duplicate ports and add clickable pills on details page

**Files to Modify**:
1. `frontend/src/pages/StackDetail.vue`

**Files to Create**:
1. `frontend/tests/stack-detail-pills.spec.ts`

**Implementation Steps**:

1. **Fix duplicate ports in StackDetail.vue**:
   - Audit existing port display code
   - Ensure v-for has unique :key (use `container.id` + port numbers)
   - Example:
     ```vue
     <div v-for="container in containers" :key="container.id">
       <n-tag
         v-for="(port, idx) in parsePortString(container.ports || '')"
         :key="`${container.id}-${port.external}-${port.internal}`"
         @click="openPort(port.external)"
       >
         {{ formatPortPill(port) }}
       </n-tag>
     </div>
     ```

2. **Add helper methods**:
   ```typescript
   import { parsePortString, buildPortURL } from '@/utils/portUtils'

   function formatPortPill(port: PortMapping): string {
     if (port.external !== null) {
       return `${port.external}:${port.internal}/${port.protocol}`
     }
     return `${port.internal}/${port.protocol}`
   }

   function openPort(port: number | null): void {
     if (port !== null) {
       const url = buildPortURL(port)
       window.open(url, '_blank', 'noopener,noreferrer')
     }
   }
   ```

3. **Add pill styling**:
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

4. **Create stack-detail-pills.spec.ts**:
   ```typescript
   import { mount } from '@vue/test-utils'
   import { describe, test, expect, vi } from 'vitest'
   import StackDetail from '@/pages/StackDetail.vue'

   describe('StackDetail port pills', () => {
     test('displays ports once per container (no duplicates)', async () => {
       // TODO: Mount with mock containers
       // TODO: Count pill elements
       // TODO: Verify count matches expected
     })

     test('formats pills correctly', async () => {
       // TODO: Verify "8080:80/tcp" format
     })

     test('clicking pill opens port URL', async () => {
       // TODO: Mock window.open
       // TODO: Click pill
       // TODO: Verify URL
     })
   })
   ```

**Validation**:
- [ ] No duplicate port displays
- [ ] Pills formatted correctly
- [ ] Pills clickable
- [ ] Click opens correct port
- [ ] Tests pass
- [ ] Manual testing: view stack details, click pills

**Estimated Time**: 2-3 hours

---

## Testing Strategy

### Unit Tests

Run after each phase:
```bash
bun run test
```

Target coverage: >90% for new code

### Component Tests

Run during Phases 2 and 5:
```bash
bun run test stack-card-ports.spec.ts
bun run test stack-detail-pills.spec.ts
```

### Manual Testing Checklist

After all phases complete:

- [ ] **Grid View**:
  - [ ] All stacks show container names
  - [ ] Container names sorted alphabetically
  - [ ] Containers with ports show ↗ and underline
  - [ ] Containers without ports show plain text
  - [ ] Clicking container with ports opens new tab to correct URL
  - [ ] Cards in same row have uniform height
  - [ ] Cards grow to fit all containers

- [ ] **Stack Details**:
  - [ ] Ports shown ONCE per container (not duplicated)
  - [ ] Ports displayed as clickable pills
  - [ ] Pill format: "8080:80/tcp"
  - [ ] Clicking pill opens new tab to correct URL
  - [ ] Multiple pills for containers with multiple ports

- [ ] **Browser Tab Title**:
  - [ ] Title shows correct format on grid view
  - [ ] Title shows correct format on details view
  - [ ] Title persists on navigation
  - [ ] Format: "[hostname] - Docker-CD"

- [ ] **Protocol Matching**:
  - [ ] Access via HTTP: port links use http://
  - [ ] Access via HTTPS: port links use https://

- [ ] **Error Handling**:
  - [ ] Container fetch failure shows error (doesn't crash)
  - [ ] Malformed port strings handled gracefully
  - [ ] Stopped containers still show links (as specified)

---

## Development Workflow

### Branch Strategy

Already on feature branch: `007-stack-ui-enhancements`

### Commit Strategy

Commit after each phase:
```bash
git add .
git commit -m "feat: add port parsing utilities (Phase 1)"

git add .
git commit -m "feat: add container display to stack cards (Phase 2)"

git add .
git commit -m "feat: add uniform card height in grid (Phase 3)"

git add .
git commit -m "feat: add hostname to browser tab title (Phase 4)"

git add .
git commit -m "fix: duplicate ports and add clickable pills (Phase 5)"
```

### Linting

Run before each commit:
```bash
bun run lint:fix
```

### Pre-Push Checklist

Before pushing to GitHub:
```bash
# Run all checks
bun run test
bun run lint
bun run build

# Verify no errors
echo "All checks passed!"
```

---

## Troubleshooting

### Containers not appearing in cards

**Check**:
- Network tab: Is `/stacks/{path}/containers` API called?
- Console: Any fetch errors?
- Component state: Is `containersLoading` or `containersError` set?

**Fix**: Verify API endpoint accessible, check CORS, check loading state logic

---

### Port links not opening

**Check**:
- Console: Click handler executing?
- URL construction: Correct protocol and hostname?
- Browser popup blocker: Blocking new tabs?

**Fix**: Verify `buildPortURL()` logic, test with browser console: `window.open('http://localhost:8080', '_blank')`

---

### Cards not uniform height

**Check**:
- CSS Grid: Is `grid-auto-rows: 1fr` applied?
- Card CSS: Is `height: 100%` on card wrapper?
- Flexbox: Is card using flex-direction column?

**Fix**: Inspect element in browser DevTools, verify computed styles

---

### Duplicate ports on details page

**Check**:
- v-for key: Is it unique per port?
- Data source: Does API return duplicates?

**Fix**: Use composite key: `:key="\`${container.id}-${port.external}-${port.internal}\`"`

---

### Title not updating

**Check**:
- Router hook: Is `afterEach` called?
- Console: Log title update function execution
- Timing: Title updated after navigation?

**Fix**: Verify router import, check hook placement (must be in setup or after router creation)

---

## Code Review Checklist

Before submitting PR:

- [ ] All tests pass (`bun run test`)
- [ ] Linting passes (`bun run lint`)
- [ ] Build succeeds (`bun run build`)
- [ ] Test coverage >90% for new code
- [ ] All manual testing scenarios pass
- [ ] No console errors or warnings
- [ ] Code follows existing patterns (Vue Composition API, typescript)
- [ ] All contractual interfaces implemented correctly
- [ ] Performance acceptable (no visible lag)
- [ ] Accessibility maintained (keyboard navigation works)

---

## Timeline Estimate

| Phase | Tasks | Time | Running Total |
|-------|-------|------|---------------|
| Phase 1 | Port utilities + tests | 2-3h | 3h |
| Phase 2 | StackCard containers + tests | 3-4h | 7h |
| Phase 3 | Grid layout CSS | 1h | 8h |
| Phase 4 | Browser title | 0.5h | 8.5h |
| Phase 5 | Details pills + tests | 2-3h | 11h |
| Testing & QA | Manual testing, fixes | 2h | 13h |
| **Total** | | **~13 hours** | |

*Assumes experienced Vue/TypeScript developer. Adjust for familiarity level.*

---

## Success Criteria

Feature is complete when:

1. ✅ All 17 functional requirements (FR-001 to FR-017) implemented
2. ✅ All 6 success criteria (SC-001 to SC-006) met
3. ✅ All 3 user stories (P1, P2, P3) deliverable
4. ✅ All test suites pass with >90% coverage
5. ✅ Manual testing checklist complete
6. ✅ No regressions in existing functionality
7. ✅ Performance acceptable (<100ms interactions)
8. ✅ Code review checklist satisfied

---

## Next Steps After Implementation

1. **Create Pull Request**:
   ```bash
   git push origin 007-stack-ui-enhancements
   ```
   
2. **PR Description Template**:
   ```markdown
   ## Feature: Stack UI Enhancements (#007)
   
   ### Summary
   Implements P1-P3 features:
   - Grid view container names with clickable port links
   - Browser tab hostname identification
   - Details page clickable port pills (fixes duplicates)
   
   ### Testing
   - ✅ All unit tests pass
   - ✅ All component tests pass
   - ✅ Manual testing completed
   - ✅ Coverage >90%
   
   ### Checklist
   - [x] Spec: specs/007-stack-ui-enhancements/spec.md
   - [x] Plan: specs/007-stack-ui-enhancements/plan.md
   - [x] Tests added
   - [x] Linting passes
   - [x] Build succeeds
   
   ### Screenshots
   [Add screenshots of grid view and details page]
   ```

3. **Monitor CI/CD**: Ensure GitHub Actions pass

4. **Request Review**: Tag team members

5. **Deploy**: After merge, verify in staging/production

---

## Resources

- **Spec**: [spec.md](../spec.md)
- **Research**: [research.md](../research.md)
- **Data Model**: [data-model.md](../data-model.md)
- **Contracts**: [contracts/component-apis.md](../contracts/component-apis.md)
- **Vue 3 Docs**: https://vuejs.org/guide/introduction.html
- **Naive UI Docs**: https://www.naiveui.com/en-US/os-theme
- **Vitest Docs**: https://vitest.dev/guide/

---

**Ready to start? Begin with Phase 1: Port Utilities** 🚀
