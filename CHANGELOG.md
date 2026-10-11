# Changelog

## Unreleased

- Add a persistent local DinD demo with a seeded Gitea provider, signed webhooks,
  browser-editable Compose files, and manual failure/recovery/approval scenarios.

- Replace the two-page status UI with a Vue/PrimeVue administration console:
  Overview, Stacks, Operations, Repository, and System.
- Add persisted operation history with parent/child links, versioned live events,
  mixed-success outcomes, and interrupted-job recovery after restart.
- Add revision-checked, asynchronous per-stack redeployment, image updates and
  structured drift acknowledgement.
- Expose effective instance configuration, runtime inspection and read-only Compose
  source through dedicated APIs.
- Add persisted applied-source comparison, bounded container log streaming with
  pause/filter/download, and webhook delivery diagnostics linked to refreshes.
- Add `DATA_DIR` (default `/data`) and `operation-data` volumes to the Compose
  deployments. Retain this volume to preserve operation history.
- Pin MIT-licensed PrimeVue 4.5.5 and PrimeUIX themes 2.0.3.
- Expand unit, backend race, headed-browser and real-Docker lifecycle coverage.
