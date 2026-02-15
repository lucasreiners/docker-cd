<!--
Sync Impact Report
- Version change: n/a -> 1.0.0
- Modified principles:
	- [PRINCIPLE_1_NAME] -> I. GitOps Source of Truth
	- [PRINCIPLE_2_NAME] -> II. Continuous Reconciliation
	- [PRINCIPLE_3_NAME] -> III. Container-First Runtime
	- [PRINCIPLE_4_NAME] -> IV. Safe Docker Compose Apply
	- [PRINCIPLE_5_NAME] -> V. Automated Testing Baseline
- Added sections: none
- Removed sections: none
- Templates requiring updates:
	- .specify/templates/plan-template.md (updated)
	- .specify/templates/spec-template.md (updated)
	- .specify/templates/tasks-template.md (updated)
	- .specify/templates/commands/*.md (missing, no update)
- Follow-up TODOs: none
-->
# Docker CD Constitution

## Core Principles

### I. GitOps Source of Truth
The desired state stored in Git MUST be the only authoritative source.
The system MUST reconcile actual state to match Git on every event or
scheduled loop. Manual changes outside Git MUST be reverted or flagged
as drift with explicit operator acknowledgement.
Rationale: Deterministic deployments require a single source of truth.

### II. Continuous Reconciliation
Reconciliation MUST be idempotent, safe to retry, and converge on the
desired state. The service MUST react to webhook events and also perform
periodic reconciliation to catch missed events.
Rationale: Webhooks are best-effort, so convergence must not depend on
any single event delivery.

### III. Container-First Runtime
The system MUST run as a containerized, long-lived service with health
checks. Configuration MUST be provided via environment variables and/or
mounted configuration files, never hard-coded.
Rationale: The product itself is meant to manage containers and should
deploy consistently across environments.

### IV. Safe Docker Compose Apply
All runtime changes MUST be applied via `docker compose` using a declared
project name and compose file(s). The reconciler MUST compute a plan
(create/update/remove) before applying changes. Destructive actions MUST
require an explicit opt-in setting.
Rationale: Safe reconciliation requires predictability and guardrails
around deletions.

### V. Automated Testing Baseline
Automated tests MUST cover webhook handling, reconciliation logic, and
compose apply behavior. Integration tests SHOULD run against real Docker
Compose in CI when feasible, and unit tests MUST isolate core logic.
Rationale: A continuous delivery agent without tests is a source of
deploy risk.

## Operational Constraints

- GitHub webhooks MUST be verified with HMAC signatures.
- Logs MUST be structured (JSON) and include correlation IDs per event.
- Configuration MUST support: repository URL, branch/ref, path to
	compose files, project name, poll interval, and drift policy.
- The service MUST not store long-lived secrets in its own state; use
	environment or mounted secrets instead.
- Technology selection (language/framework) MUST be documented in the
	first feature spec and implementation plan.

## Development Workflow

- Every feature MUST include a spec and plan under `specs/` before
	implementation work begins.
- CI MUST run automated tests on every pull request.
- Changes that affect reconciliation behavior MUST include a regression
	test that fails before the fix.
- Each release MUST include a changelog entry that calls out any
	operational impact or new configuration.

## Governance

- This constitution supersedes other project guidance.
- Amendments require a documented rationale and a version bump following
	semantic versioning: MAJOR for breaking governance changes, MINOR for
	new principles or material expansion, PATCH for clarifications.
- Every plan MUST include a Constitution Check section that verifies
	compliance with each principle.
- Compliance review is REQUIRED during PR review; non-compliance must be
	resolved or explicitly approved with justification.

**Version**: 1.0.0 | **Ratified**: 2026-02-15 | **Last Amended**: 2026-02-15
