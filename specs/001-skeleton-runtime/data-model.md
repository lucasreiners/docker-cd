# Data Model: Docker-CD Skeleton Runtime

## RuntimeConfig

**Purpose**: Configuration needed to run the skeleton service and prepare
for later GitOps features.

**Fields**:
- `port` (int): HTTP port for the web server. Default 8080.
- `project_name` (string): Display name used in ASCII art. Default
  "Docker-CD".
- `docker_socket` (string): Path to Docker socket. Default
  `/var/run/docker.sock`.
- `repo_url` (string, optional): Git repository URL for desired state.
- `repo_ref` (string, optional): Branch or ref to monitor.
- `repo_path` (string, optional): Path within the repo for compose files.
- `drift_policy` (string, optional): Placeholder for future drift
  handling (e.g., `report`, `reconcile`).

## DockerStatus

**Purpose**: Snapshot data used to render the root response.

**Fields**:
- `running_containers` (int): Count of running containers from `docker
  ps`.
- `retrieved_at` (timestamp): Time the count was retrieved.

## Relationships

- `RuntimeConfig` is used by the HTTP handler to locate the Docker socket
  and render the response.
- `DockerStatus` is derived by the Docker CLI executor and rendered by
  the response formatter.
