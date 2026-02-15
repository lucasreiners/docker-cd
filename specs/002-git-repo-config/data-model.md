# Data Model: Git Repository Configuration

## Entity: RepositoryConfig

**Purpose**: Captures repository configuration provided via environment variables.

**Fields**:
- `repo_url` (string, required): HTTPS Git repository URL.
- `access_token` (string, required, sensitive): Read-only access token.
- `revision` (string, required): Branch, tag, or commit hash.
- `deploy_dir` (string, optional): Repository-relative path. Defaults to repository root.

**Validation Rules**:
- `repo_url` must be valid HTTPS URL.
- `access_token` must be non-empty.
- `revision` must be non-empty.
- `deploy_dir` must be a relative path within the repository when provided.

## Entity: RepositoryValidationResult

**Purpose**: Represents the outcome of startup validation.

**Fields**:
- `status` (enum: `success`, `failure`)
- `error_type` (enum: `missing_config`, `invalid_url`, `auth_failed`, `ref_not_found`, `path_not_found`, `unknown`)
- `message` (string): Human-readable failure description.
- `checked_at` (timestamp)

## State Transitions

- `Unvalidated` -> `Validated` when repository access, revision, and deploy path are confirmed.
- `Unvalidated` -> `Failed` when any required config is missing or validation fails; service exits.
