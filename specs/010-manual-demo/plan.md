# Manual demo implementation plan

Use a separate Compose project containing Gitea, an internal HTTPS Git proxy,
DinD, the production backend, and the built frontend. A Python standard-library
manager bootstraps the demo account/repository/webhook and provides scenario
commands. The backend trusts a demo certificate through a container-local CA file.

## Constitution check

- Git remains authoritative; scenarios make actual commits through Gitea.
- Production reconciliation and Docker Compose execute inside the demo runtime.
- Configuration comes from Compose and an ignored, generated demo environment file.
- Removals are explicitly enabled only for this demo.
- Validation exercises startup, real signed webhook deployment and browser UI.
- Host Docker is used to provision the demo; workloads use only the DinD endpoint.
