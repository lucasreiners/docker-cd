# Manual Docker-in-Docker demo

Start the current Docker-CD code against a dedicated Docker engine and a real
local Gitea provider. Requires Docker Desktop/Engine with Compose v2 and Python 3.

```bash
python3 demo/manage.py start
```

| Service | URL |
|---|---|
| Admin console | http://localhost:18881 |
| Git repository/editor | http://localhost:13000/demo/homelab |
| Welcome application | http://localhost:18088 |
| Backend API | http://localhost:18880/api/instance |

Gitea login: **`demo` / `docker-cd-demo-2026`**.

Open a Compose file in Gitea, click the pencil, change it and commit to `main`.
Watch Docker-CD's Operations, source diffs, runtime health and logs update. The
welcome service starts healthy, `apps/unhealthy` intentionally fails its health
check, `apps/broken` intentionally fails deployment, and `infra/heartbeat` emits logs.

## One-command scenarios

```bash
python3 demo/manage.py scenario change    # Change welcome version through Git
python3 demo/manage.py scenario recover   # Fix the deliberately broken Compose
python3 demo/manage.py scenario break     # Reintroduce deployment failure
python3 demo/manage.py scenario healthy   # Fix the unhealthy worker
python3 demo/manage.py scenario approval  # Enable flag policy, then change welcome
python3 demo/manage.py policy revert      # Restore automatic reconciliation
python3 demo/manage.py scenario remove    # Remove heartbeat from Git/runtime
python3 demo/manage.py scenario restore   # Restore heartbeat
```

In approval mode, use **Acknowledge & deploy** in the stack details. Repository
checks and image updates are also available directly in the console.

## Lifecycle

```bash
python3 demo/manage.py status
python3 demo/manage.py logs               # Backend logs
python3 demo/manage.py logs git           # Gitea/webhook logs
python3 demo/manage.py stop               # Stop, preserving demo volumes
python3 demo/manage.py start              # Rebuild/restart, preserving Git edits
python3 demo/manage.py down               # Remove demo containers, retain volumes
python3 demo/manage.py reset              # Delete ONLY demo volumes and reseed
```

The Compose project is `docker-cd-demo`. Workloads run inside `dind`; the backend
connects to `tcp://dind:2375` and has no host Docker socket mounted. DinD's daemon
port is internal. Published UI/Git/application ports bind only to loopback.

Gitea provides browser editing on HTTP port 13000. An internal TLS proxy serves
`https://git-https/demo/homelab.git` to the production backend, satisfying its
HTTPS-only Git configuration. The demo certificate is trusted inside the backend
through `SSL_CERT_FILE`; nothing is installed into the host trust store. Use the
browser-facing Git URL above when browsing the repository.

`demo/.env` is generated and ignored by Git. It contains only a generated demo Git
token and the drift policy. This setup never reads the application's root `.env`.
Initial startup downloads images and compiles both application images. Subsequent
starts preserve repository edits, containers/volumes inside DinD and operation
history. Use reset to return to the seeded state.
