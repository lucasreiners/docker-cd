# Feature: Manual GitOps demo

Provide a long-lived, disposable local environment with the current frontend and
production backend, a dedicated Docker-in-Docker daemon, and a seeded Gitea Git
provider. Users can edit Compose files in Gitea and observe real signed webhook
deliveries, deployments, failures, logs, source diffs and persisted operations.

Requirements: loopback-only published ports, dedicated Compose project/volumes,
idempotent startup, preserved manual edits on restart, explicit demo-only reset,
healthy/unhealthy/failed fixtures, scenario commands, and documented credentials.

Acceptance: startup reaches a working UI and seeded repository; committing a
change triggers a signed webhook and a real container deployment inside DinD;
the lifecycle command can stop and restart without losing the repository/history.
