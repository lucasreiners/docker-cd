# Feature Specification: Git Repository Configuration

**Feature Branch**: `002-git-repo-config`  
**Created**: 2026-02-15  
**Status**: Draft  
**Input**: User description: "Add read-only git repository configuration via env vars, including https URL, access token, optional deployment directory (default root), and revision/branch; validate credentials on startup and fail if missing or invalid; optionally show non-secret repo info on root page."

## User Scenarios & Testing *(mandatory)*

Automated tests are REQUIRED for each user story per the constitution.

### User Story 1 - Configure repository access (Priority: P1)

As an operator, I want to provide repository configuration by environment variables so the service validates access on startup and refuses to run if the configuration is missing or invalid.

**Why this priority**: Without validated repository access, the service cannot safely perform deployments.

**Independent Test**: Can be fully tested by starting the service with valid and invalid environment values and observing startup success or failure.

**Acceptance Scenarios**:

1. **Given** required repository variables are missing, **When** the service starts, **Then** it fails fast with a clear error indicating which values are missing.
2. **Given** the repository URL, token, and revision are invalid or unauthorized, **When** the service starts, **Then** it fails fast with a clear error describing the access or revision failure.
3. **Given** the repository URL, token, and revision are valid, **When** the service starts, **Then** it starts successfully and records the validated configuration.

---

### User Story 2 - Confirm configuration in the UI (Priority: P2)

As an operator, I want to see the configured repository URL, revision, and deployment directory on the root status page without exposing secrets.

**Why this priority**: Operators need a quick, safe confirmation of what the service is configured to deploy.

**Independent Test**: Can be fully tested by starting the service with configuration and verifying the root page includes the non-secret values and excludes the token.

**Acceptance Scenarios**:

1. **Given** a valid configuration, **When** I visit the root page, **Then** I see the repository URL, revision, and deployment directory and never see the access token.

---

### User Story 3 - Target a deployment subdirectory (Priority: P3)

As an operator, I want to optionally set a deployment subdirectory within the repository so I can manage multiple targets from one repository.

**Why this priority**: It enables multi-host or multi-environment repositories without changing the repository structure.

**Independent Test**: Can be fully tested by starting the service with and without a deployment directory and validating default and override behavior.

**Acceptance Scenarios**:

1. **Given** no deployment directory is provided, **When** the service starts, **Then** it defaults to the repository root.
2. **Given** a deployment directory is provided, **When** the service starts, **Then** it validates that the directory exists at the configured revision.

### Edge Cases

- Repository URL is not HTTPS or has an invalid format.
- Revision does not exist or is not reachable with the provided token.
- Deployment directory points outside the repository or does not exist at the revision.
- Token is provided but does not have read access to the repository.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept repository configuration from environment variables for HTTPS URL, access token, revision/branch, and optional deployment directory.
- **FR-002**: System MUST fail startup when required repository variables are missing, with a clear error message describing what is missing.
- **FR-003**: System MUST validate repository access and revision on startup using the provided token, and fail startup on any validation error.
- **FR-004**: System MUST default the deployment directory to the repository root when none is provided.
- **FR-005**: System MUST validate that the deployment directory exists at the configured revision when provided.
- **FR-006**: System MUST display the repository URL, revision, and deployment directory on the root page and MUST NOT display the access token.
- **FR-007**: System MUST reject non-HTTPS repository URLs during startup validation.
- **FR-008**: System MUST treat repository access as read-only and MUST NOT perform write operations.

## Assumptions & Dependencies

- The runtime environment has outbound network access to the configured Git host.
- The repository supports token-based read access over HTTPS.
- The deployment directory is a repository-relative path at the configured revision.
- Access tokens are provided via environment variables and handled as sensitive values.

### Key Entities *(include if feature involves data)*

- **RepositoryConfig**: Repository URL, revision, deployment directory, access token (sensitive), validation status.
- **RepositoryValidationResult**: Validation outcome, error category, timestamp.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of startup attempts with missing required repository variables fail within 5 seconds with an actionable error message.
- **SC-002**: 95% of valid repository configurations are validated within 10 seconds during startup.
- **SC-003**: 100% of root page responses show URL, revision, and deployment directory while never displaying the access token.
- **SC-004**: Operators can confirm the intended deployment target from the root page on the first visit without consulting logs or configuration files.
