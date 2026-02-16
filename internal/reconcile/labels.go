package reconcile

// Label keys used on Docker containers to persist sync metadata.
const (
	LabelStackPath            = "com.docker-cd.stack.path"
	LabelDesiredRevision      = "com.docker-cd.desired.revision"
	LabelDesiredCommitMessage = "com.docker-cd.desired.commit_message"
	LabelDesiredComposeHash   = "com.docker-cd.desired.compose_hash"
	LabelSyncedAt             = "com.docker-cd.synced.at"
	LabelSyncAt               = "com.docker-cd.sync.at"
	LabelSyncStatus           = "com.docker-cd.sync.status"
	LabelSyncError            = "com.docker-cd.sync.error"
)

// AllLabelKeys returns the full list of label keys used by docker-cd.
func AllLabelKeys() []string {
	return []string{
		LabelStackPath,
		LabelDesiredRevision,
		LabelDesiredCommitMessage,
		LabelDesiredComposeHash,
		LabelSyncedAt,
		LabelSyncAt,
		LabelSyncStatus,
		LabelSyncError,
	}
}
