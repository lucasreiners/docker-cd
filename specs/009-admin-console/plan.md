# Implementation plan

1. Introduce durable operation records and wire refresh/deployment/image cycles.
2. Add instance metadata, runtime inspection and policy-aware stack actions.
3. Migrate to PrimeVue and implement the admin shell and five workspaces.
4. Add tabbed stack inspection and a global operation drawer.
   Include applied-source comparison, bounded log streams and webhook diagnostics.
5. Expand fixture and real-backend tests; inspect headed desktop/mobile results.

## Persistence

Use bounded, atomic JSON snapshots in a dedicated mounted data directory. Persist
operation summaries and steps, not downloaded layer bytes. Recover unfinished
records as interrupted on startup. Report storage failures explicitly. Retain at
most 1000 completed records while never evicting active work.
Applied Compose source snapshots are stored separately in `applied-sources.json`,
updated only after successful deployment and removed on successful stack removal.
Webhook diagnostic records retain outcomes and delivery IDs, not request bodies
or signatures. Log streams are bounded and not persisted.

## Dependency resolution

Registry inspection during implementation found that PrimeVue 5 and themes 3
require a PrimeUI license key. Use the verified MIT releases PrimeVue 4.5.5 and
@primeuix/themes 2.0.3, pinned exactly. The admin architecture and styled-mode
theme design are unchanged.

## Constitution check

- Git is authoritative; configuration inspection is read-only.
- Reconciliation remains serialized and uses Docker Compose.
- Targeted actions validate policy, refresh readiness and reviewed revision.
- Removal opt-in is preserved; the console adds no direct deletion action.
- Agent settings stay environment/mounted-file-owned; credentials are omitted.
- Backend regressions, unit tests and headed/real-Docker E2E verify the changes.
