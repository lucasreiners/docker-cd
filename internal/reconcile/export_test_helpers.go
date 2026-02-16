package reconcile

// TestGenerateLabelOverride is an exported wrapper around generateLabelOverride
// for use in integration tests.
func TestGenerateLabelOverride(stackPath, revision, commitMessage, composeHash string, serviceNames []string) string {
	return generateLabelOverride(stackPath, revision, commitMessage, composeHash, serviceNames)
}

// TestWriteTempComposeDir is an exported wrapper around writeTempComposeDir
// for use in integration tests.
func TestWriteTempComposeDir(composeFileName string, composeContent []byte, overrideContent string) (string, string, func(), error) {
	return writeTempComposeDir(composeFileName, composeContent, overrideContent)
}

// TestExtractServiceNames is an exported wrapper around extractServiceNames
// for use in integration tests.
func TestExtractServiceNames(content []byte) []string {
	return extractServiceNames(content)
}

// LabelDesiredCommitMessage re-exports the label key for integration tests.
const LabelDesiredCommitMessageKey = LabelDesiredCommitMessage

// LabelDesiredComposeHash re-exports the label key for integration tests.
const LabelDesiredComposeHashKey = LabelDesiredComposeHash
