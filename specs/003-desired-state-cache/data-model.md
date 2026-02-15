# Data Model: Desired State Cache and Refresh

## Entities

### DesiredStateSnapshot

Represents the latest desired state loaded from Git.

- **revision**: string (Git revision resolved at refresh time)
- **ref**: string (configured ref/branch name)
- **refType**: string (branch, tag, or commit)
- **refreshedAt**: timestamp
- **refreshStatus**: string (refreshing, queued, completed, failed) - system-wide Git refresh status
- **refreshError**: string (empty on success)
- **stacks**: list of StackRecord

### StackRecord

Represents a stack discovered in the repository.

- **path**: string (directory containing compose file)
- **composeFile**: string (docker-compose.yml or docker-compose.yaml)
- **composeHash**: string (SHA-256 of compose file contents)
- **status**: string (missing, syncing, synced, deleting) - per-stack sync status

### RefreshTrigger

Represents a refresh request source.

- **source**: string (startup, webhook, manual, periodic)
- **requestedAt**: timestamp

## Relationships

- DesiredStateSnapshot contains many StackRecord entries.
- RefreshTrigger updates DesiredStateSnapshot.refreshStatus and refreshedAt (system-wide Git refresh).

## Validation Rules

- `composeFile` must be one of docker-compose.yml or docker-compose.yaml.
- `status` must be one of missing, syncing, synced, deleting.
- `refreshStatus` must be one of refreshing, queued, completed, failed.
