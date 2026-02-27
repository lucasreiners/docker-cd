# Feature Specification: Scheduled Stack Updates

**Feature Branch**: `006-scheduled-stack-updates`  
**Created**: February 27, 2026  
**Status**: Draft  
**Input**: User description: "I want to add the ability to run periodic update checks for the managed compose stacks. I want a cron expression configurable, default every night at 3 am UTC, where all docker images are pulled again and all stacks are updated. e.g. when stacks use containers with latest tag, I want to pull and then update the containers. Afterwards I want the docker images to be pruned. I need logging for this operation"

## Clarifications

### Session 2026-02-27

- Q: If the system crashes or restarts while an update cycle is running, how should it behave when it comes back online? → A: Abandon the in-progress update and wait for the next scheduled time
- Q: Should the system pull updates for all images used by stacks, or only for specific types of tags? → A: Pull all images regardless of tag to check for updates (registries may push security fixes to pinned tags)
- Q: When an update cycle fails or encounters errors, how should administrators be notified? → A: Logging is okay
- Q: If the next scheduled update time arrives while a previous update cycle is still running, what should happen? → A: Terminate the running update and start the new scheduled update
- Q: When pruning unused images after updates, what should be removed? → A: Prune all unused and dangling images system-wide (including non-stack images)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Automatic Nightly Stack Updates (Priority: P1)

As a system administrator, I want compose stacks to be automatically updated every night at 3 AM UTC so that my stacks always run the latest container images without manual intervention. The system should pull updated images, recreate containers when images have changed, and clean up unused images to save disk space.

**Why this priority**: This is the core functionality that delivers immediate value by automating a routine maintenance task. Eliminates the need for manual updates and ensures stacks stay current with latest security patches and bug fixes.

**Independent Test**: Can be fully tested by waiting for the scheduled time (or triggering manually) and verifying that stacks with outdated images are updated, while observing logs for successful image pulls and container recreation.

**Acceptance Scenarios**:

1. **Given** multiple compose stacks are deployed with various image tags, **When** the scheduled update time (3 AM UTC) arrives, **Then** the system pulls latest versions of all images used by the stacks
2. **Given** a stack uses containers with the "latest" tag and a new version is available, **When** the scheduled update runs, **Then** the system recreates the containers with the newly pulled images
3. **Given** a stack's images have not changed since last pull, **When** the scheduled update runs, **Then** the system leaves those containers running without recreation
4. **Given** the scheduled update completes successfully, **When** checking system logs, **Then** all operations (image pulls, container updates, image pruning) are logged with timestamps and outcomes
5. **Given** the scheduled update completes, **When** checking disk usage, **Then** unused and dangling images have been removed from the system

---

### User Story 2 - Configure Update Schedule (Priority: P2)

As a system administrator, I want to configure the automatic update schedule using a cron expression so that I can run updates at a time that best suits my operational needs (e.g., during low-traffic periods or maintenance windows).

**Why this priority**: Provides flexibility for different operational requirements but not critical for MVP. Default schedule (3 AM UTC) works for most use cases.

**Independent Test**: Can be fully tested by setting a custom cron expression, verifying it's accepted by the system, and confirming updates run at the configured times instead of the default schedule.

**Acceptance Scenarios**:

1. **Given** no custom schedule is configured, **When** the system starts, **Then** updates are scheduled to run daily at 3 AM UTC by default
2. **Given** an administrator provides a valid cron expression via configuration, **When** the system loads the configuration, **Then** updates are scheduled according to the custom expression
3. **Given** an invalid cron expression is provided, **When** the system loads the configuration, **Then** the system rejects it, logs an error with details, and falls back to the default schedule
4. **Given** a custom schedule is configured (e.g., every 6 hours), **When** monitoring the system over time, **Then** updates execute at the specified intervals

---

### User Story 3 - Update Operation Visibility (Priority: P3)

As a system administrator, I want detailed logs of scheduled update operations so that I can troubleshoot issues, verify successful updates, and understand what changed during each update cycle.

**Why this priority**: Important for operational visibility and troubleshooting, but the updates can work without enhanced logging. Basic logging (from P1) is sufficient for MVP.

**Independent Test**: Can be fully tested by running an update cycle and verifying that logs contain detailed information about each operation including stack names, image names, success/failure status, and any errors encountered.

**Acceptance Scenarios**:

1. **Given** an update cycle begins, **When** checking logs, **Then** a log entry marks the start of the update cycle with timestamp
2. **Given** images are being pulled for a stack, **When** checking logs, **Then** each image pull operation is logged with the image name and result (success or failure with error message)
3. **Given** containers are being updated, **When** checking logs, **Then** each stack update is logged with stack name, containers affected, and whether images changed
4. **Given** an error occurs during any operation, **When** checking logs, **Then** the error is logged with sufficient detail to diagnose the issue, and the update continues with remaining stacks
5. **Given** the update cycle completes, **When** checking logs, **Then** a summary log entry shows total stacks processed, images pulled, containers updated, and images pruned

---

### Edge Cases

- What happens when a stack's compose file references an image that no longer exists in the registry?
- How does the system handle network failures during image pulls?
- What happens if a stack is being manually modified when the scheduled update runs?
- How does the system behave if multiple stacks share the same image?
- If the next scheduled update time arrives while a previous update cycle is still running, the system terminates the running update and starts a fresh update cycle
- How does the system handle stacks that are stopped or in error state when the scheduled update runs?
- What happens if disk space is exhausted during image pulls?
- If the system crashes or restarts during an update cycle, the in-progress update is abandoned and the system waits for the next scheduled time

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept a cron expression to configure the schedule for automatic stack updates
- **FR-002**: System MUST use a default schedule of 3 AM UTC daily (cron: `0 3 * * *`) when no custom schedule is configured
- **FR-003**: System MUST validate cron expressions and reject invalid formats with descriptive error messages
- **FR-004**: System MUST pull the latest version of all images referenced by all managed compose stacks during each scheduled update, regardless of tag type (including both mutable tags like "latest" and immutable tags like version numbers or digests)
- **FR-005**: System MUST recreate containers when their image has been updated during the pull operation
- **FR-006**: System MUST NOT recreate containers when their image has not changed
- **FR-007**: System MUST prune all unused and dangling images system-wide (including images not related to managed stacks) after completing stack updates to free disk space
- **FR-008**: System MUST log the start and end of each scheduled update cycle with timestamps
- **FR-009**: System MUST log each image pull operation including the image name and result (success or failure)
- **FR-010**: System MUST log each stack update operation including stack name, containers affected, and outcome
- **FR-011**: System MUST log the image pruning operation with the number of images removed and space recovered
- **FR-012**: System MUST continue processing remaining stacks if an error occurs with one stack
- **FR-013**: System MUST handle network failures during image pulls gracefully and log appropriate error messages
- **FR-014**: System MUST skip stacks that are stopped or in error state and log the reason for skipping
- **FR-015**: System MUST handle concurrent modifications to stacks by detecting conflicts and logging warnings
- **FR-016**: System MUST operate on all managed stacks sequentially to avoid resource contention
- **FR-017**: System MUST allow the scheduled updates feature to be enabled or disabled via configuration
- **FR-018**: System MUST abandon any in-progress update cycle if the system crashes or restarts, and wait for the next scheduled time rather than resuming or immediately restarting
- **FR-019**: System MUST notify administrators of update failures and errors solely through log entries, without requiring external notification systems
- **FR-020**: System MUST terminate any running update cycle when a new scheduled update time is reached, log the termination, and immediately start a fresh update cycle

### Key Entities

- **Update Schedule Configuration**: The cron expression that defines when automatic updates run, enabled/disabled state, and timezone (UTC)
- **Update Cycle**: A single execution of the scheduled update process, containing start time, end time, list of stacks processed, total images pulled, total containers updated, total images pruned, and any errors encountered
- **Stack Update Result**: The outcome of updating a specific stack during an update cycle, containing stack identifier, list of images pulled with results, list of containers recreated, and any errors specific to this stack
- **Update Operation Log Entry**: A record of a specific operation during the update cycle (e.g., image pull, container update, image prune) with timestamp, operation type, subject (image/stack name), result status, and error message if applicable

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Scheduled updates complete successfully for all healthy stacks within 30 minutes under normal conditions (network and registry availability)
- **SC-002**: System continues running and remains available to serve requests during scheduled update operations without service interruption
- **SC-003**: 95% of update cycles complete without critical errors that prevent multiple stacks from being updated
- **SC-004**: All update operations (image pulls, container updates, image pruning) are logged with sufficient detail to diagnose any issues that occur
- **SC-005**: Disk space used by unused container images is reduced by at least 20% after each pruning operation (when unused images exist)
- **SC-006**: Administrators can change the update schedule and observe the changes take effect within one schedule cycle
- **SC-007**: When a single stack fails to update due to errors, remaining stacks continue to be processed successfully
- **SC-008**: 100% of stacks using the "latest" tag are updated to run the newest available image within one schedule cycle after a new image is published
