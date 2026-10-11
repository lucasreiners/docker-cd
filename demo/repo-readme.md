# Docker-CD demo repository

Edit any `compose.yaml` using Gitea's pencil button and commit directly to `main`.
Gitea sends a signed webhook to Docker-CD, which checks out your change and
deploys it into the dedicated Docker-in-Docker daemon.

- **Docker-CD:** http://localhost:18881
- **Welcome application:** http://localhost:18088

## Seeded stacks

| Stack | What to test |
|---|---|
| `apps/welcome` | Healthy nginx application. Change `DEMO_VERSION` and watch deployment, source diff and the welcome page. |
| `apps/unhealthy` | Running container whose health check deliberately fails. Change `false` to `true`. |
| `apps/broken` | Invalid Compose property. Remove `deliberately_invalid_demo_property` to recover. |
| `infra/heartbeat` | Worker emitting logs every five seconds. Test Logs, filtering and download. Delete/restore the file to test removal. |

The repository, workload containers and operation history persist until the demo
is reset. All credentials and data belong to this local demo.
