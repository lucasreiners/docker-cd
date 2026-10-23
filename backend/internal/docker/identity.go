package docker

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

// IdentityInspector deliberately permits only local, read-only SDK operations.
type IdentityInspector interface {
	ContainerInspect(context.Context, string) (container.InspectResponse, error)
	ImageInspectWithRaw(context.Context, string) (image.InspectResponse, []byte, error)
}

var fullContainerID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var immutableImageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

const (
	IdentityDigestLimit       = 16
	IdentityDigestStringLimit = 1024
	IdentityReferenceLimit    = 4096
	IdentityPlatformLimit     = 128
	identityCacheLimit        = 1024
	identityCacheBudget       = 2 * 1024 * 1024
)

type imageMetadata struct {
	platform *desiredstate.ImagePlatform
	digests  []desiredstate.RepositoryDigest
	reason   string
}

// IdentityReader is scoped to one observation cycle. Its bounded cache contains
// only whitelist metadata keyed by immutable ID; raw SDK responses are discarded.
// Serializing inspections also bounds SDK concurrency to one (below the cap of four).
type IdentityReader struct {
	inspector     IdentityInspector
	mu            sync.Mutex
	images        map[string]imageMetadata
	metadataBytes int
}

func NewIdentityReader(inspector IdentityInspector) *IdentityReader {
	return &IdentityReader{inspector: inspector, images: make(map[string]imageMetadata)}
}

func safeReference(raw string) string {
	if len(raw) > IdentityReferenceLimit {
		return ""
	}
	if _, err := reference.ParseNormalizedNamed(raw); err != nil {
		return ""
	}
	return raw
}

// Resolve must only be called for containers enumerated for a discovered stack.
// Even then, exact ID and both managed and Compose ownership labels are checked
// before reading image metadata. No name, tag, or short-ID lookup is possible.
func (r *IdentityReader) Resolve(ctx context.Context, stackPath string, row desiredstate.ContainerInfo) *desiredstate.ImageIdentity {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := &desiredstate.ImageIdentity{Status: "unavailable", ObservedAt: time.Now().UTC(), Source: "container_image_inspect", Reference: safeReference(row.Image), RepositoryDigests: []desiredstate.RepositoryDigest{}, Freshness: "not_checked"}
	if stackPath == "" || !fullContainerID.MatchString(row.FullID) || row.ComposeProject == "" || row.Service == "" {
		out.Reason = "ownership_unverified"
		return out
	}
	if r.inspector == nil {
		out.Reason = "unsupported"
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		out.Reason = "inspection_failed"
		return out
	}
	inspected, err := r.inspector.ContainerInspect(ctx, row.FullID)
	if err != nil {
		out.Reason = identityError(err, "container_replaced")
		return out
	}
	if inspected.ContainerJSONBase == nil || inspected.ID != row.FullID {
		out.Reason = "container_replaced"
		return out
	}
	if inspected.Config == nil {
		out.Reason = "ownership_unverified"
		return out
	}
	labels := inspected.Config.Labels
	if labels["com.docker-cd.stack.path"] != stackPath || labels["com.docker.compose.project"] != row.ComposeProject || labels["com.docker.compose.service"] != row.Service {
		out.Reason = "ownership_unverified"
		return out
	}
	if !immutableImageID.MatchString(inspected.Image) {
		out.Reason = "inspection_failed"
		return out
	}
	out.ImageID = inspected.Image
	out.Reference = safeReference(inspected.Config.Image)
	metadata, exists := r.images[out.ImageID]
	if !exists {
		im, _, inspectErr := r.inspector.ImageInspectWithRaw(ctx, out.ImageID)
		if inspectErr != nil {
			metadata.reason = identityError(inspectErr, "image_missing")
		} else if im.ID != out.ImageID {
			metadata.reason = "inspection_failed"
		} else if !boundedImageMetadata(im) {
			// Do not publish an arbitrary prefix as the complete identity set.
			// Existing unavailable semantics keep the successful listing intact.
			metadata.reason = "inspection_failed"
		} else {
			metadata.digests = repositoryDigests(im.RepoDigests)
			if im.Os != "" && im.Architecture != "" {
				metadata.platform = &desiredstate.ImagePlatform{OS: im.Os, Architecture: im.Architecture, Variant: im.Variant}
			}
		}
		// Cap memoization, not the reported identities. No cache survives a cycle.
		cost := metadataCost(out.ImageID, metadata)
		if len(r.images) < identityCacheLimit && r.metadataBytes+cost <= identityCacheBudget {
			r.images[out.ImageID] = metadata
			r.metadataBytes += cost
		}
	}
	if metadata.reason != "" {
		out.Reason = metadata.reason
		return out
	}
	if metadata.platform != nil {
		p := *metadata.platform
		out.Platform = &p
	}
	out.RepositoryDigests = append(out.RepositoryDigests, metadata.digests...)
	configured, err := reference.ParseNormalizedNamed(out.Reference)
	matches := 0
	if err == nil {
		for _, digest := range out.RepositoryDigests {
			if digest.Repository == configured.Name() {
				matches++
				d := digest
				out.PreferredRepositoryDigest = &d
			}
		}
	}
	switch matches {
	case 1:
		out.Status = "available"
	case 0:
		out.Reason = "not_recorded"
	default:
		out.Status = "ambiguous"
		out.Reason = "repository_ambiguous"
		out.PreferredRepositoryDigest = nil
	}
	return out
}

func repositoryDigests(values []string) []desiredstate.RepositoryDigest {
	if len(values) > IdentityDigestLimit {
		return []desiredstate.RepositoryDigest{}
	}
	out := make([]desiredstate.RepositoryDigest, 0, len(values))
	seen := make(map[desiredstate.RepositoryDigest]bool)
	for _, raw := range values {
		if len(raw) > IdentityDigestStringLimit {
			return []desiredstate.RepositoryDigest{}
		}
		named, err := reference.ParseNormalizedNamed(raw)
		if err != nil {
			continue
		}
		digested, ok := named.(reference.Digested)
		if !ok {
			continue
		}
		d := desiredstate.RepositoryDigest{Repository: named.Name(), Digest: digested.Digest().String(), Kind: "unknown"}
		if !seen[d] {
			out = append(out, d)
			seen[d] = true
		}
	}
	return out
}

func boundedImageMetadata(im image.InspectResponse) bool {
	if len(im.RepoDigests) > IdentityDigestLimit || len(im.Os) > IdentityPlatformLimit || len(im.Architecture) > IdentityPlatformLimit || len(im.Variant) > IdentityPlatformLimit {
		return false
	}
	for _, raw := range im.RepoDigests {
		if len(raw) > IdentityDigestStringLimit {
			return false
		}
	}
	return true
}

func metadataCost(id string, metadata imageMetadata) int {
	// Include conservative struct/map/allocator overhead, not just wire strings.
	n := len(id) + len(metadata.reason) + 256
	if metadata.platform != nil {
		n += len(metadata.platform.OS) + len(metadata.platform.Architecture) + len(metadata.platform.Variant) + 128
	}
	for _, digest := range metadata.digests {
		n += len(digest.Repository) + len(digest.Digest) + len(digest.Kind) + 128
	}
	return n
}

func identityError(err error, missing string) string {
	switch {
	case errdefs.IsNotFound(err):
		return missing
	case errdefs.IsForbidden(err), errdefs.IsUnauthorized(err):
		return "inspection_denied"
	case errdefs.IsNotImplemented(err):
		return "unsupported"
	default:
		return "inspection_failed"
	}
}
