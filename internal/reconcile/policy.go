package reconcile

// ReconciliationPolicy governs reconciliation behavior.
type ReconciliationPolicy struct {
	// Enabled controls whether reconciliation runs at all.
	Enabled bool
	// RemoveEnabled controls whether stacks removed from desired state are torn down.
	RemoveEnabled bool
	// DriftPolicy is "revert" (auto-fix) or "flag" (require acknowledgement).
	DriftPolicy string
	// MaxConcurrency is the maximum number of stacks reconciled concurrently (fixed to 1).
	MaxConcurrency int
}

// DefaultPolicy returns the default reconciliation policy.
func DefaultPolicy() ReconciliationPolicy {
	return ReconciliationPolicy{
		Enabled:        true,
		RemoveEnabled:  false,
		DriftPolicy:    "revert",
		MaxConcurrency: 1,
	}
}
