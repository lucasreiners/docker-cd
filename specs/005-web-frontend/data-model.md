# Data Model: Minimal Web Frontend

## StackRecord

Represents a stack discovered in the repository and its sync metadata.

- `path` (string, required): Repository-relative path for the stack.
- `composeFile` (string, required): Compose file name.
- `composeHash` (string, required): Hash of compose contents.
- `status` (enum, required): `missing | syncing | synced | deleting | failed`.
- `syncedRevision` (string, optional)
- `syncedCommitMessage` (string, optional)
- `syncedComposeHash` (string, optional)
- `syncedAt` (string, optional, RFC3339)
- `lastSyncAt` (string, optional, RFC3339)
- `lastSyncStatus` (string, optional)
- `lastSyncError` (string, optional)

## RefreshSnapshot

Represents a system-wide refresh snapshot (without stacks).

- `revision` (string)
- `commitMessage` (string, optional)
- `ref` (string)
- `refType` (string)
- `refreshedAt` (string, RFC3339)
- `refreshStatus` (enum): `refreshing | queued | completed | failed`
- `refreshError` (string, optional)

## UpdateEvent

Represents a server-push SSE event to the UI.

- `id` (string): Monotonic event id.
- `event` (enum): `stack.snapshot | stack.upsert | stack.delete | refresh.status`.
- `data` (object):
  - `stack.snapshot`: `{ records: StackRecord[] }`
  - `stack.upsert`: `{ record: StackRecord }`
  - `stack.delete`: `{ path: string, deletedAt: string }`
  - `refresh.status`: `RefreshSnapshot`

## UpdateChannelState

Client-side state of the SSE channel.

- `state` (enum): `connected | reconnecting | disconnected`
- `lastEventId` (string, optional)
- `lastUpdatedAt` (string, RFC3339, optional)
