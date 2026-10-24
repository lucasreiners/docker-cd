package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

var errUnknownRuntimeStack = errors.New("Stack not found")
var errRuntimeLimit = errors.New("Runtime observation unavailable: resource limit exceeded")

const (
	runtimeStackLimit     = 4096
	runtimePathBudget     = 256 * 1024
	runtimeStackRowLimit  = 256
	runtimeRowLimit       = 4096
	runtimeStringLimit    = 4096
	runtimeStackByteLimit = 512 * 1024
	runtimeByteBudget     = 8 * 1024 * 1024
	// Fixed data headroom means adding discovered scopes can never push old
	// successes over the response budget: worst-case escaped keys consume 1.5MiB
	// and 4096 envelopes consume 1MiB, leaving at least 0.5MiB of slack.
	runtimeDataByteBudget = 5 * 1024 * 1024
	// Covers the observation envelope, timestamps, sanitized error and map
	// punctuation. Encoded scope keys and rows are counted separately.
	runtimeEnvelopeReserve = 256
)

type runtimeSignature struct {
	stack            [sha256.Size]byte
	terminal, global uint64
}

// RuntimeObservation distinguishes a failed attempt from the last successful
// listing. Identity errors never turn a successful listing into a runtime error.
type RuntimeObservation struct {
	Containers  []desiredstate.ContainerInfo `json:"containers"`
	Error       *string                      `json:"error"`
	ObservedAt  *time.Time                   `json:"observedAt"`
	AttemptedAt time.Time                    `json:"attemptedAt"`
}

// RuntimeReader is shared by home and detail. One bounded cycle is in flight,
// independent of any one HTTP waiter's cancellation. Compose calls are serial;
// identity inspection concurrency is also one, strictly below the cap of four.
// Only current discovered stacks and one cycle's immutable-image cache survive.
type RuntimeReader struct {
	mu                              sync.Mutex
	store                           *desiredstate.Store
	lister                          ContainerLister
	inspector                       docker.IdentityInspector
	broadcaster                     *desiredstate.Broadcaster
	cache                           map[string]RuntimeObservation
	signatures                      map[string]runtimeSignature
	stateRevision, terminalRevision uint64
	stacks                          []desiredstate.RuntimeStack
	terminals                       map[string]uint64
	scopes                          map[string]struct{}
	currentSignatures               map[string]runtimeSignature
	expires                         map[string]time.Time
	flight                          chan struct{}
	generation                      uint64
	lastReconnect                   time.Time
	ttl                             time.Duration
	cycleTimeout                    time.Duration
}

func NewRuntimeReader(store *desiredstate.Store, lister ContainerLister, inspector docker.IdentityInspector, broadcaster *desiredstate.Broadcaster) *RuntimeReader {
	return &RuntimeReader{store: store, lister: lister, inspector: inspector, broadcaster: broadcaster, cache: make(map[string]RuntimeObservation), signatures: make(map[string]runtimeSignature), expires: make(map[string]time.Time), ttl: 15 * time.Second, cycleTimeout: 20 * time.Second}
}

// Invalidate is also useful for an explicit server-side runtime lifecycle signal.
func (r *RuntimeReader) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	clear(r.expires)
}

// Reconnect invalidates once per burst, not once per client in a reconnect storm.
func (r *RuntimeReader) Reconnect() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastReconnect) < time.Second {
		return
	}
	r.lastReconnect = time.Now()
	r.generation++
	clear(r.expires)
}

// stateSignatures detects discovered-stack changes and terminal deployment/removal
// events on the next read, even without an SSE consumer. This avoids a permanent
// background subscription/goroutine and invalidates in-flight old observations too.
func (r *RuntimeReader) stateSignatures() error {
	// Caller holds r.mu: concurrent clients share both metadata refresh and
	// observation work. The unchanged fast path is two scalar revision checks.
	revision, stacks, withinLimits := r.store.RuntimeStacks(r.stateRevision, runtimeStackLimit, runtimePathBudget, runtimeStringLimit)
	if !withinLimits {
		return errRuntimeLimit
	}
	changed := revision != r.stateRevision
	if changed {
		r.stateRevision, r.stacks = revision, stacks
		r.scopes = make(map[string]struct{}, len(stacks)+1)
		r.scopes[""] = struct{}{}
		for _, stack := range stacks {
			r.scopes[stack.Path] = struct{}{}
		}
	}
	if r.broadcaster != nil {
		since := r.terminalRevision
		if changed {
			since = 0
		}
		revision, terminals := r.broadcaster.Operations().RuntimeTerminals(since, r.scopes)
		if changed || revision != r.terminalRevision {
			r.terminalRevision, r.terminals = revision, terminals
			changed = true
		}
	}
	if changed || r.currentSignatures == nil {
		r.currentSignatures = make(map[string]runtimeSignature, len(r.stacks))
		for _, stack := range r.stacks {
			r.currentSignatures[stack.Path] = runtimeSignature{stack.Signature, r.terminals[stack.Path], r.terminals[""]}
		}
	}
	return nil
}

func (r *RuntimeReader) Read(ctx context.Context) (map[string]RuntimeObservation, error) {
	return r.read(ctx, "")
}

func (r *RuntimeReader) read(ctx context.Context, onlyPath string) (map[string]RuntimeObservation, error) {
	// Repeated invalidations must not turn one HTTP wait into an unbounded retry.
	ctx, cancel := context.WithTimeout(ctx, r.cycleTimeout+time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.mu.Lock()
		if err := r.stateSignatures(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		stacks, signatures := r.stacks, r.currentSignatures
		if onlyPath != "" {
			if _, known := signatures[onlyPath]; !known {
				r.mu.Unlock()
				return nil, errUnknownRuntimeStack
			}
		}
		// Prune removed scopes, including old successes; callers cannot resurrect
		// their metadata by retaining a historical stack path.
		for path := range r.cache {
			if _, known := signatures[path]; !known {
				delete(r.cache, path)
				delete(r.signatures, path)
				delete(r.expires, path)
			}
		}
		stale := make([]desiredstate.RuntimeStack, 0)
		selected := make(map[string]RuntimeObservation)
		for _, stack := range stacks {
			if onlyPath != "" && stack.Path != onlyPath {
				continue
			}
			observation, known := r.cache[stack.Path]
			if !known || r.signatures[stack.Path] != signatures[stack.Path] || !time.Now().Before(r.expires[stack.Path]) {
				stale = append(stale, stack)
			} else {
				selected[stack.Path] = observation
			}
		}
		if len(stale) == 0 {
			out := cloneRuntime(selected)
			r.mu.Unlock()
			return out, nil
		}
		if r.flight == nil {
			r.flight = make(chan struct{})
			// Cache values are immutable. A cycle replaces entire observations;
			// only the response boundary deep-copies container/identity rows.
			previous := make(map[string]RuntimeObservation, len(r.cache))
			for path, observation := range r.cache {
				previous[path] = observation
			}
			go r.observe(stale, signatures, r.generation, previous)
		}
		flight := r.flight
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight:
		}
	}
}

func (r *RuntimeReader) observe(stacks []desiredstate.RuntimeStack, signatures map[string]runtimeSignature, generation uint64, previous map[string]RuntimeObservation) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cycleTimeout)
	defer cancel()
	identities := docker.NewIdentityReader(r.inspector)
	out := make(map[string]RuntimeObservation, len(stacks))
	totalRows, totalBytes, totalDataBytes := 0, 0, 0
	for path := range signatures {
		key, _ := json.Marshal(path)
		totalBytes += len(key) + runtimeEnvelopeReserve
		if _, exists := previous[path]; !exists {
			previous[path] = RuntimeObservation{Containers: []desiredstate.ContainerInfo{}}
		}
	}
	for _, observation := range previous {
		totalRows += len(observation.Containers)
		cost := runtimeRowsCost(observation.Containers)
		totalBytes += cost
		totalDataBytes += cost
	}
	for _, stack := range stacks {
		observation, ok := previous[stack.Path]
		if !ok {
			observation.Containers = []desiredstate.ContainerInfo{}
		}
		observation.AttemptedAt = time.Now().UTC()
		listCtx, listCancel := context.WithTimeout(ctx, 3*time.Second)
		var rows []desiredstate.ContainerInfo
		err := ctx.Err()
		if err == nil {
			rows, err = r.lister.GetContainers(listCtx, stack.Path)
		}
		listCancel()
		if err == nil && !validRuntimeRows(rows) {
			err = errRuntimeLimit
		}
		if err == nil {
			rows = append([]desiredstate.ContainerInfo{}, rows...)
			for i := range rows {
				rows[i].ImageIdentity = nil
			}
			cost, oldCost := runtimeRowsCost(rows), runtimeRowsCost(observation.Containers)
			if cost > runtimeStackByteLimit || totalRows-len(observation.Containers)+len(rows) > runtimeRowLimit || totalBytes-oldCost+cost > runtimeByteBudget || totalDataBytes-oldCost+cost > runtimeDataByteBudget {
				err = errRuntimeLimit
			} else {
				totalRows += len(rows) - len(observation.Containers)
				totalBytes += cost - oldCost
				totalDataBytes += cost - oldCost
			}
		}
		if err != nil {
			message := "Container observation failed"
			if ctx.Err() != nil {
				message = "Runtime observation timed out"
			} else if errors.Is(err, errRuntimeLimit) {
				message = errRuntimeLimit.Error()
			}
			observation.Error = &message
		} else {
			now := time.Now().UTC()
			observation.ObservedAt = &now
			observation.Error = nil
			observation.Containers = rows
		}
		out[stack.Path] = observation
	}
	// List first, then enrich: slow/failed identity lookups must not consume the
	// cycle budget before other stacks get a chance to report their containers.
	for _, stack := range stacks {
		observation := out[stack.Path]
		if observation.Error != nil {
			continue
		}
		baseCost := runtimeRowsCost(observation.Containers)
		for i := range observation.Containers {
			row := &observation.Containers[i]
			row.ImageIdentity = identities.Resolve(ctx, stack.Path, *row)
		}
		cost := runtimeRowsCost(observation.Containers)
		old := previous[stack.Path]
		oldCost := runtimeRowsCost(old.Containers)
		if cost > runtimeStackByteLimit || totalBytes-baseCost+cost > runtimeByteBudget || totalDataBytes-baseCost+cost > runtimeDataByteBudget {
			totalRows += len(old.Containers) - len(observation.Containers)
			totalBytes += oldCost - baseCost
			totalDataBytes += oldCost - baseCost
			message := errRuntimeLimit.Error()
			observation = old
			observation.AttemptedAt = out[stack.Path].AttemptedAt
			observation.Error = &message
			if observation.Containers == nil {
				observation.Containers = []desiredstate.ContainerInfo{}
			}
		} else {
			totalBytes += cost - baseCost
			totalDataBytes += cost - baseCost
		}
		out[stack.Path] = observation
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Preserve successful observations on explicit invalidation, but never mark
	// them fresh: all waiting readers run the next cycle before returning.
	for path, observation := range out {
		r.cache[path] = observation
		r.signatures[path] = signatures[path]
		if generation == r.generation {
			r.expires[path] = time.Now().Add(r.ttl)
		}
	}
	close(r.flight)
	r.flight = nil
}

// GetContainers preserves the existing array response and rejects arbitrary paths
// before invoking Compose. Global runtime carries partial failure/last-success
// provenance; detail retains its existing sanitized error response on failure.
func (r *RuntimeReader) GetContainers(ctx context.Context, stackPath string) ([]desiredstate.ContainerInfo, error) {
	observations, err := r.read(ctx, stackPath)
	if err != nil {
		if errors.Is(err, errUnknownRuntimeStack) {
			return nil, err
		}
		return nil, errors.New("Runtime observation unavailable")
	}
	observation, exists := observations[stackPath]
	if !exists {
		return nil, errUnknownRuntimeStack
	}
	if observation.Error != nil {
		return nil, errors.New(*observation.Error)
	}
	return observation.Containers, nil
}

func validRuntimeRows(rows []desiredstate.ContainerInfo) bool {
	if len(rows) > runtimeStackRowLimit {
		return false
	}
	for _, row := range rows {
		for _, value := range []string{row.FullID, row.ComposeProject, row.ID, row.Name, row.Service, row.State, row.Health, row.Image, row.Ports} {
			if len(value) > runtimeStringLimit {
				return false
			}
		}
	}
	return true
}

// Serialized rows bound response work; internal-only scope strings also count
// toward retention. Input rows have already passed fixed cardinality/size caps.
func runtimeRowsCost(rows []desiredstate.ContainerInfo) int {
	// Empty array punctuation is already covered by the envelope reserve.
	if len(rows) == 0 {
		return 0
	}
	encoded, _ := json.Marshal(rows)
	n := len(encoded)
	for _, row := range rows {
		n += len(row.FullID) + len(row.ComposeProject)
	}
	return n
}

func cloneRuntime(in map[string]RuntimeObservation) map[string]RuntimeObservation {
	out := make(map[string]RuntimeObservation, len(in))
	for path, observation := range in {
		observation.Containers = append([]desiredstate.ContainerInfo{}, observation.Containers...)
		if observation.ObservedAt != nil {
			t := *observation.ObservedAt
			observation.ObservedAt = &t
		}
		if observation.Error != nil {
			e := *observation.Error
			observation.Error = &e
		}
		for i := range observation.Containers {
			if identity := observation.Containers[i].ImageIdentity; identity != nil {
				cp := *identity
				cp.RepositoryDigests = append([]desiredstate.RepositoryDigest{}, identity.RepositoryDigests...)
				if identity.Platform != nil {
					p := *identity.Platform
					cp.Platform = &p
				}
				if identity.PreferredRepositoryDigest != nil {
					d := *identity.PreferredRepositoryDigest
					cp.PreferredRepositoryDigest = &d
				}
				observation.Containers[i].ImageIdentity = &cp
			}
		}
		out[path] = observation
	}
	return out
}
