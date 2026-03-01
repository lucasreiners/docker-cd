# Feature Specification: Stack UI Enhancements

**Feature Branch**: `007-stack-ui-enhancements`  
**Created**: 2026-03-01  
**Status**: Draft  
**Input**: User description: "For the next feature I want to improve the UI a bit. In the grid view of the stacks, I want to list the container names in the card and if there is a port mapping for that container, I want to show like a link icon and show that container name as a link which opens that port. If there are multiple portbindings, I want that the link opens the port with the lowest number. On the stack details page, I noticied, that all the port mappings are shown twice somehow. We should fix that. And I also want to put the port mappings into pills for visual distinction and make those clickable, so that I can open all the ports automatically. I also want to show the hostname in the browser tab's title bar like "<host> - Docker-CD" so that I can find the right tabs."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Quick Port Access from Grid View (Priority: P1)

As a Docker-CD user managing multiple stacks, I want to quickly access container web interfaces directly from the grid view so that I can navigate to running services without opening the details page first.

**Why this priority**: This provides the highest immediate value by reducing clicks required to access container services. Users frequently need to access web UIs of running containers, and making this one-click from the main grid view significantly improves workflow efficiency.

**Independent Test**: Can be fully tested by viewing the stacks grid with containers that have port mappings, clicking on a container name link, and verifying the browser opens the correct port. Delivers value even without the details page improvements.

**Acceptance Scenarios**:

1. **Given** I'm viewing the stacks grid page, **When** a stack contains containers with exposed ports, **Then** container names with port mappings are displayed as underlined text with an external link arrow (↗) prefix
2. **Given** a container has a single port mapping, **When** I click on the container name link, **Then** my browser opens a new tab/window navigating to that container's exposed port (e.g., http://hostname:8080)
3. **Given** a container has multiple port mappings (e.g., 8080:80, 8443:443, 9000:9000), **When** I click on the container name link, **Then** my browser opens the port with the lowest external port number (e.g., 8080 in this example)
4. **Given** a container has no port mappings, **When** viewing the stack card, **Then** the container name is displayed as plain text without underline or arrow prefix
5. **Given** multiple stacks with varying numbers of containers are displayed in the grid view, **When** viewing the grid, **Then** all stack cards in the same row maintain uniform height (matching the tallest card in that row), and cards grow vertically to display all their containers

---

### User Story 2 - Browser Tab Identification (Priority: P2)

As a Docker-CD user managing multiple Docker-CD instances across different hosts, I want to see the hostname in the browser tab title so that I can quickly identify which instance I'm viewing when I have multiple tabs open.

**Why this priority**: This solves a common multi-instance management problem where users confuse which Docker-CD instance they're viewing. The implementation is straightforward and provides immediate clarity for users managing multiple environments (dev, staging, production).

**Independent Test**: Can be fully tested by opening Docker-CD in any browser, checking the tab title shows "<hostname> - Docker-CD", and verifying it remains consistent across all pages. Works independently of the port mapping features.

**Acceptance Scenarios**:

1. **Given** I open Docker-CD in my browser on host "prod-server-01", **When** I view any page, **Then** the browser tab title displays "prod-server-01 - Docker-CD"
2. **Given** I have Docker-CD open on multiple hosts in different browser tabs, **When** I scan my open tabs, **Then** I can immediately identify each instance by the hostname in the tab title
3. **Given** I navigate between different pages (grid view, stack details, etc.), **When** the page changes, **Then** the hostname portion of the tab title remains consistent

---

### User Story 3 - Enhanced Port Display on Details Page (Priority: P3)

As a Docker-CD user viewing stack details, I want port mappings displayed clearly as clickable visual elements so that I can easily identify and access all exposed ports without duplication or confusion.

**Why this priority**: This improves the details page UX by fixing a visual bug (duplicate ports) and making port access more intuitive. While valuable, it's lower priority than P1 because users can still function with the current details page, whereas P1 provides new quick-access functionality.

**Independent Test**: Can be fully tested by opening a stack's details page, verifying ports appear once as clickable pills, and clicking pills to open the corresponding ports. Functions independently of grid view changes.

**Acceptance Scenarios**:

1. **Given** I'm viewing a stack details page with containers that have port mappings, **When** the page loads, **Then** each port mapping is displayed exactly once (no duplicates)
2. **Given** port mappings are displayed on the details page, **When** I view them, **Then** each port mapping appears as a visually distinct pill/badge element (e.g., "8080:80")
3. **Given** a port mapping pill is displayed, **When** I click on it, **Then** my browser opens a new tab/window navigating to that specific port
4. **Given** a container has multiple port mappings, **When** I view the details page, **Then** all port mappings are displayed as separate clickable pills

---

### Edge Cases

- What happens when a container exposes a port but the service inside isn't responding? (Link should still open, but user may see connection error from browser - this is expected behavior)
- What happens when a stopped container has port mappings? (Links remain enabled and clickable; clicking will attempt to open the port, but service won't respond - this is expected behavior)
- What happens when port mappings use non-standard protocols (UDP instead of TCP)? (Assume HTTP/HTTPS protocol for all port links, as web browsers can only open HTTP(S) URLs)
- What happens when a container has dozens of port mappings? (All should be displayed, but UI should handle overflow gracefully with wrapping or scrolling)
- What happens when a stack has many containers (e.g., 20+ containers)? (Card grows vertically to display all containers, maintaining row height uniformity with other cards in the same row)
- What happens when the hostname is very long? (Browser tab title may truncate naturally based on browser behavior - this is acceptable)
- What happens when multiple containers in a stack have the same lowest port number? (Each container link operates independently - each should open its own mapped port)
- What happens when accessing Docker-CD via IP address instead of hostname? (Display the IP address in the tab title)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Grid view stack cards MUST display a list of all container names for each stack in alphabetical order
- **FR-002**: Container names with port mappings MUST be styled with underline text decoration and an external link arrow (↗) character as a prefix
- **FR-003**: Container names with port mappings in grid view MUST be clickable links
- **FR-004**: Port links MUST remain enabled and clickable regardless of container running state (running, stopped, paused, etc.)
- **FR-005**: When a container has multiple port bindings, clicking the container name link MUST open the port with the lowest external port number
- **FR-006**: When a container has a single port binding, clicking the container name link MUST open that port
- **FR-007**: Container names without port mappings MUST be displayed as non-clickable plain text
- **FR-008**: Grid view stack cards MUST grow vertically to display all containers without truncation or scrolling
- **FR-009**: Stack cards in the same grid row MUST maintain uniform height, matching the height of the tallest card in that row
- **FR-010**: Clicking a port link MUST open the port URL in a new browser tab/window
- **FR-011**: Port URLs MUST be constructed as [current-protocol]://[current-hostname]:[external-port], where current-protocol matches the protocol used to access Docker-CD (http or https)
- **FR-012**: Stack details page MUST NOT display duplicate port mappings
- **FR-013**: Stack details page MUST display port mappings as visually distinct pill/badge elements
- **FR-013**: Port mapping pills MUST display both external and internal ports in Docker notation format "[external]:[internal]" (e.g., "8080:80")
- **FR-014**: Port mapping pills on details page MUST be clickable to open the corresponding port
- **FR-015**: Browser tab title MUST display the format "[hostname] - Docker-CD" on all pages
- **FR-016**: Browser tab title MUST dynamically use the hostname from which Docker-CD is being accessed
- **FR-017**: Port links MUST use the same protocol (HTTP or HTTPS) as the current page to avoid mixed-content security warnings

### Key Entities

- **Port Mapping**: Represents a container's network port exposure, containing:
  - External port number (the port accessible on the host)
  - Internal port number (the port inside the container)
  - Protocol (TCP/UDP, though only TCP/HTTP is link-accessible)
  
- **Container**: Contains:
  - Container name
  - Collection of zero or more port mappings
  - Running state and other metadata (already exists in current data model)

- **Stack Card (Grid View)**: Visual representation containing:
  - Stack name and status
  - List of container names
  - Visual indicators for containers with port access

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can access a container's web interface with one click from the grid view (reduced from current 2+ clicks requiring stack details page navigation)
- **SC-002**: Users can identify which Docker-CD instance they're viewing within 1 second by glancing at browser tabs
- **SC-003**: Port mapping information is displayed without duplication errors (0 duplicate entries on details page)
- **SC-004**: Users can open all ports for a container on the details page with one click per port
- **SC-005**: 100% of containers with port mappings display the underline and arrow prefix indicator in grid view
- **SC-006**: Port links successfully open in browser (navigating to the correct host:port combination) in 100% of test cases

## Clarifications

### Session 2026-03-01

- Q: Link icon positioning & style for containers with port mappings? → A: Underline container name AND show arrow (↗) as prefix before the container name
- Q: HTTPS protocol handling - should port links use HTTP or match current page protocol? → A: Match the protocol of the current page
- Q: Container list ordering in grid cards when stack has multiple containers? → A: Alphabetical order by container name
- Q: Port mapping pill format on details page - show both ports or just external? → A: Show both ports as "8080:80" format
- Q: Links for stopped containers - should they be disabled or remain clickable? → A: Still show enabled links even if container is stopped

## Assumptions

- Port links use the same protocol as the current Docker-CD page to prevent mixed-content browser warnings
- Current hostname and protocol are available from browser's location API (window.location.hostname, window.location.protocol)
- Users are accessing Docker-CD via a browser that supports opening new tabs/windows
- The existing data model already provides port mapping information for containers (no backend changes required)
- Port "lowest number" refers to the external (host) port number, not the internal container port
- The duplicate port display issue on details page is a frontend rendering bug, not a data issue from backend
