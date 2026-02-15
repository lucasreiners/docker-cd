# Research: Git Repository Configuration

## Decision 1: Use go-git for read-only validation

- **Decision**: Use `go-git` (v5) to validate repository access over HTTPS, resolve the revision, and inspect the tree without a full clone.
- **Rationale**: A pure-Go client avoids adding a Git binary to the container image, keeps the runtime smaller, and is easier to test and mock in Go.
- **Alternatives considered**:
  - Git CLI (`git ls-remote`, `git cat-file`): highest compatibility but requires a Git binary and careful token handling in subprocesses.

## Decision 2: Validate deployment directory via tree lookup

- **Decision**: After resolving the revision, perform a shallow fetch (depth 1) for the target commit and check whether the deployment directory exists in the commit tree.
- **Rationale**: This proves the revision is reachable and the path exists without a full clone or checkout.
- **Alternatives considered**:
  - Host-specific APIs (GitHub/GitLab) for path checks: lighter but not portable across Git hosts.

## Decision 3: HTTPS-only URL validation and secret handling

- **Decision**: Accept only HTTPS repository URLs and never display or log the access token.
- **Rationale**: HTTPS keeps credential handling consistent and avoids accidental secret exposure through logs or UI.
- **Alternatives considered**:
  - Allowing SSH URLs: adds key management and container-side SSH configuration complexity.
